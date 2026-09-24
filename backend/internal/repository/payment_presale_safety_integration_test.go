//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// All fixtures use the harness's disposable PostgreSQL, never a configured app
// database. Notifications are verified-payment fixtures, not network requests.
func newPresaleSafetyPostgresFixture(t *testing.T) (*service.PaymentService, *dbent.User, *dbent.SubscriptionPlan) {
	t.Helper()
	ctx := context.Background()
	c := integrationEntClient
	u := c.User.Create().SetEmail(fmt.Sprintf("presale-safety-%d@example.com", time.Now().UnixNano())).SetPasswordHash("test").SaveX(ctx)
	g := c.Group.Create().SetName(fmt.Sprintf("Presale safety %d", u.ID)).SetPlatform(service.PlatformOpenAI).SetStatus(service.StatusActive).SetSubscriptionType(service.SubscriptionTypeSubscription).SaveX(ctx)
	p := c.SubscriptionPlan.Create().SetGroupID(g.ID).SetName("Safety plan").SetPrice(100).SetForSale(true).SetPresaleEnabled(true).SaveX(ctx)
	groups := NewGroupRepository(c, integrationDB, nil)
	subs := service.NewSubscriptionService(groups, NewUserSubscriptionRepository(c), nil, c, nil)
	t.Cleanup(subs.Stop)
	cfg := service.NewPaymentConfigService(c, NewSettingRepository(c), nil)
	return service.NewPaymentService(c, payment.NewRegistry(), nil, nil, subs, cfg, NewUserRepository(c, integrationDB), groups, nil), u, p
}

func newPresaleSafetyPostgresOrder(t *testing.T, u *dbent.User, p *dbent.SubscriptionPlan, period service.PresalePeriod) *dbent.PaymentOrder {
	t.Helper()
	return integrationEntClient.PaymentOrder.Create().SetUserID(u.ID).SetUserEmail(u.Email).SetUserName(u.Username).
		SetAmount(100).SetPayAmount(100).SetRechargeCode("presale-safety").SetOutTradeNo(fmt.Sprintf("presale-safety-%d", time.Now().UnixNano())).
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(p.ID).SetSubscriptionGroupID(p.GroupID).SetSubscriptionSourceGroupID(p.GroupID).
		SetSubscriptionDays(31).SetSubscriptionMultiplier(1).SetSubscriptionConcurrency(1).
		SetPresaleStartsAt(period.StartsAt).SetPresaleExpiresAt(period.ExpiresAt).SetPresalePlanName(p.Name).
		SetStatus(service.OrderStatusPending).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("localhost").SaveX(context.Background())
}

func TestPresalePostgresLateCallbacksPreserveReplacement(t *testing.T) {
	s, u, p := newPresaleSafetyPostgresFixture(t)
	ctx := context.Background()
	period := service.NextPresalePeriod(time.Now())
	old := newPresaleSafetyPostgresOrder(t, u, p, period)
	integrationEntClient.PaymentOrder.UpdateOneID(old.ID).SetStatus(service.OrderStatusCancelled).ExecX(ctx)
	newest := newPresaleSafetyPostgresOrder(t, u, p, period)
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 2)
	for i, order := range []*dbent.PaymentOrder{old, newest} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = s.HandlePaymentNotification(ctx, &payment.PaymentNotification{
				OrderID: order.OutTradeNo, TradeNo: fmt.Sprint(order.ID), Amount: 100, Status: payment.NotificationStatusSuccess,
			}, payment.TypeAlipay)
		}()
	}
	close(start)
	wg.Wait()
	require.Equal(t, "PRESALE_ALREADY_RESERVED", infraerrors.Reason(errs[0]))
	require.NoError(t, errs[1])
	require.Equal(t, service.OrderStatusCompleted, integrationEntClient.PaymentOrder.GetX(ctx, newest.ID).Status)
	failed := integrationEntClient.PaymentOrder.GetX(ctx, old.ID)
	require.Equal(t, service.OrderStatusFailed, failed.Status)
	require.NotNil(t, failed.PaidAt)
	require.Equal(t, 100.0, integrationEntClient.User.GetX(ctx, u.ID).TotalRecharged)
	require.Equal(t, "PRESALE_ALREADY_RESERVED", infraerrors.Reason(s.RetryFulfillment(ctx, old.ID)))
	_, err := s.ActivateDuePresales(ctx, period.StartsAt)
	require.NoError(t, err)
	require.NotNil(t, integrationEntClient.PaymentOrder.GetX(ctx, newest.ID).PresaleActivatedAt)
	require.Nil(t, integrationEntClient.PaymentOrder.GetX(ctx, old.ID).PresaleActivatedAt)
	require.Equal(t, 1, integrationEntClient.UserSubscription.Query().Where(usersubscription.UserIDEQ(u.ID)).CountX(ctx))
}

func TestPresalePostgresRefundPointsSerializeAndDeductOnce(t *testing.T) {
	t.Setenv("PAYMENT_DEV_AUTO_SUCCESS", "I_UNDERSTAND_THIS_BYPASSES_REAL_PAYMENTS")
	t.Setenv("PAYMENT_DEV_ENVIRONMENT", "local")
	for _, key := range []string{"APP_ENV", "ENVIRONMENT", "NODE_ENV", "GO_ENV"} {
		t.Setenv(key, "test")
	}
	s, u, p := newPresaleSafetyPostgresFixture(t)
	ctx := context.Background()
	c := integrationEntClient
	c.User.UpdateOneID(u.ID).SetTotalRecharged(37).ExecX(ctx)
	plans := make([]*service.RefundPlan, 2)
	for i := range plans {
		period := service.NextPresalePeriod(time.Now().AddDate(0, i+1, 0))
		o := newPresaleSafetyPostgresOrder(t, u, p, period)
		c.PaymentOrder.UpdateOneID(o.ID).SetStatus(service.OrderStatusPaid).SetPaidAt(time.Now()).SetPaymentTradeNo("dev-auto-success-" + o.OutTradeNo).ExecX(ctx)
		c.PaymentAuditLog.Create().SetOrderID(fmt.Sprint(o.ID)).SetAction("DEV_PAYMENT_AUTO_SUCCESS").SetOperator("test").SetDetail(`{}`).SaveX(ctx)
		require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
		require.NoError(t, s.RequestRefund(ctx, o.ID, u.ID, "test refund", 100))
		var err error
		plans[i], _, err = s.PrepareRefund(ctx, o.ID, 100, "test refund", false, false)
		require.NoError(t, err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 4)
	for _, plan := range plans {
		for range 2 {
			copy := *plan // ExecuteRefund updates its plan while claiming.
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := s.ExecuteRefund(ctx, &copy)
				results <- err
			}()
		}
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	require.Equal(t, 2, success)
	require.Equal(t, 37.0, c.User.GetX(ctx, u.ID).TotalRecharged)
	for _, plan := range plans {
		require.Equal(t, service.OrderStatusRefunded, c.PaymentOrder.GetX(ctx, plan.OrderID).Status)
		require.Equal(t, 1, c.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(fmt.Sprint(plan.OrderID)), paymentauditlog.ActionEQ("PRESALE_MEMBERSHIP_REFUNDED")).CountX(ctx))
	}
}

func TestPresalePostgresActivationRacingRefundCannotWaiveFee(t *testing.T) {
	s, u, p := newPresaleSafetyPostgresFixture(t)
	ctx := context.Background()
	c := integrationEntClient
	period := service.NextPresalePeriod(time.Now())
	period.StartsAt = time.Now().Add(-time.Hour)
	period.ExpiresAt = period.StartsAt.Add(31 * 24 * time.Hour)
	o := newPresaleSafetyPostgresOrder(t, u, p, period)
	c.PaymentOrder.UpdateOneID(o.ID).SetStatus(service.OrderStatusPaid).SetPaidAt(time.Now()).ExecX(ctx)
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	q, err := s.GetPresaleRefundQuote(ctx, c.PaymentOrder.GetX(ctx, o.ID), time.Now())
	require.NoError(t, err)
	require.Equal(t, 80.0, q.GatewayAmount)
	var wg sync.WaitGroup
	var refundErr, activationErr error
	start := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		refundErr = s.RequestRefund(ctx, o.ID, u.ID, "cancel", q.GatewayAmount)
	}()
	go func() { defer wg.Done(); <-start; _, activationErr = s.ActivateDuePresales(ctx, time.Now()) }()
	close(start)
	wg.Wait()
	require.NoError(t, activationErr)
	o = c.PaymentOrder.GetX(ctx, o.ID)
	if refundErr == nil {
		require.Equal(t, service.OrderStatusRefundRequested, o.Status)
		require.Nil(t, o.PresaleActivatedAt)
	} else {
		require.Equal(t, "PRESALE_REFUND_AMOUNT_CHANGED", infraerrors.Reason(refundErr))
		require.Equal(t, service.OrderStatusCompleted, o.Status)
		require.NotNil(t, o.PresaleActivatedAt)
	}
	q, err = s.GetPresaleRefundQuote(ctx, o, time.Now())
	require.NoError(t, err)
	require.Equal(t, 20, q.FeePercent)
	require.LessOrEqual(t, q.GatewayAmount, 80.0)
}
