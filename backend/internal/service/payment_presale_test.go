//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionconcurrencyentitlement"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestPresaleCalendar(t *testing.T) {
	for _, tc := range []struct {
		now, month string
		days       int
	}{
		{"2026-09-30T15:59:59Z", "2026-10", 31},
		{"2026-09-30T16:00:00Z", "2026-11", 30},
		{"2027-12-31T12:00:00Z", "2028-01", 31},
		{"2028-01-15T00:00:00Z", "2028-02", 29},
		{"2027-01-15T00:00:00Z", "2027-02", 28},
	} {
		now, err := time.Parse(time.RFC3339, tc.now)
		require.NoError(t, err)
		p := NextPresalePeriod(now)
		require.Equal(t, tc.month, p.Month)
		require.Equal(t, tc.days, int(p.ExpiresAt.Sub(p.StartsAt).Hours()/24))
		require.Equal(t, 72*time.Hour, p.StartsAt.Sub(p.FullRefundBefore))
		require.Equal(t, 0, p.StartsAt.Hour())
	}
}

func TestPresalePlanValidation(t *testing.T) {
	now := time.Now()
	month := NextPresalePeriod(now).Month
	for _, tc := range []struct {
		name, month            string
		enabled, visible, sale bool
		reason                 string
	}{
		{"base", month, true, true, true, ""},
		{"legacy immediate", "", false, true, true, ""},
		{"cannot bypass", "", true, true, true, "PRESALE_REQUIRED"},
		{"disabled", month, false, true, true, "PRESALE_NOT_AVAILABLE"},
		{"legacy hidden flag ignored", month, true, false, true, ""},
		{"not for sale", month, true, true, false, "PRESALE_NOT_AVAILABLE"},
		{"stale", "2000-01", true, true, true, "PRESALE_MONTH_CHANGED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePresaleOrderPlan(&dbent.SubscriptionPlan{ForSale: tc.sale, PresaleEnabled: tc.enabled, PresaleVisible: tc.visible}, CreateOrderRequest{PresaleMonth: tc.month}, now)
			if tc.reason == "" {
				require.NoError(t, err)
			} else {
				require.Equal(t, tc.reason, infraerrors.Reason(err))
			}
		})
	}
}

func newPresaleFixture(t *testing.T) (*PaymentService, *dbent.User, *dbent.SubscriptionPlan) {
	t.Helper()
	ctx := context.Background()
	c := newPaymentConfigServiceTestClient(t)
	u := c.User.Create().SetEmail("presale@example.com").SetPasswordHash("hash").SetUsername("presale").SaveX(ctx)
	g := c.Group.Create().SetName("presale group").SetPlatform(PlatformOpenAI).SetStatus(StatusActive).SetSubscriptionType(SubscriptionTypeSubscription).SaveX(ctx)
	p := c.SubscriptionPlan.Create().SetGroupID(g.ID).SetName("Monthly").SetPrice(100).SetForSale(true).SetPresaleEnabled(true).SetPresaleVisible(true).SetPresaleResetCards(2).SetConcurrency(3).SaveX(ctx)
	gr := &subscriptionGroupRepoStub{group: &Group{ID: g.ID, Name: g.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription, Hydrated: true}}
	ss := NewSubscriptionService(gr, &paymentFulfillmentSubscriptionRepo{client: c}, nil, c, nil)
	t.Cleanup(ss.Stop)
	return &PaymentService{entClient: c, configService: &PaymentConfigService{entClient: c}, groupRepo: gr, subscriptionSvc: ss, providersLoaded: true}, u, p
}

func newPresaleOrder(t *testing.T, s *PaymentService, u *dbent.User, p *dbent.SubscriptionPlan, period PresalePeriod) *dbent.PaymentOrder {
	t.Helper()
	// Legacy paid snapshots retain any reset cards already promised at purchase.
	ctx := context.Background()
	return s.entClient.PaymentOrder.Create().SetUserID(u.ID).SetUserEmail(u.Email).SetUserName(u.Username).
		SetAmount(p.Price).SetPayAmount(p.Price).SetRechargeCode("presale").SetOutTradeNo(generateOutTradeNo()).SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeSubscription).SetPlanID(p.ID).SetSubscriptionGroupID(p.GroupID).SetSubscriptionSourceGroupID(p.GroupID).SetSubscriptionDays(int(period.ExpiresAt.Sub(period.StartsAt).Hours() / 24)).SetSubscriptionConcurrency(p.Concurrency).SetSubscriptionMultiplier(1).
		SetStatus(OrderStatusPaid).SetPaidAt(time.Now()).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("localhost").
		SetPresaleStartsAt(period.StartsAt).SetPresaleExpiresAt(period.ExpiresAt).SetPresalePlanName(p.Name).SetPresaleResetCards(p.PresaleResetCards).SaveX(ctx)
}

func TestPresaleNewOrderIgnoresLegacyVisibilityAndResetBonus(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	p = s.entClient.SubscriptionPlan.UpdateOneID(p.ID).SetPresaleVisible(false).SetPresaleResetCards(9).SaveX(ctx)
	period := NextPresalePeriod(time.Now())
	order, err := s.createOrderInTx(ctx, CreateOrderRequest{
		UserID: u.ID, PlanID: p.ID, Multiplier: 1, OrderType: payment.OrderTypeSubscription,
		PaymentType: payment.TypeAlipay, PresaleMonth: period.Month, ClientIP: "127.0.0.1", SrcHost: "localhost",
	}, &User{ID: u.ID, Email: u.Email, Username: u.Username}, p, &PaymentConfig{}, p.Price, p.Price, 0, p.Price, nil)
	require.NoError(t, err)
	require.Zero(t, order.PresaleResetCards)
	require.True(t, order.PresaleStartsAt.Equal(period.StartsAt))
	s.entClient.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusPaid).SetPaidAt(time.Now()).ExecX(ctx)
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, order.ID))
	activated, err := s.activatePresale(ctx, order.ID, period.StartsAt)
	require.NoError(t, err)
	require.True(t, activated)
	require.Zero(t, s.entClient.UserSubscription.Query().OnlyX(ctx).ResetCount)
}

func TestPresaleAdminConfigOnlyNeedsEnabledAndForSale(t *testing.T) {
	s, _, p := newPresaleFixture(t)
	ctx := context.Background()
	var req CreatePlanRequest
	require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{
		"group_id": %d, "name": "Simple presale", "price": 100, "validity_days": 30,
		"validity_unit": "days", "for_sale": true, "presale_enabled": true,
		"presale_badge": "  Next month  ", "presale_visible": false, "presale_reset_cards": 99
	}`, p.GroupID)), &req))
	plan, err := s.configService.CreatePlan(ctx, req)
	require.NoError(t, err)
	require.True(t, plan.PresaleEnabled)
	require.Zero(t, plan.PresaleResetCards)
	require.Equal(t, "Next month", plan.PresaleBadge)
	plans, err := s.configService.ListPresalePlans(ctx)
	require.NoError(t, err)
	require.Len(t, plans, 2)

	// Old fields are ignored, not a hidden second switch or a configurable grant.
	var patch UpdatePlanRequest
	require.NoError(t, json.Unmarshal([]byte(`{"presale_badge":" Updated ","presale_visible":false,"presale_reset_cards":99}`), &patch))
	plan, err = s.configService.UpdatePlan(ctx, plan.ID, patch)
	require.NoError(t, err)
	require.Zero(t, plan.PresaleResetCards)
	require.Equal(t, "Updated", plan.PresaleBadge)
	require.NoError(t, validatePresaleOrderPlan(plan, CreateOrderRequest{PresaleMonth: NextPresalePeriod(time.Now()).Month}, time.Now()))
	require.NoError(t, validatePresalePlanFields(strings.Repeat("月", 40)))
	require.Equal(t, "PRESALE_CONFIG_INVALID", infraerrors.Reason(validatePresalePlanFields(strings.Repeat("月", 41))))
}

func TestPresalePaymentDefersActivationAndIsIdempotent(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	period := NextPresalePeriod(time.Now())
	o := newPresaleOrder(t, s, u, p, period)
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	require.Zero(t, s.entClient.UserSubscription.Query().CountX(ctx))
	require.Equal(t, OrderStatusCompleted, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
	require.Equal(t, 100.0, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
	yes, err := s.activatePresale(ctx, o.ID, period.StartsAt.Add(-time.Nanosecond))
	require.NoError(t, err)
	require.False(t, yes)
	yes, err = s.activatePresale(ctx, o.ID, period.StartsAt.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, yes)
	sub := s.entClient.UserSubscription.Query().OnlyX(ctx)
	require.True(t, sub.StartsAt.Equal(period.StartsAt))
	require.True(t, sub.ExpiresAt.Equal(period.ExpiresAt))
	require.Equal(t, 2, sub.ResetCount)
	ent := s.entClient.SubscriptionConcurrencyEntitlement.Query().OnlyX(ctx)
	require.True(t, ent.StartsAt.Equal(period.StartsAt))
	require.True(t, ent.ExpiresAt.Equal(period.ExpiresAt))
	require.Equal(t, 3, ent.Concurrency)
	yes, err = s.activatePresale(ctx, o.ID, period.StartsAt.Add(time.Hour))
	require.NoError(t, err)
	require.False(t, yes)
	require.Equal(t, 2, s.entClient.UserSubscription.GetX(ctx, sub.ID).ResetCount)
	require.Equal(t, 1, s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("PRESALE_ACTIVATED")).CountX(ctx))
}

func TestPresaleRenewalDoesNotChangeCurrentTerm(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	period := NextPresalePeriod(time.Now())
	current := s.entClient.UserSubscription.Create().SetUserID(u.ID).SetGroupID(p.GroupID).SetStartsAt(time.Now().AddDate(0, 0, -10)).SetExpiresAt(period.StartsAt).SetDailyUsageUsd(10).SetResetCount(1).SaveX(ctx)
	q, err := s.GetPresaleQuote(ctx, u.ID, p.ID)
	require.NoError(t, err)
	require.True(t, q.Renewal)
	require.Equal(t, current.ID, *q.CurrentSubscriptionID)
	o := newPresaleOrder(t, s, u, p, period)
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	unchanged := s.entClient.UserSubscription.GetX(ctx, current.ID)
	require.True(t, unchanged.ExpiresAt.Equal(current.ExpiresAt))
	require.Equal(t, 10.0, unchanged.DailyUsageUsd)
	yes, err := s.activatePresale(ctx, o.ID, period.StartsAt)
	require.NoError(t, err)
	require.True(t, yes)
	renewed := s.entClient.UserSubscription.GetX(ctx, current.ID)
	require.True(t, renewed.ExpiresAt.Equal(period.ExpiresAt))
	require.Zero(t, renewed.DailyUsageUsd)
	require.Equal(t, 3, renewed.ResetCount)
}

func TestPresaleSlotAndOverlap(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	period := NextPresalePeriod(time.Now())
	_, err := s.GetPresaleQuote(ctx, u.ID, p.ID)
	require.NoError(t, err)
	o := newPresaleOrder(t, s, u, p, period)
	_, err = s.GetPresaleQuote(ctx, u.ID, p.ID)
	require.Equal(t, "PRESALE_ALREADY_RESERVED", infraerrors.Reason(err))
	require.Equal(t, OrderStatusPaid, infraerrors.FromError(err).Metadata["order_status"])
	s.entClient.PaymentOrder.UpdateOneID(o.ID).SetStatus(OrderStatusPartiallyRefunded).ExecX(ctx)
	_, err = s.GetPresaleQuote(ctx, u.ID, p.ID)
	require.NoError(t, err)
	s.entClient.UserSubscription.Create().SetUserID(u.ID).SetGroupID(p.GroupID).SetStartsAt(time.Now()).SetExpiresAt(period.StartsAt.Add(time.Hour)).SaveX(ctx)
	_, err = s.GetPresaleQuote(ctx, u.ID, p.ID)
	require.Equal(t, "PRESALE_COVERAGE_OVERLAP", infraerrors.Reason(err))
}

func TestPresaleEligibilityReservationStatus(t *testing.T) {
	for _, tc := range []struct {
		status                 string
		paid, expired, blocked bool
	}{
		{OrderStatusPending, false, false, true},
		{OrderStatusPending, false, true, false},
		{OrderStatusPaid, true, true, true},
		{OrderStatusRecharging, true, true, true},
		{OrderStatusCompleted, true, true, true},
		{OrderStatusFailed, true, true, true},
		{OrderStatusFailed, false, false, false},
		{OrderStatusRefundRequested, true, true, true},
		{OrderStatusRefunding, true, true, true},
		{OrderStatusRefundFailed, true, true, true},
		{OrderStatusRefunded, true, true, false},
		{OrderStatusPartiallyRefunded, true, true, false},
		{OrderStatusCancelled, false, false, false},
		{OrderStatusExpired, false, true, false},
	} {
		t.Run(fmt.Sprintf("%s/paid=%t/expired=%t", tc.status, tc.paid, tc.expired), func(t *testing.T) {
			s, u, p := newPresaleFixture(t)
			ctx := context.Background()
			o := newPresaleOrder(t, s, u, p, NextPresalePeriod(time.Now()))
			update := s.entClient.PaymentOrder.UpdateOneID(o.ID).SetStatus(tc.status)
			if !tc.paid {
				update.ClearPaidAt()
			}
			if tc.expired {
				update.SetExpiresAt(time.Now().Add(-time.Hour))
			}
			update.ExecX(ctx)
			_, err := s.GetPresaleQuote(ctx, u.ID, p.ID)
			if tc.blocked {
				require.Equal(t, "PRESALE_ALREADY_RESERVED", infraerrors.Reason(err))
				require.Equal(t, map[string]string{"order_status": tc.status, "order_id": fmt.Sprint(o.ID), "order_plan_id": fmt.Sprint(p.ID)}, infraerrors.FromError(err).Metadata)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPresaleEligibilityChecksOwnedGroupAndMonth(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	period := NextPresalePeriod(time.Now())
	otherUser := s.entClient.User.Create().SetEmail("other@example.com").SetPasswordHash("hash").SaveX(ctx)
	newPresaleOrder(t, s, otherUser, p, period)
	newPresaleOrder(t, s, u, p, NextPresalePeriod(time.Now().AddDate(0, 1, 0)))
	_, err := s.GetPresaleQuote(ctx, u.ID, p.ID)
	require.NoError(t, err, "another user or month must not occupy this user's slot")
	newPresaleOrder(t, s, u, p, period)
	sibling := s.entClient.SubscriptionPlan.Create().SetGroupID(p.GroupID).SetName("Sibling plan").SetPrice(200).SetForSale(true).SetPresaleEnabled(true).SetPresaleVisible(true).SaveX(ctx)
	_, err = s.GetPresaleQuote(ctx, u.ID, sibling.ID)
	require.Equal(t, "PRESALE_ALREADY_RESERVED", infraerrors.Reason(err))
	require.Equal(t, OrderStatusPaid, infraerrors.FromError(err).Metadata["order_status"])
}

func TestPresaleRefundPolicyBoundaries(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, presaleLocation)
	end := start.AddDate(0, 1, 0)
	for _, tc := range []struct {
		name           string
		now            time.Time
		activated      bool
		amount         float64
		policy, reason string
	}{
		{"before cutoff", start.Add(-72*time.Hour - time.Nanosecond), false, 100, "full", ""},
		{"exact cutoff", start.Add(-72 * time.Hour), false, 80, "preparation", ""},
		{"last second", start.Add(-time.Second), false, 80, "preparation", ""},
		{"at start", start, true, 80, "unused_days", ""},
		{"one used day", start.Add(24 * time.Hour), true, 77.42, "unused_days", ""},
		{"partial used day", start.Add(time.Hour), true, 77.42, "unused_days", ""},
		{"just before last week", end.Add(-7*24*time.Hour - time.Second), true, 18.06, "unused_days", ""},
		{"last week", end.Add(-7 * 24 * time.Hour), true, 0, "", "PRESALE_REFUND_LAST_WEEK"},
		{"failed activation", start, false, 100, "unfulfilled", ""},
		{"missed whole period", end.Add(time.Hour), false, 100, "unfulfilled", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := &dbent.PaymentOrder{Status: OrderStatusCompleted, Amount: 100, PayAmount: 90, PresaleStartsAt: &start, PresaleExpiresAt: &end}
			if tc.activated {
				o.PresaleActivatedAt = &start
			}
			q, err := PresaleRefundQuoteForOrder(o, tc.now)
			if tc.reason != "" {
				require.Equal(t, tc.reason, infraerrors.Reason(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.amount, q.RefundAmount)
			require.Equal(t, tc.policy, q.Policy)
			if tc.policy == "preparation" || tc.policy == "unused_days" {
				require.Equal(t, 20, q.FeePercent)
			} else {
				require.Zero(t, q.FeePercent)
			}
			if tc.amount == 80 {
				require.Equal(t, 72.0, q.GatewayAmount)
			}
			if tc.amount == 77.42 {
				require.Equal(t, 69.68, q.GatewayAmount)
			}
		})
	}
	requested := start.Add(-96 * time.Hour)
	q, err := PresaleRefundQuoteForOrder(&dbent.PaymentOrder{Status: OrderStatusRefundRequested, Amount: 100, PayAmount: 100, PresaleStartsAt: &start, PresaleExpiresAt: &end, RefundRequestedAt: &requested}, end)
	require.NoError(t, err)
	require.Equal(t, 100.0, q.GatewayAmount)
}

func TestPresalePendingRefundNeverDeductsCurrentSubscription(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	period := NextPresalePeriod(time.Now())
	current := s.entClient.UserSubscription.Create().SetUserID(u.ID).SetGroupID(p.GroupID).SetStartsAt(time.Now().Add(-time.Hour)).SetExpiresAt(period.StartsAt).SaveX(ctx)
	o := newPresaleOrder(t, s, u, p, period)
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	require.NoError(t, s.RequestRefund(ctx, o.ID, u.ID, "changed my mind", presaleTestRefundAmount(t, s, o.ID)))
	currentAfter := s.entClient.UserSubscription.GetX(ctx, current.ID)
	require.True(t, currentAfter.ExpiresAt.Equal(current.ExpiresAt))
	yes, err := s.activatePresale(ctx, o.ID, period.StartsAt)
	require.NoError(t, err)
	require.False(t, yes)
	require.Equal(t, OrderStatusRefundRequested, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
	require.NoError(t, s.RequestRefund(ctx, o.ID, u.ID, "retry", presaleTestRefundAmount(t, s, o.ID)))
	require.Error(t, s.RequestRefund(ctx, o.ID, u.ID+1, "wrong owner", 100))
}

func TestPresaleActiveRefundCancelsOnlyPurchasedTerm(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	period := NextPresalePeriod(time.Now().AddDate(0, -1, 0))
	// Keep the actual clock before the final week regardless of test run date.
	period.StartsAt = time.Now().Add(-24 * time.Hour).Truncate(time.Second)
	period.ExpiresAt = period.StartsAt.Add(31 * 24 * time.Hour)
	o := newPresaleOrder(t, s, u, p, period)
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	yes, err := s.activatePresale(ctx, o.ID, time.Now())
	require.NoError(t, err)
	require.True(t, yes)
	require.NoError(t, s.RequestRefund(ctx, o.ID, u.ID, "cancel", presaleTestRefundAmount(t, s, o.ID)))
	sub := s.entClient.UserSubscription.Query().Where(usersubscription.UserIDEQ(u.ID)).OnlyX(ctx)
	require.Equal(t, SubscriptionStatusExpired, sub.Status)
	require.LessOrEqual(t, sub.ExpiresAt.Unix(), time.Now().Unix())
	require.Zero(t, s.entClient.SubscriptionConcurrencyEntitlement.Query().Where(subscriptionconcurrencyentitlement.ExpiresAtGT(time.Now())).CountX(ctx))
	require.NoError(t, s.RequestRefund(ctx, o.ID, u.ID, "retry", presaleTestRefundAmount(t, s, o.ID)))
}

func TestPresaleCatalogFiltering(t *testing.T) {
	s, _, p := newPresaleFixture(t)
	ctx := context.Background()
	for _, tc := range []struct {
		sale, enabled, visible bool
		count                  int
	}{{true, true, true, 1}, {false, true, true, 0}, {true, false, true, 0}, {true, true, false, 1}, {false, true, false, 0}, {true, false, false, 0}} {
		s.entClient.SubscriptionPlan.UpdateOneID(p.ID).SetForSale(tc.sale).SetPresaleEnabled(tc.enabled).SetPresaleVisible(tc.visible).ExecX(ctx)
		plans, err := s.configService.ListPresalePlans(ctx)
		require.NoError(t, err)
		require.Len(t, plans, tc.count)
	}
	s.entClient.SubscriptionPlan.UpdateOneID(p.ID).SetForSale(true).SetPresaleEnabled(true).ExecX(ctx)
	s.entClient.Group.UpdateOneID(p.GroupID).SetStatus(StatusDisabled).ExecX(ctx)
	plans, err := s.configService.ListPresalePlans(ctx)
	require.NoError(t, err)
	require.Empty(t, plans)
}

func presaleTestRefundAmount(t *testing.T, s *PaymentService, id int64) float64 {
	t.Helper()
	q, err := s.GetPresaleRefundQuote(context.Background(), s.entClient.PaymentOrder.GetX(context.Background(), id), time.Now())
	require.NoError(t, err)
	return q.GatewayAmount
}

func TestPresaleRefundQuoteMustBeConfirmed(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	o := newPresaleOrder(t, s, u, p, NextPresalePeriod(time.Now()))
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	err := s.RequestRefund(ctx, o.ID, u.ID, "cancel")
	require.Equal(t, "PRESALE_REFUND_AMOUNT_CHANGED", infraerrors.Reason(err))
	err = s.RequestRefund(ctx, o.ID, u.ID, "cancel", 12.34)
	require.Equal(t, "PRESALE_REFUND_AMOUNT_CHANGED", infraerrors.Reason(err))
	require.Equal(t, OrderStatusCompleted, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
}

func TestPresaleRefundableProviderRequired(t *testing.T) {
	s, _, _ := newPresaleFixture(t)
	ctx := context.Background()
	provider := s.entClient.PaymentProviderInstance.Create().SetName("test only").SetProviderKey("alipay").SetConfig("{}").SetEnabled(true).SaveX(ctx)
	req := CreateOrderRequest{PresaleMonth: NextPresalePeriod(time.Now()).Month}
	selection := &payment.InstanceSelection{InstanceID: fmt.Sprint(provider.ID)}
	err := s.validateSelectedCreateOrderInstance(ctx, req, selection)
	require.Equal(t, "PRESALE_REFUND_CHANNEL_REQUIRED", infraerrors.Reason(err))
	s.entClient.PaymentProviderInstance.UpdateOneID(provider.ID).SetRefundEnabled(true).ExecX(ctx)
	require.NoError(t, s.validateSelectedCreateOrderInstance(ctx, req, selection))
	require.NoError(t, s.validateSelectedCreateOrderInstance(ctx, CreateOrderRequest{}, selection))
}

func TestPresaleRefundRetryFailureNeverReactivatesReservation(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	period := NextPresalePeriod(time.Now())
	o := newPresaleOrder(t, s, u, p, period)
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	require.NoError(t, s.RequestRefund(ctx, o.ID, u.ID, "cancel", presaleTestRefundAmount(t, s, o.ID)))
	failed := s.entClient.PaymentOrder.UpdateOneID(o.ID).SetStatus(OrderStatusRefundFailed).SaveX(ctx)
	s.restoreStatus(ctx, &RefundPlan{OrderID: o.ID, Order: failed})
	require.Equal(t, OrderStatusRefundRequested, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
	yes, err := s.activatePresale(ctx, o.ID, period.StartsAt)
	require.NoError(t, err)
	require.False(t, yes)
}

func TestPresaleRefundPreservesEarlierResetCardsAtCap(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	now := time.Now()
	period := NextPresalePeriod(now)
	period.StartsAt = now.Add(-24 * time.Hour).Truncate(time.Second)
	period.ExpiresAt = period.StartsAt.Add(31 * 24 * time.Hour)
	old := s.entClient.UserSubscription.Create().SetUserID(u.ID).SetGroupID(p.GroupID).SetStartsAt(period.StartsAt.Add(-30 * 24 * time.Hour)).SetExpiresAt(period.StartsAt).SetResetCount(999).SaveX(ctx)
	o := newPresaleOrder(t, s, u, p, period)
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	yes, err := s.activatePresale(ctx, o.ID, now)
	require.NoError(t, err)
	require.True(t, yes)
	require.Equal(t, 1000, s.entClient.UserSubscription.GetX(ctx, old.ID).ResetCount)
	require.NoError(t, s.RequestRefund(ctx, o.ID, u.ID, "cancel", presaleTestRefundAmount(t, s, o.ID)))
	require.Equal(t, 999, s.entClient.UserSubscription.GetX(ctx, old.ID).ResetCount)
}
