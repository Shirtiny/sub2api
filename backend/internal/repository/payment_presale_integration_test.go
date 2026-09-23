//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Runs exclusively against the integration harness's disposable PostgreSQL.
// The guarded dev provider never sends a payment request to a real gateway.
func TestPresalePostgresConcurrentPurchaseAndActivation(t *testing.T) {
	t.Setenv("PAYMENT_DEV_AUTO_SUCCESS", "I_UNDERSTAND_THIS_BYPASSES_REAL_PAYMENTS")
	t.Setenv("PAYMENT_DEV_ENVIRONMENT", "local")
	for _, key := range []string{"APP_ENV", "ENVIRONMENT", "NODE_ENV", "GO_ENV"} {
		t.Setenv(key, "test")
	}
	ctx := context.Background()
	client := integrationEntClient
	u := client.User.Create().SetEmail(fmt.Sprintf("presale-%d@example.com", time.Now().UnixNano())).SetPasswordHash("test-only").SetUsername("presale-test").SaveX(ctx)
	group := client.Group.Create().SetName("Presale test").SetPlatform(service.PlatformOpenAI).SetSubscriptionType(service.SubscriptionTypeSubscription).SetStatus(service.StatusActive).SaveX(ctx)
	plan := client.SubscriptionPlan.Create().SetGroupID(group.ID).SetName("Presale PG").SetPrice(100).SetPresaleEnabled(true).SetPresaleVisible(true).SetForSale(true).SetConcurrency(4).SetPresaleResetCards(3).SaveX(ctx)
	settings := NewSettingRepository(client)
	require.NoError(t, settings.Set(ctx, service.SettingPaymentEnabled, "true"))
	cfg := service.NewPaymentConfigService(client, settings, nil)
	groups := NewGroupRepository(client, integrationDB, nil)
	subs := service.NewSubscriptionService(groups, NewUserSubscriptionRepository(client), nil, client, nil)
	defer subs.Stop()
	svc := service.NewPaymentService(client, payment.NewRegistry(), nil, nil, subs, cfg, NewUserRepository(client, integrationDB), groups, nil)
	period := service.NextPresalePeriod(time.Now())
	req := service.CreateOrderRequest{UserID: u.ID, OrderType: payment.OrderTypeSubscription, PlanID: plan.ID, PaymentType: payment.TypeAlipay, PresaleMonth: period.Month, ClientIP: "127.0.0.1", SrcHost: "localhost"}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := svc.CreateOrder(ctx, req); results <- err }()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	require.Equal(t, 1, successes, "only one concurrent purchase may occupy a user/group/month")
	orders := client.PaymentOrder.Query().Where(paymentorder.UserIDEQ(u.ID)).AllX(ctx)
	require.Len(t, orders, 1)
	order := orders[0]
	require.Equal(t, service.OrderStatusCompleted, order.Status)
	require.True(t, order.PresaleStartsAt.Equal(period.StartsAt))
	require.True(t, order.PresaleExpiresAt.Equal(period.ExpiresAt))
	require.Zero(t, client.UserSubscription.Query().Where(usersubscription.UserIDEQ(u.ID)).CountX(ctx))
	require.Equal(t, 100.0, client.User.GetX(ctx, u.ID).TotalRecharged)
	// Change plan after payment; the reserved term and grant must use snapshots.
	client.SubscriptionPlan.UpdateOneID(plan.ID).SetPresaleResetCards(99).SetPrice(300).ExecX(ctx)
	results = make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := svc.ActivateDuePresales(ctx, period.StartsAt); results <- err }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	sub := client.UserSubscription.Query().Where(usersubscription.UserIDEQ(u.ID)).OnlyX(ctx)
	require.True(t, sub.StartsAt.Equal(period.StartsAt))
	require.True(t, sub.ExpiresAt.Equal(period.ExpiresAt))
	require.Equal(t, 3, sub.ResetCount)
	require.NotNil(t, client.PaymentOrder.GetX(ctx, order.ID).PresaleActivatedAt)
}

// Exercise the real repository's early-reset CAS and refund row locks, not the
// SQLite unit-test adapter. No payment gateway or production database is used.
func TestPresalePostgresEarlyResetRefund(t *testing.T) {
	for _, multiplier := range []int{1, 3} {
		t.Run(fmt.Sprint(multiplier), func(t *testing.T) {
			ctx := context.Background()
			client := integrationEntClient
			u := client.User.Create().SetEmail(fmt.Sprintf("presale-refund-%d@example.com", time.Now().UnixNano())).SetPasswordHash("test-only").SetUsername("presale-refund").SaveX(ctx)
			group := client.Group.Create().SetName(fmt.Sprintf("Presale refund test %d", u.ID)).SetPlatform(service.PlatformOpenAI).SetSubscriptionType(service.SubscriptionTypeSubscription).SetStatus(service.StatusActive).SaveX(ctx)
			plan := client.SubscriptionPlan.Create().SetGroupID(group.ID).SetName("Presale refund PG").SetPrice(310).SetConcurrency(4).SaveX(ctx)
			groups := NewGroupRepository(client, integrationDB, nil)
			subs := service.NewSubscriptionService(groups, NewUserSubscriptionRepository(client), nil, client, nil)
			defer subs.Stop()
			cfg := service.NewPaymentConfigService(client, NewSettingRepository(client), nil)
			svc := service.NewPaymentService(client, payment.NewRegistry(), nil, nil, subs, cfg, NewUserRepository(client, integrationDB), groups, nil)
			now := time.Now()
			// A synthetic paid window keeps the test independent of the day of month.
			start := now.Add(-2 * time.Hour).Truncate(time.Second)
			end := start.Add(31 * 24 * time.Hour)
			order := client.PaymentOrder.Create().SetUserID(u.ID).SetUserEmail(u.Email).SetUserName(u.Username).
				SetAmount(310).SetPayAmount(248).SetRechargeCode(fmt.Sprintf("presale-refund-%d", u.ID)).
				SetOutTradeNo(fmt.Sprintf("presale-refund-%d", u.ID)).SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").
				SetOrderType(payment.OrderTypeSubscription).SetPlanID(plan.ID).SetSubscriptionGroupID(group.ID).SetSubscriptionSourceGroupID(group.ID).
				SetSubscriptionDays(31).SetSubscriptionConcurrency(4).SetSubscriptionMultiplier(multiplier).
				SetSubscriptionEarlyResetEnabled(true).SetSubscriptionEarlyResetDurationDays(2).
				SetStatus(service.OrderStatusCompleted).SetPaidAt(now).SetExpiresAt(now.Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("localhost").
				SetPresaleStartsAt(start).SetPresaleExpiresAt(end).SetPresalePlanName(plan.Name).SetPresaleResetCards(2).SaveX(ctx)
			_, err := svc.ActivateDuePresales(ctx, now)
			require.NoError(t, err)
			order = client.PaymentOrder.GetX(ctx, order.ID)
			require.NotNil(t, order.PresaleActivatedAt)
			sub := client.UserSubscription.Query().Where(usersubscription.UserIDEQ(u.ID)).OnlyX(ctx)
			beforeReset, err := svc.GetPresaleRefundQuote(ctx, order, now)
			require.NoError(t, err)
			_, err = subs.EarlyResetSubscription(ctx, u.ID, sub.ID)
			require.NoError(t, err)
			err = svc.RequestRefund(ctx, order.ID, u.ID, "stale quote", beforeReset.GatewayAmount)
			require.Equal(t, "PRESALE_REFUND_AMOUNT_CHANGED", infraerrors.Reason(err))
			require.Equal(t, service.SubscriptionStatusActive, client.UserSubscription.GetX(ctx, sub.ID).Status)
			quote, err := svc.GetPresaleRefundQuote(ctx, order, time.Now())
			require.NoError(t, err)
			require.Equal(t, 28, quote.UnusedDays)
			require.Equal(t, 280.0, quote.RefundAmount)
			require.Equal(t, 224.0, quote.GatewayAmount)
			require.NoError(t, svc.RequestRefund(ctx, order.ID, u.ID, "reviewed refund", quote.GatewayAmount))
			require.Equal(t, service.SubscriptionStatusExpired, client.UserSubscription.GetX(ctx, sub.ID).Status)
			order = client.PaymentOrder.GetX(ctx, order.ID)
			require.True(t, order.PresaleExpiresAt.Equal(end))
			frozen, err := svc.GetPresaleRefundQuote(ctx, order, now.AddDate(0, 2, 0))
			require.NoError(t, err)
			require.Equal(t, quote, frozen)
		})
	}
}
