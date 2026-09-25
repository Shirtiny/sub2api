//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func offlineRequest(o *dbent.PaymentOrder, mode string) PresaleOfflineRequest {
	r := PresaleOfflineRequest{Mode: mode, Reason: "Customer contacted support", Confirmed: true, ExpectedUpdatedAt: o.UpdatedAt}
	if mode == "refund" {
		r.Amount = o.PayAmount
		r.Reference = "external-receipt-123"
	}
	return r
}

func TestPresaleOfflineCancelRebookAndLateCallback(t *testing.T) {
	ctx := context.Background()
	s, u, p := newPresaleFixture(t)
	o := newPresaleOrder(t, s, u, p, NextPresalePeriod(time.Now()))
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	o = s.entClient.PaymentOrder.GetX(ctx, o.ID)
	req := offlineRequest(o, "cancel")
	for range 2 {
		result, err := s.ProcessPresaleOffline(ctx, o.ID, 99, req)
		require.NoError(t, err)
		require.Equal(t, OrderStatusPresaleCancelled, result.Status)
	}
	cancelled := s.entClient.PaymentOrder.GetX(ctx, o.ID)
	stats, err := s.GetDashboardStats(ctx, 30)
	require.NoError(t, err)
	require.Equal(t, 100.0, stats.TotalAmount)
	require.Nil(t, cancelled.RefundAt)
	require.Zero(t, cancelled.RefundAmount)
	require.True(t, cancelled.PaidAt.Equal(*o.PaidAt))
	require.True(t, cancelled.CompletedAt.Equal(*o.CompletedAt))
	require.Equal(t, o.PayAmount, cancelled.PayAmount)
	require.Equal(t, 100.0, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
	audit := s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("PRESALE_CANCELLED")).OnlyX(ctx)
	require.Equal(t, "admin:99", audit.Operator)
	_, err = s.GetPresaleQuote(ctx, u.ID, p.ID)
	require.NoError(t, err)
	// Paid cancellation is not unpaid CANCELLED: late callbacks cannot resurrect it.
	require.NoError(t, s.toPaid(ctx, o, "late-duplicate", 100, payment.TypeAlipay))
	require.Equal(t, OrderStatusPresaleCancelled, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
	require.Error(t, s.RetryFulfillment(ctx, o.ID))
	replacement := createPresaleSafetyAttempt(t, s, u, p)
	require.NoError(t, s.toPaid(ctx, replacement, "new-payment", 100, payment.TypeAlipay))
	count, err := s.ActivateDuePresales(ctx, *o.PresaleStartsAt)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	sub := s.entClient.UserSubscription.Query().OnlyX(ctx)
	require.Nil(t, s.entClient.PaymentOrder.GetX(ctx, o.ID).PresaleActivatedAt)
	// Recording money returned on the OLD order must leave the new term intact.
	_, err = s.ProcessPresaleOffline(ctx, o.ID, 99, offlineRequest(cancelled, "refund"))
	require.NoError(t, err)
	latest := s.entClient.UserSubscription.GetX(ctx, sub.ID)
	require.True(t, latest.ExpiresAt.Equal(sub.ExpiresAt))
	require.Equal(t, sub.ResetCount, latest.ResetCount)
	require.Equal(t, SubscriptionStatusActive, latest.Status)
	require.Equal(t, 100.0, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
}

func TestPresaleOfflineActiveCancellationPreservesUsage(t *testing.T) {
	for _, mode := range []string{"cancel", "refund"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			s, o, sub := activePresaleRefundFixture(t, 3)
			anchor := time.Now().Add(-time.Hour).Truncate(time.Second)
			s.entClient.UserSubscription.UpdateOneID(sub.ID).SetDailyUsageUsd(11).SetWeeklyUsageUsd(22).SetMonthlyUsageUsd(33).SetWeeklyWindowStart(anchor).ExecX(ctx)
			req := offlineRequest(o, mode)
			if mode == "refund" {
				req.Amount = 124
			}
			for range 2 {
				_, err := s.ProcessPresaleOffline(ctx, o.ID, 42, req)
				require.NoError(t, err)
			}
			ended := s.entClient.UserSubscription.GetX(ctx, sub.ID)
			require.Equal(t, SubscriptionStatusExpired, ended.Status)
			require.Equal(t, 11.0, ended.DailyUsageUsd)
			require.Equal(t, 22.0, ended.WeeklyUsageUsd)
			require.Equal(t, 33.0, ended.MonthlyUsageUsd)
			require.True(t, ended.WeeklyWindowStart.Equal(anchor))
			require.Zero(t, ended.ResetCount)
			entitlement := s.entClient.SubscriptionConcurrencyEntitlement.Query().OnlyX(ctx)
			require.False(t, entitlement.ExpiresAt.After(time.Now()))
			expected := 248.0
			if mode == "refund" {
				expected = 124
				recorded := s.entClient.PaymentOrder.GetX(ctx, o.ID)
				require.Equal(t, OrderStatusPartiallyRefunded, recorded.Status)
				require.Equal(t, 155.0, recorded.RefundAmount)
				require.NotNil(t, recorded.RefundAt)
				require.Equal(t, 1, s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("PRESALE_MEMBERSHIP_REFUNDED")).CountX(ctx))
				req.Amount++
				_, err := s.ProcessPresaleOffline(ctx, o.ID, 42, req)
				require.Equal(t, "PRESALE_OFFLINE_STATE", infraerrors.Reason(err))
			}
			require.Equal(t, expected, s.entClient.User.GetX(ctx, o.UserID).TotalRecharged)
		})
	}
}

func TestPresaleOfflineRejectsInvalidOrStaleActions(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		change       func(*PresaleOfflineRequest)
	}{
		{"unconfirmed", "PRESALE_OFFLINE_INVALID", func(r *PresaleOfflineRequest) { r.Confirmed = false }},
		{"no version", "PRESALE_OFFLINE_INVALID", func(r *PresaleOfflineRequest) { r.ExpectedUpdatedAt = time.Time{} }},
		{"stale", "PRESALE_OFFLINE_STALE", func(r *PresaleOfflineRequest) { r.ExpectedUpdatedAt = r.ExpectedUpdatedAt.Add(-time.Second) }},
		{"no reason", "PRESALE_OFFLINE_INVALID", func(r *PresaleOfflineRequest) { r.Reason = " " }},
		{"no receipt", "PRESALE_OFFLINE_INVALID", func(r *PresaleOfflineRequest) { r.Reference = " " }},
		{"too much", "PRESALE_OFFLINE_AMOUNT", func(r *PresaleOfflineRequest) { r.Amount = 101 }},
		{"precision", "PRESALE_OFFLINE_AMOUNT", func(r *PresaleOfflineRequest) { r.Amount = 1.001 }},
		{"negative", "PRESALE_OFFLINE_INVALID", func(r *PresaleOfflineRequest) { r.Amount = -1 }},
		{"nan", "PRESALE_OFFLINE_INVALID", func(r *PresaleOfflineRequest) { r.Amount = math.NaN() }},
		{"infinite", "PRESALE_OFFLINE_INVALID", func(r *PresaleOfflineRequest) { r.Amount = math.Inf(1) }},
		{"cancel money", "PRESALE_OFFLINE_INVALID", func(r *PresaleOfflineRequest) { r.Mode = "cancel" }},
		{"unknown mode", "PRESALE_OFFLINE_INVALID", func(r *PresaleOfflineRequest) { r.Mode = "force" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s, u, p := newPresaleFixture(t)
			o := newPresaleOrder(t, s, u, p, NextPresalePeriod(time.Now()))
			require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
			o = s.entClient.PaymentOrder.GetX(ctx, o.ID)
			req := offlineRequest(o, "refund")
			tc.change(&req)
			_, err := s.ProcessPresaleOffline(ctx, o.ID, 99, req)
			require.Equal(t, tc.reason, infraerrors.Reason(err))
			require.Equal(t, OrderStatusCompleted, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
			require.Equal(t, 100.0, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
		})
	}
}

func TestPresaleOfflineRejectsOnlineAttemptsAndUnsafeStates(t *testing.T) {
	ctx := context.Background()
	for _, status := range []string{OrderStatusPending, OrderStatusPaid, OrderStatusRecharging, OrderStatusRefunding, OrderStatusRefundFailed, OrderStatusRefunded, OrderStatusPartiallyRefunded, OrderStatusCancelled, OrderStatusExpired} {
		t.Run("status/"+status, func(t *testing.T) {
			s, u, p := newPresaleFixture(t)
			o := newPresaleOrder(t, s, u, p, NextPresalePeriod(time.Now()))
			o = s.entClient.PaymentOrder.UpdateOneID(o.ID).SetStatus(status).SaveX(ctx)
			_, err := s.ProcessPresaleOffline(ctx, o.ID, 99, offlineRequest(o, "refund"))
			require.Equal(t, "PRESALE_OFFLINE_STATE", infraerrors.Reason(err))
		})
	}
	for _, action := range []string{"PRESALE_ONLINE_REFUND_STARTED", "REFUND_SUCCESS", "REFUND_FAILED", "REFUND_GATEWAY_FAILED", "REFUND_ROLLBACK_FAILED", "DEV_PAYMENT_REFUND_SUCCESS"} {
		t.Run("audit/"+action, func(t *testing.T) {
			s, u, p := newPresaleFixture(t)
			o := newPresaleOrder(t, s, u, p, NextPresalePeriod(time.Now()))
			o = s.entClient.PaymentOrder.UpdateOneID(o.ID).SetStatus(OrderStatusCompleted).SaveX(ctx)
			require.NoError(t, s.writeAuditLogStrict(ctx, o.ID, action, "test", map[string]any{}))
			for _, mode := range []string{"cancel", "refund"} {
				_, err := s.ProcessPresaleOffline(ctx, o.ID, 99, offlineRequest(o, mode))
				require.Equal(t, "PRESALE_OFFLINE_ONLINE_REFUND", infraerrors.Reason(err))
			}
		})
	}
}

func TestPresaleOfflineRefundRequestAndStaleOnlinePlan(t *testing.T) {
	ctx := context.Background()
	s, o, sub := activePresaleRefundFixture(t, 1)
	quote, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
	require.NoError(t, err)
	require.NoError(t, s.RequestRefund(ctx, o.ID, o.UserID, "cancel", quote.GatewayAmount))
	o = s.entClient.PaymentOrder.GetX(ctx, o.ID)
	plan, err := s.preparePresaleRefund(ctx, o, quote.RefundAmount, "refund")
	require.NoError(t, err)
	ended := s.entClient.UserSubscription.GetX(ctx, sub.ID)
	_, err = s.ProcessPresaleOffline(ctx, o.ID, 99, offlineRequest(o, "refund"))
	require.NoError(t, err)
	require.True(t, ended.ExpiresAt.Equal(s.entClient.UserSubscription.GetX(ctx, sub.ID).ExpiresAt))
	_, err = s.ExecuteRefund(ctx, plan)
	require.Error(t, err)
	require.Equal(t, OrderStatusRefunded, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
}

func TestPresaleOfflineAuditFailureRollsBackEverything(t *testing.T) {
	for _, action := range []string{"PRESALE_CANCELLED", "PRESALE_OFFLINE_REFUND", "PRESALE_MEMBERSHIP_REFUNDED"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			s, o, sub := activePresaleRefundFixture(t, 1)
			fail := true
			s.entClient.PaymentAuditLog.Use(func(next dbent.Mutator) dbent.Mutator {
				return dbent.MutateFunc(func(ctx context.Context, m dbent.Mutation) (dbent.Value, error) {
					mutation, ok := m.(*dbent.PaymentAuditLogMutation)
					if !ok {
						return next.Mutate(ctx, m)
					}
					a, _ := mutation.Action()
					if fail && a == action {
						return nil, errors.New("offline audit unavailable")
					}
					return next.Mutate(ctx, m)
				})
			})
			mode := "refund"
			if action == "PRESALE_CANCELLED" {
				mode = "cancel"
			}
			req := offlineRequest(o, mode)
			_, err := s.ProcessPresaleOffline(ctx, o.ID, 99, req)
			require.ErrorContains(t, err, "offline audit unavailable")
			require.Equal(t, OrderStatusCompleted, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
			require.Equal(t, 248.0, s.entClient.User.GetX(ctx, o.UserID).TotalRecharged)
			current := s.entClient.UserSubscription.GetX(ctx, sub.ID)
			require.Equal(t, SubscriptionStatusActive, current.Status)
			require.True(t, sub.ExpiresAt.Equal(current.ExpiresAt))
			require.Equal(t, sub.ResetCount, current.ResetCount)
			fail = false
			_, err = s.ProcessPresaleOffline(ctx, o.ID, 99, req)
			require.NoError(t, err)
		})
	}
}

func TestPresaleOfflineFailedUnfulfilledCurrencyAndAuthentication(t *testing.T) {
	ctx := context.Background()
	s, u, p := newPresaleFixture(t)
	o := newPresaleOrder(t, s, u, p, NextPresalePeriod(time.Now()))
	o = s.entClient.PaymentOrder.UpdateOneID(o.ID).SetStatus(OrderStatusFailed).SetProviderSnapshot(map[string]any{"currency": "JPY"}).SaveX(ctx)
	req := offlineRequest(o, "refund")
	req.Amount = 10.5
	_, err := s.ProcessPresaleOffline(ctx, o.ID, 99, req)
	require.Equal(t, "PRESALE_OFFLINE_AMOUNT", infraerrors.Reason(err))
	req.Amount = 100
	_, err = s.ProcessPresaleOffline(ctx, o.ID, 0, req)
	require.Equal(t, "FORBIDDEN", infraerrors.Reason(err))
	// No provider configuration needed; no grant means no membership deduction.
	_, err = s.ProcessPresaleOffline(ctx, o.ID, 99, req)
	require.NoError(t, err)
	require.Zero(t, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
	audit := s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(fmt.Sprint(o.ID)), paymentauditlog.ActionEQ("PRESALE_OFFLINE_REFUND")).OnlyX(ctx)
	require.Contains(t, audit.Detail, `"currency":"JPY"`)
}

func TestPresaleOnlineRefundMarkerPrecedesProviderRequest(t *testing.T) {
	ctx := context.Background()
	s, u, p := newPresaleFixture(t)
	o := newPresaleOrder(t, s, u, p, NextPresalePeriod(time.Now()))
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, s.entClient)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		exists, err := s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(fmt.Sprint(o.ID)), paymentauditlog.ActionEQ("PRESALE_ONLINE_REFUND_STARTED")).Exist(ctx)
		if err != nil || !exists {
			t.Error("provider was called without durable online-refund audit")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"ok"}`))
	}))
	defer server.Close()
	inst := s.entClient.PaymentProviderInstance.Create().SetProviderKey("easypay").SetName("local mock only").SetSupportedTypes("alipay").SetEnabled(true).SetRefundEnabled(true).SetConfig(encryptWebhookProviderConfig(t, map[string]string{
		"pid": "test", "pkey": "test", "apiBase": server.URL, "notifyUrl": server.URL, "returnUrl": server.URL,
	})).SaveX(ctx)
	s.loadBalancer = newWebhookProviderTestLoadBalancer(s.entClient)
	o = s.entClient.PaymentOrder.UpdateOneID(o.ID).SetStatus(OrderStatusRefunding).SetProviderInstanceID(fmt.Sprint(inst.ID)).SetPaymentTradeNo("mock-trade").SaveX(ctx)
	plan := &RefundPlan{OrderID: o.ID, Order: o, RefundAmount: 80, GatewayAmount: 80, Reason: "test"}
	failAudit := true
	s.entClient.PaymentAuditLog.Use(func(next dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(ctx context.Context, m dbent.Mutation) (dbent.Value, error) {
			mutation, ok := m.(*dbent.PaymentAuditLogMutation)
			if !ok {
				return next.Mutate(ctx, m)
			}
			action, _ := mutation.Action()
			if failAudit && action == "PRESALE_ONLINE_REFUND_STARTED" {
				return nil, errors.New("marker unavailable")
			}
			return next.Mutate(ctx, m)
		})
	})
	require.ErrorContains(t, s.gwRefund(ctx, plan), "marker unavailable")
	require.Zero(t, calls.Load())
	failAudit = false
	require.NoError(t, s.gwRefund(ctx, plan))
	require.Equal(t, int32(1), calls.Load())
	// Retries keep the original unique marker and may still reach the provider.
	require.NoError(t, s.gwRefund(ctx, plan))
	require.Equal(t, int32(2), calls.Load())
	require.Equal(t, 1, s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("PRESALE_ONLINE_REFUND_STARTED")).CountX(ctx))
	// Even if an uncertain online attempt restores the review status, offline
	// processing must not treat it as an order that never reached the provider.
	o = s.entClient.PaymentOrder.UpdateOneID(o.ID).SetStatus(OrderStatusRefundRequested).SaveX(ctx)
	_, err := s.ProcessPresaleOffline(ctx, o.ID, 99, offlineRequest(o, "refund"))
	require.Equal(t, "PRESALE_OFFLINE_ONLINE_REFUND", infraerrors.Reason(err))
}

type presaleOfflineAffiliateRepo struct {
	*affiliateRepoThresholdStub
	calls int
}

func (r *presaleOfflineAffiliateRepo) ClawbackQuotaForOrder(_ context.Context, _ int64, _ float64) (float64, error) {
	r.calls++
	if r.calls == 1 {
		return 0, errors.New("temporary affiliate failure")
	}
	return 0, nil
}
func TestPresaleOfflineRetryCompletesSecondaryBookkeepingOnly(t *testing.T) {
	ctx := context.Background()
	s, u, p := newPresaleFixture(t)
	o := newPresaleOrder(t, s, u, p, NextPresalePeriod(time.Now()))
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	o = s.entClient.PaymentOrder.GetX(ctx, o.ID)
	repo := &presaleOfflineAffiliateRepo{affiliateRepoThresholdStub: &affiliateRepoThresholdStub{}}
	s.affiliateService = &AffiliateService{repo: repo}
	req := offlineRequest(o, "refund")
	result, err := s.ProcessPresaleOffline(ctx, o.ID, 99, req)
	require.NoError(t, err)
	require.True(t, result.AffiliatePending)
	result, err = s.ProcessPresaleOffline(ctx, o.ID, 99, req)
	require.NoError(t, err)
	require.False(t, result.AffiliatePending)
	require.Equal(t, 2, repo.calls)
	require.Zero(t, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
	require.Equal(t, 1, s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("PRESALE_OFFLINE_REFUND")).CountX(ctx))
	require.Equal(t, 1, s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("PRESALE_MEMBERSHIP_REFUNDED")).CountX(ctx))
}
