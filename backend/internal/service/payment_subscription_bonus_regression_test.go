package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivityparticipation"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivityplan"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func createSubscriptionBonusRegressionUser(t *testing.T, client *dbent.Client, suffix string) *dbent.User {
	t.Helper()
	return client.User.Create().
		SetEmail(fmt.Sprintf("bonus-regression-%s@example.com", suffix)).
		SetPasswordHash("hash").
		SetUsername("bonus-regression-" + suffix).
		SaveX(context.Background())
}

func createSubscriptionBonusRegressionOrder(t *testing.T, client *dbent.Client, user *dbent.User, groupID, planID int64, status string, activityID int64, bonusDays int, updatedAt time.Time) *dbent.PaymentOrder {
	t.Helper()
	builder := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(10).
		SetPayAmount(10).
		SetFeeRate(0).
		SetRechargeCode("").
		SetCafeCouponDiscount(0).
		SetOutTradeNo(fmt.Sprintf("bonus-regression-%d-%d", user.ID, updatedAt.UnixNano())).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(status).
		SetExpiresAt(updatedAt.Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("test.example.com").
		SetCreatedAt(updatedAt).
		SetUpdatedAt(updatedAt).
		SetSubscriptionDays(30).
		SetSubscriptionBonusDays(bonusDays).
		SetSubscriptionConcurrency(1).
		SetSubscriptionEarlyResetEnabled(false).
		SetSubscriptionEarlyResetDurationDays(0).
		SetSubscriptionMultiplier(1)
	if groupID > 0 {
		builder.SetSubscriptionGroupID(groupID).SetPlanID(planID)
	}
	if activityID > 0 {
		builder.SetSubscriptionBonusActivityID(activityID)
	}
	return builder.SaveX(context.Background())
}

func TestLatePaymentBeyondGraceDoesNotReacquireSubscriptionBonus(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user := createSubscriptionBonusRegressionUser(t, client, "expired")
	group := client.Group.Create().
		SetName("bonus-expired-group").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SaveX(ctx)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("bonus-expired-plan").
		SetPrice(10).
		SetValidityDays(30).
		SetValidityUnit("days").
		SaveX(ctx)
	activity := client.PromotionActivity.Create().
		SetName("expired-payment-activity").
		SetActivityType(PromotionActivityTypeSubscriptionBonusDays).
		SetEnabled(true).
		SetStartsAt(time.Now().Add(-time.Hour)).
		SetEndsAt(time.Now().Add(time.Hour)).
		SetMaxUsesPerUser(1).
		SaveX(ctx)

	old := time.Now().Add(-(paymentGraceMinutes*time.Minute + time.Minute))
	order := createSubscriptionBonusRegressionOrder(t, client, user, group.ID, plan.ID, OrderStatusExpired, activity.ID, 5, old)
	client.PromotionActivityParticipation.Create().
		SetActivityID(activity.ID).
		SetUserID(user.ID).
		SetOrderID(order.ID).
		SetPlanID(plan.ID).
		SetBonusDays(5).
		SetStatus(PromotionParticipationStatusReleased).
		SetReservedAt(old).
		SetReleasedAt(old).
		SetReleaseReason("ORDER_EXPIRED").
		SaveX(ctx)

	svc := &PaymentService{entClient: client}
	require.NoError(t, svc.toPaid(ctx, order, "late-trade-beyond-grace", 10, payment.TypeAlipay))

	current := client.PaymentOrder.GetX(ctx, order.ID)
	require.Equal(t, OrderStatusFailed, current.Status)
	require.NotNil(t, current.PaidAt)
	require.Equal(t, "late-trade-beyond-grace", current.PaymentTradeNo)
	require.NotNil(t, current.FailedReason)
	require.Equal(t, paymentFailureReasonAfterExpiry, *current.FailedReason)
	participation := client.PromotionActivityParticipation.Query().Where(
		promotionactivityparticipation.OrderIDEQ(order.ID),
	).OnlyX(ctx)
	require.Equal(t, PromotionParticipationStatusReleased, participation.Status)

	// A duplicate webhook must keep this payment refundable and must not retry
	// fulfillment or reacquire the released promotion reservation.
	require.NoError(t, svc.toPaid(ctx, order, "late-trade-beyond-grace", 10, payment.TypeAlipay))
	current = client.PaymentOrder.GetX(ctx, order.ID)
	require.Equal(t, OrderStatusFailed, current.Status)
	require.Equal(t, paymentFailureReasonAfterExpiry, *current.FailedReason)
}

func TestPrepareRefundForLateBalancePaymentDoesNotDeductExistingBalance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user := createSubscriptionBonusRegressionUser(t, client, "late-balance-refund")
	provider := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("late-balance-refund-provider").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeAlipay).
		SetEnabled(true).
		SetRefundEnabled(true).
		SaveX(ctx)
	providerID := fmt.Sprintf("%d", provider.ID)
	now := time.Now()
	order := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(10).
		SetPayAmount(10).
		SetRechargeCode("late-balance-refund-code").
		SetOutTradeNo("late-balance-refund-order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("late-balance-refund-trade").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusFailed).
		SetProviderInstanceID(providerID).
		SetProviderKey(payment.TypeAlipay).
		SetProviderSnapshot(map[string]any{"schema_version": 2, "provider_instance_id": providerID, "provider_key": payment.TypeAlipay}).
		SetExpiresAt(now.Add(-time.Hour)).
		SetPaidAt(now).
		SetFailedAt(now).
		SetFailedReason(paymentFailureReasonAfterExpiry).
		SetClientIP("127.0.0.1").
		SetSrcHost("test.example.com").
		SaveX(ctx)

	svc := &PaymentService{
		entClient: client,
		userRepo:  &latePaymentRefundUserRepo{user: &User{ID: user.ID, Balance: 100}},
	}
	refundPlan, earlyResult, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, true)
	require.NoError(t, err)
	require.Nil(t, earlyResult)
	require.NotNil(t, refundPlan)
	require.True(t, refundPlan.DeductBalance)
	require.Equal(t, payment.DeductionTypeNone, refundPlan.DeductionType)
	require.Zero(t, refundPlan.BalanceToDeduct)
}

func TestSuccessfulRefundReleasesUnfulfilledSubscriptionBonusReservation(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user := createSubscriptionBonusRegressionUser(t, client, "refund-release")
	group := client.Group.Create().
		SetName("bonus-refund-release-group").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SaveX(ctx)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("bonus-refund-release-plan").
		SetPrice(10).
		SetValidityDays(30).
		SetValidityUnit("days").
		SaveX(ctx)
	activity := client.PromotionActivity.Create().
		SetName("refund-release-activity").
		SetActivityType(PromotionActivityTypeSubscriptionBonusDays).
		SetEnabled(true).
		SetStartsAt(time.Now().Add(-time.Hour)).
		SetEndsAt(time.Now().Add(time.Hour)).
		SetMaxUsesPerUser(1).
		SaveX(ctx)
	order := createSubscriptionBonusRegressionOrder(t, client, user, group.ID, plan.ID, OrderStatusRefunding, activity.ID, 5, time.Now().Add(-time.Minute))
	client.PromotionActivityParticipation.Create().
		SetActivityID(activity.ID).
		SetUserID(user.ID).
		SetOrderID(order.ID).
		SetPlanID(plan.ID).
		SetBonusDays(5).
		SetStatus(PromotionParticipationStatusReserved).
		SetReservedAt(order.CreatedAt).
		SaveX(ctx)

	refreshed := client.PaymentOrder.GetX(ctx, order.ID)
	svc := &PaymentService{entClient: client}
	released, err := svc.finalizeSuccessfulRefund(ctx, &RefundPlan{
		OrderID:      order.ID,
		Order:        refreshed,
		RefundAmount: order.Amount,
		Reason:       "test refund",
	}, OrderStatusRefunded, time.Now())
	require.NoError(t, err)
	require.True(t, released)
	require.Equal(t, OrderStatusRefunded, client.PaymentOrder.GetX(ctx, order.ID).Status)
	participation := client.PromotionActivityParticipation.Query().Where(
		promotionactivityparticipation.OrderIDEQ(order.ID),
	).OnlyX(ctx)
	require.Equal(t, PromotionParticipationStatusReleased, participation.Status)
}

func TestSubscriptionValidityUnitsSupportSingularAndPluralForms(t *testing.T) {
	require.Equal(t, 30, psComputeValidityDays(30, "day"))
	require.Equal(t, 30, psComputeValidityDays(30, "days"))
	require.Equal(t, 28, psComputeValidityDays(4, "week"))
	require.Equal(t, 28, psComputeValidityDays(4, "weeks"))
	require.Equal(t, 30, psComputeValidityDays(1, "month"))
	require.Equal(t, 30, psComputeValidityDays(1, "months"))
	validated, err := validateSubscriptionPlanValidity(4, "weeks")
	require.NoError(t, err)
	require.Equal(t, 28, validated)
}

func TestPlanUpdateRejectsValidityThatBreaksEnabledBonusActivity(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	group := client.Group.Create().
		SetName("bonus-plan-update-group").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SaveX(ctx)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("bonus-plan-update-plan").
		SetPrice(10).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetForSale(true).
		SaveX(ctx)
	_, err := svc.CreatePromotionActivity(ctx, UpsertPromotionActivityRequest{
		Name: "plan validity guard", Type: PromotionActivityTypeSubscriptionBonusDays, Enabled: true,
		StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(time.Hour), MaxUsesPerUser: 1,
		PlanBonuses: []PromotionActivityPlanInput{{PlanID: plan.ID, BonusDays: 5}},
	})
	require.NoError(t, err)
	newValidity := 36500
	_, err = svc.UpdatePlan(ctx, plan.ID, UpdatePlanRequest{ValidityDays: &newValidity})
	require.Error(t, err)
	require.Contains(t, err.Error(), "promotion bonus")
	require.Equal(t, 30, client.SubscriptionPlan.GetX(ctx, plan.ID).ValidityDays)
}

func TestPlanDeletionBlocksPaidFailedOrderThatCanStillBeRetried(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	user := createSubscriptionBonusRegressionUser(t, client, "delete-paid-failed")
	group := client.Group.Create().
		SetName("delete-paid-failed-group").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SaveX(ctx)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("delete-paid-failed-plan").
		SetPrice(10).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetForSale(true).
		SaveX(ctx)
	order := createSubscriptionBonusRegressionOrder(t, client, user, group.ID, plan.ID, OrderStatusFailed, 0, 0, time.Now().Add(-time.Minute))
	client.PaymentOrder.UpdateOneID(order.ID).SetPaidAt(time.Now()).SaveX(ctx)
	err := svc.DeletePlan(ctx, plan.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "in-progress orders")
	require.NotNil(t, client.SubscriptionPlan.GetX(ctx, plan.ID))
}

func TestPaidOrderRetainsPaymentFactsWhenBonusReacquisitionFails(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user := createSubscriptionBonusRegressionUser(t, client, "reacquire")
	group := client.Group.Create().
		SetName("bonus-reacquire-group").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SaveX(ctx)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("bonus-reacquire-plan").
		SetPrice(10).
		SetValidityDays(30).
		SetValidityUnit("days").
		SaveX(ctx)
	activity := client.PromotionActivity.Create().
		SetName("reacquire-limit-activity").
		SetActivityType(PromotionActivityTypeSubscriptionBonusDays).
		SetEnabled(true).
		SetStartsAt(time.Now().Add(-time.Hour)).
		SetEndsAt(time.Now().Add(time.Hour)).
		SetMaxUsesPerUser(1).
		SaveX(ctx)

	usedOrder := createSubscriptionBonusRegressionOrder(t, client, user, group.ID, plan.ID, OrderStatusCompleted, activity.ID, 5, time.Now().Add(-2*time.Hour))
	client.PromotionActivityParticipation.Create().
		SetActivityID(activity.ID).
		SetUserID(user.ID).
		SetOrderID(usedOrder.ID).
		SetPlanID(plan.ID).
		SetBonusDays(5).
		SetStatus(PromotionParticipationStatusGranted).
		SetReservedAt(usedOrder.CreatedAt).
		SetGrantedAt(time.Now().Add(-time.Hour)).
		SaveX(ctx)

	createdAt := time.Now().Add(-time.Minute)
	order := createSubscriptionBonusRegressionOrder(t, client, user, group.ID, plan.ID, OrderStatusCancelled, activity.ID, 5, createdAt)
	client.PromotionActivityParticipation.Create().
		SetActivityID(activity.ID).
		SetUserID(user.ID).
		SetOrderID(order.ID).
		SetPlanID(plan.ID).
		SetBonusDays(5).
		SetStatus(PromotionParticipationStatusReleased).
		SetReservedAt(createdAt).
		SetReleasedAt(createdAt).
		SetReleaseReason("ORDER_CANCELLED").
		SaveX(ctx)

	svc := &PaymentService{entClient: client}
	err := svc.toPaid(ctx, order, "late-trade-reacquire-failed", 10, payment.TypeAlipay)
	require.Error(t, err)

	current := client.PaymentOrder.GetX(ctx, order.ID)
	require.Equal(t, OrderStatusFailed, current.Status)
	require.NotNil(t, current.PaidAt, "provider payment facts must survive fulfillment failure")
	require.Equal(t, "late-trade-reacquire-failed", current.PaymentTradeNo)
	participation := client.PromotionActivityParticipation.Query().Where(
		promotionactivityparticipation.OrderIDEQ(order.ID),
	).OnlyX(ctx)
	require.Equal(t, PromotionParticipationStatusReleased, participation.Status)
}

func TestPaidOrderRetainsPaymentFactsWhenCafeCouponReacquisitionFails(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user := createSubscriptionBonusRegressionUser(t, client, "coupon-reacquire")
	now := time.Now()
	order := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(10).
		SetPayAmount(9).
		SetFeeRate(0).
		SetRechargeCode("coupon-reacquire").
		SetCafeCouponCode("CAFE-REACQUIRE-USED").
		SetCafeCouponDiscount(1).
		SetOutTradeNo("coupon-reacquire-order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCancelled).
		SetExpiresAt(now.Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("test.example.com").
		SaveX(ctx)
	client.CafeCoupon.Create().
		SetCode("CAFE-REACQUIRE-USED").
		SetUserID(user.ID).
		SetMembershipLevel(1).
		SetCouponType(CafeCouponTypeCash).
		SetValue(1).
		SetPeriod(CafeCouponPeriodMonth).
		SetPeriodStart(now.Add(-time.Hour)).
		SetPeriodEnd(now.Add(time.Hour)).
		SetStatus(CafeCouponStatusApplied).
		SetOrderID(order.ID + 1000).
		SetAppliedAt(now).
		SaveX(ctx)

	svc := &PaymentService{entClient: client}
	err := svc.toPaid(ctx, order, "coupon-paid-trade", 9, payment.TypeAlipay)
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_USED", infraerrors.Reason(err))
	current := client.PaymentOrder.GetX(ctx, order.ID)
	require.Equal(t, OrderStatusFailed, current.Status)
	require.NotNil(t, current.PaidAt)
	require.Equal(t, "coupon-paid-trade", current.PaymentTradeNo)
}

func TestTerminalTransitionDoesNotReleaseReservationAfterPaymentWins(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user := createSubscriptionBonusRegressionUser(t, client, "race")
	group := client.Group.Create().
		SetName("bonus-race-group").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SaveX(ctx)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("bonus-race-plan").
		SetPrice(10).
		SetValidityDays(30).
		SetValidityUnit("days").
		SaveX(ctx)
	activity := client.PromotionActivity.Create().
		SetName("race-activity").
		SetActivityType(PromotionActivityTypeSubscriptionBonusDays).
		SetEnabled(true).
		SetStartsAt(time.Now().Add(-time.Hour)).
		SetEndsAt(time.Now().Add(time.Hour)).
		SetMaxUsesPerUser(1).
		SaveX(ctx)
	order := createSubscriptionBonusRegressionOrder(t, client, user, group.ID, plan.ID, OrderStatusPending, activity.ID, 5, time.Now())
	client.PromotionActivityParticipation.Create().
		SetActivityID(activity.ID).
		SetUserID(user.ID).
		SetOrderID(order.ID).
		SetPlanID(plan.ID).
		SetBonusDays(5).
		SetStatus(PromotionParticipationStatusReserved).
		SetReservedAt(time.Now()).
		SaveX(ctx)
	client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusPaid).SaveX(ctx)

	svc := &PaymentService{entClient: client}
	changed, err := svc.transitionPendingOrderWithBonusRelease(ctx, order, OrderStatusCancelled, "ORDER_CANCELLED")
	require.NoError(t, err)
	require.False(t, changed)
	participation := client.PromotionActivityParticipation.Query().Where(
		promotionactivityparticipation.OrderIDEQ(order.ID),
	).OnlyX(ctx)
	require.Equal(t, PromotionParticipationStatusReserved, participation.Status)
}

func TestDeletePlanDetachesInactivePromotionActivityReference(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	group := client.Group.Create().
		SetName("bonus-delete-group").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SaveX(ctx)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("bonus-delete-plan").
		SetPrice(10).
		SetValidityDays(30).
		SetValidityUnit("days").
		SaveX(ctx)
	activity := client.PromotionActivity.Create().
		SetName("delete-activity").
		SetActivityType(PromotionActivityTypeSubscriptionBonusDays).
		SetEnabled(true).
		SetStartsAt(time.Now().Add(-time.Hour)).
		SetEndsAt(time.Now().Add(time.Hour)).
		SetMaxUsesPerUser(1).
		SaveX(ctx)
	client.PromotionActivityPlan.Create().
		SetActivityID(activity.ID).
		SetPlanID(plan.ID).
		SetBonusDays(5).
		SaveX(ctx)

	err := svc.DeletePlan(ctx, plan.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "active or scheduled")

	client.PromotionActivity.UpdateOneID(activity.ID).SetEnabled(false).SaveX(ctx)
	require.NoError(t, svc.DeletePlan(ctx, plan.ID))
	_, err = client.SubscriptionPlan.Get(ctx, plan.ID)
	require.Error(t, err)
	require.Zero(t, client.PromotionActivityPlan.Query().Where(
		promotionactivityplan.PlanIDEQ(plan.ID),
	).CountX(ctx))
}

func TestDetachedPromotionActivityWithParticipationRemainsEditable(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	configSvc := &PaymentConfigService{entClient: client}
	group := client.Group.Create().
		SetName("bonus-detached-edit-group").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SaveX(ctx)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("bonus-detached-edit-plan").
		SetPrice(10).
		SetValidityDays(30).
		SetValidityUnit("days").
		SaveX(ctx)
	activity := client.PromotionActivity.Create().
		SetName("detached-edit-activity").
		SetActivityType(PromotionActivityTypeSubscriptionBonusDays).
		SetEnabled(false).
		SetStartsAt(time.Now().Add(-time.Hour)).
		SetEndsAt(time.Now().Add(time.Hour)).
		SetMaxUsesPerUser(1).
		SaveX(ctx)
	client.PromotionActivityPlan.Create().
		SetActivityID(activity.ID).
		SetPlanID(plan.ID).
		SetBonusDays(5).
		SaveX(ctx)
	user := createSubscriptionBonusRegressionUser(t, client, "detached-edit")
	order := createSubscriptionBonusRegressionOrder(t, client, user, group.ID, plan.ID, OrderStatusCompleted, activity.ID, 5, time.Now().Add(-time.Hour))
	client.PromotionActivityParticipation.Create().
		SetActivityID(activity.ID).
		SetUserID(user.ID).
		SetOrderID(order.ID).
		SetPlanID(plan.ID).
		SetBonusDays(5).
		SetStatus(PromotionParticipationStatusGranted).
		SetReservedAt(order.CreatedAt).
		SetGrantedAt(time.Now().Add(-time.Minute)).
		SaveX(ctx)

	err := configSvc.DeletePlan(ctx, plan.ID)
	// The activity is disabled, so deleting the plan detaches the association
	// while preserving the participation snapshot.
	require.NoError(t, err)
	updated, err := configSvc.UpdatePromotionActivity(ctx, activity.ID, UpsertPromotionActivityRequest{
		Name:           "detached-edit-activity-renamed",
		Type:           PromotionActivityTypeSubscriptionBonusDays,
		Enabled:        false,
		StartsAt:       activity.StartsAt,
		EndsAt:         activity.EndsAt,
		MaxUsesPerUser: 1,
	})
	require.NoError(t, err)
	require.Equal(t, "detached-edit-activity-renamed", updated.Name)
	require.Empty(t, updated.PlanBonuses)
}

func TestPrepareRefundDeductsGrantedSubscriptionBonusDays(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user := createSubscriptionBonusRegressionUser(t, client, "refund-granted")
	group := client.Group.Create().
		SetName("bonus-refund-granted-group").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SaveX(ctx)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("bonus-refund-granted-plan").
		SetPrice(10).
		SetValidityDays(30).
		SetValidityUnit("days").
		SaveX(ctx)
	client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(time.Now().Add(-time.Hour)).
		SetExpiresAt(time.Now().Add(35 * 24 * time.Hour)).
		SetNotes("bonus refund regression").
		SaveX(ctx)
	provider := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("bonus-refund-granted-provider").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeAlipay).
		SetEnabled(true).
		SetRefundEnabled(true).
		SaveX(ctx)
	order := createSubscriptionBonusRegressionOrder(t, client, user, group.ID, plan.ID, OrderStatusCompleted, 99, 5, time.Now().Add(-time.Minute))
	providerID := fmt.Sprintf("%d", provider.ID)
	client.PaymentOrder.UpdateOneID(order.ID).
		SetProviderInstanceID(providerID).
		SetProviderKey(payment.TypeAlipay).
		SetProviderSnapshot(map[string]any{"schema_version": 2, "provider_instance_id": providerID, "provider_key": payment.TypeAlipay}).
		SaveX(ctx)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: group.ID, Name: group.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subscriptionSvc := NewSubscriptionService(groupRepo, &bonusRefundSubscriptionRepo{sub: &UserSubscription{ID: 1, UserID: user.ID, GroupID: group.ID, Status: SubscriptionStatusActive, ExpiresAt: time.Now().Add(35 * 24 * time.Hour)}}, nil, client, nil)
	svc := &PaymentService{entClient: client, subscriptionSvc: subscriptionSvc}
	refundPlan, earlyResult, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, true)
	require.NoError(t, err)
	require.Nil(t, earlyResult)
	require.NotNil(t, refundPlan)
	require.Equal(t, payment.DeductionTypeSubscription, refundPlan.DeductionType)
	require.Equal(t, 35, refundPlan.SubDaysToDeduct)
}

func TestCreateOrderRejectsStaleSubscriptionPlanSnapshot(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user := createSubscriptionBonusRegressionUser(t, client, "stale-plan")
	group := client.Group.Create().
		SetName("bonus-stale-plan-group").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SaveX(ctx)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("bonus-stale-plan").
		SetPrice(10).
		SetValidityDays(30).
		SetValidityUnit("days").
		SaveX(ctx)
	stalePlan := client.SubscriptionPlan.GetX(ctx, plan.ID)
	client.SubscriptionPlan.UpdateOneID(plan.ID).
		SetPrice(12).
		SetUpdatedAt(time.Now().Add(time.Second)).
		SaveX(ctx)

	svc := &PaymentService{entClient: client}
	_, err := svc.createOrderInTx(ctx, CreateOrderRequest{
		UserID:      user.ID,
		PaymentType: payment.TypeAlipay,
		OrderType:   payment.OrderTypeSubscription,
		PlanID:      plan.ID,
		Multiplier:  1,
		ClientIP:    "127.0.0.1",
		SrcHost:     "test.example.com",
	}, &User{ID: user.ID, Email: user.Email, Username: user.Username}, stalePlan, &PaymentConfig{MaxPendingOrders: 3, OrderTimeoutMin: 30}, 10, 10, 0, 10, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "SUBSCRIPTION_PLAN_CHANGED")
}

func TestPrepareRefundForPaidUnfulfilledFailureDoesNotDeductSubscription(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user := createSubscriptionBonusRegressionUser(t, client, "refund-unfulfilled")
	group := client.Group.Create().
		SetName("bonus-refund-unfulfilled-group").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SaveX(ctx)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("bonus-refund-unfulfilled-plan").
		SetPrice(10).
		SetValidityDays(30).
		SetValidityUnit("days").
		SaveX(ctx)
	client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(time.Now().Add(-time.Hour)).
		SetExpiresAt(time.Now().Add(90 * 24 * time.Hour)).
		SetNotes("unrelated active subscription").
		SaveX(ctx)
	provider := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("bonus-refund-unfulfilled-provider").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeAlipay).
		SetEnabled(true).
		SetRefundEnabled(true).
		SaveX(ctx)
	order := createSubscriptionBonusRegressionOrder(t, client, user, group.ID, plan.ID, OrderStatusFailed, 100, 5, time.Now().Add(-time.Minute))
	providerID := fmt.Sprintf("%d", provider.ID)
	client.PaymentOrder.UpdateOneID(order.ID).
		SetPaidAt(time.Now()).
		SetPaymentTradeNo("paid-unfulfilled-trade").
		SetProviderInstanceID(providerID).
		SetProviderKey(payment.TypeAlipay).
		SetProviderSnapshot(map[string]any{"schema_version": 2, "provider_instance_id": providerID, "provider_key": payment.TypeAlipay}).
		SaveX(ctx)

	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: group.ID, Name: group.Name, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subscriptionSvc := NewSubscriptionService(groupRepo, &bonusRefundSubscriptionRepo{sub: &UserSubscription{ID: 1, UserID: user.ID, GroupID: group.ID, Status: SubscriptionStatusActive, ExpiresAt: time.Now().Add(90 * 24 * time.Hour)}}, nil, client, nil)
	svc := &PaymentService{entClient: client, subscriptionSvc: subscriptionSvc}
	refundPlan, earlyResult, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, true)
	require.NoError(t, err)
	require.Nil(t, earlyResult)
	require.NotNil(t, refundPlan)
	require.Equal(t, payment.DeductionTypeNone, refundPlan.DeductionType)
	require.Zero(t, refundPlan.SubDaysToDeduct)
}

type bonusRefundSubscriptionRepo struct {
	userSubRepoNoop
	sub *UserSubscription
}

type latePaymentRefundUserRepo struct {
	UserRepository
	user *User
}

func (r *latePaymentRefundUserRepo) GetByID(context.Context, int64) (*User, error) {
	return r.user, nil
}

func (r *bonusRefundSubscriptionRepo) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	return r.sub, nil
}
