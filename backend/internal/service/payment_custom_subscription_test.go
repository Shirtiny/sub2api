//go:build unit

package service

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/accountgroup"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestResolveSubscriptionOrderMultiplierValidatesRangeAndUsesActiveCustomRenewal(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().
		SetEmail("custom-multiplier@example.com").
		SetPasswordHash("hash").
		SetUsername("custom-multiplier").
		Save(ctx)
	require.NoError(t, err)

	sourceGroup, err := client.Group.Create().
		SetName("source-plan-group").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	plan, err := client.SubscriptionPlan.Create().
		SetName("Custom Pro").
		SetDescription("custom").
		SetGroupID(sourceGroup.ID).
		SetPrice(100).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetForSale(true).
		SetCustomMultiplierEnabled(true).
		SetCustomMultiplierMin(2).
		SetCustomMultiplierMax(5).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}

	got, err := svc.resolveSubscriptionOrderMultiplier(ctx, user.ID, plan, 0)
	require.NoError(t, err)
	require.Equal(t, 2, got)

	got, err = svc.resolveSubscriptionOrderMultiplier(ctx, user.ID, plan, 4)
	require.NoError(t, err)
	require.Equal(t, 4, got)

	_, err = svc.resolveSubscriptionOrderMultiplier(ctx, user.ID, plan, 6)
	require.Error(t, err)
	require.Equal(t, "INVALID_SUBSCRIPTION_MULTIPLIER", infraerrors.Reason(err))

	customGroup, err := client.Group.Create().
		SetName("Custom Pro-user").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetIsCustomSubscriptionGroup(true).
		SetCustomOwnerUserID(user.ID).
		SetCustomSourcePlanID(plan.ID).
		SetCustomSourceGroupID(sourceGroup.ID).
		SetCustomMultiplier(3).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(customGroup.ID).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(time.Now().Add(-time.Hour)).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetNotes("active custom").
		Save(ctx)
	require.NoError(t, err)

	got, err = svc.resolveSubscriptionOrderMultiplier(ctx, user.ID, plan, 5)
	require.NoError(t, err)
	require.Equal(t, 3, got)
}

func TestResolveSubscriptionOrderMultiplierIgnoresSoftDeletedCustomRenewal(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().
		SetEmail("custom-soft-deleted-renewal@example.com").
		SetPasswordHash("hash").
		SetUsername("custom-soft-deleted-renewal").
		Save(ctx)
	require.NoError(t, err)

	sourceGroup, err := client.Group.Create().
		SetName("soft-deleted-renewal-source").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	plan, err := client.SubscriptionPlan.Create().
		SetName("Soft Deleted Renewal").
		SetDescription("custom").
		SetGroupID(sourceGroup.ID).
		SetPrice(100).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetForSale(true).
		SetCustomMultiplierEnabled(true).
		SetCustomMultiplierMin(1).
		SetCustomMultiplierMax(5).
		Save(ctx)
	require.NoError(t, err)

	customGroup, err := client.Group.Create().
		SetName("[2x]Soft Deleted Renewal#" + strconv.FormatInt(user.ID, 10)).
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetIsCustomSubscriptionGroup(true).
		SetCustomOwnerUserID(user.ID).
		SetCustomSourcePlanID(plan.ID).
		SetCustomSourceGroupID(sourceGroup.ID).
		SetCustomMultiplier(2).
		Save(ctx)
	require.NoError(t, err)

	deletedSub, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(customGroup.ID).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(time.Now().Add(-time.Hour)).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetNotes("revoked custom renewal").
		Save(ctx)
	require.NoError(t, err)
	err = client.UserSubscription.DeleteOneID(deletedSub.ID).Exec(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	got, err := svc.resolveSubscriptionOrderMultiplier(ctx, user.ID, plan, 4)
	require.NoError(t, err)
	require.Equal(t, 4, got)
}

func TestResolveSubscriptionOrderMultiplierIgnoresIncompleteVirtualCustomEntitlement(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().
		SetEmail("custom-incomplete-virtual@example.com").
		SetPasswordHash("hash").
		SetUsername("custom-incomplete-virtual").
		Save(ctx)
	require.NoError(t, err)

	sourceGroup, err := client.Group.Create().
		SetName("incomplete-virtual-source").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	plan, err := client.SubscriptionPlan.Create().
		SetName("Incomplete Virtual Plan").
		SetDescription("custom").
		SetGroupID(sourceGroup.ID).
		SetPrice(100).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetForSale(true).
		SetCustomMultiplierEnabled(true).
		SetCustomMultiplierMin(1).
		SetCustomMultiplierMax(5).
		Save(ctx)
	require.NoError(t, err)

	staleSub, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(sourceGroup.ID).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(time.Now().Add(-time.Hour)).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetCustomMultiplier(2).
		SetCustomSourcePlanID(plan.ID).
		SetNotes("incomplete early virtual custom metadata").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	got, err := svc.resolveSubscriptionOrderMultiplier(ctx, user.ID, plan, 1)
	require.NoError(t, err)
	require.Equal(t, 1, got)

	customExpiresAt := time.Now().Add(24 * time.Hour)
	_, err = client.UserSubscription.UpdateOneID(staleSub.ID).
		SetCustomSourceGroupID(sourceGroup.ID).
		SetCustomExpiresAt(customExpiresAt).
		Save(ctx)
	require.NoError(t, err)

	got, err = svc.resolveSubscriptionOrderMultiplier(ctx, user.ID, plan, 1)
	require.NoError(t, err)
	require.Equal(t, 2, got)
}

func TestResolveSubscriptionOrderMultiplierUsesActiveCustomRenewalWhenPlanCustomizationDisabled(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-disabled-plan-renewal@example.com").SetPasswordHash("hash").SetUsername("disabled-plan-renewal").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("disabled-plan-renewal-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Disabled Plan Renewal").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(false).SetCustomMultiplierMin(2).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	customGroup, err := client.Group.Create().SetName("Disabled Plan Renewal-custom").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(user.ID).SetCustomSourcePlanID(plan.ID).SetCustomSourceGroupID(sourceGroup.ID).SetCustomMultiplier(4).Save(ctx)
	require.NoError(t, err)
	_, err = client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(customGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().Add(-time.Hour)).SetExpiresAt(time.Now().Add(24 * time.Hour)).SetNotes("active custom renewal").Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	got, err := svc.resolveSubscriptionOrderMultiplier(ctx, user.ID, plan, 0)
	require.NoError(t, err)
	require.Equal(t, 4, got)
}

func TestResolveSubscriptionOrderMultiplierRejectsDisabledActiveCustomRenewal(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-disabled-renewal@example.com").SetPasswordHash("hash").SetUsername("disabled-renewal").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("disabled-renewal-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Disabled Renewal").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(2).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	customGroup, err := client.Group.Create().SetName("Disabled Renewal-custom").SetStatus(StatusDisabled).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(user.ID).SetCustomSourcePlanID(plan.ID).SetCustomSourceGroupID(sourceGroup.ID).SetCustomMultiplier(3).Save(ctx)
	require.NoError(t, err)
	_, err = client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(customGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().Add(-time.Hour)).SetExpiresAt(time.Now().Add(24 * time.Hour)).SetNotes("active but disabled group").Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	_, err = svc.resolveSubscriptionOrderMultiplier(ctx, user.ID, plan, 4)
	require.Error(t, err)
	require.Equal(t, "CUSTOM_SUBSCRIPTION_GROUP_INACTIVE", infraerrors.Reason(err))
}

func TestSyncCustomGroupsForSourceGroupUpdateRefreshesActiveCustomLimits(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-source-sync@example.com").SetPasswordHash("hash").SetUsername("custom-source-sync").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().
		SetName("custom-source-sync-source").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(10).
		SetWeeklyLimitUsd(20).
		SetMonthlyLimitUsd(30).
		SetVideoRateIndependent(true).
		SetVideoRateMultiplier(1.25).
		SetVideoPrice480p(0.08).
		SetVideoPrice720p(0.14).
		SetVideoPrice1080p(0.25).
		Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Source Sync Plan").SetDescription("sync").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	customGroup, err := client.Group.Create().
		SetName("[2x]Source Sync Plan#" + strconv.FormatInt(user.ID, 10)).
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetIsCustomSubscriptionGroup(true).
		SetCustomOwnerUserID(user.ID).
		SetCustomSourcePlanID(plan.ID).
		SetCustomSourceGroupID(sourceGroup.ID).
		SetCustomMultiplier(2).
		SetDailyLimitUsd(20).
		SetWeeklyLimitUsd(40).
		SetMonthlyLimitUsd(60).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(customGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().Add(-time.Hour)).SetExpiresAt(time.Now().Add(24 * time.Hour)).SetNotes("active custom").Save(ctx)
	require.NoError(t, err)

	_, err = client.Group.UpdateOneID(sourceGroup.ID).SetDailyLimitUsd(15).SetWeeklyLimitUsd(25).SetMonthlyLimitUsd(35).Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	ids, err := svc.syncCustomGroupsForSourceGroupUpdate(ctx, &Group{ID: sourceGroup.ID, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription})
	require.NoError(t, err)
	require.Equal(t, []int64{customGroup.ID}, ids)

	updated, err := client.Group.Get(ctx, customGroup.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.DailyLimitUsd)
	require.InDelta(t, 30, *updated.DailyLimitUsd, 0.0001)
	require.NotNil(t, updated.WeeklyLimitUsd)
	require.InDelta(t, 50, *updated.WeeklyLimitUsd, 0.0001)
	require.NotNil(t, updated.MonthlyLimitUsd)
	require.InDelta(t, 70, *updated.MonthlyLimitUsd, 0.0001)
	require.True(t, updated.VideoRateIndependent)
	require.InDelta(t, 1.25, updated.VideoRateMultiplier, 0.0001)
	require.InDelta(t, 0.08, *updated.VideoPrice480p, 0.0001)
	require.InDelta(t, 0.14, *updated.VideoPrice720p, 0.0001)
	require.InDelta(t, 0.25, *updated.VideoPrice1080p, 0.0001)
}

func TestSyncCustomGroupsForSourceGroupUpdateDisablesCustomGroupsWhenSourceDisabled(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-source-disable@example.com").SetPasswordHash("hash").SetUsername("custom-source-disable").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("custom-source-disable-source").SetStatus(StatusDisabled).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Source Disable Plan").SetDescription("sync").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	customGroup, err := client.Group.Create().SetName("[2x]Source Disable Plan#" + strconv.FormatInt(user.ID, 10)).SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(user.ID).SetCustomSourcePlanID(plan.ID).SetCustomSourceGroupID(sourceGroup.ID).SetCustomMultiplier(2).Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	ids, err := svc.syncCustomGroupsForSourceGroupUpdate(ctx, &Group{ID: sourceGroup.ID, Status: StatusDisabled, SubscriptionType: SubscriptionTypeSubscription})
	require.NoError(t, err)
	require.Equal(t, []int64{customGroup.ID}, ids)

	updated, err := client.Group.Get(ctx, customGroup.ID)
	require.NoError(t, err)
	require.Equal(t, StatusDisabled, updated.Status)
}

func TestPreviewCafeCouponForOrderUsesResolvedSubscriptionMultiplier(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)

	user, err := client.User.Create().
		SetEmail("custom-coupon@example.com").
		SetPasswordHash("hash").
		SetUsername("custom-coupon").
		Save(ctx)
	require.NoError(t, err)

	sourceGroup, err := client.Group.Create().
		SetName("coupon-source").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	plan, err := client.SubscriptionPlan.Create().
		SetName("Coupon Custom").
		SetDescription("custom").
		SetGroupID(sourceGroup.ID).
		SetPrice(100).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetForSale(true).
		SetCustomMultiplierEnabled(true).
		SetCustomMultiplierMin(2).
		SetCustomMultiplierMax(5).
		Save(ctx)
	require.NoError(t, err)

	start, end := cafeCouponRollingPeriodWindow(time.Now(), CafeCouponPeriodMonth)
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-CUSTOM-MULT").SetUserID(user.ID).SetMembershipLevel(1).
		SetCouponType(CafeCouponTypeDiscount).SetValue(25).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(start).SetPeriodEnd(end).
		SetStatus(CafeCouponStatusIssued).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
		configService: &PaymentConfigService{entClient: client, settingRepo: cafeCouponSettingsRepo(map[string]string{
			SettingPaymentEnabled:      "true",
			SettingEnabledPaymentTypes: "alipay",
			SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"discount","value":25,"period":"month"}}}`,
		})},
		groupRepo: &subscriptionGroupRepoStub{group: &Group{ID: sourceGroup.ID, Name: sourceGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}},
	}

	preview, err := svc.PreviewCafeCouponForOrder(ctx, CreateOrderRequest{
		UserID:         user.ID,
		OrderType:      payment.OrderTypeSubscription,
		PlanID:         plan.ID,
		Multiplier:     3,
		CafeCouponCode: coupon.Code,
	})
	require.NoError(t, err)
	require.InDelta(t, 75, preview.DiscountAmount, 1e-9)
	require.InDelta(t, 225, preview.PayableAmount, 1e-9)
}

func TestPreviewCafeCouponForOrderRejectsRequestedMultiplierMismatch(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)

	user, err := client.User.Create().SetEmail("custom-coupon-mismatch@example.com").SetPasswordHash("hash").SetUsername("custom-coupon-mismatch").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("coupon-mismatch-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Coupon Mismatch Custom").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(429).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	activeGroup, err := client.Group.Create().SetName("[2x]Coupon Mismatch Custom#" + strconv.FormatInt(user.ID, 10)).SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(user.ID).SetCustomSourcePlanID(plan.ID).SetCustomSourceGroupID(sourceGroup.ID).SetCustomMultiplier(2).Save(ctx)
	require.NoError(t, err)
	_, err = client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(activeGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().Add(-time.Hour)).SetExpiresAt(time.Now().Add(24 * time.Hour)).SetNotes("active custom 2x").Save(ctx)
	require.NoError(t, err)
	start, end := cafeCouponRollingPeriodWindow(time.Now(), CafeCouponPeriodMonth)
	coupon, err := client.CafeCoupon.Create().SetCode("CAFE-MULT-MISMATCH").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeDiscount).SetValue(20).SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(start).SetPeriodEnd(end).SetStatus(CafeCouponStatusIssued).Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
		configService: &PaymentConfigService{entClient: client, settingRepo: cafeCouponSettingsRepo(map[string]string{
			SettingPaymentEnabled:      "true",
			SettingEnabledPaymentTypes: "alipay",
			SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"discount","value":20,"period":"month"}}}`,
		})},
		groupRepo: &subscriptionGroupRepoStub{group: &Group{ID: sourceGroup.ID, Name: sourceGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}},
	}

	_, err = svc.PreviewCafeCouponForOrder(ctx, CreateOrderRequest{UserID: user.ID, OrderType: payment.OrderTypeSubscription, PlanID: plan.ID, Multiplier: 4, CafeCouponCode: coupon.Code})
	require.Error(t, err)
	require.Equal(t, "SUBSCRIPTION_STATE_CHANGED", infraerrors.Reason(err))
}

func TestCreateOrderRejectsRequestedMultiplierMismatchWithActiveCustomRenewal(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-order-mismatch@example.com").SetPasswordHash("hash").SetUsername("custom-order-mismatch").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("order-mismatch-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Order Mismatch Custom").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(429).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	activeGroup, err := client.Group.Create().SetName("[2x]Order Mismatch Custom#" + strconv.FormatInt(user.ID, 10)).SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(user.ID).SetCustomSourcePlanID(plan.ID).SetCustomSourceGroupID(sourceGroup.ID).SetCustomMultiplier(2).Save(ctx)
	require.NoError(t, err)
	_, err = client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(activeGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().Add(-time.Hour)).SetExpiresAt(time.Now().Add(24 * time.Hour)).SetNotes("active custom 2x").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: sourceGroup.ID, Name: sourceGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	svc := &PaymentService{
		entClient: client,
		configService: &PaymentConfigService{entClient: client, settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
			SettingPaymentEnabled:      "true",
			SettingEnabledPaymentTypes: payment.TypeAlipay,
		}}},
		groupRepo: groupRepo,
		userRepo:  &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: StatusActive}},
	}

	t.Setenv(paymentDevAutoSuccessEnv, paymentDevAutoSuccessToken)
	t.Setenv(paymentDevEnvironmentEnv, "development")
	_, err = svc.CreateOrder(ctx, CreateOrderRequest{UserID: user.ID, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeSubscription, PlanID: plan.ID, Multiplier: 4, ClientIP: "127.0.0.1", SrcHost: "app.example.com"})
	require.Error(t, err)
	require.Equal(t, "SUBSCRIPTION_STATE_CHANGED", infraerrors.Reason(err))
	count, err := client.PaymentOrder.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestExecuteSubscriptionFulfillmentCreatesCustomGroupAndCopiesAccountBindings(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().
		SetEmail("custom-fulfillment@example.com").
		SetPasswordHash("hash").
		SetUsername("Anro").
		Save(ctx)
	require.NoError(t, err)

	daily, weekly, monthly := 150.0, 300.0, 600.0
	sourceGroup, err := client.Group.Create().
		SetName("Specially").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(daily).
		SetWeeklyLimitUsd(weekly).
		SetMonthlyLimitUsd(monthly).
		SetDefaultValidityDays(30).
		Save(ctx)
	require.NoError(t, err)

	account, err := client.Account.Create().
		SetName("source-account").
		SetPlatform(PlatformOpenAI).
		SetType("api_key").
		SetCredentials(map[string]any{"api_key": "sk-test"}).
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.AccountGroup.Create().SetAccountID(account.ID).SetGroupID(sourceGroup.ID).SetPriority(17).Save(ctx)
	require.NoError(t, err)

	plan, err := client.SubscriptionPlan.Create().
		SetName("Select Plan").
		SetDescription("custom").
		SetGroupID(sourceGroup.ID).
		SetPrice(100).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetForSale(true).
		SetCustomMultiplierEnabled(true).
		SetCustomMultiplierMin(2).
		SetCustomMultiplierMax(5).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(300).
		SetPayAmount(300).
		SetFeeRate(0).
		SetRechargeCode("CUSTOM-SUB-001").
		SetOutTradeNo("sub2_custom_sub_001").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).
		SetSubscriptionGroupID(sourceGroup.ID).
		SetSubscriptionDays(30).
		SetSubscriptionMultiplier(3).
		SetSubscriptionSourceGroupID(sourceGroup.ID).
		SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusPaid).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("app.example.com").
		Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: sourceGroup.ID, Name: sourceGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{entClient: client, groupRepo: groupRepo, subscriptionSvc: subSvc, providersLoaded: true}

	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))

	customGroups, err := client.Group.Query().Where(group.IsCustomSubscriptionGroupEQ(true)).All(ctx)
	require.NoError(t, err)
	require.Len(t, customGroups, 0)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, sourceGroup.ID, *updatedOrder.SubscriptionGroupID)
	require.Equal(t, OrderStatusCompleted, updatedOrder.Status)

	sub, err := client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(user.ID), usersubscription.GroupIDEQ(sourceGroup.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, *sub.CustomMultiplier)
	require.Equal(t, plan.ID, *sub.CustomSourcePlanID)
	require.Equal(t, sourceGroup.ID, *sub.CustomSourceGroupID)
	require.Equal(t, "Specially-3x", *sub.CustomDisplayName)

	copied, err := client.AccountGroup.Query().
		Where(accountgroup.AccountIDEQ(account.ID), accountgroup.GroupIDEQ(sourceGroup.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, 17, copied.Priority)
}

func TestExecuteSubscriptionFulfillmentCreatesOneXCustomGroup(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-1x@example.com").SetPasswordHash("hash").SetUsername("one-x").Save(ctx)
	require.NoError(t, err)

	sourceGroup, err := client.Group.Create().
		SetName("one-x-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(10).SetWeeklyLimitUsd(20).SetMonthlyLimitUsd(30).Save(ctx)
	require.NoError(t, err)

	plan, err := client.SubscriptionPlan.Create().
		SetName("One X Plan").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).
		SetValidityDays(30).SetValidityUnit("days").SetForSale(true).
		SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(100).SetPayAmount(100).SetFeeRate(0).SetRechargeCode("CUSTOM-1X-001").SetOutTradeNo("sub2_custom_1x_001").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).SetSubscriptionGroupID(sourceGroup.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(1).SetSubscriptionSourceGroupID(sourceGroup.ID).SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusPaid).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: sourceGroup.ID, Name: sourceGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{entClient: client, groupRepo: groupRepo, subscriptionSvc: subSvc, providersLoaded: true}

	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))
	customGroups, err := client.Group.Query().Where(group.IsCustomSubscriptionGroupEQ(true)).All(ctx)
	require.NoError(t, err)
	require.Len(t, customGroups, 0)

	sub, err := client.UserSubscription.Query().Where(usersubscription.UserIDEQ(user.ID), usersubscription.GroupIDEQ(sourceGroup.ID)).Only(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, *sub.CustomMultiplier)
	require.Equal(t, plan.ID, *sub.CustomSourcePlanID)
	require.Equal(t, sourceGroup.ID, *sub.CustomSourceGroupID)
	require.Equal(t, "one-x-source-1x", *sub.CustomDisplayName)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, sourceGroup.ID, *updatedOrder.SubscriptionGroupID)
	require.Equal(t, OrderStatusCompleted, updatedOrder.Status)
}

func TestExecuteSubscriptionFulfillmentMigratesSourceGroupAPIKeysToCustomGroup(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-key-migrate@example.com").SetPasswordHash("hash").SetUsername("key-migrate").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().
		SetName("key-migrate-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(10).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().
		SetName("Key Migrate Plan").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).
		SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	key, err := client.APIKey.Create().SetUserID(user.ID).SetKey("sk-custom-key-migrate").SetName("old key").SetGroupID(sourceGroup.ID).Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(200).SetPayAmount(200).SetFeeRate(0).SetRechargeCode("CUSTOM-KEY-MIGRATE-001").SetOutTradeNo("sub2_custom_key_migrate_001").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).SetSubscriptionGroupID(sourceGroup.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(2).SetSubscriptionSourceGroupID(sourceGroup.ID).SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusPaid).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: sourceGroup.ID, Name: sourceGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{entClient: client, groupRepo: groupRepo, subscriptionSvc: subSvc, providersLoaded: true}

	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))
	customGroups, err := client.Group.Query().Where(group.IsCustomSubscriptionGroupEQ(true)).All(ctx)
	require.NoError(t, err)
	require.Len(t, customGroups, 0)
	updatedKey, err := client.APIKey.Get(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, updatedKey.GroupID)
	require.Equal(t, sourceGroup.ID, *updatedKey.GroupID)
}

func TestExecuteSubscriptionFulfillmentVirtualCustomDoesNotUpgradeLongNormalRemainder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-normal-overlap@example.com").SetPasswordHash("hash").SetUsername("normal-overlap").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("normal-overlap-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetDailyLimitUsd(10).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Overlap Custom").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	normalExpiresAt := time.Now().AddDate(0, 0, 365).UTC().Truncate(time.Second)
	_, err = client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(sourceGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().AddDate(0, 0, -1)).SetExpiresAt(normalExpiresAt).SetNotes("active normal").Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(300).SetPayAmount(300).SetFeeRate(0).SetRechargeCode("CUSTOM-OVERLAP-001").SetOutTradeNo("sub2_custom_overlap_001").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).SetSubscriptionGroupID(sourceGroup.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(3).SetSubscriptionSourceGroupID(sourceGroup.ID).SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusPaid).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: sourceGroup.ID, Name: sourceGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{entClient: client, groupRepo: groupRepo, subscriptionSvc: subSvc, providersLoaded: true}

	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))
	customGroups, err := client.Group.Query().Where(group.IsCustomSubscriptionGroupEQ(true)).All(ctx)
	require.NoError(t, err)
	require.Len(t, customGroups, 0)
	sub, err := client.UserSubscription.Query().Where(usersubscription.UserIDEQ(user.ID), usersubscription.GroupIDEQ(sourceGroup.ID)).Only(ctx)
	require.NoError(t, err)
	require.WithinDuration(t, normalExpiresAt, sub.ExpiresAt, time.Second, "custom entitlement must not convert a longer normal remainder into custom time")
	require.NotNil(t, sub.CustomExpiresAt)
	require.True(t, sub.CustomExpiresAt.Before(sub.ExpiresAt))
	require.True(t, sub.CustomExpiresAt.After(time.Now().AddDate(0, 0, 29)))
	require.Equal(t, 3, *sub.CustomMultiplier)
}

func TestCreateOrderStoresResolvedCustomMultiplierSnapshots(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().
		SetEmail("custom-order@example.com").
		SetPasswordHash("hash").
		SetUsername("custom-order").
		Save(ctx)
	require.NoError(t, err)

	sourceGroup, err := client.Group.Create().
		SetName("custom-order-source").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(10).
		Save(ctx)
	require.NoError(t, err)

	originalPrice := 150.0
	plan, err := client.SubscriptionPlan.Create().
		SetName("Order Custom").
		SetDescription("custom").
		SetGroupID(sourceGroup.ID).
		SetPrice(100).
		SetOriginalPrice(originalPrice).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetForSale(true).
		SetCustomMultiplierEnabled(true).
		SetCustomMultiplierMin(2).
		SetCustomMultiplierMax(5).
		Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: sourceGroup.ID, Name: sourceGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{
		entClient: client,
		configService: &PaymentConfigService{entClient: client, settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
			SettingPaymentEnabled:      "true",
			SettingEnabledPaymentTypes: payment.TypeAlipay,
		}}},
		groupRepo:       groupRepo,
		subscriptionSvc: subSvc,
		userRepo:        &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: StatusActive}},
	}

	t.Setenv(paymentDevAutoSuccessEnv, paymentDevAutoSuccessToken)
	t.Setenv(paymentDevEnvironmentEnv, "development")
	resp, err := svc.CreateOrder(ctx, CreateOrderRequest{
		UserID:      user.ID,
		PaymentType: payment.TypeAlipay,
		OrderType:   payment.OrderTypeSubscription,
		PlanID:      plan.ID,
		Multiplier:  4,
		ClientIP:    "127.0.0.1",
		SrcHost:     "app.example.com",
	})
	require.NoError(t, err)
	require.Equal(t, 400.0, resp.Amount)
	require.Equal(t, 400.0, resp.PayAmount)

	order, err := client.PaymentOrder.Get(ctx, resp.OrderID)
	require.NoError(t, err)
	require.Equal(t, 400.0, order.Amount)
	require.Equal(t, plan.ID, *order.PlanID)
	require.Equal(t, sourceGroup.ID, *order.SubscriptionGroupID)
	require.Equal(t, 4, *order.SubscriptionMultiplier)
	sub, err := client.UserSubscription.Query().Where(usersubscription.UserIDEQ(user.ID), usersubscription.GroupIDEQ(sourceGroup.ID)).Only(ctx)
	require.NoError(t, err)
	require.Equal(t, 4, *sub.CustomMultiplier)
	require.Equal(t, plan.ID, *sub.CustomSourcePlanID)
	require.Equal(t, sourceGroup.ID, *order.SubscriptionSourceGroupID)
	require.InDelta(t, 100, *order.SubscriptionSourcePrice, 1e-9)
	require.InDelta(t, originalPrice, *order.SubscriptionSourceOriginalPrice, 1e-9)
}

func TestExecuteSubscriptionFulfillmentReusesActiveCustomGroupAndKeepsExpiredHistory(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-reuse@example.com").SetPasswordHash("hash").SetUsername("reuse-user").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().
		SetName("reuse-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(10).SetWeeklyLimitUsd(20).SetMonthlyLimitUsd(30).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().
		SetName("Reuse Plan").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).
		SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(2).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)

	expiredGroup, err := client.Group.Create().
		SetName("Reuse Plan-expired").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).
		SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(user.ID).SetCustomSourcePlanID(plan.ID).SetCustomSourceGroupID(sourceGroup.ID).SetCustomMultiplier(2).Save(ctx)
	require.NoError(t, err)
	_, err = client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(expiredGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().AddDate(0, 0, -60)).SetExpiresAt(time.Now().AddDate(0, 0, -1)).SetNotes("expired custom").Save(ctx)
	require.NoError(t, err)

	activeGroup, err := client.Group.Create().
		SetName("Reuse Plan-active").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).
		SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(user.ID).SetCustomSourcePlanID(plan.ID).SetCustomSourceGroupID(sourceGroup.ID).SetCustomMultiplier(3).Save(ctx)
	require.NoError(t, err)
	activeSub, err := client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(activeGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().AddDate(0, 0, -1)).SetExpiresAt(time.Now().AddDate(0, 0, 10)).SetNotes("active custom").Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(300).SetPayAmount(300).SetFeeRate(0).SetRechargeCode("CUSTOM-REUSE-001").SetOutTradeNo("sub2_custom_reuse_001").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).SetSubscriptionGroupID(sourceGroup.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(3).SetSubscriptionSourceGroupID(sourceGroup.ID).SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusPaid).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: activeGroup.ID, Name: activeGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{entClient: client, groupRepo: groupRepo, subscriptionSvc: subSvc, providersLoaded: true}

	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))
	customGroups, err := client.Group.Query().Where(group.IsCustomSubscriptionGroupEQ(true)).All(ctx)
	require.NoError(t, err)
	require.Len(t, customGroups, 2, "expired custom group must be kept; active one reused")
	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, activeGroup.ID, *updatedOrder.SubscriptionGroupID)
	updatedSub, err := client.UserSubscription.Get(ctx, activeSub.ID)
	require.NoError(t, err)
	require.True(t, updatedSub.ExpiresAt.After(activeSub.ExpiresAt))
}

func TestExecuteSubscriptionFulfillmentReusedCustomGroupSyncsCurrentSourceGroup(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-source-sync@example.com").SetPasswordHash("hash").SetUsername("source-sync").Save(ctx)
	require.NoError(t, err)
	oldSource, err := client.Group.Create().
		SetName("source-sync-old").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(10).SetWeeklyLimitUsd(20).SetMonthlyLimitUsd(30).Save(ctx)
	require.NoError(t, err)
	newSource, err := client.Group.Create().
		SetName("source-sync-new").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(100).SetWeeklyLimitUsd(200).SetMonthlyLimitUsd(300).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().
		SetName("Source Sync Plan").SetDescription("custom").SetGroupID(newSource.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).
		SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(2).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)

	activeGroup, err := client.Group.Create().
		SetName("Source Sync Plan-active").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).
		SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(user.ID).SetCustomSourcePlanID(plan.ID).SetCustomSourceGroupID(oldSource.ID).SetCustomMultiplier(2).
		SetDailyLimitUsd(20).SetWeeklyLimitUsd(40).SetMonthlyLimitUsd(60).Save(ctx)
	require.NoError(t, err)
	_, err = client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(activeGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().AddDate(0, 0, -1)).SetExpiresAt(time.Now().AddDate(0, 0, 10)).SetNotes("active custom").Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(200).SetPayAmount(200).SetFeeRate(0).SetRechargeCode("CUSTOM-SOURCE-SYNC-001").SetOutTradeNo("sub2_custom_source_sync_001").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).SetSubscriptionGroupID(newSource.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(2).SetSubscriptionSourceGroupID(newSource.ID).SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusPaid).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: activeGroup.ID, Name: activeGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{entClient: client, groupRepo: groupRepo, subscriptionSvc: subSvc, providersLoaded: true}

	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))
	updatedGroup, err := client.Group.Get(ctx, activeGroup.ID)
	require.NoError(t, err)
	require.Equal(t, newSource.ID, *updatedGroup.CustomSourceGroupID)
	require.Equal(t, 2, *updatedGroup.CustomMultiplier)
	require.InDelta(t, 200, *updatedGroup.DailyLimitUsd, 1e-9)
	require.InDelta(t, 400, *updatedGroup.WeeklyLimitUsd, 1e-9)
	require.InDelta(t, 600, *updatedGroup.MonthlyLimitUsd, 1e-9)
}

func TestExecuteSubscriptionFulfillmentReusesExpiredCustomGroupAndUpdatesMultiplier(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-expired-new@example.com").SetPasswordHash("hash").SetUsername("expired-new").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("expired-new-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetDailyLimitUsd(10).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Expired New").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(2).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	expiredGroup, err := client.Group.Create().SetName("Expired New-old").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(user.ID).SetCustomSourcePlanID(plan.ID).SetCustomSourceGroupID(sourceGroup.ID).SetCustomMultiplier(2).Save(ctx)
	require.NoError(t, err)
	_, err = client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(expiredGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().AddDate(0, 0, -60)).SetExpiresAt(time.Now().AddDate(0, 0, -1)).SetNotes("expired custom").Save(ctx)
	require.NoError(t, err)
	legacyKey, err := client.APIKey.Create().SetUserID(user.ID).SetKey("legacy-custom-key").SetName("legacy custom key").SetGroupID(expiredGroup.ID).SetStatus(StatusActive).Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(300).SetPayAmount(300).SetFeeRate(0).SetRechargeCode("CUSTOM-EXPIRED-NEW-001").SetOutTradeNo("sub2_custom_expired_new_001").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).SetSubscriptionGroupID(sourceGroup.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(3).SetSubscriptionSourceGroupID(sourceGroup.ID).SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusPaid).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: sourceGroup.ID, Name: sourceGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{entClient: client, groupRepo: groupRepo, subscriptionSvc: subSvc, providersLoaded: true}

	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))
	customGroups, err := client.Group.Query().Where(group.IsCustomSubscriptionGroupEQ(true)).All(ctx)
	require.NoError(t, err)
	require.Len(t, customGroups, 1)
	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, sourceGroup.ID, *updatedOrder.SubscriptionGroupID)
	updatedGroup, err := client.Group.Get(ctx, expiredGroup.ID)
	require.NoError(t, err)
	require.True(t, updatedGroup.IsCustomSubscriptionGroup)
	require.Equal(t, StatusDisabled, updatedGroup.Status)
	require.Equal(t, 2, *updatedGroup.CustomMultiplier)
	updatedKey, err := client.APIKey.Get(ctx, legacyKey.ID)
	require.NoError(t, err)
	require.Equal(t, sourceGroup.ID, *updatedKey.GroupID)
	sub, err := client.UserSubscription.Query().Where(usersubscription.UserIDEQ(user.ID), usersubscription.GroupIDEQ(sourceGroup.ID)).Only(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, *sub.CustomMultiplier)
	require.Equal(t, plan.ID, *sub.CustomSourcePlanID)
	require.Equal(t, sourceGroup.ID, *sub.CustomSourceGroupID)
	require.Equal(t, "expired-new-source-3x", *sub.CustomDisplayName)
}

func TestRetireLegacyCustomSubscriptionGroupsKeepsGroupWithAnyActiveSubscription(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	owner, err := client.User.Create().SetEmail("legacy-retire-owner@example.com").SetPasswordHash("hash").SetUsername("legacy-retire-owner").Save(ctx)
	require.NoError(t, err)
	other, err := client.User.Create().SetEmail("legacy-retire-other@example.com").SetPasswordHash("hash").SetUsername("legacy-retire-other").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("legacy-retire-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Legacy Retire").SetDescription("legacy retire").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	legacyGroup, err := client.Group.Create().SetName("[2x]Legacy Retire#owner").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(owner.ID).SetCustomSourcePlanID(plan.ID).SetCustomSourceGroupID(sourceGroup.ID).SetCustomMultiplier(2).Save(ctx)
	require.NoError(t, err)
	_, err = client.UserSubscription.Create().SetUserID(other.ID).SetGroupID(legacyGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().Add(-time.Hour)).SetExpiresAt(time.Now().Add(24 * time.Hour)).SetNotes("unexpected active assignee").Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	svc.retireLegacyCustomSubscriptionGroups(ctx, owner.ID, sourceGroup.ID, []int64{legacyGroup.ID})

	updated, err := client.Group.Get(ctx, legacyGroup.ID)
	require.NoError(t, err)
	require.Equal(t, StatusActive, updated.Status, "must not disable a legacy custom group that still has any active subscription")
}

func TestExecuteSubscriptionFulfillmentRejectsActiveCustomGroupMultiplierMismatch(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-active-mismatch@example.com").SetPasswordHash("hash").SetUsername("active-mismatch").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("active-mismatch-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetDailyLimitUsd(10).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Active Mismatch").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(2).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	activeGroup, err := client.Group.Create().SetName("[2x]Active Mismatch#" + strconv.FormatInt(user.ID, 10)).SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(user.ID).SetCustomSourcePlanID(plan.ID).SetCustomSourceGroupID(sourceGroup.ID).SetCustomMultiplier(2).SetDailyLimitUsd(20).Save(ctx)
	require.NoError(t, err)
	activeSub, err := client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(activeGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().AddDate(0, 0, -1)).SetExpiresAt(time.Now().AddDate(0, 0, 10)).SetNotes("active custom").Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(300).SetPayAmount(300).SetFeeRate(0).SetRechargeCode("CUSTOM-ACTIVE-MISMATCH-001").SetOutTradeNo("sub2_custom_active_mismatch_001").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).SetSubscriptionGroupID(sourceGroup.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(3).SetSubscriptionSourceGroupID(sourceGroup.ID).SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusPaid).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: activeGroup.ID, Name: activeGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{entClient: client, groupRepo: groupRepo, subscriptionSvc: subSvc, providersLoaded: true}

	err = svc.ExecuteSubscriptionFulfillment(ctx, order.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "multiplier mismatch")
	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusFailed, updatedOrder.Status)
	updatedGroup, err := client.Group.Get(ctx, activeGroup.ID)
	require.NoError(t, err)
	require.Equal(t, 2, *updatedGroup.CustomMultiplier)
	updatedSub, err := client.UserSubscription.Get(ctx, activeSub.ID)
	require.NoError(t, err)
	require.True(t, activeSub.ExpiresAt.Equal(updatedSub.ExpiresAt), "expires_at changed: before=%s after=%s", activeSub.ExpiresAt, updatedSub.ExpiresAt)
}

func TestCreateOrderRejectsSecondPendingCustomSubscriptionForSamePlan(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-pending@example.com").SetPasswordHash("hash").SetUsername("custom-pending").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("pending-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Pending Custom").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(2).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(200).SetPayAmount(200).SetFeeRate(0).SetRechargeCode("CUSTOM-PENDING-001").SetOutTradeNo("sub2_custom_pending_001").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).SetSubscriptionGroupID(sourceGroup.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(2).SetSubscriptionSourceGroupID(sourceGroup.ID).SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusPending).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: sourceGroup.ID, Name: sourceGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{
		entClient: client,
		configService: &PaymentConfigService{entClient: client, settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
			SettingPaymentEnabled:      "true",
			SettingEnabledPaymentTypes: payment.TypeAlipay,
		}}},
		groupRepo:       groupRepo,
		subscriptionSvc: subSvc,
		userRepo:        &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: StatusActive}},
	}

	t.Setenv(paymentDevAutoSuccessEnv, paymentDevAutoSuccessToken)
	t.Setenv(paymentDevEnvironmentEnv, "development")
	_, err = svc.CreateOrder(ctx, CreateOrderRequest{UserID: user.ID, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeSubscription, PlanID: plan.ID, Multiplier: 3, ClientIP: "127.0.0.1", SrcHost: "app.example.com"})
	require.Error(t, err)
	require.Equal(t, "CUSTOM_SUBSCRIPTION_ORDER_PENDING", infraerrors.Reason(err))
}

func TestCreateOrderRejectsInFlightCustomSubscriptionForSamePlan(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-inflight@example.com").SetPasswordHash("hash").SetUsername("custom-inflight").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("inflight-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Inflight Custom").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(2).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(200).SetPayAmount(200).SetFeeRate(0).SetRechargeCode("CUSTOM-INFLIGHT-001").SetOutTradeNo("sub2_custom_inflight_001").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).SetSubscriptionGroupID(sourceGroup.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(2).SetSubscriptionSourceGroupID(sourceGroup.ID).SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusRecharging).SetExpiresAt(time.Now().Add(-time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: sourceGroup.ID, Name: sourceGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{
		entClient: client,
		configService: &PaymentConfigService{entClient: client, settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
			SettingPaymentEnabled:      "true",
			SettingEnabledPaymentTypes: payment.TypeAlipay,
		}}},
		groupRepo:       groupRepo,
		subscriptionSvc: subSvc,
		userRepo:        &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: StatusActive}},
	}

	t.Setenv(paymentDevAutoSuccessEnv, paymentDevAutoSuccessToken)
	t.Setenv(paymentDevEnvironmentEnv, "development")
	_, err = svc.CreateOrder(ctx, CreateOrderRequest{UserID: user.ID, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeSubscription, PlanID: plan.ID, Multiplier: 3, ClientIP: "127.0.0.1", SrcHost: "app.example.com"})
	require.Error(t, err)
	require.Equal(t, "CUSTOM_SUBSCRIPTION_ORDER_PENDING", infraerrors.Reason(err))
}

func TestCreateOrderInTxRejectsStaleCustomSubscriptionMultiplier(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-stale-multiplier@example.com").SetPasswordHash("hash").SetUsername("custom-stale-multiplier").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("stale-multiplier-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Stale Multiplier").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(2).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	customGroup, err := client.Group.Create().SetName("[2x]Stale Multiplier#" + strconv.FormatInt(user.ID, 10)).SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(user.ID).SetCustomSourcePlanID(plan.ID).SetCustomSourceGroupID(sourceGroup.ID).SetCustomMultiplier(2).Save(ctx)
	require.NoError(t, err)
	_, err = client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(customGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().Add(-time.Hour)).SetExpiresAt(time.Now().Add(24 * time.Hour)).SetNotes("active custom").Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	_, err = svc.createOrderInTx(ctx,
		CreateOrderRequest{UserID: user.ID, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeSubscription, PlanID: plan.ID, Multiplier: 3, ClientIP: "127.0.0.1", SrcHost: "app.example.com"},
		&User{ID: user.ID, Email: user.Email, Username: user.Username, Status: StatusActive},
		plan,
		&PaymentConfig{MaxPendingOrders: 10, OrderTimeoutMin: 15},
		300,
		300,
		0,
		300,
		nil,
	)
	require.Error(t, err)
	require.Equal(t, "SUBSCRIPTION_STATE_CHANGED", infraerrors.Reason(err))
}

func TestUniqueCustomSubscriptionGroupNameTruncatesToSchemaLimit(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)
	svc := &PaymentService{entClient: client}

	longName := strings.Repeat("LongPlan", 30) + "-" + strings.Repeat("Custom", 30)
	name, err := svc.uniqueCustomSubscriptionGroupName(ctx, longName)
	require.NoError(t, err)
	require.LessOrEqual(t, len([]rune(name)), 100)

	_, err = client.Group.Create().SetName(name).SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	second, err := svc.uniqueCustomSubscriptionGroupName(ctx, longName)
	require.NoError(t, err)
	require.LessOrEqual(t, len([]rune(second)), 100)
	require.NotEqual(t, name, second)
	require.True(t, strings.HasSuffix(second, "-2"))
}

func TestCustomSubscriptionGroupNameUsesSourceGroupNameAndMultiplierSuffix(t *testing.T) {
	name := customSubscriptionGroupName(strings.Repeat("??", 80), 12)
	require.LessOrEqual(t, len([]rune(name)), 100)
	require.True(t, strings.HasSuffix(name, "-12x"))
	require.False(t, strings.HasPrefix(name, "[12x]"))
	require.NotContains(t, name, "#")
}

func TestExecuteSubscriptionFulfillmentReusedCustomGroupSyncsAccountBindings(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-account-sync@example.com").SetPasswordHash("hash").SetUsername("account-sync").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().
		SetName("account-sync-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(10).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().
		SetName("Account Sync Plan").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).
		SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(2).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)

	activeGroup, err := client.Group.Create().
		SetName("Account Sync Plan-active").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).
		SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(user.ID).SetCustomSourcePlanID(plan.ID).SetCustomSourceGroupID(sourceGroup.ID).SetCustomMultiplier(2).Save(ctx)
	require.NoError(t, err)
	_, err = client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(activeGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().AddDate(0, 0, -1)).SetExpiresAt(time.Now().AddDate(0, 0, 10)).SetNotes("active custom").Save(ctx)
	require.NoError(t, err)

	staleAccount, err := client.Account.Create().SetName("stale-account").SetPlatform(PlatformOpenAI).SetType("api_key").SetCredentials(map[string]any{"api_key": "stale"}).SetStatus(StatusActive).Save(ctx)
	require.NoError(t, err)
	keptAccount, err := client.Account.Create().SetName("kept-account").SetPlatform(PlatformOpenAI).SetType("api_key").SetCredentials(map[string]any{"api_key": "kept"}).SetStatus(StatusActive).Save(ctx)
	require.NoError(t, err)
	newAccount, err := client.Account.Create().SetName("new-account").SetPlatform(PlatformOpenAI).SetType("api_key").SetCredentials(map[string]any{"api_key": "new"}).SetStatus(StatusActive).Save(ctx)
	require.NoError(t, err)

	_, err = client.AccountGroup.Create().SetAccountID(staleAccount.ID).SetGroupID(activeGroup.ID).SetPriority(1).Save(ctx)
	require.NoError(t, err)
	_, err = client.AccountGroup.Create().SetAccountID(keptAccount.ID).SetGroupID(activeGroup.ID).SetPriority(2).Save(ctx)
	require.NoError(t, err)
	_, err = client.AccountGroup.Create().SetAccountID(keptAccount.ID).SetGroupID(sourceGroup.ID).SetPriority(7).Save(ctx)
	require.NoError(t, err)
	_, err = client.AccountGroup.Create().SetAccountID(newAccount.ID).SetGroupID(sourceGroup.ID).SetPriority(8).Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(200).SetPayAmount(200).SetFeeRate(0).SetRechargeCode("CUSTOM-ACCOUNT-SYNC-001").SetOutTradeNo("sub2_custom_account_sync_001").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).SetSubscriptionGroupID(sourceGroup.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(2).SetSubscriptionSourceGroupID(sourceGroup.ID).SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusPaid).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: activeGroup.ID, Name: activeGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{entClient: client, groupRepo: groupRepo, subscriptionSvc: subSvc, providersLoaded: true}

	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))

	bindings, err := client.AccountGroup.Query().Where(accountgroup.GroupIDEQ(activeGroup.ID)).All(ctx)
	require.NoError(t, err)
	require.Len(t, bindings, 2)
	priorities := map[int64]int{}
	for _, binding := range bindings {
		priorities[binding.AccountID] = binding.Priority
	}
	require.NotContains(t, priorities, staleAccount.ID, "custom group must drop accounts removed from the source group")
	require.Equal(t, 7, priorities[keptAccount.ID], "custom group must refresh changed source priorities")
	require.Equal(t, 8, priorities[newAccount.ID], "custom group must copy newly added source accounts")
}

func TestExecuteSubscriptionFulfillmentCustomGroupCopiesChannelBinding(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	_, err := client.ExecContext(ctx, `CREATE TABLE channels (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)
	require.NoError(t, err)
	_, err = client.ExecContext(ctx, `CREATE TABLE channel_groups (id INTEGER PRIMARY KEY AUTOINCREMENT, channel_id INTEGER NOT NULL, group_id INTEGER NOT NULL UNIQUE)`)
	require.NoError(t, err)

	user, err := client.User.Create().SetEmail("custom-channel-sync@example.com").SetPasswordHash("hash").SetUsername("channel-sync").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().
		SetName("channel-sync-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(10).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().
		SetName("Channel Sync Plan").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).
		SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	_, err = client.ExecContext(ctx, `INSERT INTO channels (id, name) VALUES (77, 'source-channel')`)
	require.NoError(t, err)
	_, err = client.ExecContext(ctx, `INSERT INTO channel_groups (channel_id, group_id) VALUES (77, `+strconv.FormatInt(sourceGroup.ID, 10)+`)`)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(200).SetPayAmount(200).SetFeeRate(0).SetRechargeCode("CUSTOM-CHANNEL-SYNC-001").SetOutTradeNo("sub2_custom_channel_sync_001").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).SetSubscriptionGroupID(sourceGroup.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(2).SetSubscriptionSourceGroupID(sourceGroup.ID).SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusPaid).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: sourceGroup.ID, Name: sourceGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{entClient: client, groupRepo: groupRepo, subscriptionSvc: subSvc, providersLoaded: true}

	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))
	customGroups, err := client.Group.Query().Where(group.IsCustomSubscriptionGroupEQ(true)).All(ctx)
	require.NoError(t, err)
	require.Len(t, customGroups, 0)
	rows, err := client.QueryContext(ctx, `SELECT channel_id FROM channel_groups WHERE group_id = `+strconv.FormatInt(sourceGroup.ID, 10))
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	require.True(t, rows.Next(), "source group channel binding must remain available")
	var channelID int64
	require.NoError(t, rows.Scan(&channelID))
	require.Equal(t, int64(77), channelID)
}

func TestExecuteSubscriptionFulfillmentRejectsReusedCustomGroupWhenSourceInactive(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-inactive-source@example.com").SetPasswordHash("hash").SetUsername("inactive-source").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("inactive-source").SetStatus(StatusDisabled).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetDailyLimitUsd(10).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Inactive Source Plan").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(2).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)

	activeGroup, err := client.Group.Create().
		SetName("Inactive Source Plan-active").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).
		SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(user.ID).SetCustomSourcePlanID(plan.ID).SetCustomSourceGroupID(sourceGroup.ID).SetCustomMultiplier(2).Save(ctx)
	require.NoError(t, err)
	activeSub, err := client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(activeGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().AddDate(0, 0, -1)).SetExpiresAt(time.Now().AddDate(0, 0, 10)).SetNotes("active custom").Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(200).SetPayAmount(200).SetFeeRate(0).SetRechargeCode("CUSTOM-INACTIVE-SOURCE-001").SetOutTradeNo("sub2_custom_inactive_source_001").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).SetSubscriptionGroupID(sourceGroup.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(2).SetSubscriptionSourceGroupID(sourceGroup.ID).SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusPaid).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: activeGroup.ID, Name: activeGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{entClient: client, groupRepo: groupRepo, subscriptionSvc: subSvc, providersLoaded: true}

	err = svc.ExecuteSubscriptionFulfillment(ctx, order.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no longer active")

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusFailed, updatedOrder.Status)
	updatedSub, err := client.UserSubscription.Get(ctx, activeSub.ID)
	require.NoError(t, err)
	require.True(t, activeSub.ExpiresAt.Equal(updatedSub.ExpiresAt), "failed fulfillment must not extend the subscription: before=%s after=%s", activeSub.ExpiresAt, updatedSub.ExpiresAt)
}

func TestExecuteSubscriptionFulfillmentRejectsVirtualCustomSourceGroupMismatch(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-virtual-source-mismatch@example.com").SetPasswordHash("hash").SetUsername("virtual-source-mismatch").Save(ctx)
	require.NoError(t, err)
	planSource, err := client.Group.Create().SetName("virtual-source-plan").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetDailyLimitUsd(10).Save(ctx)
	require.NoError(t, err)
	forgedSource, err := client.Group.Create().SetName("virtual-source-forged").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetDailyLimitUsd(1000).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Virtual Source Mismatch").SetDescription("custom").SetGroupID(planSource.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(300).SetPayAmount(300).SetFeeRate(0).SetRechargeCode("CUSTOM-VIRTUAL-SOURCE-MISMATCH-001").SetOutTradeNo("sub2_custom_virtual_source_mismatch_001").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).SetSubscriptionGroupID(forgedSource.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(3).SetSubscriptionSourceGroupID(forgedSource.ID).SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusPaid).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: forgedSource.ID, Name: forgedSource.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{entClient: client, groupRepo: groupRepo, subscriptionSvc: subSvc, providersLoaded: true}

	err = svc.ExecuteSubscriptionFulfillment(ctx, order.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "source group mismatch")
	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusFailed, updatedOrder.Status)
	subCount, err := client.UserSubscription.Query().Where(usersubscription.UserIDEQ(user.ID)).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, subCount, "forged source group must not create or extend a subscription")
}

func TestExecuteSubscriptionFulfillmentRejectsLegacyCustomSourceGroupMismatchBeforeSync(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-legacy-source-mismatch@example.com").SetPasswordHash("hash").SetUsername("legacy-source-mismatch").Save(ctx)
	require.NoError(t, err)
	planSource, err := client.Group.Create().SetName("legacy-source-plan").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetDailyLimitUsd(10).Save(ctx)
	require.NoError(t, err)
	forgedSource, err := client.Group.Create().SetName("legacy-source-forged").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetDailyLimitUsd(1000).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Legacy Source Mismatch").SetDescription("custom").SetGroupID(planSource.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	activeGroup, err := client.Group.Create().
		SetName("Legacy Source Mismatch-active").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).
		SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(user.ID).SetCustomSourcePlanID(plan.ID).SetCustomMultiplier(2).SetDailyLimitUsd(20).Save(ctx)
	require.NoError(t, err)
	activeSub, err := client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(activeGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().AddDate(0, 0, -1)).SetExpiresAt(time.Now().AddDate(0, 0, 10)).SetNotes("active legacy custom with missing source metadata").Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(200).SetPayAmount(200).SetFeeRate(0).SetRechargeCode("CUSTOM-LEGACY-SOURCE-MISMATCH-001").SetOutTradeNo("sub2_custom_legacy_source_mismatch_001").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).SetSubscriptionGroupID(planSource.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(2).SetSubscriptionSourceGroupID(forgedSource.ID).SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusPaid).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: activeGroup.ID, Name: activeGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{entClient: client, groupRepo: groupRepo, subscriptionSvc: subSvc, providersLoaded: true}

	err = svc.ExecuteSubscriptionFulfillment(ctx, order.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "source group mismatch")
	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusFailed, updatedOrder.Status)
	updatedGroup, err := client.Group.Get(ctx, activeGroup.ID)
	require.NoError(t, err)
	require.Nil(t, updatedGroup.CustomSourceGroupID, "failed fulfillment must not rewrite legacy custom group metadata")
	require.Equal(t, 2, *updatedGroup.CustomMultiplier)
	require.InDelta(t, 20, *updatedGroup.DailyLimitUsd, 1e-9)
	updatedSub, err := client.UserSubscription.Get(ctx, activeSub.ID)
	require.NoError(t, err)
	require.True(t, activeSub.ExpiresAt.Equal(updatedSub.ExpiresAt), "failed fulfillment must not extend the active legacy custom subscription: before=%s after=%s", activeSub.ExpiresAt, updatedSub.ExpiresAt)
}

func TestExecuteSubscriptionFulfillmentRejectsCustomSourceGroupTypeMismatch(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-type-mismatch@example.com").SetPasswordHash("hash").SetUsername("type-mismatch").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("type-mismatch-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeStandard).SetDailyLimitUsd(10).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Type Mismatch Plan").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(2).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(200).SetPayAmount(200).SetFeeRate(0).SetRechargeCode("CUSTOM-TYPE-MISMATCH-001").SetOutTradeNo("sub2_custom_type_mismatch_001").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).SetSubscriptionGroupID(sourceGroup.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(2).SetSubscriptionSourceGroupID(sourceGroup.ID).SetSubscriptionSourcePrice(100).
		SetStatus(OrderStatusPaid).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: sourceGroup.ID, Name: sourceGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard}}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{entClient: client, groupRepo: groupRepo, subscriptionSvc: subSvc, providersLoaded: true}

	err = svc.ExecuteSubscriptionFulfillment(ctx, order.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not subscription type")

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusFailed, updatedOrder.Status)
	customCount, err := client.Group.Query().Where(group.IsCustomSubscriptionGroupEQ(true)).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, customCount, "failed fulfillment must not create a forged custom subscription group")
	subCount, err := client.UserSubscription.Query().Where(usersubscription.UserIDEQ(user.ID)).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, subCount, "failed fulfillment must not extend subscription")
}

func TestAdminUpdateCustomSubscriptionGroupMultiplierRecalculatesLimits(t *testing.T) {
	daily, weekly := 10.0, 20.0
	sourceID := int64(100)
	existingGroup := &Group{
		ID:                        200,
		Name:                      "custom-admin",
		Platform:                  PlatformOpenAI,
		Status:                    StatusActive,
		SubscriptionType:          SubscriptionTypeSubscription,
		DailyLimitUSD:             &daily,
		WeeklyLimitUSD:            &weekly,
		IsCustomSubscriptionGroup: true,
		CustomSourceGroupID:       &sourceID,
		CustomMultiplier:          customSubTestIntPtr(2),
	}
	sourceGroup := &Group{ID: sourceID, Platform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily, WeeklyLimitUSD: &weekly, MonthlyLimitUSD: nil}
	repo := &customMultiplierGroupRepoStub{groupRepoStubForAdmin: groupRepoStubForAdmin{getByID: existingGroup}, source: sourceGroup}
	svc := &adminServiceImpl{groupRepo: repo}
	newMultiplier := 4

	_, err := svc.UpdateGroup(context.Background(), existingGroup.ID, &UpdateGroupInput{CustomMultiplier: &newMultiplier})
	require.NoError(t, err)
	require.NotNil(t, repo.updated)
	require.Equal(t, 4, *repo.updated.CustomMultiplier)
	require.InDelta(t, 40, *repo.updated.DailyLimitUSD, 1e-9)
	require.InDelta(t, 80, *repo.updated.WeeklyLimitUSD, 1e-9)
	require.Nil(t, repo.updated.MonthlyLimitUSD)
}

func TestPrepareRefundUsesCustomGroupSubscriptionForDeduction(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)
	user, err := client.User.Create().SetEmail("custom-refund@example.com").SetPasswordHash("hash").SetUsername("refund").Save(ctx)
	require.NoError(t, err)
	customGroup, err := client.Group.Create().SetName("refund-custom").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).SetIsCustomSubscriptionGroup(true).SetCustomOwnerUserID(user.ID).SetCustomSourcePlanID(10).SetCustomSourceGroupID(20).SetCustomMultiplier(3).Save(ctx)
	require.NoError(t, err)
	sub, err := client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(customGroup.ID).SetStatus(SubscriptionStatusActive).SetStartsAt(time.Now().AddDate(0, 0, -1)).SetExpiresAt(time.Now().AddDate(0, 0, 45)).SetNotes("custom refund").Save(ctx)
	require.NoError(t, err)
	inst, err := client.PaymentProviderInstance.Create().SetProviderKey(payment.TypeAlipay).SetName("refund-provider").SetConfig("{}").SetSupportedTypes("alipay").SetEnabled(true).SetRefundEnabled(true).Save(ctx)
	require.NoError(t, err)
	instID := "" + strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).SetAmount(300).SetPayAmount(300).SetFeeRate(0).
		SetRechargeCode("CUSTOM-REFUND-001").SetOutTradeNo("sub2_custom_refund_001").SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("trade-custom-refund").
		SetOrderType(payment.OrderTypeSubscription).SetStatus(OrderStatusCompleted).SetExpiresAt(time.Now().Add(time.Hour)).SetPaidAt(time.Now()).SetCompletedAt(time.Now()).
		SetSubscriptionGroupID(customGroup.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(3).SetSubscriptionSourceGroupID(20).SetSubscriptionSourcePrice(100).
		SetProviderInstanceID(instID).SetProviderKey(payment.TypeAlipay).SetProviderSnapshot(map[string]any{"schema_version": 2, "provider_instance_id": instID, "provider_key": payment.TypeAlipay}).
		SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: customGroup.ID, Name: customGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subSvc := NewSubscriptionService(groupRepo, &paymentFulfillmentSubscriptionRepo{client: client}, nil, client, nil)
	svc := &PaymentService{entClient: client, subscriptionSvc: subSvc}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, true)
	require.NoError(t, err)
	require.Nil(t, result)
	require.NotNil(t, plan)
	require.Equal(t, payment.DeductionTypeSubscription, plan.DeductionType)
	require.Equal(t, sub.ID, plan.SubscriptionID)
	require.Equal(t, 30, plan.SubDaysToDeduct)
}

func TestPrepareRefundRequiresForceWhenVirtualCustomEntitlementExpired(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("custom-refund-expired-overlay@example.com").SetPasswordHash("hash").SetUsername("refund-expired-overlay").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("refund-expired-overlay-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetName("Refund Expired Overlay").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	customExpiredAt := time.Now().AddDate(0, 0, -1)
	baseExpiresAt := time.Now().AddDate(0, 0, 365)
	sub, err := client.UserSubscription.Create().
		SetUserID(user.ID).SetGroupID(sourceGroup.ID).SetStatus(SubscriptionStatusActive).
		SetStartsAt(time.Now().AddDate(0, 0, -10)).SetExpiresAt(baseExpiresAt).SetNotes("expired virtual custom over long base").
		SetCustomMultiplier(3).SetCustomSourcePlanID(plan.ID).SetCustomSourceGroupID(sourceGroup.ID).SetCustomExpiresAt(customExpiredAt).
		Save(ctx)
	require.NoError(t, err)
	inst, err := client.PaymentProviderInstance.Create().SetProviderKey(payment.TypeAlipay).SetName("refund-expired-overlay-provider").SetConfig("{}").SetSupportedTypes("alipay").SetEnabled(true).SetRefundEnabled(true).Save(ctx)
	require.NoError(t, err)
	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).SetAmount(300).SetPayAmount(300).SetFeeRate(0).
		SetRechargeCode("CUSTOM-REFUND-EXPIRED-OVERLAY-001").SetOutTradeNo("sub2_custom_refund_expired_overlay_001").SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("trade-custom-refund-expired-overlay").
		SetOrderType(payment.OrderTypeSubscription).SetStatus(OrderStatusCompleted).SetExpiresAt(time.Now().Add(time.Hour)).SetPaidAt(time.Now()).SetCompletedAt(time.Now()).
		SetPlanID(plan.ID).SetSubscriptionGroupID(sourceGroup.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(3).SetSubscriptionSourceGroupID(sourceGroup.ID).SetSubscriptionSourcePrice(100).
		SetProviderInstanceID(instID).SetProviderKey(payment.TypeAlipay).SetProviderSnapshot(map[string]any{"schema_version": 2, "provider_instance_id": instID, "provider_key": payment.TypeAlipay}).
		SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: sourceGroup.ID, Name: sourceGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subSvc := NewSubscriptionService(groupRepo, &paymentFulfillmentSubscriptionRepo{client: client}, nil, client, nil)
	svc := &PaymentService{entClient: client, subscriptionSvc: subSvc}

	planResult, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, true)
	require.NoError(t, err)
	require.Nil(t, planResult)
	require.NotNil(t, result)
	require.True(t, result.RequireForce)
	require.Contains(t, result.Warning, "custom subscription entitlement already expired")

	forcedPlan, forcedResult, err := svc.PrepareRefund(ctx, order.ID, 0, "", true, true)
	require.NoError(t, err)
	require.Nil(t, forcedResult)
	require.NotNil(t, forcedPlan)
	require.Equal(t, payment.DeductionTypeSubscription, forcedPlan.DeductionType)
	require.Equal(t, sub.ID, forcedPlan.SubscriptionID)
	require.Zero(t, forcedPlan.SubDaysToDeduct, "force refund for an already-expired custom overlay must not deduct the remaining base subscription")

	unchangedSub, err := client.UserSubscription.Get(ctx, sub.ID)
	require.NoError(t, err)
	require.True(t, sub.ExpiresAt.Equal(unchangedSub.ExpiresAt), "expires_at changed: before=%s after=%s", sub.ExpiresAt, unchangedSub.ExpiresAt)
	require.NotNil(t, sub.CustomExpiresAt)
	require.NotNil(t, unchangedSub.CustomExpiresAt)
	require.True(t, sub.CustomExpiresAt.Equal(*unchangedSub.CustomExpiresAt), "custom_expires_at changed: before=%s after=%s", *sub.CustomExpiresAt, *unchangedSub.CustomExpiresAt)
}

func TestPrepareRefundPlainSubscriptionDeductsBaseEvenWithExpiredVirtualCustomMetadata(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().SetEmail("plain-refund-expired-custom@example.com").SetPasswordHash("hash").SetUsername("plain-refund-expired-custom").Save(ctx)
	require.NoError(t, err)
	sourceGroup, err := client.Group.Create().SetName("plain-refund-source").SetStatus(StatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	plainPlan, err := client.SubscriptionPlan.Create().SetName("Plain Refund Plan").SetDescription("plain").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(false).SetCustomMultiplierMin(1).SetCustomMultiplierMax(1).Save(ctx)
	require.NoError(t, err)
	customPlan, err := client.SubscriptionPlan.Create().SetName("Historical Custom Plan").SetDescription("custom").SetGroupID(sourceGroup.ID).SetPrice(100).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).Save(ctx)
	require.NoError(t, err)
	customExpiredAt := time.Now().AddDate(0, 0, -1)
	baseExpiresAt := time.Now().AddDate(0, 0, 365)
	sub, err := client.UserSubscription.Create().
		SetUserID(user.ID).SetGroupID(sourceGroup.ID).SetStatus(SubscriptionStatusActive).
		SetStartsAt(time.Now().AddDate(0, 0, -10)).SetExpiresAt(baseExpiresAt).SetNotes("plain refund over expired custom metadata").
		SetCustomMultiplier(3).SetCustomSourcePlanID(customPlan.ID).SetCustomSourceGroupID(sourceGroup.ID).SetCustomExpiresAt(customExpiredAt).
		Save(ctx)
	require.NoError(t, err)
	inst, err := client.PaymentProviderInstance.Create().SetProviderKey(payment.TypeAlipay).SetName("plain-refund-provider").SetConfig("{}").SetSupportedTypes("alipay").SetEnabled(true).SetRefundEnabled(true).Save(ctx)
	require.NoError(t, err)
	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).SetAmount(100).SetPayAmount(100).SetFeeRate(0).
		SetRechargeCode("PLAIN-REFUND-EXPIRED-CUSTOM-001").SetOutTradeNo("sub2_plain_refund_expired_custom_001").SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("trade-plain-refund-expired-custom").
		SetOrderType(payment.OrderTypeSubscription).SetStatus(OrderStatusCompleted).SetExpiresAt(time.Now().Add(time.Hour)).SetPaidAt(time.Now()).SetCompletedAt(time.Now()).
		SetPlanID(plainPlan.ID).SetSubscriptionGroupID(sourceGroup.ID).SetSubscriptionDays(30).SetSubscriptionMultiplier(1).SetSubscriptionSourceGroupID(sourceGroup.ID).SetSubscriptionSourcePrice(100).
		SetProviderInstanceID(instID).SetProviderKey(payment.TypeAlipay).SetProviderSnapshot(map[string]any{"schema_version": 2, "provider_instance_id": instID, "provider_key": payment.TypeAlipay}).
		SetClientIP("127.0.0.1").SetSrcHost("app.example.com").Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: sourceGroup.ID, Name: sourceGroup.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subSvc := NewSubscriptionService(groupRepo, &paymentFulfillmentSubscriptionRepo{client: client}, nil, client, nil)
	svc := &PaymentService{entClient: client, subscriptionSvc: subSvc}

	planResult, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, true)
	require.NoError(t, err)
	require.Nil(t, result)
	require.NotNil(t, planResult)
	require.Equal(t, payment.DeductionTypeSubscription, planResult.DeductionType)
	require.Equal(t, sub.ID, planResult.SubscriptionID)
	require.Equal(t, 30, planResult.SubDaysToDeduct, "plain subscription refunds must still deduct base days")
}

func TestAdminCreateCustomSubscriptionGroupMetadataRejected(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}
	ownerID := int64(10)
	sourcePlanID := int64(20)
	sourceGroupID := int64(30)
	multiplier := 3

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                      "forged-custom",
		Platform:                  PlatformOpenAI,
		RateMultiplier:            1,
		SubscriptionType:          SubscriptionTypeSubscription,
		IsCustomSubscriptionGroup: true,
		CustomOwnerUserID:         &ownerID,
		CustomSourcePlanID:        &sourcePlanID,
		CustomSourceGroupID:       &sourceGroupID,
		CustomMultiplier:          &multiplier,
	})
	require.Error(t, err)
	require.Nil(t, repo.created)
}

func TestAdminUpdateCustomSubscriptionGroupSystemMetadataRejected(t *testing.T) {
	ownerID := int64(10)
	planID := int64(20)
	sourceID := int64(30)
	existingGroup := &Group{
		ID:                        200,
		Name:                      "custom-admin",
		Platform:                  PlatformOpenAI,
		Status:                    StatusActive,
		SubscriptionType:          SubscriptionTypeSubscription,
		IsCustomSubscriptionGroup: true,
		CustomOwnerUserID:         &ownerID,
		CustomSourcePlanID:        &planID,
		CustomSourceGroupID:       &sourceID,
		CustomMultiplier:          customSubTestIntPtr(2),
	}
	repo := &groupRepoStubForAdmin{getByID: existingGroup}
	svc := &adminServiceImpl{groupRepo: repo}
	otherPlanID := int64(99)

	_, err := svc.UpdateGroup(context.Background(), existingGroup.ID, &UpdateGroupInput{CustomSourcePlanID: &otherPlanID})
	require.Error(t, err)
	require.Nil(t, repo.updated)
}

func TestAdminUpdatePlainGroupCustomMultiplierRejected(t *testing.T) {
	existingGroup := &Group{ID: 200, Name: "plain", Platform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}
	repo := &groupRepoStubForAdmin{getByID: existingGroup}
	svc := &adminServiceImpl{groupRepo: repo}
	multiplier := 3

	_, err := svc.UpdateGroup(context.Background(), existingGroup.ID, &UpdateGroupInput{CustomMultiplier: &multiplier})
	require.Error(t, err)
	require.Nil(t, repo.updated)
}

type customMultiplierGroupRepoStub struct {
	groupRepoStubForAdmin
	source *Group
}

func (s *customMultiplierGroupRepoStub) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	if s.source != nil && s.source.ID == id {
		return s.source, nil
	}
	return s.groupRepoStubForAdmin.GetByIDLite(context.Background(), id)
}

func customSubTestIntPtr(v int) *int { return &v }
