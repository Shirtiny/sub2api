//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func newPresaleMultiplierFixture(t *testing.T, multiplier int) (*PaymentService, *dbent.SubscriptionPlan, *dbent.UserSubscription, CreateOrderRequest) {
	t.Helper()
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	period := NextPresalePeriod(now)
	p = s.entClient.SubscriptionPlan.UpdateOneID(p.ID).SetCustomMultiplierEnabled(true).
		SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).SaveX(ctx)
	current := s.entClient.UserSubscription.Create().SetUserID(u.ID).SetGroupID(p.GroupID).
		SetStartsAt(now.Add(-15 * 24 * time.Hour)).SetExpiresAt(period.StartsAt).
		SetDailyUsageUsd(5).SetWeeklyUsageUsd(120).SetMonthlyUsageUsd(150).
		SetDailyWindowStart(now.Add(-time.Hour)).SetWeeklyWindowStart(now.Add(-3 * 24 * time.Hour)).
		SetMonthlyWindowStart(now.Add(-20 * 24 * time.Hour)).SaveX(ctx)
	if multiplier > 1 {
		current = s.entClient.UserSubscription.UpdateOneID(current.ID).SetCustomMultiplier(multiplier).
			SetCustomSourcePlanID(p.ID).SetCustomSourceGroupID(p.GroupID).SetCustomExpiresAt(period.StartsAt).
			SetCustomDisplayName("Current term").SaveX(ctx)
	}
	s.userRepo = &mockUserRepo{getByIDUser: &User{ID: u.ID, Email: u.Email, Username: u.Username, Status: StatusActive}}
	s.configService.settingRepo = cafeCouponSettingsRepo(map[string]string{
		SettingPaymentEnabled: "true", SettingEnabledPaymentTypes: "alipay",
		SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"discount","value":25,"period":"month"}}}`,
	})
	// Isolated unit-test database and explicitly guarded simulated payments only.
	t.Setenv(paymentDevAutoSuccessEnv, paymentDevAutoSuccessToken)
	t.Setenv(paymentDevEnvironmentEnv, "local")
	for _, key := range []string{"APP_ENV", "ENVIRONMENT", "NODE_ENV", "GO_ENV"} {
		t.Setenv(key, "test")
	}
	req := CreateOrderRequest{UserID: u.ID, PlanID: p.ID, PresaleMonth: period.Month,
		OrderType: payment.OrderTypeSubscription, PaymentType: payment.TypeAlipay, ClientIP: "127.0.0.1", SrcHost: "localhost"}
	return s, p, current, req
}

func TestPresaleRenewalMultiplierChangesOnlyAtActivation(t *testing.T) {
	for _, pair := range [][2]int{{1, 3}, {2, 4}, {4, 2}, {3, 1}} {
		t.Run(fmt.Sprintf("%dx_to_%dx", pair[0], pair[1]), func(t *testing.T) {
			s, p, current, req := newPresaleMultiplierFixture(t, pair[0])
			ctx := context.Background()
			req.Multiplier, req.Amount = pair[1], 1 // Never trust the client price.
			res, err := s.CreateOrder(ctx, req)
			require.NoError(t, err)
			require.Equal(t, p.Price*float64(pair[1]), res.Amount)
			require.Equal(t, res.Amount, res.PayAmount)
			o := s.entClient.PaymentOrder.GetX(ctx, res.OrderID)
			require.Equal(t, pair[1], *o.SubscriptionMultiplier)
			require.True(t, o.PresaleRenewal)
			require.Equal(t, OrderStatusCompleted, o.Status)
			require.Nil(t, o.PresaleActivatedAt)
			before := s.entClient.UserSubscription.GetX(ctx, current.ID)
			require.Equal(t, current.CustomMultiplier, before.CustomMultiplier)
			require.True(t, current.ExpiresAt.Equal(before.ExpiresAt))
			require.Equal(t, current.WeeklyUsageUsd, before.WeeklyUsageUsd)
			// A different multiplier still cannot create a second same-month slot.
			req.Multiplier = 5
			_, err = s.CreateOrder(ctx, req)
			require.Equal(t, "PRESALE_ALREADY_RESERVED", infraerrors.Reason(err))
			// Later plan edits must not rewrite the purchased multiplier/price.
			s.entClient.SubscriptionPlan.UpdateOneID(p.ID).SetCustomMultiplierMax(1).SetPrice(999).ExecX(ctx)
			activated, err := s.activatePresale(ctx, o.ID, *o.PresaleStartsAt)
			require.NoError(t, err)
			require.True(t, activated)
			after := s.entClient.UserSubscription.GetX(ctx, current.ID)
			require.True(t, after.ExpiresAt.Equal(*o.PresaleExpiresAt))
			require.Equal(t, current.DailyUsageUsd, after.DailyUsageUsd)
			require.Equal(t, current.WeeklyUsageUsd, after.WeeklyUsageUsd)
			require.Equal(t, current.MonthlyUsageUsd, after.MonthlyUsageUsd)
			require.Equal(t, current.DailyWindowStart, after.DailyWindowStart)
			require.Equal(t, current.WeeklyWindowStart, after.WeeklyWindowStart)
			require.Equal(t, current.MonthlyWindowStart, after.MonthlyWindowStart)
			if pair[1] == 1 {
				require.Nil(t, after.CustomMultiplier)
				require.Nil(t, after.CustomSourcePlanID)
				require.Nil(t, after.CustomSourceGroupID)
				require.Nil(t, after.CustomExpiresAt)
				require.Nil(t, after.CustomDisplayName)
			} else {
				require.Equal(t, pair[1], *after.CustomMultiplier)
			}
			usage, err := s.subscriptionSvc.userSubRepo.GetByID(ctx, current.ID)
			require.NoError(t, err)
			limit := 100.0
			effective := EffectiveSubscriptionGroup(usage, &Group{ID: p.GroupID, WeeklyLimitUSD: &limit})
			require.Equal(t, limit*float64(pair[1]), *effective.WeeklyLimitUSD)
			if pair[1] == 1 {
				require.False(t, usage.CheckWeeklyLimit(effective, 0), "downgrading must not erase already consumed usage")
			}
			activated, err = s.activatePresale(ctx, o.ID, o.PresaleStartsAt.Add(time.Minute))
			require.NoError(t, err)
			require.False(t, activated)
		})
	}
}

func TestPresaleRenewalMultiplierValidation(t *testing.T) {
	s, p, current, req := newPresaleMultiplierFixture(t, 4)
	ctx := context.Background()
	p = s.entClient.SubscriptionPlan.UpdateOneID(p.ID).SetCustomMultiplierMin(2).SetCustomMultiplierMax(3).SaveX(ctx)
	for _, selected := range []int{-1, 1, 4, 6, 101} {
		req.Multiplier = selected
		_, err := s.CreateOrder(ctx, req)
		require.Equal(t, "INVALID_SUBSCRIPTION_MULTIPLIER", infraerrors.Reason(err))
	}
	require.Zero(t, s.entClient.PaymentOrder.Query().CountX(ctx))
	// The previous legacy immediate-renewal rule is unchanged.
	legacy, err := s.resolveSubscriptionOrderMultiplier(ctx, req.UserID, p, 2)
	require.NoError(t, err)
	require.Equal(t, 4, legacy)
	// Changing multiplier does not bypass the first fourteen days.
	s.entClient.UserSubscription.UpdateOneID(current.ID).SetStartsAt(time.Now().Add(-24 * time.Hour)).ExecX(ctx)
	req.Multiplier = 2
	_, err = s.CreateOrder(ctx, req)
	require.Equal(t, "PRESALE_RENEWAL_TOO_EARLY", infraerrors.Reason(err))
	s.entClient.UserSubscription.UpdateOneID(current.ID).SetStartsAt(current.StartsAt).ExecX(ctx)
	s.entClient.SubscriptionPlan.UpdateOneID(p.ID).SetCustomMultiplierEnabled(false).ExecX(ctx)
	req.Multiplier = 4
	_, err = s.CreateOrder(ctx, req)
	require.Equal(t, "SUBSCRIPTION_STATE_CHANGED", infraerrors.Reason(err))
	req.Multiplier = 1
	res, err := s.CreateOrder(ctx, req)
	require.NoError(t, err)
	require.Equal(t, p.Price, res.Amount)
}

func TestPresaleRenewalMultiplierCouponMatchesCheckout(t *testing.T) {
	s, p, current, req := newPresaleMultiplierFixture(t, 2)
	ctx := context.Background()
	userRepo, ok := s.userRepo.(*mockUserRepo)
	require.True(t, ok)
	userRepo.getByIDUser.TotalRecharged = MembershipLevel1Threshold + 1
	s.entClient.User.UpdateOneID(req.UserID).SetTotalRecharged(MembershipLevel1Threshold + 1).ExecX(ctx)
	start, end := cafeCouponRollingPeriodWindow(time.Now(), CafeCouponPeriodMonth)
	coupon := s.entClient.CafeCoupon.Create().SetCode("CAFE-PRESALE-RENEWAL").SetUserID(req.UserID).SetMembershipLevel(1).
		SetCouponType(CafeCouponTypeDiscount).SetValue(25).SetPeriod(CafeCouponPeriodMonth).
		SetPeriodStart(start).SetPeriodEnd(end).SetStatus(CafeCouponStatusIssued).SaveX(ctx)
	req.Multiplier, req.CafeCouponCode = 4, coupon.Code
	preview, err := s.PreviewCafeCouponForOrder(ctx, req)
	require.NoError(t, err)
	require.Equal(t, p.Price*4*.25, preview.DiscountAmount)
	require.Equal(t, p.Price*4*.75, preview.PayableAmount)
	res, err := s.CreateOrder(ctx, req)
	require.NoError(t, err)
	require.Equal(t, p.Price*4, res.Amount)
	require.Equal(t, preview.PayableAmount, res.PayAmount)
	require.Equal(t, current.CustomMultiplier, s.entClient.UserSubscription.GetX(ctx, current.ID).CustomMultiplier)
}
