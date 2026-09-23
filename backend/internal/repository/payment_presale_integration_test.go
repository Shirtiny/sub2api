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
