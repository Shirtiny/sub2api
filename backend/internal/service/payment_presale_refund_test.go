//go:build unit

package service

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionearlyresetentitlement"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/payment"
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
			require.Equal(t, 224.0, quote.RefundAmount)
			require.Equal(t, 179.2, quote.GatewayAmount)
			require.Equal(t, 20, quote.FeePercent)
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
	// A request accepted under v0.0.84 had no daily refund fee. A later rule
	// change must not reduce its already-approved amount on review or retry.
	quote.RefundAmount, quote.GatewayAmount, quote.FeePercent = 300, 240, 0
	s.entClient.PaymentAuditLog.Update().Where(paymentauditlog.ActionEQ("PRESALE_REFUND_REQUESTED")).
		SetDetail(fmt.Sprintf(`{"policy":"unused_days","amount":%v,"gateway_amount":%v}`, quote.RefundAmount, quote.GatewayAmount)).ExecX(ctx)
	frozen, err := s.GetPresaleRefundQuote(ctx, s.entClient.PaymentOrder.GetX(ctx, o.ID), time.Now().AddDate(0, 2, 0))
	require.NoError(t, err)
	require.Equal(t, quote, frozen)
}

func TestPresaleRefundFullSnapshotRetainsPreviouslyAcceptedNoFeeQuote(t *testing.T) {
	ctx := context.Background()
	s, o, _ := activePresaleRefundFixture(t, 1)
	quote, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
	require.NoError(t, err)
	require.NoError(t, s.RequestRefund(ctx, o.ID, o.UserID, "cancel", quote.GatewayAmount))
	s.entClient.PaymentAuditLog.Update().Where(paymentauditlog.ActionEQ("PRESALE_REFUND_REQUESTED")).
		SetDetail(`{"quote":{"policy":"unused_days","refund_amount":300,"gateway_amount":240,"fee_percent":0,"unused_days":30,"currency":"CNY"}}`).ExecX(ctx)
	plan, err := s.preparePresaleRefund(ctx, s.entClient.PaymentOrder.GetX(ctx, o.ID), 300, "retry accepted refund")
	require.NoError(t, err)
	require.Equal(t, 300.0, plan.RefundAmount)
	require.Equal(t, 240.0, plan.GatewayAmount)
	require.NoError(t, s.claimPresaleRefund(ctx, plan))
	require.Equal(t, 240.0, plan.GatewayAmount)
}

func TestPresaleDailyRefundFeeUsesActualPaymentAndCurrencyRounding(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, presaleLocation)
	end := start.AddDate(0, 1, 0)
	for _, tc := range []struct {
		currency   string
		paid, want float64
	}{
		{"CNY", 90, 69.68},
		{"JPY", 990, 766},
	} {
		t.Run(tc.currency, func(t *testing.T) {
			o := &dbent.PaymentOrder{
				Status: OrderStatusCompleted, Amount: 100, PayAmount: tc.paid,
				PresaleStartsAt: &start, PresaleExpiresAt: &end, PresaleActivatedAt: &start,
				ProviderSnapshot: map[string]any{"currency": tc.currency},
			}
			quote, err := PresaleRefundQuoteForOrder(o, start.Add(24*time.Hour))
			require.NoError(t, err)
			require.Equal(t, 30, quote.UnusedDays)
			require.Equal(t, 20, quote.FeePercent)
			require.Equal(t, 77.42, quote.RefundAmount)
			require.Equal(t, tc.want, quote.GatewayAmount)
			require.Equal(t, tc.currency, quote.Currency)
		})
	}
}

func TestPresaleRefundExcludesPaymentFeeBeforeProration(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, presaleLocation)
	end := start.AddDate(0, 1, 0)
	for _, tc := range []struct {
		name, currency       string
		amount, discount     float64
		rate, paid, wantBase float64
	}{
		{"current two-times order", "CNY", 2, 0, 1, 2.02, 2},
		{"zero fee", "CNY", 100, 10, 0, 90, 90},
		{"discounted purchase", "CNY", 100, 10, 1, 90.9, 90},
		{"rounded-up fee", "CNY", 10, 0, 3.33, 10.34, 10},
		{"fractional discounted price", "CNY", 10, .01, 1, 10.09, 9.99},
		{"zero-decimal currency", "JPY", 100, 10, 1, 91, 90},
	} {
		t.Run(tc.name, func(t *testing.T) {
			charged, err := strconv.ParseFloat(payment.CalculatePayAmountForCurrency(tc.amount-tc.discount, tc.rate, tc.currency), 64)
			require.NoError(t, err)
			require.Equal(t, tc.paid, charged, "test payments must match checkout fee rounding")
			for _, policy := range []struct {
				name      string
				now       time.Time
				activated bool
			}{
				{"full", start.Add(-96 * time.Hour), false},
				{"preparation", start.Add(-72 * time.Hour), false},
				{"unused_days", start.Add(24 * time.Hour), true},
				{"unfulfilled", end.Add(time.Hour), false},
			} {
				t.Run(policy.name, func(t *testing.T) {
					o := &dbent.PaymentOrder{
						Status: OrderStatusCompleted, Amount: tc.amount, PayAmount: tc.paid,
						FeeRate: tc.rate, CafeCouponDiscount: tc.discount,
						PresaleStartsAt: &start, PresaleExpiresAt: &end,
						ProviderSnapshot: map[string]any{"currency": tc.currency},
					}
					if policy.activated {
						o.PresaleActivatedAt = &start
					}
					quote, err := PresaleRefundQuoteForOrder(o, policy.now)
					require.NoError(t, err)
					// Identical to a payment of the same principal with no payment fee.
					o.PayAmount, o.FeeRate = tc.wantBase, 0
					withoutFee, err := PresaleRefundQuoteForOrder(o, policy.now)
					require.NoError(t, err)
					require.Equal(t, policy.name, quote.Policy)
					require.Equal(t, withoutFee, quote)
					require.LessOrEqual(t, quote.GatewayAmount, tc.wantBase)
					if policy.name == "full" || policy.name == "unfulfilled" {
						require.Equal(t, tc.wantBase, quote.GatewayAmount)
					}
				})
			}
		})
	}
}

func TestPresalePaymentFeeQuoteMustBeReviewedAndStaysFrozen(t *testing.T) {
	ctx := context.Background()
	s, u, p := newPresaleFixture(t)
	// Keep the full-refund test away from the real clock's monthly cutoff.
	period := NextPresalePeriod(time.Now().AddDate(0, 1, 0))
	o := newPresaleOrder(t, s, u, p, period)
	s.entClient.PaymentOrder.UpdateOneID(o.ID).SetAmount(2).SetPayAmount(2.02).SetFeeRate(1).ExecX(ctx)
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	o = s.entClient.PaymentOrder.GetX(ctx, o.ID)
	quote, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
	require.NoError(t, err)
	require.Equal(t, 2.0, quote.GatewayAmount)
	err = s.RequestRefund(ctx, o.ID, o.UserID, "stale fee-inclusive quote", 2.02)
	require.Equal(t, "PRESALE_REFUND_AMOUNT_CHANGED", infraerrors.Reason(err))
	require.Equal(t, OrderStatusCompleted, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
	require.NoError(t, s.RequestRefund(ctx, o.ID, o.UserID, "reviewed quote", quote.GatewayAmount))
	requested := s.entClient.PaymentOrder.GetX(ctx, o.ID)
	plan, err := s.preparePresaleRefund(ctx, requested, quote.RefundAmount, "admin review")
	require.NoError(t, err)
	require.Equal(t, 2.0, plan.GatewayAmount)
	require.NoError(t, s.claimPresaleRefund(ctx, plan))
	frozen, err := s.GetPresaleRefundQuote(ctx, requested, period.ExpiresAt.AddDate(0, 1, 0))
	require.NoError(t, err)
	require.Equal(t, quote, frozen, "the payment fee must not be subtracted again on retry")
}

func TestPresalePreviouslyAcceptedQuoteKeepsItsPaymentFee(t *testing.T) {
	ctx := context.Background()
	s, u, p := newPresaleFixture(t)
	o := newPresaleOrder(t, s, u, p, NextPresalePeriod(time.Now().AddDate(0, 1, 0)))
	s.entClient.PaymentOrder.UpdateOneID(o.ID).SetAmount(2).SetPayAmount(2.02).SetFeeRate(1).ExecX(ctx)
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	require.NoError(t, s.RequestRefund(ctx, o.ID, o.UserID, "accepted refund", 2))
	for _, detail := range []string{
		`{"quote":{"policy":"full","refund_amount":2,"gateway_amount":2.02,"fee_percent":0,"unused_days":31,"currency":"CNY"}}`,
		`{"policy":"full","amount":2,"gateway_amount":2.02}`,
	} {
		s.entClient.PaymentAuditLog.Update().Where(paymentauditlog.ActionEQ("PRESALE_REFUND_REQUESTED")).SetDetail(detail).ExecX(ctx)
		plan, err := s.preparePresaleRefund(ctx, s.entClient.PaymentOrder.GetX(ctx, o.ID), 2, "previously accepted refund")
		require.NoError(t, err)
		require.Equal(t, 2.02, plan.GatewayAmount, "do not reprice a refund the user already accepted")
		require.NoError(t, s.claimPresaleRefund(ctx, plan))
		require.Equal(t, 2.02, plan.GatewayAmount)
	}
}
