//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionearlyresetentitlement"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// The real early-reset service owns the entitlement/window updates; this adapter
// implements the repository CAS against the isolated unit-test database.
type presaleEarlyResetTestRepo struct {
	*paymentFulfillmentSubscriptionRepo
}

func (r *presaleEarlyResetTestRepo) EarlyReset(ctx context.Context, in EarlyResetSubscriptionParams) error {
	client := paymentFulfillmentSubscriptionClientFromContext(ctx, r.client)
	update := client.UserSubscription.Update().Where(
		usersubscription.IDEQ(in.ID), usersubscription.UserIDEQ(in.UserID),
		usersubscription.StatusEQ(SubscriptionStatusActive), usersubscription.ExpiresAtEQ(in.ExpectedExpiresAt),
	).SetExpiresAt(in.NewExpiresAt).SetDailyUsageUsd(0).SetWeeklyUsageUsd(0).SetMonthlyUsageUsd(0)
	if in.NewCustomExpiresAt != nil {
		update.SetCustomExpiresAt(*in.NewCustomExpiresAt)
	}
	n, err := update.Save(ctx)
	if err == nil && n != 1 {
		return ErrEarlyResetConflict
	}
	return err
}

func activePresaleRefundFixture(t *testing.T, multiplier int) (*PaymentService, *dbent.PaymentOrder, *dbent.UserSubscription) {
	t.Helper()
	s, u, p := newPresaleFixture(t)
	s.subscriptionSvc.userSubRepo = &presaleEarlyResetTestRepo{&paymentFulfillmentSubscriptionRepo{client: s.entClient}}
	ctx := context.Background()
	now := time.Now()
	period := NextPresalePeriod(now)
	// Keep refund tests independent of the calendar day and away from rounding boundaries.
	period.StartsAt = now.Add(-2 * time.Hour).Truncate(time.Second)
	period.ExpiresAt = period.StartsAt.Add(31 * 24 * time.Hour)
	o := newPresaleOrder(t, s, u, p, period)
	s.entClient.PaymentOrder.UpdateOneID(o.ID).SetAmount(310).SetPayAmount(248).
		SetSubscriptionEarlyResetEnabled(true).SetSubscriptionEarlyResetDurationDays(2).
		SetSubscriptionMultiplier(multiplier).ExecX(ctx)
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	activated, err := s.activatePresale(ctx, o.ID, now)
	require.NoError(t, err)
	require.True(t, activated)
	return s, s.entClient.PaymentOrder.GetX(ctx, o.ID), s.entClient.UserSubscription.Query().OnlyX(ctx)
}

func TestPresaleEarlyResetRefundUsesActualTermAndFreezesQuote(t *testing.T) {
	for _, multiplier := range []int{1, 3} {
		t.Run(fmt.Sprint(multiplier), func(t *testing.T) {
			ctx := context.Background()
			s, o, sub := activePresaleRefundFixture(t, multiplier)
			shortened, err := s.subscriptionSvc.EarlyResetSubscription(ctx, o.UserID, sub.ID)
			require.NoError(t, err)
			require.True(t, shortened.ExpiresAt.Equal(o.PresaleExpiresAt.AddDate(0, 0, -2)))
			quote, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
			require.NoError(t, err)
			require.Equal(t, 28, quote.UnusedDays)
			require.Equal(t, 280.0, quote.RefundAmount)
			require.Equal(t, 224.0, quote.GatewayAmount)
			require.NoError(t, s.RequestRefund(ctx, o.ID, o.UserID, "cancel after reset", quote.GatewayAmount))
			cancelled := s.entClient.UserSubscription.GetX(ctx, sub.ID)
			require.Equal(t, SubscriptionStatusExpired, cancelled.Status)
			requested := s.entClient.PaymentOrder.GetX(ctx, o.ID)
			require.True(t, requested.PresaleExpiresAt.Equal(*o.PresaleExpiresAt), "purchase dates remain immutable")
			frozen, err := s.GetPresaleRefundQuote(ctx, requested, time.Now().AddDate(0, 2, 0))
			require.NoError(t, err)
			require.Equal(t, quote, frozen, "cancellation must not change the refund")

			// A provider retry after another subscription term begins must not read
			// or cancel that term, nor recalculate the old refund from its dates.
			laterStart, laterEnd := *o.PresaleExpiresAt, o.PresaleExpiresAt.AddDate(0, 1, 0)
			s.entClient.UserSubscription.UpdateOneID(sub.ID).SetStatus(SubscriptionStatusActive).
				SetStartsAt(laterStart).SetExpiresAt(laterEnd).ExecX(ctx)
			s.entClient.PaymentOrder.UpdateOneID(o.ID).SetStatus(OrderStatusRefundFailed).ExecX(ctx)
			plan, err := s.preparePresaleRefund(ctx, s.entClient.PaymentOrder.GetX(ctx, o.ID), quote.RefundAmount, "retry")
			require.NoError(t, err)
			require.Equal(t, quote.GatewayAmount, plan.GatewayAmount)
			require.NoError(t, s.claimPresaleRefund(ctx, plan))
			require.Equal(t, quote.GatewayAmount, plan.GatewayAmount)
			unchanged := s.entClient.UserSubscription.GetX(ctx, sub.ID)
			require.True(t, unchanged.ExpiresAt.Equal(laterEnd))
			require.Equal(t, SubscriptionStatusActive, unchanged.Status)
		})
	}
}

func TestPresaleRefundRejectsQuoteChangedByAnotherEarlyReset(t *testing.T) {
	ctx := context.Background()
	s, o, sub := activePresaleRefundFixture(t, 1)
	quote, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
	require.NoError(t, err)
	_, err = s.subscriptionSvc.EarlyResetSubscription(ctx, o.UserID, sub.ID)
	require.NoError(t, err)
	err = s.RequestRefund(ctx, o.ID, o.UserID, "stale quote", quote.GatewayAmount)
	require.Equal(t, "PRESALE_REFUND_AMOUNT_CHANGED", infraerrors.Reason(err))
	require.Equal(t, OrderStatusCompleted, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
	require.Equal(t, SubscriptionStatusActive, s.entClient.UserSubscription.GetX(ctx, sub.ID).Status)
	latest, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
	require.NoError(t, err)
	require.Less(t, latest.GatewayAmount, quote.GatewayAmount)
	require.NoError(t, s.RequestRefund(ctx, o.ID, o.UserID, "reviewed again", latest.GatewayAmount))
}

func TestPresaleEarlyResetRefundLastWeekUsesActualExpiry(t *testing.T) {
	ctx := context.Background()
	s, o, sub := activePresaleRefundFixture(t, 1)
	shortened, err := s.subscriptionSvc.EarlyResetSubscription(ctx, o.UserID, sub.ID)
	require.NoError(t, err)
	boundary := shortened.ExpiresAt.Add(-7 * 24 * time.Hour)
	_, err = s.GetPresaleRefundQuote(ctx, o, boundary.Add(-time.Second))
	require.NoError(t, err)
	_, err = s.GetPresaleRefundQuote(ctx, o, boundary)
	require.Equal(t, "PRESALE_REFUND_LAST_WEEK", infraerrors.Reason(err))
}

func TestPresaleRefundStillRejectsUnrelatedTermChanges(t *testing.T) {
	for _, change := range []string{"manual shortening", "manual extension", "later term", "unrelated entitlement"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			s, o, sub := activePresaleRefundFixture(t, 1)
			quote, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
			require.NoError(t, err)
			switch change {
			case "manual shortening":
				s.entClient.UserSubscription.UpdateOneID(sub.ID).SetExpiresAt(sub.ExpiresAt.AddDate(0, 0, -2)).ExecX(ctx)
			case "manual extension":
				end := sub.ExpiresAt.AddDate(0, 0, 2)
				s.entClient.UserSubscription.UpdateOneID(sub.ID).SetExpiresAt(end).ExecX(ctx)
				s.entClient.SubscriptionEarlyResetEntitlement.Update().Where(subscriptionearlyresetentitlement.SourceOrderIDEQ(o.ID)).SetExpiresAt(end).ExecX(ctx)
			case "later term":
				s.entClient.UserSubscription.UpdateOneID(sub.ID).SetStartsAt(sub.ExpiresAt).SetExpiresAt(sub.ExpiresAt.AddDate(0, 1, 0)).ExecX(ctx)
			case "unrelated entitlement":
				s.entClient.SubscriptionEarlyResetEntitlement.Update().Where(subscriptionearlyresetentitlement.SourceOrderIDEQ(o.ID)).SetUserID(o.UserID + 1).ExecX(ctx)
			}
			_, err = s.GetPresaleRefundQuote(ctx, o, time.Now())
			require.Error(t, err)
			require.Error(t, s.RequestRefund(ctx, o.ID, o.UserID, "cancel", quote.GatewayAmount))
			require.Equal(t, OrderStatusCompleted, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
		})
	}
}

func TestPresaleRefundLegacyFrozenQuoteCompatibility(t *testing.T) {
	ctx := context.Background()
	s, o, _ := activePresaleRefundFixture(t, 1)
	quote, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
	require.NoError(t, err)
	require.NoError(t, s.RequestRefund(ctx, o.ID, o.UserID, "cancel", quote.GatewayAmount))
	// This was the entire persisted audit payload in v0.0.84.
	s.entClient.PaymentAuditLog.Update().Where(paymentauditlog.ActionEQ("PRESALE_REFUND_REQUESTED")).
		SetDetail(fmt.Sprintf(`{"policy":"unused_days","amount":%v,"gateway_amount":%v}`, quote.RefundAmount, quote.GatewayAmount)).ExecX(ctx)
	frozen, err := s.GetPresaleRefundQuote(ctx, s.entClient.PaymentOrder.GetX(ctx, o.ID), time.Now().AddDate(0, 2, 0))
	require.NoError(t, err)
	require.Equal(t, quote, frozen)
}
