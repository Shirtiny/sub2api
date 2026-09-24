//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func createPresaleSafetyAttempt(t *testing.T, s *PaymentService, u *dbent.User, p *dbent.SubscriptionPlan) *dbent.PaymentOrder {
	t.Helper()
	order, err := s.createOrderInTx(context.Background(), CreateOrderRequest{
		UserID: u.ID, PlanID: p.ID, Multiplier: 1, OrderType: payment.OrderTypeSubscription,
		PaymentType: payment.TypeAlipay, PresaleMonth: NextPresalePeriod(time.Now()).Month,
		ClientIP: "127.0.0.1", SrcHost: "localhost",
	}, &User{ID: u.ID, Email: u.Email, Username: u.Username}, p, &PaymentConfig{}, p.Price, p.Price, 0, p.Price, nil)
	require.NoError(t, err)
	return order
}

func TestPresaleLatePaymentCannotPoisonReplacement(t *testing.T) {
	for _, oldStatus := range []string{OrderStatusCancelled, OrderStatusExpired, OrderStatusPending, OrderStatusFailed} {
		for _, lateFirst := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/late-first=%t", oldStatus, lateFirst), func(t *testing.T) {
				s, u, p := newPresaleFixture(t)
				ctx := context.Background()
				old := createPresaleSafetyAttempt(t, s, u, p)
				old = s.entClient.PaymentOrder.UpdateOneID(old.ID).SetStatus(oldStatus).SetExpiresAt(time.Now().Add(-time.Second)).SaveX(ctx)
				replacement := createPresaleSafetyAttempt(t, s, u, p)
				payOld := func() {
					err := s.toPaid(ctx, old, "late-verified-payment", p.Price, payment.TypeAlipay)
					require.Equal(t, "PRESALE_ALREADY_RESERVED", infraerrors.Reason(err))
					failed := s.entClient.PaymentOrder.GetX(ctx, old.ID)
					require.Equal(t, OrderStatusFailed, failed.Status)
					require.NotNil(t, failed.PaidAt)
					require.Equal(t, "late-verified-payment", failed.PaymentTradeNo)
				}
				if lateFirst {
					payOld()
				}
				require.NoError(t, s.toPaid(ctx, replacement, "replacement-payment", p.Price, payment.TypeAlipay))
				payOld() // Includes repeated callbacks after the replacement completed.
				require.Equal(t, "PRESALE_ALREADY_RESERVED", infraerrors.Reason(s.RetryFulfillment(ctx, old.ID)))
				require.NoError(t, s.RequestRefund(ctx, old.ID, u.ID, "refund superseded payment", presaleTestRefundAmount(t, s, old.ID)))
				_, err := s.GetPresaleQuote(ctx, u.ID, p.ID)
				require.Equal(t, fmt.Sprint(replacement.ID), infraerrors.FromError(err).Metadata["order_id"])
				require.Equal(t, OrderStatusCompleted, s.entClient.PaymentOrder.GetX(ctx, replacement.ID).Status)
				count, err := s.ActivateDuePresales(ctx, *replacement.PresaleStartsAt)
				require.NoError(t, err)
				require.Equal(t, 1, count)
				require.Equal(t, 1, s.entClient.UserSubscription.Query().CountX(ctx))
				require.Equal(t, p.Price, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
			})
		}
	}
}

func TestPresaleRetiredAttemptNeverReclaimsReleasedSlot(t *testing.T) {
	for _, status := range []string{OrderStatusCancelled, OrderStatusExpired, OrderStatusRefunded, OrderStatusPartiallyRefunded} {
		t.Run(status, func(t *testing.T) {
			s, u, p := newPresaleFixture(t)
			ctx := context.Background()
			old := createPresaleSafetyAttempt(t, s, u, p)
			old = s.entClient.PaymentOrder.UpdateOneID(old.ID).SetStatus(OrderStatusCancelled).SaveX(ctx)
			replacement := createPresaleSafetyAttempt(t, s, u, p)
			s.entClient.PaymentOrder.UpdateOneID(replacement.ID).SetStatus(status).ExecX(ctx)
			require.Equal(t, "PRESALE_ALREADY_RESERVED", infraerrors.Reason(s.toPaid(ctx, old, "old-payment", p.Price, payment.TypeAlipay)))
			_, err := s.GetPresaleQuote(ctx, u.ID, p.ID)
			require.NoError(t, err, "a retired paid failure must not prevent another legitimate attempt")
			third := createPresaleSafetyAttempt(t, s, u, p)
			require.NoError(t, s.toPaid(ctx, third, "new-payment", p.Price, payment.TypeAlipay))
			require.Equal(t, p.Price, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
		})
	}
}

func TestPresaleExistingMutuallyFailedOrdersRecoverNewestOnly(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	old := createPresaleSafetyAttempt(t, s, u, p)
	s.entClient.PaymentOrder.UpdateOneID(old.ID).SetStatus(OrderStatusCancelled).ExecX(ctx)
	newest := createPresaleSafetyAttempt(t, s, u, p)
	// Reproduce the paid facts left by the previous mutual-conflict bug.
	for _, id := range []int64{old.ID, newest.ID} {
		s.entClient.PaymentOrder.UpdateOneID(id).SetStatus(OrderStatusFailed).SetPaidAt(time.Now()).SetFailedAt(time.Now()).SetPaymentTradeNo(fmt.Sprint(id)).ExecX(ctx)
	}
	require.NoError(t, s.RetryFulfillment(ctx, newest.ID))
	require.Equal(t, "PRESALE_ALREADY_RESERVED", infraerrors.Reason(s.RetryFulfillment(ctx, old.ID)))
	require.Equal(t, OrderStatusCompleted, s.entClient.PaymentOrder.GetX(ctx, newest.ID).Status)
	require.Equal(t, p.Price, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
}

func enablePresaleSafetyDevRefund(t *testing.T) {
	t.Helper()
	t.Setenv(paymentDevAutoSuccessEnv, paymentDevAutoSuccessToken)
	t.Setenv(paymentDevEnvironmentEnv, "local")
	for _, key := range []string{"APP_ENV", "ENVIRONMENT", "NODE_ENV", "GO_ENV"} {
		t.Setenv(key, "test")
	}
}

func TestPresaleRefundReversesOnlyEarnedRefundedPoints(t *testing.T) {
	for _, tc := range []struct {
		name                                       string
		paid, coupon, fee, remaining               float64
		preparation, unreserved, legacy, activated bool
	}{
		{name: "full", paid: 100},
		{name: "legacy grant", paid: 100, legacy: true},
		{name: "payment fee retained", paid: 101, fee: 1, remaining: 1},
		{name: "coupon and fee", paid: 80.8, coupon: 20, fee: 1, remaining: .8},
		{name: "preparation cancellation", paid: 100, preparation: true, remaining: 20},
		{name: "used days retained", paid: 100, activated: true, remaining: 22.58},
		{name: "paid but never reserved", paid: 100, unreserved: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			enablePresaleSafetyDevRefund(t)
			s, u, p := newPresaleFixture(t)
			ctx := context.Background()
			const priorPoints = 37.25
			s.entClient.User.UpdateOneID(u.ID).SetTotalRecharged(priorPoints).ExecX(ctx)
			period := NextPresalePeriod(time.Now().AddDate(0, 1, 0))
			if tc.preparation {
				period.StartsAt = time.Now().Add(24 * time.Hour)
				period.ExpiresAt = period.StartsAt.Add(31 * 24 * time.Hour)
			}
			if tc.activated {
				period.StartsAt = time.Now().Add(-time.Hour).Truncate(time.Second)
				period.ExpiresAt = period.StartsAt.Add(31 * 24 * time.Hour)
			}
			o := newPresaleOrder(t, s, u, p, period)
			require.NoError(t, s.writeAuditLogStrict(ctx, o.ID, "DEV_PAYMENT_AUTO_SUCCESS", "test", map[string]any{}))
			s.entClient.PaymentOrder.UpdateOneID(o.ID).SetPayAmount(tc.paid).SetCafeCouponDiscount(tc.coupon).SetFeeRate(tc.fee).SetPaymentTradeNo("dev-auto-success-" + o.OutTradeNo).ExecX(ctx)
			if tc.unreserved {
				s.entClient.PaymentOrder.UpdateOneID(o.ID).SetStatus(OrderStatusFailed).SetFailedAt(time.Now()).ExecX(ctx)
			} else {
				require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
				if tc.legacy {
					s.entClient.PaymentAuditLog.Update().Where(paymentauditlog.OrderIDEQ(fmt.Sprint(o.ID)), paymentauditlog.ActionEQ("PRESALE_RESERVED")).SetDetail(`{}`).ExecX(ctx)
				}
				if tc.activated {
					activated, err := s.activatePresale(ctx, o.ID, time.Now())
					require.NoError(t, err)
					require.True(t, activated)
				}
			}
			o = s.entClient.PaymentOrder.GetX(ctx, o.ID)
			q, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
			require.NoError(t, err)
			before := s.entClient.User.GetX(ctx, u.ID).TotalRecharged
			require.NoError(t, s.RequestRefund(ctx, o.ID, u.ID, "cancel", q.GatewayAmount))
			require.Equal(t, before, s.entClient.User.GetX(ctx, u.ID).TotalRecharged, "a request alone must not refund points")
			plan, _, err := s.PrepareRefund(ctx, o.ID, q.RefundAmount, "refund", false, false)
			require.NoError(t, err)
			// A provider failure leaves the request retryable without losing points.
			s.entClient.PaymentOrder.UpdateOneID(o.ID).SetStatus(OrderStatusRefunding).ExecX(ctx)
			result, err := s.handleGwFail(ctx, plan, errors.New("test gateway failure"))
			require.NoError(t, err)
			require.False(t, result.Success)
			require.Equal(t, before, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
			spy := &authCacheInvalidatorSpy{}
			s.authCacheInvalidator = spy
			result, err = s.ExecuteRefund(ctx, plan)
			require.NoError(t, err)
			require.True(t, result.Success)
			require.InDelta(t, priorPoints+tc.remaining, s.entClient.User.GetX(ctx, u.ID).TotalRecharged, 1e-8)
			require.Equal(t, []int64{u.ID}, spy.userIDs)
			_, err = s.ExecuteRefund(ctx, plan)
			require.Error(t, err)
			require.InDelta(t, priorPoints+tc.remaining, s.entClient.User.GetX(ctx, u.ID).TotalRecharged, 1e-8)
			audits := s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("PRESALE_MEMBERSHIP_REFUNDED")).CountX(ctx)
			if tc.unreserved {
				require.Zero(t, audits)
			} else {
				require.Equal(t, 1, audits)
			}
		})
	}
}

func TestPresaleRepeatedFullRefundsDoNotRaiseMembership(t *testing.T) {
	enablePresaleSafetyDevRefund(t)
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	period := NextPresalePeriod(time.Now().AddDate(0, 1, 0))
	for range 3 {
		o := newPresaleOrder(t, s, u, p, period)
		require.NoError(t, s.writeAuditLogStrict(ctx, o.ID, "DEV_PAYMENT_AUTO_SUCCESS", "test", map[string]any{}))
		s.entClient.PaymentOrder.UpdateOneID(o.ID).SetPaymentTradeNo("dev-auto-success-" + o.OutTradeNo).ExecX(ctx)
		require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
		require.NoError(t, s.RequestRefund(ctx, o.ID, u.ID, "full refund", 100))
		plan, _, err := s.PrepareRefund(ctx, o.ID, 100, "full refund", false, false)
		require.NoError(t, err)
		result, err := s.ExecuteRefund(ctx, plan)
		require.NoError(t, err)
		require.True(t, result.Success)
		after := s.entClient.User.GetX(ctx, u.ID)
		require.Zero(t, after.TotalRecharged)
		require.Zero(t, CalculateMembershipLevel(after.TotalRecharged))
	}
}

func TestPresaleMembershipRefundIsAtomicAndClamped(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	o := newPresaleOrder(t, s, u, p, NextPresalePeriod(time.Now()))
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	s.entClient.User.UpdateOneID(u.ID).SetTotalRecharged(12).ExecX(ctx) // earlier admin adjustment
	o = s.entClient.PaymentOrder.UpdateOneID(o.ID).SetStatus(OrderStatusRefunding).SaveX(ctx)
	plan := &RefundPlan{OrderID: o.ID, Order: o, RefundAmount: 100, GatewayAmount: 100}
	failAudit := true
	s.entClient.PaymentAuditLog.Use(func(next dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(ctx context.Context, m dbent.Mutation) (dbent.Value, error) {
			mutation := m.(*dbent.PaymentAuditLogMutation)
			action, _ := mutation.Action()
			if failAudit && action == "PRESALE_MEMBERSHIP_REFUNDED" {
				return nil, errors.New("test audit unavailable")
			}
			return next.Mutate(ctx, m)
		})
	})
	_, err := s.finalizeSuccessfulRefund(ctx, plan, OrderStatusRefunded, time.Now())
	require.ErrorContains(t, err, "test audit unavailable")
	require.Equal(t, 12.0, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
	require.Equal(t, OrderStatusRefunding, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
	failAudit = false
	_, err = s.finalizeSuccessfulRefund(ctx, plan, OrderStatusRefunded, time.Now())
	require.NoError(t, err)
	require.Zero(t, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
	_, err = s.finalizeSuccessfulRefund(ctx, plan, OrderStatusRefunded, time.Now())
	require.Error(t, err)
	require.Equal(t, 1, s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("PRESALE_MEMBERSHIP_REFUNDED")).CountX(ctx))
}

func TestPresaleNormalActivationDelayKeepsFeeAndFrozenQuote(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	period := NextPresalePeriod(time.Now())
	period.StartsAt = time.Now().Add(-time.Second).Truncate(time.Second)
	period.ExpiresAt = period.StartsAt.Add(31 * 24 * time.Hour)
	o := newPresaleOrder(t, s, u, p, period)
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	o = s.entClient.PaymentOrder.GetX(ctx, o.ID)
	quote, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
	require.NoError(t, err)
	require.Equal(t, 80.0, quote.GatewayAmount)
	require.Equal(t, 20, quote.FeePercent)
	require.Equal(t, "PRESALE_REFUND_AMOUNT_CHANGED", infraerrors.Reason(s.RequestRefund(ctx, o.ID, u.ID, "stale full refund", 100)))
	require.NoError(t, s.RequestRefund(ctx, o.ID, u.ID, "cancel", quote.GatewayAmount))
	count, err := s.ActivateDuePresales(ctx, time.Now())
	require.NoError(t, err)
	require.Zero(t, count)
	frozen, err := s.GetPresaleRefundQuote(ctx, s.entClient.PaymentOrder.GetX(ctx, o.ID), period.ExpiresAt.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, quote, frozen)
}

func TestPresaleActualActivationFailureStillRefundsAndRecoveryIgnoresFailure(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	period := NextPresalePeriod(time.Now())
	period.StartsAt = time.Now().Add(-time.Hour).Truncate(time.Second)
	period.ExpiresAt = period.StartsAt.Add(31 * 24 * time.Hour)
	o := newPresaleOrder(t, s, u, p, period)
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	s.entClient.User.UpdateOneID(u.ID).SetStatus(StatusDisabled).ExecX(ctx)
	for range 2 {
		count, err := s.ActivateDuePresales(ctx, time.Now())
		require.NoError(t, err)
		require.Zero(t, count)
	}
	require.Equal(t, 1, s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("PRESALE_ACTIVATION_FAILED")).CountX(ctx))
	quote, err := s.GetPresaleRefundQuote(ctx, s.entClient.PaymentOrder.GetX(ctx, o.ID), time.Now())
	require.NoError(t, err)
	require.Equal(t, "unfulfilled", quote.Policy)
	require.Equal(t, 100.0, quote.GatewayAmount)
	s.entClient.User.UpdateOneID(u.ID).SetStatus(StatusActive).ExecX(ctx)
	count, err := s.ActivateDuePresales(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	quote, err = s.GetPresaleRefundQuote(ctx, s.entClient.PaymentOrder.GetX(ctx, o.ID), time.Now())
	require.NoError(t, err)
	require.Equal(t, "unused_days", quote.Policy)
	require.Equal(t, 20, quote.FeePercent)
	// A failure quote cannot cancel a now-working subscription at the old amount.
	require.Equal(t, "PRESALE_REFUND_AMOUNT_CHANGED", infraerrors.Reason(s.RequestRefund(ctx, o.ID, u.ID, "old failure quote", 100)))
}

func TestPresaleAcceptedLegacyUnfulfilledQuotesAreNotRepriced(t *testing.T) {
	for _, fullQuote := range []bool{true, false} {
		t.Run(fmt.Sprint(fullQuote), func(t *testing.T) {
			s, u, p := newPresaleFixture(t)
			ctx := context.Background()
			period := NextPresalePeriod(time.Now())
			period.StartsAt = time.Now().Add(-time.Hour)
			period.ExpiresAt = period.StartsAt.Add(31 * 24 * time.Hour)
			o := newPresaleOrder(t, s, u, p, period)
			o = s.entClient.PaymentOrder.UpdateOneID(o.ID).SetStatus(OrderStatusRefundRequested).SetRefundRequestedAt(time.Now()).SaveX(ctx)
			detail := map[string]any{"amount": 100, "gateway_amount": 100}
			if fullQuote {
				detail["quote"] = &PresaleRefundQuote{RefundAmount: 100, GatewayAmount: 100, Policy: "unfulfilled", Currency: "CNY", UnusedDays: 31}
			}
			require.NoError(t, s.writeAuditLogStrict(ctx, o.ID, "PRESALE_REFUND_REQUESTED", "test", detail))
			quote, err := s.GetPresaleRefundQuote(ctx, o, period.ExpiresAt.Add(time.Hour))
			require.NoError(t, err)
			require.Equal(t, 100.0, quote.GatewayAmount)
			require.Equal(t, "unfulfilled", quote.Policy)
			require.Zero(t, quote.FeePercent)
		})
	}
}
