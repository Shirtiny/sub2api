//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/subscriptionconcurrencyentitlement"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionearlyresetentitlement"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/stretchr/testify/require"
)

type adminPlanSubscriptionRepo struct {
	*paymentFulfillmentSubscriptionRepo
}

func (r *adminPlanSubscriptionRepo) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	client := paymentFulfillmentSubscriptionClientFromContext(ctx, r.client)
	return client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID), usersubscription.GroupIDEQ(groupID)).
		Exist(ctx)
}

func TestAssignPlanSubscriptionPersistsOneXPlanEntitlements(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)
	user, err := client.User.Create().
		SetEmail("admin-plan-assign@example.com").
		SetPasswordHash("hash").
		SetUsername("admin-plan-assign").
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetName("Specialty").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().
		SetName("Specialty Plan").
		SetDescription("custom plan").
		SetGroupID(group.ID).
		SetPrice(449).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetConcurrency(12).
		SetEarlyResetEnabled(true).
		SetEarlyResetDurationDays(5).
		SetCustomMultiplierEnabled(true).
		SetCustomMultiplierMin(1).
		SetCustomMultiplierMax(2).
		Save(ctx)
	require.NoError(t, err)

	repo := &adminPlanSubscriptionRepo{paymentFulfillmentSubscriptionRepo: &paymentFulfillmentSubscriptionRepo{client: client}}
	groupRepo := &subscriptionGroupRepoStub{group: &Group{
		ID:               group.ID,
		Name:             group.Name,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
	}}
	svc := NewSubscriptionService(groupRepo, repo, nil, client, nil)
	multiplier := 1
	before := time.Now()
	sub, err := svc.AssignPlanSubscription(ctx, &AssignPlanSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		PlanID:       plan.ID,
		Multiplier:   &multiplier,
		ValidityDays: 900,
	})
	require.NoError(t, err)
	require.NotNil(t, sub.CustomMultiplier)
	require.Equal(t, 1, *sub.CustomMultiplier)
	require.Equal(t, plan.ID, *sub.CustomSourcePlanID)
	require.Equal(t, group.ID, *sub.CustomSourceGroupID)
	require.Equal(t, "Specialty-1x", sub.CustomDisplayName)
	require.True(t, sub.ExpiresAt.After(before.AddDate(0, 0, 899)))
	require.True(t, sub.ExpiresAt.Before(before.AddDate(0, 0, 901)))

	concurrencyGrant, err := client.SubscriptionConcurrencyEntitlement.Query().
		Where(subscriptionconcurrencyentitlement.SubscriptionIDEQ(sub.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, 12, concurrencyGrant.Concurrency)
	require.Nil(t, concurrencyGrant.SourceOrderID)
	require.Equal(t, sub.ExpiresAt, concurrencyGrant.ExpiresAt)

	resetGrant, err := client.SubscriptionEarlyResetEntitlement.Query().
		Where(subscriptionearlyresetentitlement.SubscriptionIDEQ(sub.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.True(t, resetGrant.Enabled)
	require.Equal(t, 5, resetGrant.DurationDays)
	require.True(t, resetGrant.CustomTerm)
}

func TestAssignPlanSubscriptionReusesOnlyMatchingPlanConcurrency(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)
	user, err := client.User.Create().
		SetEmail("admin-plan-reuse@example.com").
		SetPasswordHash("hash").
		SetUsername("admin-plan-reuse").
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetName("Reuse Plan Group").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().
		SetName("Reuse Plan").
		SetGroupID(group.ID).
		SetPrice(100).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetConcurrency(12).
		Save(ctx)
	require.NoError(t, err)

	repo := &adminPlanSubscriptionRepo{paymentFulfillmentSubscriptionRepo: &paymentFulfillmentSubscriptionRepo{client: client}}
	groupRepo := &subscriptionGroupRepoStub{group: &Group{
		ID:               group.ID,
		Name:             group.Name,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
	}}
	svc := NewSubscriptionService(groupRepo, repo, nil, client, nil)
	input := &AssignPlanSubscriptionInput{UserID: user.ID, GroupID: group.ID, PlanID: plan.ID, ValidityDays: 30}
	first, err := svc.AssignPlanSubscription(ctx, input)
	require.NoError(t, err)
	second, err := svc.AssignPlanSubscription(ctx, input)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)

	count, err := client.SubscriptionConcurrencyEntitlement.Query().
		Where(subscriptionconcurrencyentitlement.SubscriptionIDEQ(first.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestAssignPlanSubscriptionRejectsExistingSubscriptionWithoutPlanConcurrency(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)
	user, err := client.User.Create().
		SetEmail("admin-plan-mismatch@example.com").
		SetPasswordHash("hash").
		SetUsername("admin-plan-mismatch").
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetName("Mismatch Plan Group").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().
		SetName("Mismatch Plan").
		SetGroupID(group.ID).
		SetPrice(100).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetConcurrency(16).
		Save(ctx)
	require.NoError(t, err)
	startsAt := time.Now().Add(-time.Minute)
	_, err = client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStartsAt(startsAt).
		SetExpiresAt(startsAt.AddDate(0, 0, 30)).
		SetStatus(SubscriptionStatusActive).
		Save(ctx)
	require.NoError(t, err)

	repo := &adminPlanSubscriptionRepo{paymentFulfillmentSubscriptionRepo: &paymentFulfillmentSubscriptionRepo{client: client}}
	groupRepo := &subscriptionGroupRepoStub{group: &Group{
		ID:               group.ID,
		Name:             group.Name,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
	}}
	svc := NewSubscriptionService(groupRepo, repo, nil, client, nil)
	_, err = svc.AssignPlanSubscription(ctx, &AssignPlanSubscriptionInput{
		UserID: user.ID, GroupID: group.ID, PlanID: plan.ID, ValidityDays: 30,
	})
	require.ErrorIs(t, err, ErrSubscriptionAssignConflict)
}

func TestUpdateSubscriptionMultiplierPreservesTermAndUsage(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)
	user, err := client.User.Create().
		SetEmail("admin-plan-update@example.com").
		SetPasswordHash("hash").
		SetUsername("admin-plan-update").
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetName("Specialty").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().
		SetName("Specialty Plan").
		SetDescription("custom plan").
		SetGroupID(group.ID).
		SetPrice(449).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetConcurrency(12).
		SetEarlyResetEnabled(false).
		SetCustomMultiplierEnabled(true).
		SetCustomMultiplierMin(1).
		SetCustomMultiplierMax(2).
		Save(ctx)
	require.NoError(t, err)

	startsAt := time.Now().Add(-time.Hour)
	expiresAt := startsAt.AddDate(0, 0, 900)
	plain, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStartsAt(startsAt).
		SetExpiresAt(expiresAt).
		SetStatus(SubscriptionStatusActive).
		SetDailyUsageUsd(12.5).
		SetWeeklyUsageUsd(34.5).
		SetMonthlyUsageUsd(56.5).
		Save(ctx)
	require.NoError(t, err)

	repo := &adminPlanSubscriptionRepo{paymentFulfillmentSubscriptionRepo: &paymentFulfillmentSubscriptionRepo{client: client}}
	groupRepo := &subscriptionGroupRepoStub{group: &Group{
		ID:               group.ID,
		Name:             group.Name,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
	}}
	svc := NewSubscriptionService(groupRepo, repo, nil, client, nil)

	updated, err := svc.UpdateSubscriptionMultiplier(ctx, &UpdateSubscriptionMultiplierInput{
		SubscriptionID: plain.ID,
		PlanID:         plan.ID,
		Multiplier:     1,
	})
	require.NoError(t, err)
	require.True(t, expiresAt.Equal(updated.ExpiresAt))
	require.Equal(t, 12.5, updated.DailyUsageUSD)
	require.Equal(t, 34.5, updated.WeeklyUsageUSD)
	require.Equal(t, 56.5, updated.MonthlyUsageUSD)
	require.Equal(t, 1, *updated.CustomMultiplier)
	require.True(t, expiresAt.Equal(*updated.CustomExpiresAt))
	require.Equal(t, "Specialty-1x", updated.CustomDisplayName)

	updated, err = svc.UpdateSubscriptionMultiplier(ctx, &UpdateSubscriptionMultiplierInput{
		SubscriptionID: plain.ID,
		PlanID:         plan.ID,
		Multiplier:     2,
	})
	require.NoError(t, err)
	require.True(t, expiresAt.Equal(updated.ExpiresAt))
	require.Equal(t, 2, *updated.CustomMultiplier)
	require.Equal(t, "Specialty-2x", updated.CustomDisplayName)

	concurrencyCount, err := client.SubscriptionConcurrencyEntitlement.Query().
		Where(
			subscriptionconcurrencyentitlement.SubscriptionIDEQ(plain.ID),
			subscriptionconcurrencyentitlement.SourceOrderIDIsNil(),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, concurrencyCount)
	resetCount, err := client.SubscriptionEarlyResetEntitlement.Query().
		Where(
			subscriptionearlyresetentitlement.SubscriptionIDEQ(plain.ID),
			subscriptionearlyresetentitlement.SourceOrderIDIsNil(),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, resetCount)
}
