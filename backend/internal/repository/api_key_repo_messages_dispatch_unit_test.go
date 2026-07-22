package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupEntityToService_PreservesMessagesDispatchModelConfig(t *testing.T) {
	group := &dbent.Group{
		ID:                    1,
		Name:                  "openai-dispatch",
		Platform:              service.PlatformOpenAI,
		Status:                service.StatusActive,
		SubscriptionType:      service.SubscriptionTypeStandard,
		RateMultiplier:        1,
		AllowMessagesDispatch: true,
		DefaultMappedModel:    "gpt-5.4",
		MessagesDispatchModelConfig: service.OpenAIMessagesDispatchModelConfig{
			OpusMappedModel:   "gpt-5.4-nano",
			SonnetMappedModel: "gpt-5.3-codex",
			HaikuMappedModel:  "gpt-5.4-mini",
			ExactModelMappings: map[string]string{
				"claude-sonnet-4.5": "gpt-5.4-nano",
			},
		},
	}

	got := groupEntityToService(group)
	require.NotNil(t, got)
	require.Equal(t, group.MessagesDispatchModelConfig, got.MessagesDispatchModelConfig)
}

func TestAPIKeyRepository_GetByKeyForAuthLoadsActivePlanConcurrency_SQLite(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "getbykey-auth-plan-concurrency@test.com")
	_, err := client.User.UpdateOneID(user.ID).SetConcurrency(2).Save(ctx)
	require.NoError(t, err)

	group, err := client.Group.Create().
		SetName("g-auth-plan-concurrency").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		SetRateMultiplier(1).
		Save(ctx)
	require.NoError(t, err)

	now := time.Now()
	planConcurrency := 7
	_, err = client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(time.Hour)).
		SetStatus(service.SubscriptionStatusActive).
		SetPlanConcurrency(planConcurrency).
		SetPlanConcurrencyExpiresAt(now.Add(30 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)
	futureGroup, err := client.Group.Create().
		SetName("g-auth-future-plan-concurrency").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		SetRateMultiplier(1).
		Save(ctx)
	require.NoError(t, err)
	futureSub, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(futureGroup.ID).
		SetStartsAt(now.Add(time.Hour)).
		SetExpiresAt(now.Add(3 * time.Hour)).
		SetStatus(service.SubscriptionStatusActive).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.SubscriptionConcurrencyEntitlement.Create().
		SetUserID(user.ID).
		SetSubscriptionID(futureSub.ID).
		SetConcurrency(11).
		SetStartsAt(now.Add(time.Hour)).
		SetExpiresAt(now.Add(3 * time.Hour)).
		Save(ctx)
	require.NoError(t, err)

	key := &service.APIKey{UserID: user.ID, Key: "sk-getbykey-auth-plan-concurrency", Name: "Plan concurrency key", GroupID: &group.ID, Status: service.StatusActive}
	require.NoError(t, repo.Create(ctx, key))

	got, err := repo.GetByKeyForAuth(ctx, key.Key)
	require.NoError(t, err)
	require.NotNil(t, got.User)
	require.Equal(t, planConcurrency, got.User.EffectiveConcurrencyAt(now))
	require.Equal(t, 11, got.User.EffectiveConcurrencyAt(now.Add(2*time.Hour)))
	require.Len(t, got.User.PlanConcurrencyEntitlements, 2)
}

func TestAPIKeyRepository_GetByKeyForAuth_PreservesMessagesDispatchModelConfig_SQLite(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "getbykey-auth-dispatch-unit@test.com")

	group, err := client.Group.Create().
		SetName("g-auth-dispatch-unit").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		SetRateMultiplier(1).
		SetAllowMessagesDispatch(true).
		SetDefaultMappedModel("gpt-5.4").
		SetMessagesDispatchModelConfig(service.OpenAIMessagesDispatchModelConfig{
			OpusMappedModel:   "gpt-5.4-nano",
			SonnetMappedModel: "gpt-5.3-codex",
			HaikuMappedModel:  "gpt-5.4-mini",
			ExactModelMappings: map[string]string{
				"claude-sonnet-4.5": "gpt-5.4-nano",
			},
		}).
		Save(ctx)
	require.NoError(t, err)

	key := &service.APIKey{
		UserID:  user.ID,
		Key:     "sk-getbykey-auth-dispatch-unit",
		Name:    "Dispatch Key Unit",
		GroupID: &group.ID,
		Status:  service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))

	got, err := repo.GetByKeyForAuth(ctx, key.Key)
	require.NoError(t, err)
	require.Equal(t, key.Name, got.Name)
	require.NotNil(t, got.Group)
	require.Equal(t, group.MessagesDispatchModelConfig, got.Group.MessagesDispatchModelConfig)
}
