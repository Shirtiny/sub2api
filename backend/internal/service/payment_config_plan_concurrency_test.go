package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

type authCacheInvalidatorSpy struct {
	userIDs []int64
}

func (s *authCacheInvalidatorSpy) InvalidateAuthCacheByKey(context.Context, string) {}

func (s *authCacheInvalidatorSpy) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	s.userIDs = append(s.userIDs, userID)
}

func (s *authCacheInvalidatorSpy) InvalidateAuthCacheByGroupID(context.Context, int64) {}

func TestUpdatePlanRaisesActiveConcurrencyEntitlements(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	spy := &authCacheInvalidatorSpy{}
	svc := &PaymentConfigService{entClient: client, settingRepo: &paymentConfigSettingRepoStub{}}
	svc.SetAuthCacheInvalidator(spy)

	now := time.Now()
	group, err := client.Group.Create().
		SetName("plan-propagation-group").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("Latte").
		SetPrice(139).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetConcurrency(4).
		Save(ctx)
	require.NoError(t, err)

	// staleUser bought the plan at concurrency 4 and must be raised to 8.
	// boostedUser already sits above the new plan value and must be left alone.
	staleUser := createPlanPropagationUser(t, client, "stale@example.com")
	boostedUser := createPlanPropagationUser(t, client, "boosted@example.com")
	otherUser := createPlanPropagationUser(t, client, "other@example.com")

	staleSub := createPlanPropagationSubscription(t, client, staleUser.ID, group.ID, now)
	boostedSub := createPlanPropagationSubscription(t, client, boostedUser.ID, group.ID, now)

	staleEntitlement := createPlanPropagationEntitlement(t, client, staleUser.ID, staleSub.ID, 4, now.Add(-time.Hour), now.Add(24*time.Hour))
	boostedEntitlement := createPlanPropagationEntitlement(t, client, boostedUser.ID, boostedSub.ID, 16, now.Add(-time.Hour), now.Add(24*time.Hour))
	expiredEntitlement := createPlanPropagationEntitlement(t, client, staleUser.ID, staleSub.ID, 4, now.Add(-48*time.Hour), now.Add(-time.Hour))
	// An early renewal queues its term behind the current one; it must be raised
	// too, otherwise the user drops back to 4 when that term takes over.
	queuedEntitlement := createPlanPropagationEntitlement(t, client, staleUser.ID, staleSub.ID, 4, now.Add(24*time.Hour), now.Add(48*time.Hour))

	// A subscription in another group must not be touched by this plan's edit.
	otherGroup, err := client.Group.Create().
		SetName("other-group").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)
	otherSub := createPlanPropagationSubscription(t, client, otherUser.ID, otherGroup.ID, now)
	otherEntitlement := createPlanPropagationEntitlement(t, client, otherUser.ID, otherSub.ID, 4, now.Add(-time.Hour), now.Add(24*time.Hour))

	concurrency := 8
	updated, err := svc.UpdatePlan(ctx, plan.ID, UpdatePlanRequest{Concurrency: &concurrency})
	require.NoError(t, err)
	require.Equal(t, 8, updated.Concurrency)

	require.Equal(t, 8, client.SubscriptionConcurrencyEntitlement.GetX(ctx, staleEntitlement.ID).Concurrency)
	require.Equal(t, 8, client.SubscriptionConcurrencyEntitlement.GetX(ctx, queuedEntitlement.ID).Concurrency)
	require.Equal(t, 16, client.SubscriptionConcurrencyEntitlement.GetX(ctx, boostedEntitlement.ID).Concurrency)
	require.Equal(t, 4, client.SubscriptionConcurrencyEntitlement.GetX(ctx, expiredEntitlement.ID).Concurrency)
	require.Equal(t, 4, client.SubscriptionConcurrencyEntitlement.GetX(ctx, otherEntitlement.ID).Concurrency)

	require.Equal(t, []int64{staleUser.ID}, spy.userIDs)
}

func TestUpdatePlanWithoutConcurrencyChangeLeavesEntitlements(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	spy := &authCacheInvalidatorSpy{}
	svc := &PaymentConfigService{entClient: client, settingRepo: &paymentConfigSettingRepoStub{}}
	svc.SetAuthCacheInvalidator(spy)

	now := time.Now()
	group, err := client.Group.Create().
		SetName("plan-noop-group").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("Americano").
		SetPrice(99).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetConcurrency(4).
		Save(ctx)
	require.NoError(t, err)

	user := createPlanPropagationUser(t, client, "noop@example.com")
	sub := createPlanPropagationSubscription(t, client, user.ID, group.ID, now)
	entitlement := createPlanPropagationEntitlement(t, client, user.ID, sub.ID, 4, now.Add(-time.Hour), now.Add(24*time.Hour))

	// Editing the price, and re-submitting the same concurrency, must both be no-ops.
	price := 129.0
	same := 4
	_, err = svc.UpdatePlan(ctx, plan.ID, UpdatePlanRequest{Price: &price, Concurrency: &same})
	require.NoError(t, err)

	require.Equal(t, 4, client.SubscriptionConcurrencyEntitlement.GetX(ctx, entitlement.ID).Concurrency)
	require.Empty(t, spy.userIDs)
}

func createPlanPropagationUser(t *testing.T, client *dbent.Client, email string) *dbent.User {
	t.Helper()
	user, err := client.User.Create().
		SetEmail(email).
		SetUsername(email).
		SetPasswordHash("hash").
		SetStatus(StatusActive).
		Save(context.Background())
	require.NoError(t, err)
	return user
}

func createPlanPropagationSubscription(t *testing.T, client *dbent.Client, userID, groupID int64, now time.Time) *dbent.UserSubscription {
	t.Helper()
	sub, err := client.UserSubscription.Create().
		SetUserID(userID).
		SetGroupID(groupID).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(30 * 24 * time.Hour)).
		Save(context.Background())
	require.NoError(t, err)
	return sub
}

func createPlanPropagationEntitlement(
	t *testing.T,
	client *dbent.Client,
	userID, subscriptionID int64,
	concurrency int,
	startsAt, expiresAt time.Time,
) *dbent.SubscriptionConcurrencyEntitlement {
	t.Helper()
	entitlement, err := client.SubscriptionConcurrencyEntitlement.Create().
		SetUserID(userID).
		SetSubscriptionID(subscriptionID).
		SetConcurrency(concurrency).
		SetStartsAt(startsAt).
		SetExpiresAt(expiresAt).
		Save(context.Background())
	require.NoError(t, err)
	return entitlement
}
