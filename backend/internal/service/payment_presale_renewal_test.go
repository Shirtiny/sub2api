//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestPresaleRenewalFirstTwoWeeks(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, presaleLocation)
	opens := start.Add(14 * 24 * time.Hour)
	for _, tc := range []struct {
		name string
		now  time.Time
		deny bool
	}{
		{"month starts", start, true},
		{"fourteenth day", opens.Add(-time.Nanosecond), true},
		{"fifteenth day UTC clock", opens.In(time.UTC), false},
		{"later in month", opens.Add(24 * time.Hour), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, u, p := newPresaleFixture(t)
			ctx := context.Background()
			current := s.entClient.UserSubscription.Create().SetUserID(u.ID).SetGroupID(p.GroupID).
				SetStartsAt(start).SetExpiresAt(start.AddDate(0, 1, 0)).SaveX(ctx)
			q, err := s.presaleQuoteForPlan(ctx, u.ID, p, tc.now, 0)
			if tc.deny {
				require.Equal(t, "PRESALE_RENEWAL_TOO_EARLY", infraerrors.Reason(err))
				require.Equal(t, "2026-10-15T00:00:00+08:00", infraerrors.FromError(err).Metadata["renewal_opens_at"])
				return
			}
			require.NoError(t, err)
			require.True(t, q.Renewal)
			require.Equal(t, current.ID, *q.CurrentSubscriptionID)
			require.Equal(t, "2026-11", q.Month)
		})
	}
}

func TestPresaleRenewalWindowUsesCurrentTermAndSourceGroup(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	start := time.Date(2026, 10, 10, 12, 0, 0, 0, presaleLocation)
	now := start.Add(5 * 24 * time.Hour)
	// A first purchase is not blocked just because the calendar month is young.
	_, err := s.presaleQuoteForPlan(ctx, u.ID, p, start, 0)
	require.NoError(t, err)
	current := s.entClient.UserSubscription.Create().SetUserID(u.ID).SetGroupID(p.GroupID).
		SetStartsAt(start).SetExpiresAt(NextPresalePeriod(start).StartsAt).SaveX(ctx)
	sibling := s.entClient.SubscriptionPlan.Create().SetGroupID(p.GroupID).SetName("Same group, different plan").
		SetPrice(200).SetForSale(true).SetPresaleEnabled(true).SaveX(ctx)
	_, err = s.presaleQuoteForPlan(ctx, u.ID, sibling, now, 0)
	require.Equal(t, "PRESALE_RENEWAL_TOO_EARLY", infraerrors.Reason(err))
	require.Equal(t, "2026-10-24T12:00:00+08:00", infraerrors.FromError(err).Metadata["renewal_opens_at"])
	otherGroup := s.entClient.Group.Create().SetName("Other source group").SetSubscriptionType(SubscriptionTypeSubscription).SaveX(ctx)
	otherPlan := s.entClient.SubscriptionPlan.Create().SetGroupID(otherGroup.ID).SetName("Other plan").SetPrice(100).SaveX(ctx)
	_, err = s.presaleQuoteForPlan(ctx, u.ID, otherPlan, now, 0)
	require.NoError(t, err)
	otherUser := s.entClient.User.Create().SetEmail("other-renewal@example.com").SetPasswordHash("hash").SaveX(ctx)
	_, err = s.presaleQuoteForPlan(ctx, otherUser.ID, p, now, 0)
	require.NoError(t, err)
	// Expired/cancelled service does not count as a currently running term.
	s.entClient.UserSubscription.UpdateOneID(current.ID).SetExpiresAt(now).SetStatus(SubscriptionStatusExpired).ExecX(ctx)
	_, err = s.presaleQuoteForPlan(ctx, u.ID, p, now, 0)
	require.NoError(t, err)
}

func TestPresaleRenewalWindowBeforeActivation(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, presaleLocation)
	period := NextPresalePeriod(start.Add(-time.Hour))
	for _, tc := range []struct {
		status string
		deny   bool
	}{
		{OrderStatusPaid, true}, {OrderStatusRecharging, true}, {OrderStatusCompleted, true},
		{OrderStatusPending, false}, {OrderStatusCancelled, false}, {OrderStatusExpired, false},
		{OrderStatusRefundRequested, false}, {OrderStatusRefunding, false},
		{OrderStatusRefunded, false}, {OrderStatusPartiallyRefunded, false}, {OrderStatusFailed, false},
	} {
		t.Run(tc.status, func(t *testing.T) {
			s, u, p := newPresaleFixture(t)
			ctx := context.Background()
			o := newPresaleOrder(t, s, u, p, period)
			s.entClient.PaymentOrder.UpdateOneID(o.ID).SetStatus(tc.status).ExecX(ctx)
			_, err := s.presaleQuoteForPlan(ctx, u.ID, p, start, 0)
			if tc.deny {
				require.Equal(t, "PRESALE_RENEWAL_TOO_EARLY", infraerrors.Reason(err))
			} else {
				require.NoError(t, err)
			}
			// Worker delay must not restart or extend the fixed 14-day wait.
			_, err = s.presaleQuoteForPlan(ctx, u.ID, p, start.Add(14*24*time.Hour), 0)
			require.NoError(t, err)
		})
	}
}

func TestPresaleRenewalWindowEnforcedWhenCreatingOrder(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	now := time.Now()
	period := NextPresalePeriod(now)
	current := s.entClient.UserSubscription.Create().SetUserID(u.ID).SetGroupID(p.GroupID).
		SetStartsAt(now.Add(-24 * time.Hour)).SetExpiresAt(period.StartsAt).SaveX(ctx)
	_, err := s.GetPresaleQuote(ctx, u.ID, p.ID)
	require.Equal(t, "PRESALE_RENEWAL_TOO_EARLY", infraerrors.Reason(err))
	req := CreateOrderRequest{UserID: u.ID, PlanID: p.ID, Multiplier: 1, OrderType: payment.OrderTypeSubscription,
		PaymentType: payment.TypeAlipay, PresaleMonth: period.Month, ClientIP: "127.0.0.1", SrcHost: "localhost"}
	buyer := &User{ID: u.ID, Email: u.Email, Username: u.Username}
	// A client skipping the landing quote cannot create an early renewal order.
	_, err = s.createOrderInTx(ctx, req, buyer, p, &PaymentConfig{}, 100, 100, 0, 100, nil)
	require.Equal(t, "PRESALE_RENEWAL_TOO_EARLY", infraerrors.Reason(err))
	require.Zero(t, s.entClient.PaymentOrder.Query().CountX(ctx))
	s.entClient.UserSubscription.UpdateOneID(current.ID).SetStartsAt(now.Add(-14 * 24 * time.Hour)).ExecX(ctx)
	o, err := s.createOrderInTx(ctx, req, buyer, p, &PaymentConfig{}, 100, 100, 0, 100, nil)
	require.NoError(t, err)
	require.True(t, o.PresaleRenewal)
	require.True(t, o.PresaleStartsAt.Equal(period.StartsAt))
	require.True(t, o.PresaleExpiresAt.Equal(period.ExpiresAt))
}

func TestPresaleRenewalWindowHonorsExistingPayments(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	now := time.Now()
	period := NextPresalePeriod(now)
	current := s.entClient.UserSubscription.Create().SetUserID(u.ID).SetGroupID(p.GroupID).
		SetStartsAt(now.Add(-24 * time.Hour)).SetExpiresAt(period.StartsAt).SetDailyUsageUsd(12).SaveX(ctx)
	// An order accepted before this rule remains payable/fulfillable; it also
	// keeps its slot so the UI can resume it instead of offering a new purchase.
	o := newPresaleOrder(t, s, u, p, period)
	_, err := s.GetPresaleQuote(ctx, u.ID, p.ID)
	require.Equal(t, "PRESALE_ALREADY_RESERVED", infraerrors.Reason(err))
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	require.Equal(t, OrderStatusCompleted, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
	unchanged := s.entClient.UserSubscription.GetX(ctx, current.ID)
	require.True(t, unchanged.ExpiresAt.Equal(current.ExpiresAt))
	require.Equal(t, 12.0, unchanged.DailyUsageUsd)
}
