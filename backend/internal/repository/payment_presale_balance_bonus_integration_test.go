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
	"github.com/Wei-Shaw/sub2api/ent/promotionactivityparticipation"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPresaleBalanceGiftPostgresConcurrentPurchaseAndRefund(t *testing.T) {
	t.Setenv("PAYMENT_DEV_AUTO_SUCCESS", "I_UNDERSTAND_THIS_BYPASSES_REAL_PAYMENTS")
	t.Setenv("PAYMENT_DEV_ENVIRONMENT", "local")
	for _, key := range []string{"APP_ENV", "ENVIRONMENT", "NODE_ENV", "GO_ENV"} {
		t.Setenv(key, "test")
	}
	ctx := context.Background()
	s, u, p := newPresaleSafetyPostgresFixture(t)
	c := integrationEntClient
	// This fixture commits real transactions. Remove its history as well as FKs:
	// later repository suites run against the same disposable database.
	var activityID, otherPlanID, otherGroupID int64
	t.Cleanup(func() {
		for _, stmt := range []string{
			"DELETE FROM promotion_activity_participations WHERE user_id=$1",
			"DELETE FROM redeem_codes WHERE used_by=$1",
			"DELETE FROM payment_audit_logs WHERE order_id IN (SELECT id::text FROM payment_orders WHERE user_id=$1)",
			"DELETE FROM payment_orders WHERE user_id=$1",
		} {
			_, err := c.ExecContext(context.Background(), stmt, u.ID)
			require.NoError(t, err)
		}
		for _, item := range []struct {
			query string
			args  []any
		}{
			{"DELETE FROM promotion_activity_plans WHERE activity_id=$1", []any{activityID}},
			{"DELETE FROM promotion_activities WHERE id=$1", []any{activityID}},
			{"DELETE FROM subscription_plans WHERE id IN ($1,$2)", []any{p.ID, otherPlanID}},
			{"DELETE FROM groups WHERE id IN ($1,$2)", []any{p.GroupID, otherGroupID}},
			{"DELETE FROM users WHERE id=$1", []any{u.ID}},
		} {
			_, err := c.ExecContext(context.Background(), item.query, item.args...)
			require.NoError(t, err)
		}
	})
	settings := NewSettingRepository(c)
	require.NoError(t, settings.Set(ctx, service.SettingPaymentEnabled, "true"))
	require.NoError(t, settings.Set(ctx, service.SettingEnabledPaymentTypes, "alipay"))
	previousRate, _ := settings.Get(ctx, service.SettingBalanceRechargeMult)
	t.Cleanup(func() {
		value := "1"
		if previousRate != nil {
			value = previousRate.Value
		}
		_ = settings.Set(ctx, service.SettingBalanceRechargeMult, value)
	})
	require.NoError(t, settings.Set(ctx, service.SettingBalanceRechargeMult, "0.2"))
	g := c.Group.Create().SetName(fmt.Sprintf("gift-%d", u.ID)).SetPlatform(service.PlatformOpenAI).SetStatus(service.StatusActive).SetSubscriptionType(service.SubscriptionTypeSubscription).SaveX(ctx)
	other := c.SubscriptionPlan.Create().SetName("Other gift").SetGroupID(g.ID).SetPrice(100).SetForSale(true).SetPresaleEnabled(true).SaveX(ctx)
	otherPlanID, otherGroupID = other.ID, g.ID
	cfg := service.NewPaymentConfigService(c, settings, nil)
	a, err := cfg.CreatePromotionActivity(ctx, service.UpsertPromotionActivityRequest{Name: "Isolated gift", Type: service.PromotionActivityTypePresaleBalance, BonusCurrency: "CNY", Enabled: true, StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(time.Hour), MaxUsesPerUser: 1, PlanBonuses: []service.PromotionActivityPlanInput{{PlanID: p.ID, BonusBalance: 50}, {PlanID: other.ID, BonusBalance: 50}}})
	require.NoError(t, err)
	activityID = a.ID
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 2)
	for i, plan := range []*dbent.SubscriptionPlan{p, other} {
		quote, err := s.GetPresaleQuote(ctx, u.ID, plan.ID)
		require.NoError(t, err)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = s.CreateOrder(ctx, service.CreateOrderRequest{UserID: u.ID, PlanID: plan.ID, Multiplier: 1, OrderType: payment.OrderTypeSubscription, PaymentType: payment.TypeAlipay, PresaleMonth: service.NextPresalePeriod(time.Now()).Month, ExpectedSubscriptionBonusActivityID: a.ID, ExpectedPresaleBonusVersion: quote.BalanceBonus.Version, ClientIP: "127.0.0.1", SrcHost: "localhost"})
		}()
	}
	close(start)
	wg.Wait()
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else {
			require.Equal(t, "ACTIVITY_BENEFIT_CHANGED", infraerrors.Reason(err), err)
		}
	}
	require.Equal(t, 1, successes)
	part := c.PromotionActivityParticipation.Query().Where(promotionactivityparticipation.ActivityIDEQ(a.ID)).OnlyX(ctx)
	o := c.PaymentOrder.GetX(ctx, part.OrderID)
	require.Equal(t, 10.0, c.User.GetX(ctx, u.ID).Balance)
	require.Equal(t, 100.0, c.User.GetX(ctx, u.ID).TotalRecharged)
	require.Equal(t, .2, o.PresaleBalanceBonusRate)
	require.Equal(t, "CNY", o.PresaleBalanceBonusCurrency)
	require.Equal(t, 50.0, o.PresaleBalanceBonusFaceAmount)
	// Gateway redelivery must not create additional gift history.
	errs = make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = s.HandlePaymentNotification(ctx, &payment.PaymentNotification{OrderID: o.OutTradeNo, TradeNo: o.PaymentTradeNo, Amount: o.PayAmount, Status: payment.NotificationStatusSuccess}, payment.TypeAlipay)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, 1, c.RedeemCode.Query().Where(redeemcode.UsedByEQ(u.ID)).CountX(ctx))
	// Change the runtime rate; settlement must keep the order's .2 snapshot.
	require.NoError(t, settings.Set(ctx, service.SettingBalanceRechargeMult, "2"))
	c.User.UpdateOneID(u.ID).SetBalance(6).ExecX(ctx)
	q, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
	require.NoError(t, err)
	require.Equal(t, 20.0, q.BalanceBonusDeduction)
	require.Equal(t, 6.0, q.BalanceBonusReclaim)
	errs = make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = s.RequestRefund(ctx, o.ID, u.ID, "integration test", q.GatewayAmount)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.Zero(t, c.User.GetX(ctx, u.ID).Balance)
	require.Equal(t, 2, c.RedeemCode.Query().Where(redeemcode.UsedByEQ(u.ID)).CountX(ctx))
	require.Equal(t, 1, c.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(fmt.Sprint(o.ID)), paymentauditlog.ActionEQ("PRESALE_BALANCE_BONUS_RECLAIMED")).CountX(ctx))
	part = c.PromotionActivityParticipation.GetX(ctx, part.ID)
	require.NotNil(t, part.BalanceReclaimedAt)
	require.Equal(t, service.PromotionParticipationStatusGranted, part.Status)
	// The migration's money constraints fail closed on mismatched snapshots.
	for _, stmt := range []string{
		"UPDATE payment_orders SET presale_balance_bonus_activity_id=NULL WHERE id=$1",
		"UPDATE payment_orders SET presale_balance_bonus_rate=0 WHERE id=$1",
		"UPDATE payment_orders SET presale_balance_bonus_currency='EUR' WHERE id=$1",
		"UPDATE payment_orders SET presale_balance_bonus_face_amount=0 WHERE id=$1",
		"UPDATE payment_orders SET presale_balance_bonus_amount='NaN' WHERE id=$1",
	} {
		_, err = c.ExecContext(ctx, stmt, o.ID)
		require.Error(t, err)
	}
	_, err = c.ExecContext(ctx, "UPDATE promotion_activity_participations SET bonus_days=2 WHERE id=$1", part.ID)
	require.Error(t, err)
}
