//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/cafecampaignuse"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func postgresCafeCampaign(t *testing.T, s *service.PaymentService) *dbent.CafeCampaign {
	t.Helper()
	now := time.Now().UTC()
	c, err := s.CreateCafeCampaign(context.Background(), 99, service.CreateCafeCampaignRequest{Code: fmt.Sprintf("TEST-%d", time.Now().UnixNano()), Name: "Isolated test", DiscountPercent: 40, StartDate: now.AddDate(0, 0, -1).Format("2006-01-02"), EndDate: now.AddDate(0, 0, 2).Format("2006-01-02"), Enabled: true})
	require.NoError(t, err)
	return c
}
func TestCafeCampaignPostgresConstraintsAndAdminAudits(t *testing.T) {
	ctx := context.Background()
	s, u, p := newPresaleSafetyPostgresFixture(t)
	c := postgresCafeCampaign(t, s)
	db := integrationEntClient
	for _, enabled := range []bool{false, true, false, true} {
		var err error
		c, err = s.SetCafeCampaignEnabled(ctx, c.ID, 99, enabled, c.UpdatedAt)
		require.NoError(t, err)
	}
	o := newPresaleSafetyPostgresOrder(t, u, p, service.NextPresalePeriod(time.Now()))
	db.CafeCampaignUse.Create().SetCampaignID(c.ID).SetUserID(u.ID).SetOrderID(o.ID).SaveX(ctx)
	another := newPresaleSafetyPostgresOrder(t, u, p, service.NextPresalePeriod(time.Now()))
	_, err := db.CafeCampaignUse.Create().SetCampaignID(c.ID).SetUserID(u.ID).SetOrderID(another.ID).Save(ctx)
	require.True(t, dbent.IsConstraintError(err))
	_, err = db.ExecContext(ctx, "UPDATE cafe_campaigns SET discount_percent = 101 WHERE id=$1", c.ID)
	require.Error(t, err)
	_, err = db.ExecContext(ctx, "UPDATE cafe_campaigns SET expires_at = starts_at WHERE id=$1", c.ID)
	require.Error(t, err)
	_, err = db.ExecContext(ctx, "DELETE FROM cafe_campaigns WHERE id=$1", c.ID)
	require.Error(t, err, "financial usage cannot be erased with its campaign")
}

func TestCafeCampaignPostgresConcurrentCheckout(t *testing.T) {
	// Guarded simulated payment, disposable PostgreSQL only, no real gateway.
	t.Setenv("PAYMENT_DEV_AUTO_SUCCESS", "I_UNDERSTAND_THIS_BYPASSES_REAL_PAYMENTS")
	t.Setenv("PAYMENT_DEV_ENVIRONMENT", "local")
	for _, key := range []string{"APP_ENV", "ENVIRONMENT", "NODE_ENV", "GO_ENV"} {
		t.Setenv(key, "test")
	}
	ctx := context.Background()
	s, u, p := newPresaleSafetyPostgresFixture(t)
	db := integrationEntClient
	c := postgresCafeCampaign(t, s)
	settings := NewSettingRepository(db)
	require.NoError(t, settings.Set(ctx, service.SettingPaymentEnabled, "true"))
	require.NoError(t, settings.Set(ctx, service.SettingEnabledPaymentTypes, "alipay"))
	// Different source groups avoid relying on the separate same-group presale lock.
	g := db.Group.Create().SetName(fmt.Sprintf("other-%d", u.ID)).SetPlatform(service.PlatformOpenAI).SetStatus(service.StatusActive).SetSubscriptionType(service.SubscriptionTypeSubscription).SaveX(ctx)
	other := db.SubscriptionPlan.Create().SetName("Other").SetGroupID(g.ID).SetPrice(100).SetForSale(true).SetPresaleEnabled(true).SaveX(ctx)
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, plan := range []*dbent.SubscriptionPlan{p, other} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = s.CreateOrder(ctx, service.CreateOrderRequest{UserID: u.ID, PlanID: plan.ID, Multiplier: 1, OrderType: payment.OrderTypeSubscription, PaymentType: payment.TypeAlipay, PresaleMonth: service.NextPresalePeriod(time.Now()).Month, CafeCouponCode: c.Code, ClientIP: "127.0.0.1", SrcHost: "localhost"})
		}()
	}
	close(start)
	wg.Wait()
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else {
			require.Contains(t, []string{"CAFE_CAMPAIGN_RESERVED", "CAFE_CAMPAIGN_USAGE_LIMIT"}, infraerrors.Reason(err), err)
		}
	}
	require.Equal(t, 1, successes)
	use := db.CafeCampaignUse.Query().Where(cafecampaignuse.CampaignIDEQ(c.ID), cafecampaignuse.UserIDEQ(u.ID)).OnlyX(ctx)
	require.NotNil(t, use.UsedAt)
	o := db.PaymentOrder.GetX(ctx, use.OrderID)
	require.Equal(t, service.OrderStatusCompleted, o.Status)
	require.Equal(t, 60.0, o.PayAmount)
	// Multiple verified duplicate notifications remain idempotent.
	errs = make([]error, 3)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = s.HandlePaymentNotification(ctx, &payment.PaymentNotification{OrderID: o.OutTradeNo, TradeNo: o.PaymentTradeNo, Amount: 60, Status: payment.NotificationStatusSuccess}, payment.TypeAlipay)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, 60.0, db.User.GetX(ctx, u.ID).TotalRecharged)
	_, err := s.ProcessPresaleOffline(ctx, o.ID, 99, service.PresaleOfflineRequest{Mode: "refund", Amount: 60, Reference: "test", Reason: "test", Confirmed: true, ExpectedUpdatedAt: o.UpdatedAt})
	require.NoError(t, err)
	_, err = s.PreviewCafeCouponForOrder(ctx, service.CreateOrderRequest{UserID: u.ID, PlanID: p.ID, Multiplier: 1, OrderType: payment.OrderTypeSubscription, PresaleMonth: service.NextPresalePeriod(time.Now()).Month, CafeCouponCode: c.Code})
	require.Equal(t, "CAFE_CAMPAIGN_USAGE_LIMIT", infraerrors.Reason(err))
}

func TestCafeCampaignPostgresDelayedConcurrentCallbacksKeepFirstReceipt(t *testing.T) {
	ctx := context.Background()
	s, u, p := newPresaleSafetyPostgresFixture(t)
	c := postgresCafeCampaign(t, s)
	db := integrationEntClient
	o := newPresaleSafetyPostgresOrder(t, u, p, service.NextPresalePeriod(time.Now()))
	// Persist the state left by a timely verified payment whose coupon audit
	// failed. Retry is now beyond the deadline/grace; its first receipt is not.
	now := time.Now().UTC().Truncate(time.Microsecond)
	paidAt, expiresAt := now.Add(-10*time.Minute), now.Add(-6*time.Minute)
	db.PaymentOrder.UpdateOneID(o.ID).SetStatus(service.OrderStatusFailed).
		SetPaidAt(paidAt).SetExpiresAt(expiresAt).SetPayAmount(60).
		SetPaymentTradeNo("first-receipt").SetCafeCouponCode(c.Code).SetCafeCouponDiscount(40).
		SetFailedReason("transient consume audit failure").ExecX(ctx)
	_, err := db.ExecContext(ctx, "UPDATE payment_orders SET created_at=$1 WHERE id=$2", paidAt.Add(-time.Minute), o.ID)
	require.NoError(t, err)
	db.CafeCampaignUse.Create().SetCampaignID(c.ID).SetUserID(u.ID).SetOrderID(o.ID).SaveX(ctx)
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = s.HandlePaymentNotification(ctx, &payment.PaymentNotification{OrderID: o.OutTradeNo, TradeNo: "first-receipt", Amount: 60, Status: payment.NotificationStatusSuccess}, payment.TypeAlipay)
		}()
	}
	close(start)
	wg.Wait()
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else {
			require.Equal(t, "CONFLICT", infraerrors.Reason(err), "only a concurrent fulfillment lease may defer a retry")
		}
	}
	require.Positive(t, successes)
	current := db.PaymentOrder.GetX(ctx, o.ID)
	require.Equal(t, service.OrderStatusCompleted, current.Status)
	require.True(t, current.PaidAt.Equal(paidAt))
	require.Equal(t, "first-receipt", current.PaymentTradeNo)
	require.Equal(t, 60.0, current.PayAmount)
	require.Equal(t, 60.0, db.User.GetX(ctx, u.ID).TotalRecharged)
	use := db.CafeCampaignUse.Query().Where(cafecampaignuse.OrderIDEQ(o.ID)).OnlyX(ctx)
	require.True(t, use.UsedAt.Equal(paidAt))
	require.Equal(t, 1, db.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(fmt.Sprint(o.ID)), paymentauditlog.ActionEQ("CAFE_CAMPAIGN_USED")).CountX(ctx))
}

func TestCafeCampaignPostgresUnicodeNameLimit(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newPresaleSafetyPostgresFixture(t)
	for _, char := range []string{"券", "🌙"} {
		name := strings.Repeat(char, 100)
		c, err := s.CreateCafeCampaign(ctx, 99, service.CreateCafeCampaignRequest{Code: fmt.Sprintf("UNICODE-%d", time.Now().UnixNano()), Name: name, DiscountPercent: 40, StartDate: "2026-09-25", EndDate: "2026-09-30"})
		require.NoError(t, err)
		require.Equal(t, name, integrationEntClient.CafeCampaign.GetX(ctx, c.ID).Name)
	}
}
