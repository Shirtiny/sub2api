package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/promotionactivityparticipation"
	"github.com/stretchr/testify/require"
)

func TestPromotionActivityEligibilityUsesSharedPerUserLimit(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client, settingRepo: &paymentConfigSettingRepoStub{}}

	group := client.Group.Create().
		SetName("bonus-group").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SaveX(ctx)
	planA := client.SubscriptionPlan.Create().SetGroupID(group.ID).SetName("A").SetPrice(10).SetValidityDays(30).SetValidityUnit("days").SaveX(ctx)
	planB := client.SubscriptionPlan.Create().SetGroupID(group.ID).SetName("B").SetPrice(20).SetValidityDays(30).SetValidityUnit("days").SaveX(ctx)
	user := createPlanPropagationUser(t, client, "bonus@example.com")

	now := time.Now()
	activity, err := svc.CreatePromotionActivity(ctx, UpsertPromotionActivityRequest{
		Name: "Launch bonus", Type: PromotionActivityTypeSubscriptionBonusDays, Enabled: true,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), MaxUsesPerUser: 1,
		PlanBonuses: []PromotionActivityPlanInput{{PlanID: planA.ID, BonusDays: 3}, {PlanID: planB.ID, BonusDays: 7}},
	})
	require.NoError(t, err)

	benefits, err := svc.GetEligibleSubscriptionBonuses(ctx, user.ID, []int64{planA.ID, planB.ID})
	require.NoError(t, err)
	require.Equal(t, 3, benefits[planA.ID].Days)
	require.Equal(t, 7, benefits[planB.ID].Days)

	client.PromotionActivityParticipation.Create().
		SetActivityID(activity.ID).
		SetUserID(user.ID).
		SetOrderID(101).
		SetPlanID(planA.ID).
		SetBonusDays(3).
		SetStatus(PromotionParticipationStatusReserved).
		SetReservedAt(now).
		SaveX(ctx)

	benefits, err = svc.GetEligibleSubscriptionBonuses(ctx, user.ID, []int64{planA.ID, planB.ID})
	require.NoError(t, err)
	require.Empty(t, benefits)

	client.PromotionActivityParticipation.Update().
		Where(promotionactivityparticipation.OrderIDEQ(101)).
		SetStatus(PromotionParticipationStatusReleased).
		SetReleasedAt(time.Now()).
		SaveX(ctx)
	benefits, err = svc.GetEligibleSubscriptionBonuses(ctx, user.ID, []int64{planA.ID, planB.ID})
	require.NoError(t, err)
	require.Len(t, benefits, 2)
}

func TestPromotionActivityRejectsOverlappingPlanWindow(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client, settingRepo: &paymentConfigSettingRepoStub{}}
	group := client.Group.Create().SetName("overlap-group").SetPlatform(PlatformOpenAI).SetStatus(StatusActive).SetSubscriptionType(SubscriptionTypeSubscription).SaveX(ctx)
	plan := client.SubscriptionPlan.Create().SetGroupID(group.ID).SetName("Overlap").SetPrice(10).SetValidityDays(30).SetValidityUnit("days").SaveX(ctx)
	now := time.Now()

	request := UpsertPromotionActivityRequest{
		Name: "First", Type: PromotionActivityTypeSubscriptionBonusDays, Enabled: true,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), MaxUsesPerUser: 1,
		PlanBonuses: []PromotionActivityPlanInput{{PlanID: plan.ID, BonusDays: 5}},
	}
	_, err := svc.CreatePromotionActivity(ctx, request)
	require.NoError(t, err)
	request.Name = "Second"
	_, err = svc.CreatePromotionActivity(ctx, request)
	require.Error(t, err)
	require.Contains(t, err.Error(), "overlaps")
}

func TestPromotionActivityReleasedParticipationAllowsUpdateButPreservesHistory(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client, settingRepo: &paymentConfigSettingRepoStub{}}
	group := client.Group.Create().SetName("released-group").SetPlatform(PlatformOpenAI).SetStatus(StatusActive).SetSubscriptionType(SubscriptionTypeSubscription).SaveX(ctx)
	plan := client.SubscriptionPlan.Create().SetGroupID(group.ID).SetName("Released").SetPrice(10).SetValidityDays(30).SetValidityUnit("days").SaveX(ctx)
	user := client.User.Create().SetEmail("released-activity@example.com").SetPasswordHash("hash").SetUsername("released-activity").SaveX(ctx)
	now := time.Now()
	request := UpsertPromotionActivityRequest{
		Name: "Released activity", Type: PromotionActivityTypeSubscriptionBonusDays, Enabled: false,
		StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour), MaxUsesPerUser: 1,
		PlanBonuses: []PromotionActivityPlanInput{{PlanID: plan.ID, BonusDays: 3}},
	}
	activity, err := svc.CreatePromotionActivity(ctx, request)
	require.NoError(t, err)
	client.PromotionActivityParticipation.Create().
		SetActivityID(activity.ID).
		SetUserID(user.ID).
		SetOrderID(201).
		SetPlanID(plan.ID).
		SetBonusDays(3).
		SetStatus(PromotionParticipationStatusReleased).
		SetReservedAt(now).
		SetReleasedAt(now).
		SetReleaseReason("ORDER_CANCELLED").
		SaveX(ctx)

	request.Name = "Released activity updated"
	request.StartsAt = now.Add(3 * time.Hour)
	request.EndsAt = now.Add(5 * time.Hour)
	request.MaxUsesPerUser = 2
	request.PlanBonuses = []PromotionActivityPlanInput{{PlanID: plan.ID, BonusDays: 7}}
	updated, err := svc.UpdatePromotionActivity(ctx, activity.ID, request)
	require.NoError(t, err)
	require.Equal(t, request.Name, updated.Name)
	require.Equal(t, 2, updated.MaxUsesPerUser)
	require.Equal(t, 7, updated.PlanBonuses[0].BonusDays)

	err = svc.DeletePromotionActivity(ctx, activity.ID)
	require.ErrorContains(t, err, "participation history")
	_, err = client.PromotionActivity.Get(ctx, activity.ID)
	require.NoError(t, err)
	remaining, err := client.PromotionActivityParticipation.Query().
		Where(promotionactivityparticipation.ActivityIDEQ(activity.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, remaining)
}

func TestPromotionActivityReservedOrGrantedParticipationBlocksMutation(t *testing.T) {
	for _, status := range []string{PromotionParticipationStatusReserved, PromotionParticipationStatusGranted} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			svc := &PaymentConfigService{entClient: client, settingRepo: &paymentConfigSettingRepoStub{}}
			group := client.Group.Create().SetName(status + "-group").SetPlatform(PlatformOpenAI).SetStatus(StatusActive).SetSubscriptionType(SubscriptionTypeSubscription).SaveX(ctx)
			plan := client.SubscriptionPlan.Create().SetGroupID(group.ID).SetName(status).SetPrice(10).SetValidityDays(30).SetValidityUnit("days").SaveX(ctx)
			user := client.User.Create().SetEmail(status + "-activity@example.com").SetPasswordHash("hash").SetUsername(status + "-activity").SaveX(ctx)
			now := time.Now()
			request := UpsertPromotionActivityRequest{
				Name: status + " activity", Type: PromotionActivityTypeSubscriptionBonusDays, Enabled: false,
				StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour), MaxUsesPerUser: 1,
				PlanBonuses: []PromotionActivityPlanInput{{PlanID: plan.ID, BonusDays: 3}},
			}
			activity, err := svc.CreatePromotionActivity(ctx, request)
			require.NoError(t, err)
			participation := client.PromotionActivityParticipation.Create().
				SetActivityID(activity.ID).
				SetUserID(user.ID).
				SetOrderID(301).
				SetPlanID(plan.ID).
				SetBonusDays(3).
				SetStatus(status).
				SetReservedAt(now)
			if status == PromotionParticipationStatusGranted {
				participation.SetGrantedAt(now)
			}
			participation.SaveX(ctx)

			request.PlanBonuses = []PromotionActivityPlanInput{{PlanID: plan.ID, BonusDays: 7}}
			_, err = svc.UpdatePromotionActivity(ctx, activity.ID, request)
			require.ErrorContains(t, err, "reserved or granted participation")
			err = svc.DeletePromotionActivity(ctx, activity.ID)
			require.ErrorContains(t, err, "cannot be deleted")
		})
	}
}
