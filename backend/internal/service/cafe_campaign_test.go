//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func newCafeCampaignFixture(t *testing.T) (*PaymentService, *dbent.User, *dbent.SubscriptionPlan, *dbent.CafeCampaign) {
	t.Helper()
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	s.configService.settingRepo = cafeCouponSettingsRepo(map[string]string{SettingPaymentEnabled: "true", SettingEnabledPaymentTypes: "alipay"})
	s.userRepo = &mockUserRepo{getByIDUser: &User{ID: u.ID, Email: u.Email, Username: u.Username, Status: StatusActive}}
	now := time.Now().In(presaleLocation)
	c, err := s.CreateCafeCampaign(ctx, 99, CreateCafeCampaignRequest{Code: "SEP40", Name: "September presale", DiscountPercent: 40, StartDate: now.AddDate(0, 0, -1).Format("2006-01-02"), EndDate: now.AddDate(0, 0, 6).Format("2006-01-02"), Enabled: true})
	require.NoError(t, err)
	return s, u, p, c
}
func cafeCampaignOrder(t *testing.T, s *PaymentService, u *dbent.User, p *dbent.SubscriptionPlan, c *dbent.CafeCampaign) (*dbent.PaymentOrder, error) {
	t.Helper()
	amount := p.Price
	return s.createOrderInTx(context.Background(), CreateOrderRequest{UserID: u.ID, OrderType: payment.OrderTypeSubscription, PlanID: p.ID, Multiplier: 1, PresaleMonth: NextPresalePeriod(time.Now()).Month, CafeCouponCode: c.Code, PaymentType: payment.TypeAlipay}, &User{ID: u.ID, Email: u.Email, Username: u.Username}, p, &PaymentConfig{}, amount, amount*.6, 0, amount*.6, nil)
}
func TestCafeCampaignCalendarAndWindow(t *testing.T) {
	starts, end, err := cafeCampaignDates("2026-09-25", "2026-09-30")
	require.NoError(t, err)
	require.Equal(t, "2026-09-24T16:00:00Z", starts.UTC().Format(time.RFC3339))
	require.Equal(t, "2026-09-30T16:00:00Z", end.UTC().Format(time.RFC3339))
	c := &dbent.CafeCampaign{Enabled: true, StartsAt: starts, ExpiresAt: end}
	require.Equal(t, "CAFE_CAMPAIGN_NOT_STARTED", infraerrors.Reason(validateCafeCampaignWindow(c, starts.Add(-time.Nanosecond))))
	require.NoError(t, validateCafeCampaignWindow(c, starts))
	require.NoError(t, validateCafeCampaignWindow(c, end.Add(-time.Nanosecond)))
	require.Equal(t, "CAFE_COUPON_EXPIRED", infraerrors.Reason(validateCafeCampaignWindow(c, end)))
	for _, dates := range [][2]string{{"2026-09-31", "2026-10-01"}, {"2026-10-01", "2026-09-30"}, {"", "2026-09-30"}} {
		_, _, err := cafeCampaignDates(dates[0], dates[1])
		require.Error(t, err)
	}
}
func TestCafeCampaignAdminValidationAndAudit(t *testing.T) {
	ctx := context.Background()
	s, _, _, c := newCafeCampaignFixture(t)
	require.Equal(t, "CAFE-PUBLIC-SEP40", c.Code)
	req := CreateCafeCampaignRequest{Code: "SEP40", Name: "test", DiscountPercent: 40, StartDate: "2026-09-25", EndDate: "2026-09-30"}
	_, err := s.CreateCafeCampaign(ctx, 99, req)
	require.Equal(t, "CAFE_CAMPAIGN_EXISTS", infraerrors.Reason(err))
	_, err = s.CreateCafeCampaign(ctx, 0, req)
	require.Equal(t, "FORBIDDEN", infraerrors.Reason(err))
	for _, code := range []string{"", "-BAD", "CODE$", "<script>"} {
		req.Code = code
		_, err = s.CreateCafeCampaign(ctx, 99, req)
		require.Equal(t, "CAFE_CAMPAIGN_INVALID", infraerrors.Reason(err))
	}
	req.Code = "OTHER"
	for _, pct := range []int{0, -40, 100, 101} {
		req.DiscountPercent = pct
		_, err = s.CreateCafeCampaign(ctx, 99, req)
		require.Equal(t, "CAFE_CAMPAIGN_INVALID", infraerrors.Reason(err))
	}
	original := c
	for _, enabled := range []bool{false, true, false, true} {
		c, err = s.SetCafeCampaignEnabled(ctx, c.ID, 99, enabled, c.UpdatedAt)
		require.NoError(t, err)
		require.Equal(t, enabled, c.Enabled)
	}
	_, err = s.SetCafeCampaignEnabled(ctx, c.ID, 99, false, original.UpdatedAt)
	require.Equal(t, "CAFE_CAMPAIGN_STALE", infraerrors.Reason(err))
	require.Equal(t, 5, s.entClient.PaymentAuditLog.Query().CountX(ctx))
	require.Equal(t, 0, s.entClient.CafeCoupon.Query().CountX(ctx), "public codes never create or consume personal membership coupons")
}
func TestCafeCampaignPreviewAndRealCheckout(t *testing.T) {
	ctx := context.Background()
	s, u, p, c := newCafeCampaignFixture(t)
	p = s.entClient.SubscriptionPlan.UpdateOneID(p.ID).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).SaveX(ctx)
	req := CreateOrderRequest{UserID: u.ID, OrderType: payment.OrderTypeSubscription, PlanID: p.ID, Multiplier: 3, PresaleMonth: NextPresalePeriod(time.Now()).Month, CafeCouponCode: "  cafe-public-sep40 ", Amount: .01, PaymentType: payment.TypeAlipay, ClientIP: "127.0.0.1", SrcHost: "localhost"}
	preview, err := s.PreviewCafeCouponForOrder(ctx, req)
	require.NoError(t, err)
	require.Equal(t, 300.0, preview.OriginalAmount)
	require.Equal(t, 120.0, preview.DiscountAmount)
	require.Equal(t, 180.0, preview.PayableAmount)
	require.True(t, preview.PresaleOnly)
	require.True(t, preview.ExpiresAt.Equal(c.ExpiresAt))
	require.Zero(t, s.entClient.CafeCampaignUse.Query().CountX(ctx), "preview is read only")
	_, _, _, err = s.prepareCafeCouponForOrder(ctx, CreateOrderRequest{UserID: u.ID, OrderType: payment.OrderTypeBalance, Amount: 100, CafeCouponCode: c.Code}, nil, &PaymentConfig{}, 100, "CNY", 0)
	require.Equal(t, "CAFE_CAMPAIGN_PRESALE_ONLY", infraerrors.Reason(err))
	_, err = s.previewCafeCouponForPurchase(ctx, CreateOrderRequest{UserID: u.ID, OrderType: payment.OrderTypeSubscription, CafeCouponCode: c.Code}, 100)
	require.Equal(t, "CAFE_CAMPAIGN_PRESALE_ONLY", infraerrors.Reason(err))
	_, limit, pay, err := s.prepareCafeCouponForOrder(ctx, req, p, &PaymentConfig{}, 300, "CNY", 1)
	require.NoError(t, err)
	require.Equal(t, 180.0, limit)
	require.Equal(t, 181.8, pay)
	enablePresaleSafetyDevRefund(t)
	res, err := s.CreateOrder(ctx, req)
	require.NoError(t, err)
	require.Equal(t, 180.0, res.PayAmount)
	o := s.entClient.PaymentOrder.GetX(ctx, res.OrderID)
	require.Equal(t, OrderStatusCompleted, o.Status)
	require.Equal(t, 120.0, o.CafeCouponDiscount)
	require.Equal(t, c.Code, *o.CafeCouponCode)
	require.NotNil(t, s.entClient.CafeCampaignUse.Query().OnlyX(ctx).UsedAt)
	require.Equal(t, 180.0, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
	// Offline refunds and admin cancellation never restore a consumed public code.
	_, err = s.ProcessPresaleOffline(ctx, o.ID, 99, offlineRequest(o, "refund"))
	require.NoError(t, err)
	_, err = s.PreviewCafeCouponForOrder(ctx, req)
	require.Equal(t, "CAFE_COUPON_USED", infraerrors.Reason(err))
	require.NoError(t, s.toPaid(ctx, o, "duplicate-paid", 180, payment.TypeAlipay))
	require.Equal(t, 1, s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("CAFE_CAMPAIGN_USED")).CountX(ctx))
}
func TestCafeCampaignReleasedAttemptNeverReclaimsSlot(t *testing.T) {
	for _, status := range []string{OrderStatusCancelled, OrderStatusExpired, OrderStatusFailed} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			s, u, p, c := newCafeCampaignFixture(t)
			first, err := cafeCampaignOrder(t, s, u, p, c)
			require.NoError(t, err)
			_, err = s.cafeCampaignInfo(ctx, u.ID, c.Code)
			require.Equal(t, "CAFE_CAMPAIGN_RESERVED", infraerrors.Reason(err))
			s.entClient.PaymentOrder.UpdateOneID(first.ID).SetStatus(status).ExecX(ctx)
			second, err := cafeCampaignOrder(t, s, u, p, c)
			require.NoError(t, err)
			require.Equal(t, second.ID, s.entClient.CafeCampaignUse.Query().OnlyX(ctx).OrderID)
			require.Equal(t, "CAFE_COUPON_USED", infraerrors.Reason(s.toPaid(ctx, first, "late-first", 60, payment.TypeAlipay)))
			old := s.entClient.PaymentOrder.GetX(ctx, first.ID)
			require.NotNil(t, old.PaidAt)
			require.Equal(t, OrderStatusFailed, old.Status)
			require.NoError(t, s.toPaid(ctx, second, "second", 60, payment.TypeAlipay))
			require.Equal(t, OrderStatusCompleted, s.entClient.PaymentOrder.GetX(ctx, second.ID).Status)
			require.Equal(t, 60.0, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
		})
	}
}
func TestCafeCampaignCrossGroupAndAccountIsolation(t *testing.T) {
	ctx := context.Background()
	s, u, p, c := newCafeCampaignFixture(t)
	first, err := cafeCampaignOrder(t, s, u, p, c)
	require.NoError(t, err)
	g := s.entClient.Group.Create().SetName("other").SetPlatform(PlatformOpenAI).SetStatus(StatusActive).SetSubscriptionType(SubscriptionTypeSubscription).SaveX(ctx)
	other := s.entClient.SubscriptionPlan.Create().SetGroupID(g.ID).SetName("other").SetPrice(100).SetPresaleEnabled(true).SetForSale(true).SaveX(ctx)
	_, err = cafeCampaignOrder(t, s, u, other, c)
	require.Equal(t, "CAFE_CAMPAIGN_RESERVED", infraerrors.Reason(err))
	v := s.entClient.User.Create().SetEmail("another@example.test").SetPasswordHash("test").SaveX(ctx)
	_, err = cafeCampaignOrder(t, s, v, p, c)
	require.NoError(t, err)
	require.Equal(t, 2, s.entClient.CafeCampaignUse.Query().CountX(ctx))
	// A payment confirmed before fulfillment already closes the usage slot.
	s.entClient.PaymentOrder.UpdateOneID(first.ID).SetPaidAt(time.Now()).SetStatus(OrderStatusFailed).ExecX(ctx)
	_, err = cafeCampaignOrder(t, s, u, other, c)
	require.Equal(t, "CAFE_COUPON_USED", infraerrors.Reason(err))
}
func TestCafeCampaignDisableAndExpiryHonorBoundPayment(t *testing.T) {
	ctx := context.Background()
	s, u, p, c := newCafeCampaignFixture(t)
	// Test-only mutation, not an admin feature: a campaign ending seconds from now.
	_, err := s.entClient.ExecContext(ctx, "UPDATE cafe_campaigns SET expires_at = ? WHERE id = ?", time.Now().Add(time.Minute), c.ID)
	require.NoError(t, err)
	c = s.entClient.CafeCampaign.GetX(ctx, c.ID)
	o, err := cafeCampaignOrder(t, s, u, p, c)
	require.NoError(t, err)
	require.True(t, o.ExpiresAt.Equal(c.ExpiresAt))
	_, err = s.SetCafeCampaignEnabled(ctx, c.ID, 99, false, c.UpdatedAt)
	require.NoError(t, err)
	_, err = s.cafeCampaignInfo(ctx, u.ID, c.Code)
	require.Equal(t, "CAFE_CAMPAIGN_DISABLED", infraerrors.Reason(err))
	require.NoError(t, s.toPaid(ctx, o, "already-issued-payment", 60, payment.TypeAlipay))
	require.Equal(t, OrderStatusCompleted, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
}
func TestCafeCampaignTransactionFailureRollsBackAndTamperingRejected(t *testing.T) {
	ctx := context.Background()
	s, u, p, c := newCafeCampaignFixture(t)
	fail := true
	s.entClient.PaymentAuditLog.Use(func(next dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(ctx context.Context, m dbent.Mutation) (dbent.Value, error) {
			mutation, ok := m.(*dbent.PaymentAuditLogMutation)
			if ok {
				a, _ := mutation.Action()
				if fail && a == "CAFE_CAMPAIGN_RESERVED" {
					return nil, errors.New("audit unavailable")
				}
			}
			return next.Mutate(ctx, m)
		})
	})
	_, err := cafeCampaignOrder(t, s, u, p, c)
	require.ErrorContains(t, err, "audit unavailable")
	require.Zero(t, s.entClient.PaymentOrder.Query().CountX(ctx))
	require.Zero(t, s.entClient.CafeCampaignUse.Query().CountX(ctx))
	fail = false
	o, err := cafeCampaignOrder(t, s, u, p, c)
	require.NoError(t, err)
	s.entClient.PaymentOrder.UpdateOneID(o.ID).SetCafeCouponDiscount(99).ExecX(ctx)
	require.Equal(t, "CAFE_COUPON_CHANGED", infraerrors.Reason(s.toPaid(ctx, o, "tampered", 60, payment.TypeAlipay)))
	require.Nil(t, s.entClient.CafeCampaignUse.Query().OnlyX(ctx).UsedAt)
	require.Zero(t, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
	// List records are administrator-only and show the actual paid failed order.
	records, total, err := s.ListCafeCampaignUses(ctx, c.ID, 1, 20)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, o.ID, records[0].OrderID)
	require.NotNil(t, records[0].PaidAt)
	_, _, err = s.ListCafeCampaignUses(ctx, c.ID+999, 1, 20)
	require.Error(t, err)
	require.Equal(t, 1, s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(fmt.Sprint(o.ID)), paymentauditlog.ActionEQ("CAFE_CAMPAIGN_RESERVED")).CountX(ctx))
}

func TestCafeCampaignExpiryRejectsNewButNotAcceptedOrders(t *testing.T) {
	ctx := context.Background()
	s, u, p, c := newCafeCampaignFixture(t)
	o, err := cafeCampaignOrder(t, s, u, p, c)
	require.NoError(t, err)
	// Exercise delayed delivery after campaign expiry without sleeping. These
	// fixture-only SQL updates are not exposed by any administrative API.
	cutoff := time.Now().Add(-time.Minute)
	_, err = s.entClient.ExecContext(ctx, "UPDATE cafe_campaigns SET expires_at = ? WHERE id = ?", cutoff, c.ID)
	require.NoError(t, err)
	_, err = s.entClient.ExecContext(ctx, "UPDATE payment_orders SET created_at = ?, expires_at = ? WHERE id = ?", cutoff.Add(-time.Minute), cutoff, o.ID)
	require.NoError(t, err)
	_, err = s.cafeCampaignInfo(ctx, u.ID, c.Code)
	require.Equal(t, "CAFE_COUPON_EXPIRED", infraerrors.Reason(err))
	require.NoError(t, s.toPaid(ctx, o, "delayed-notification", 60, payment.TypeAlipay))
	require.Equal(t, OrderStatusCompleted, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
}

func TestCafeCampaignConsumeAuditIsAtomic(t *testing.T) {
	ctx := context.Background()
	s, u, p, c := newCafeCampaignFixture(t)
	o, err := cafeCampaignOrder(t, s, u, p, c)
	require.NoError(t, err)
	fail := true
	s.entClient.PaymentAuditLog.Use(func(next dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(ctx context.Context, m dbent.Mutation) (dbent.Value, error) {
			mutation, ok := m.(*dbent.PaymentAuditLogMutation)
			if ok {
				action, _ := mutation.Action()
				if fail && action == "CAFE_CAMPAIGN_USED" {
					return nil, errors.New("consume audit unavailable")
				}
			}
			return next.Mutate(ctx, m)
		})
	})
	require.ErrorContains(t, s.toPaid(ctx, o, "paid", 60, payment.TypeAlipay), "consume audit unavailable")
	require.Nil(t, s.entClient.CafeCampaignUse.Query().OnlyX(ctx).UsedAt)
	require.Zero(t, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
	_, err = s.cafeCampaignInfo(ctx, u.ID, c.Code)
	require.Equal(t, "CAFE_COUPON_USED", infraerrors.Reason(err), "paid facts already prevent another discounted order")
	fail = false
	require.NoError(t, s.RetryFulfillment(ctx, o.ID))
	require.NotNil(t, s.entClient.CafeCampaignUse.Query().OnlyX(ctx).UsedAt)
	require.Equal(t, 60.0, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
}

func TestCafeCampaignRevalidatesAtOrderCreation(t *testing.T) {
	ctx := context.Background()
	s, u, p, c := newCafeCampaignFixture(t)
	req := CreateOrderRequest{UserID: u.ID, OrderType: payment.OrderTypeSubscription, PlanID: p.ID, Multiplier: 1, PresaleMonth: NextPresalePeriod(time.Now()).Month, CafeCouponCode: c.Code, PaymentType: payment.TypeAlipay}
	_, err := s.PreviewCafeCouponForOrder(ctx, req)
	require.NoError(t, err)
	paused, err := s.SetCafeCampaignEnabled(ctx, c.ID, 99, false, c.UpdatedAt)
	require.NoError(t, err)
	_, err = cafeCampaignOrder(t, s, u, p, c)
	require.Equal(t, "CAFE_CAMPAIGN_DISABLED", infraerrors.Reason(err))
	require.Zero(t, s.entClient.PaymentOrder.Query().CountX(ctx))
	_, err = s.SetCafeCampaignEnabled(ctx, c.ID, 99, true, paused.UpdatedAt)
	require.NoError(t, err)
	buyer := &User{ID: u.ID, Email: u.Email, Username: u.Username}
	_, err = s.createOrderInTx(ctx, req, buyer, p, &PaymentConfig{}, 100, 1, 0, 1, nil)
	require.Equal(t, "CAFE_COUPON_CHANGED", infraerrors.Reason(err), "a supplied discounted amount is never authoritative")
	req.OrderType = payment.OrderTypeBalance
	req.PresaleMonth = ""
	req.PlanID = 0
	_, err = s.createOrderInTx(ctx, req, buyer, nil, &PaymentConfig{}, 100, 60, 0, 60, nil)
	require.Equal(t, "CAFE_CAMPAIGN_PRESALE_ONLY", infraerrors.Reason(err))
	require.Zero(t, s.entClient.PaymentOrder.Query().CountX(ctx))
	require.Zero(t, s.entClient.CafeCampaignUse.Query().CountX(ctx))
}

func TestCafeCampaignStalePendingCannotBypassPaymentDeadline(t *testing.T) {
	ctx := context.Background()
	s, u, p, c := newCafeCampaignFixture(t)
	o, err := cafeCampaignOrder(t, s, u, p, c)
	require.NoError(t, err)
	s.entClient.PaymentOrder.UpdateOneID(o.ID).SetExpiresAt(time.Now().Add(-(paymentGraceMinutes + 1) * time.Minute)).ExecX(ctx)
	err = s.toPaid(ctx, o, "late-paid-without-expiry-worker", 60, payment.TypeAlipay)
	require.Equal(t, "CAFE_CAMPAIGN_PAYMENT_EXPIRED", infraerrors.Reason(err))
	failed := s.entClient.PaymentOrder.GetX(ctx, o.ID)
	require.NotNil(t, failed.PaidAt)
	require.Equal(t, OrderStatusFailed, failed.Status)
	require.Zero(t, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
	require.Nil(t, s.entClient.CafeCampaignUse.Query().OnlyX(ctx).UsedAt)
	// A replay must not turn a genuinely late first payment into a valid one.
	err = s.toPaid(ctx, o, "late-replay", 60, payment.TypeAlipay)
	require.Equal(t, "CAFE_CAMPAIGN_PAYMENT_EXPIRED", infraerrors.Reason(err))
	replayed := s.entClient.PaymentOrder.GetX(ctx, o.ID)
	require.True(t, replayed.PaidAt.Equal(*failed.PaidAt))
	require.Equal(t, failed.PaymentTradeNo, replayed.PaymentTradeNo)
	require.Zero(t, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
}

func TestCafeCampaignTimelyPaymentSurvivesDelayedCallbackRetry(t *testing.T) {
	for _, callback := range []bool{false, true} {
		t.Run(fmt.Sprintf("callback=%t", callback), func(t *testing.T) {
			ctx := context.Background()
			s, u, p, c := newCafeCampaignFixture(t)
			o, err := cafeCampaignOrder(t, s, u, p, c)
			require.NoError(t, err)
			fail := true
			s.entClient.PaymentAuditLog.Use(func(next dbent.Mutator) dbent.Mutator {
				return dbent.MutateFunc(func(ctx context.Context, m dbent.Mutation) (dbent.Value, error) {
					if mutation, ok := m.(*dbent.PaymentAuditLogMutation); ok {
						action, _ := mutation.Action()
						if fail && action == "CAFE_CAMPAIGN_USED" {
							return nil, errors.New("transient consume audit failure")
						}
					}
					return next.Mutate(ctx, m)
				})
			})
			require.ErrorContains(t, s.toPaid(ctx, o, "timely-payment", 60, payment.TypeAlipay), "transient consume audit failure")
			require.Nil(t, s.entClient.CafeCampaignUse.Query().OnlyX(ctx).UsedAt)
			// Simulate elapsed time without sleeping: the first verified receipt was
			// before expiry, but recovery is after expiry plus callback grace.
			now := time.Now().Truncate(time.Microsecond)
			paidAt, expiresAt := now.Add(-10*time.Minute), now.Add(-6*time.Minute)
			_, err = s.entClient.ExecContext(ctx, "UPDATE payment_orders SET created_at = ?, paid_at = ?, expires_at = ? WHERE id = ?", paidAt.Add(-time.Minute), paidAt, expiresAt, o.ID)
			require.NoError(t, err)
			fail = false
			if callback {
				err = s.HandlePaymentNotification(ctx, &payment.PaymentNotification{OrderID: o.OutTradeNo, TradeNo: "timely-payment", Amount: 60, Status: payment.NotificationStatusSuccess}, payment.TypeAlipay)
			} else {
				err = s.RetryFulfillment(ctx, o.ID)
			}
			require.NoError(t, err)
			current := s.entClient.PaymentOrder.GetX(ctx, o.ID)
			require.Equal(t, OrderStatusCompleted, current.Status)
			require.True(t, current.PaidAt.Equal(paidAt))
			require.Equal(t, "timely-payment", current.PaymentTradeNo)
			require.Equal(t, 60.0, current.PayAmount)
			require.True(t, s.entClient.CafeCampaignUse.Query().OnlyX(ctx).UsedAt.Equal(paidAt))
			require.NoError(t, s.toPaid(ctx, o, "timely-payment", 60, payment.TypeAlipay))
			require.Equal(t, 60.0, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
			require.Equal(t, 1, s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("CAFE_CAMPAIGN_USED")).CountX(ctx))
		})
	}
}

func TestCafeCampaignNameUsesCharacterLimit(t *testing.T) {
	ctx := context.Background()
	s, _, _, _ := newCafeCampaignFixture(t)
	for _, char := range []string{"券", "☕", "🌙", "A"} {
		for _, length := range []int{34, 100, 101} {
			t.Run(fmt.Sprintf("%s-%d", char, length), func(t *testing.T) {
				name := strings.Repeat(char, length)
				c, err := s.CreateCafeCampaign(ctx, 99, CreateCafeCampaignRequest{Code: fmt.Sprintf("NAME-%X-%d", []rune(char)[0], length), Name: name, DiscountPercent: 40, StartDate: "2026-09-25", EndDate: "2026-09-30"})
				if length > 100 {
					require.Equal(t, "CAFE_CAMPAIGN_INVALID", infraerrors.Reason(err))
					return
				}
				require.NoError(t, err)
				require.Equal(t, name, s.entClient.CafeCampaign.GetX(ctx, c.ID).Name)
			})
		}
	}
}
