package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserRepositoryLoadsEffectivePlanConcurrencyForDetailAndList(t *testing.T) {
	repo, client := newUserEntRepo(t)
	ctx := context.Background()
	user := &service.User{
		Email:        "plan-concurrency-user@example.com",
		Username:     "plan-concurrency-user",
		PasswordHash: "hash",
		Role:         service.RoleAdmin,
		Status:       service.StatusActive,
		Concurrency:  5,
	}
	require.NoError(t, repo.Create(ctx, user))

	group, err := client.Group.Create().
		SetName("plan-concurrency-group").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)
	now := time.Now()
	subscriptionExpiresAt := now.Add(time.Hour)
	subscription, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStatus(service.SubscriptionStatusActive).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(subscriptionExpiresAt).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.SubscriptionConcurrencyEntitlement.Create().
		SetUserID(user.ID).
		SetSubscriptionID(subscription.ID).
		SetConcurrency(16).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(2 * time.Hour)).
		Save(ctx)
	require.NoError(t, err)

	detail, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, 5, detail.Concurrency)
	require.Equal(t, 16, detail.EffectiveConcurrencyAt(now))
	require.Len(t, detail.PlanConcurrencyEntitlements, 1)
	require.Equal(t, subscription.ID, detail.PlanConcurrencyEntitlements[0].SubscriptionID)
	require.WithinDuration(t, subscriptionExpiresAt, detail.PlanConcurrencyEntitlements[0].ExpiresAt, time.Second)

	firstAdmin, err := repo.GetFirstAdmin(ctx)
	require.NoError(t, err)
	require.Equal(t, 16, firstAdmin.EffectiveConcurrencyAt(now))

	customExpiresAt := now.Add(45 * time.Minute)
	_, err = client.UserSubscription.UpdateOneID(subscription.ID).
		SetCustomExpiresAt(customExpiresAt).
		Save(ctx)
	require.NoError(t, err)
	detail, err = repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.WithinDuration(t, customExpiresAt, detail.PlanConcurrencyEntitlements[0].ExpiresAt, time.Second)

	_, err = client.SubscriptionConcurrencyEntitlement.Create().
		SetUserID(user.ID).
		SetSubscriptionID(subscription.ID).
		SetConcurrency(8).
		SetStartsAt(now.Add(-30 * time.Minute)).
		SetExpiresAt(now.Add(2 * time.Hour)).
		Save(ctx)
	require.NoError(t, err)

	users, _, err := repo.List(ctx, pagination.PaginationParams{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, users, 1)
	require.Equal(t, 8, users[0].EffectiveConcurrencyAt(now))

	baseOnlyUser := &service.User{
		Email:        "base-concurrency-user@example.com",
		Username:     "base-concurrency-user",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Concurrency:  10,
	}
	require.NoError(t, repo.Create(ctx, baseOnlyUser))

	ascending, _, err := repo.List(ctx, pagination.PaginationParams{Page: 1, PageSize: 10, SortBy: "concurrency", SortOrder: "asc"})
	require.NoError(t, err)
	require.Len(t, ascending, 2)
	require.Equal(t, user.ID, ascending[0].ID)
	require.Equal(t, baseOnlyUser.ID, ascending[1].ID)

	descending, _, err := repo.List(ctx, pagination.PaginationParams{Page: 1, PageSize: 10, SortBy: "concurrency", SortOrder: "desc"})
	require.NoError(t, err)
	require.Len(t, descending, 2)
	require.Equal(t, baseOnlyUser.ID, descending[0].ID)
	require.Equal(t, user.ID, descending[1].ID)

	// Base concurrency above the active plan entitlement must drive both the
	// effective value and the sort order.
	_, err = client.User.UpdateOneID(user.ID).SetConcurrency(32).Save(ctx)
	require.NoError(t, err)

	boosted, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, 32, boosted.EffectiveConcurrencyAt(now))

	descending, _, err = repo.List(ctx, pagination.PaginationParams{Page: 1, PageSize: 10, SortBy: "concurrency", SortOrder: "desc"})
	require.NoError(t, err)
	require.Len(t, descending, 2)
	require.Equal(t, user.ID, descending[0].ID)
	require.Equal(t, baseOnlyUser.ID, descending[1].ID)
}
