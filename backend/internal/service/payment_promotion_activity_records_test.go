package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestPromotionActivityHistoryQueries(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client, settingRepo: &paymentConfigSettingRepoStub{}}
	group := client.Group.Create().SetName("history-group").SetPlatform(PlatformOpenAI).SetStatus(StatusActive).SetSubscriptionType(SubscriptionTypeSubscription).SaveX(ctx)
	plan := client.SubscriptionPlan.Create().SetGroupID(group.ID).SetName("History Plan").SetPrice(20).SetValidityDays(30).SetValidityUnit("days").SaveX(ctx)
	userA := client.User.Create().SetEmail("history-a@example.com").SetPasswordHash("hash").SetUsername("history-a").SaveX(ctx)
	userB := client.User.Create().SetEmail("history-b@example.com").SetPasswordHash("hash").SetUsername("history-b").SaveX(ctx)
	now := time.Now().UTC()
	activity, err := svc.CreatePromotionActivity(ctx, UpsertPromotionActivityRequest{
		Name: "History activity", Type: PromotionActivityTypeSubscriptionBonusDays, Enabled: false,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), MaxUsesPerUser: 2,
		PlanBonuses: []PromotionActivityPlanInput{{PlanID: plan.ID, BonusDays: 5}},
	})
	require.NoError(t, err)

	orderA1 := createPromotionHistoryOrder(t, client, userA, plan.ID, group.ID, activity.ID, "HISTORY-A-1", OrderStatusCompleted, 5)
	orderA2 := createPromotionHistoryOrder(t, client, userA, plan.ID, group.ID, activity.ID, "HISTORY-A-2", OrderStatusCancelled, 5)
	orderB := createPromotionHistoryOrder(t, client, userB, plan.ID, group.ID, activity.ID, "HISTORY-B-1", OrderStatusPending, 5)
	client.PromotionActivityParticipation.Create().SetActivityID(activity.ID).SetUserID(userA.ID).SetOrderID(orderA1.ID).SetPlanID(plan.ID).SetBonusDays(5).SetStatus(PromotionParticipationStatusGranted).SetReservedAt(now.Add(-30 * time.Minute)).SetGrantedAt(now.Add(-20 * time.Minute)).SaveX(ctx)
	client.PromotionActivityParticipation.Create().SetActivityID(activity.ID).SetUserID(userA.ID).SetOrderID(orderA2.ID).SetPlanID(plan.ID).SetBonusDays(5).SetStatus(PromotionParticipationStatusReleased).SetReservedAt(now.Add(-15 * time.Minute)).SetReleasedAt(now.Add(-10 * time.Minute)).SetReleaseReason("ORDER_CANCELLED").SaveX(ctx)
	client.PromotionActivityParticipation.Create().SetActivityID(activity.ID).SetUserID(userB.ID).SetOrderID(orderB.ID).SetPlanID(plan.ID).SetBonusDays(5).SetStatus(PromotionParticipationStatusReserved).SetReservedAt(now.Add(-5 * time.Minute)).SaveX(ctx)

	records, total, err := svc.AdminListPromotionActivityRecords(ctx, PromotionActivityRecordListParams{Page: 1, PageSize: 20, Keyword: "History"})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, records, 1)
	require.Equal(t, 2, records[0].ParticipantCount)
	require.Equal(t, 3, records[0].ParticipationCount)
	require.Equal(t, 1, records[0].ReservedCount)
	require.Equal(t, 1, records[0].GrantedCount)
	require.Equal(t, 1, records[0].ReleasedCount)
	require.Equal(t, 5, records[0].GrantedBonusDays)

	participants, total, err := svc.AdminListPromotionActivityParticipants(ctx, activity.ID, PromotionActivityParticipantListParams{Page: 1, PageSize: 20, Keyword: userA.Email})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, participants, 1)
	require.Equal(t, userA.ID, participants[0].UserID)
	require.Equal(t, 2, participants[0].ParticipationCount)
	require.Equal(t, 5, participants[0].GrantedBonusDays)

	participations, total, err := svc.AdminListPromotionActivityParticipations(ctx, activity.ID, PromotionActivityParticipationListParams{Page: 1, PageSize: 20, UserID: userA.ID, Status: PromotionParticipationStatusGranted})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, participations, 1)
	require.Equal(t, orderA1.ID, participations[0].OrderID)
	require.Equal(t, "HISTORY-A-1", participations[0].OutTradeNo)
	require.Equal(t, "History Plan", participations[0].PlanName)
	require.Equal(t, 5, participations[0].BonusDays)
}

func createPromotionHistoryOrder(t *testing.T, client *dbent.Client, user *dbent.User, planID, groupID, activityID int64, outTradeNo, status string, bonusDays int) *dbent.PaymentOrder {
	t.Helper()
	return client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(20).SetPayAmount(20).SetFeeRate(0).SetRechargeCode(fmt.Sprintf("CODE-%s", outTradeNo)).SetOutTradeNo(outTradeNo).
		SetPaymentType("alipay").SetPaymentTradeNo("").SetOrderType("subscription").SetPlanID(planID).SetSubscriptionGroupID(groupID).
		SetSubscriptionDays(30).SetSubscriptionBonusActivityID(activityID).SetSubscriptionBonusDays(bonusDays).
		SetStatus(status).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").
		SaveX(context.Background())
}
