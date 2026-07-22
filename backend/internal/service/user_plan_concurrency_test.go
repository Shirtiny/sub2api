package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestUserEffectiveConcurrencyAtUsesHighestActivePlan(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	user := &User{
		Concurrency: 2,
		PlanConcurrencyEntitlements: []PlanConcurrencyEntitlement{
			{Concurrency: 4, StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)},
			{Concurrency: 8, StartsAt: now.Add(-time.Minute), ExpiresAt: now.Add(2 * time.Hour)},
		},
	}

	require.Equal(t, 8, user.EffectiveConcurrencyAt(now))
}

func TestUserEffectiveConcurrencyAtUsesLatestTermPerSubscription(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	user := &User{
		Concurrency: 2,
		PlanConcurrencyEntitlements: []PlanConcurrencyEntitlement{
			{SubscriptionID: 1, Concurrency: 16, StartsAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(time.Hour)},
			{SubscriptionID: 1, Concurrency: 5, StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)},
		},
	}

	require.Equal(t, 5, user.EffectiveConcurrencyAt(now))
	user.PlanConcurrencyEntitlements = append(user.PlanConcurrencyEntitlements,
		PlanConcurrencyEntitlement{SubscriptionID: 2, Concurrency: 8, StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)},
	)
	require.Equal(t, 8, user.EffectiveConcurrencyAt(now))
}

func TestUserEffectiveConcurrencyAtFallsBackAfterPlanExpiry(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	user := &User{
		Concurrency: 3,
		PlanConcurrencyEntitlements: []PlanConcurrencyEntitlement{
			{Concurrency: 10, StartsAt: now.Add(-2 * time.Hour), ExpiresAt: now},
			{Concurrency: 20, StartsAt: now.Add(time.Minute), ExpiresAt: now.Add(time.Hour)},
		},
	}

	require.Equal(t, 3, user.EffectiveConcurrencyAt(now))
}

func TestActivePlanConcurrencyEntitlementUsesEarlierPlanExpiry(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	planExpiresAt := now.Add(time.Hour)
	concurrency := 6
	sub := &UserSubscription{
		StartsAt:                 now.Add(-time.Hour),
		ExpiresAt:                now.Add(24 * time.Hour),
		Status:                   SubscriptionStatusActive,
		PlanConcurrency:          &concurrency,
		PlanConcurrencyExpiresAt: &planExpiresAt,
	}

	entitlement, ok := sub.ActivePlanConcurrencyEntitlementAt(now)
	require.True(t, ok)
	require.Equal(t, concurrency, entitlement.Concurrency)
	require.Equal(t, planExpiresAt, entitlement.ExpiresAt)
	_, ok = sub.ActivePlanConcurrencyEntitlementAt(planExpiresAt)
	require.False(t, ok)
}

func TestSubscriptionOrderConcurrencyIgnoresCustomMultiplier(t *testing.T) {
	concurrency, err := subscriptionOrderConcurrency(&dbent.SubscriptionPlan{Concurrency: 4})
	require.NoError(t, err)
	require.Equal(t, 4, concurrency)
}

func TestPlanConcurrencyWindowStartsAfterExistingSubscriptionTerm(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	existingExpiresAt := now.Add(10 * 24 * time.Hour)
	newExpiresAt := existingExpiresAt.Add(30 * 24 * time.Hour)
	concurrency := 2

	startsAt, expiresAt, ok := planConcurrencyWindow(
		&UserSubscription{ExpiresAt: existingExpiresAt},
		&AssignSubscriptionInput{PlanConcurrency: &concurrency},
		now,
		newExpiresAt,
		nil,
		false,
	)

	require.True(t, ok)
	require.Equal(t, existingExpiresAt, startsAt)
	require.Equal(t, newExpiresAt, expiresAt)
}

func TestPlanConcurrencyWindowStartsImmediatelyForFirstCustomOverlay(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	baseExpiresAt := now.Add(90 * 24 * time.Hour)
	customExpiresAt := now.Add(30 * 24 * time.Hour)
	concurrency := 16
	multiplier := 4

	startsAt, expiresAt, ok := planConcurrencyWindow(
		&UserSubscription{ExpiresAt: baseExpiresAt},
		&AssignSubscriptionInput{PlanConcurrency: &concurrency, CustomMultiplier: &multiplier},
		now,
		baseExpiresAt,
		&customExpiresAt,
		false,
	)

	require.True(t, ok)
	require.Equal(t, now, startsAt)
	require.Equal(t, customExpiresAt, expiresAt)
}

func TestPlanConcurrencyWindowQueuesAfterActiveCustomTerm(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	baseExpiresAt := now.Add(90 * 24 * time.Hour)
	currentCustomExpiresAt := now.Add(10 * 24 * time.Hour)
	nextCustomExpiresAt := now.Add(40 * 24 * time.Hour)
	concurrency := 16
	multiplier := 4
	planID := int64(7)
	groupID := int64(9)

	startsAt, expiresAt, ok := planConcurrencyWindow(
		&UserSubscription{
			Status:              SubscriptionStatusActive,
			ExpiresAt:           baseExpiresAt,
			CustomMultiplier:    &multiplier,
			CustomSourcePlanID:  &planID,
			CustomSourceGroupID: &groupID,
			CustomExpiresAt:     &currentCustomExpiresAt,
		},
		&AssignSubscriptionInput{PlanConcurrency: &concurrency, CustomMultiplier: &multiplier},
		now,
		baseExpiresAt,
		&nextCustomExpiresAt,
		false,
	)

	require.True(t, ok)
	require.Equal(t, currentCustomExpiresAt, startsAt)
	require.Equal(t, nextCustomExpiresAt, expiresAt)
}

func TestRecordPlanConcurrencyEntitlementUpsertsBySourceOrderID(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("plan-concurrency-upsert@example.com").
		SetPasswordHash("hash").
		SetUsername("plan-concurrency-upsert").
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetName("plan-concurrency-upsert-group").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)
	now := time.Now()
	subscription, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now).
		SetExpiresAt(now.Add(60 * 24 * time.Hour)).
		Save(ctx)
	require.NoError(t, err)

	svc := &SubscriptionService{entClient: client}
	sourceOrderID := int64(991)
	initialConcurrency := 5
	initialStart := now
	initialExpiry := now.Add(30 * 24 * time.Hour)
	require.NoError(t, svc.recordPlanConcurrencyEntitlement(ctx, subscription.ID, &AssignSubscriptionInput{
		UserID:                  user.ID,
		PlanConcurrency:         &initialConcurrency,
		PlanConcurrencySourceID: &sourceOrderID,
	}, initialStart, initialExpiry))

	reconciledConcurrency := 16
	reconciledStart := now.Add(time.Minute)
	reconciledExpiry := now.Add(31 * 24 * time.Hour)
	require.NoError(t, svc.recordPlanConcurrencyEntitlement(ctx, subscription.ID, &AssignSubscriptionInput{
		UserID:                  user.ID,
		PlanConcurrency:         &reconciledConcurrency,
		PlanConcurrencySourceID: &sourceOrderID,
	}, reconciledStart, reconciledExpiry))

	entitlements, err := client.SubscriptionConcurrencyEntitlement.Query().All(ctx)
	require.NoError(t, err)
	require.Len(t, entitlements, 1)
	require.Equal(t, reconciledConcurrency, entitlements[0].Concurrency)
	require.WithinDuration(t, reconciledStart, entitlements[0].StartsAt, time.Second)
	require.WithinDuration(t, reconciledExpiry, entitlements[0].ExpiresAt, time.Second)
}

func TestExtendLatestPlanConcurrencyEntitlementOnlyExtendsLatestTerm(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("plan-concurrency-extension@example.com").
		SetPasswordHash("hash").
		SetUsername("plan-concurrency-extension").
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetName("plan-concurrency-extension-group").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)
	now := time.Now()
	subscription, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now.Add(-2 * time.Hour)).
		SetExpiresAt(now.Add(time.Hour)).
		Save(ctx)
	require.NoError(t, err)
	older, err := client.SubscriptionConcurrencyEntitlement.Create().
		SetUserID(user.ID).
		SetSubscriptionID(subscription.ID).
		SetConcurrency(16).
		SetStartsAt(now.Add(-2 * time.Hour)).
		SetExpiresAt(now.Add(time.Hour)).
		Save(ctx)
	require.NoError(t, err)
	latest, err := client.SubscriptionConcurrencyEntitlement.Create().
		SetUserID(user.ID).
		SetSubscriptionID(subscription.ID).
		SetConcurrency(8).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(time.Hour)).
		Save(ctx)
	require.NoError(t, err)

	newExpiresAt := now.Add(3 * time.Hour)
	svc := &SubscriptionService{entClient: client}
	require.NoError(t, svc.extendLatestPlanConcurrencyEntitlement(ctx, subscription.ID, newExpiresAt))

	older, err = client.SubscriptionConcurrencyEntitlement.Get(ctx, older.ID)
	require.NoError(t, err)
	latest, err = client.SubscriptionConcurrencyEntitlement.Get(ctx, latest.ID)
	require.NoError(t, err)
	require.WithinDuration(t, now.Add(time.Hour), older.ExpiresAt, time.Second)
	require.WithinDuration(t, newExpiresAt, latest.ExpiresAt, time.Second)
}

func TestSubscriptionOrderConcurrencyRejectsValuesOutsidePostgresInt(t *testing.T) {
	_, err := subscriptionOrderConcurrency(&dbent.SubscriptionPlan{Concurrency: maxPlanConcurrency + 1})
	require.Error(t, err)
}
