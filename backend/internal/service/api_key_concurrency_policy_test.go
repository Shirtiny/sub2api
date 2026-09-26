//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyConcurrencyUsesLiveBalanceOnAuthCacheHits(t *testing.T) {
	ctx := context.Background()
	entry := &APIKeyAuthCacheEntry{Snapshot: &APIKeyAuthSnapshot{
		Version: apiKeyAuthSnapshotVersion, APIKeyID: 1, UserID: 2, Status: StatusActive,
		User: APIKeyAuthUserSnapshot{ID: 2, Status: StatusActive, Balance: 999, Concurrency: 32},
	}}
	cache := &authCacheStub{getAuthCache: func(context.Context, string) (*APIKeyAuthCacheEntry, error) { return entry, nil }}
	svc := NewAPIKeyService(&authRepoStub{}, nil, nil, nil, nil, cache, &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{L2TTLSeconds: 60},
	})
	reader := &wsCacheOnlyBillingCache{}
	svc.SetUserBalanceReader(reader)
	svc.concurrencySettings = NewSettingService(&concurrencyRulesRepo{}, nil)
	for _, balance := range []float64{100, 20, 19, 100} {
		reader.balance = balance
		key, err := svc.GetByKey(ctx, "test-concurrency")
		require.NoError(t, err)
		require.Equal(t, BalanceConcurrency(balance), key.User.EffectiveConcurrencyAt(time.Now()))
		require.Equal(t, float64(999), entry.Snapshot.User.Balance, "immutable auth cache")
	}
	require.NoError(t, svc.concurrencySettings.SetUserConcurrencyRules(ctx, UserConcurrencyRules{[]BalanceConcurrencyRule{{0, 4}, {100, 7}}}))
	key, err := svc.GetByKey(ctx, "test-concurrency")
	require.NoError(t, err)
	require.Equal(t, 7, key.User.EffectiveConcurrencyAt(time.Now()), "auth cache hits use the current configured policy")
	reader.balanceErr = errors.New("cache and database unavailable")
	_, err = svc.GetByKey(ctx, "test-concurrency")
	require.ErrorContains(t, err, "resolve concurrency balance")

	now := time.Now()
	entry.Snapshot.User.SubscriptionPeriods = []SubscriptionPeriod{{StartsAt: now, ExpiresAt: now.Add(time.Hour)}}
	key, err = svc.GetByKey(ctx, "test-concurrency")
	require.NoError(t, err)
	require.Equal(t, 2, key.User.EffectiveConcurrencyAt(now), "active subscriptions skip balance tiers")
	require.Equal(t, entry.Snapshot.User.SubscriptionPeriods, key.User.SubscriptionPeriods)

	// An active grant must also survive a failure of the balance-policy store,
	// not only a balance-cache failure. Keep the auth snapshot immutable.
	svc.concurrencySettings = NewSettingService(&concurrencyRulesRepo{getErr: errors.New("settings unavailable")}, nil)
	entry.Snapshot.User.PlanConcurrencyEntitlements = []PlanConcurrencyEntitlement{{
		Concurrency: 4, StartsAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour),
	}}
	key, err = svc.GetByKey(ctx, "test-concurrency")
	require.NoError(t, err)
	require.Equal(t, 4, key.User.EffectiveConcurrencyAt(now))
	require.Nil(t, svc.concurrencySettings.userConcurrencyRulesCache.Load(), "subscription admission must not query balance rules")
	require.Equal(t, float64(999), entry.Snapshot.User.Balance)

	// After subscription expiry, neither the grant nor the former balance rules
	// can silently remain in force. Failed refreshes reject balance admission.
	entry.Snapshot.User.PlanConcurrencyEntitlements[0].ExpiresAt = now.Add(-time.Second)
	entry.Snapshot.User.SubscriptionPeriods[0].ExpiresAt = now.Add(-time.Second)
	_, err = svc.GetByKey(ctx, "test-concurrency")
	require.ErrorContains(t, err, "load concurrency rules")
	reader.balanceErr = nil
	reader.balance = 25
	require.NoError(t, svc.concurrencySettings.SetUserConcurrencyRules(ctx, UserConcurrencyRules{[]BalanceConcurrencyRule{{0, 1}, {20, 6}}}))
	key, err = svc.GetByKey(ctx, "test-concurrency")
	require.NoError(t, err)
	require.Equal(t, 6, key.User.EffectiveConcurrencyAt(time.Now()), "expiry returns to the current policy and live balance")
}

func TestAPIKeyConcurrencyColdSubscriptionSkipsBalanceRules(t *testing.T) {
	now := time.Now()
	for _, legacy := range []bool{false, true} {
		name, want := "configured", 4
		if legacy {
			name, want = "legacy", 2
		}
		t.Run(name, func(t *testing.T) {
			user := &User{ID: 2, Status: StatusActive, Balance: 1000,
				SubscriptionPeriods: []SubscriptionPeriod{{StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)}},
			}
			if !legacy {
				user.PlanConcurrencyEntitlements = []PlanConcurrencyEntitlement{{Concurrency: 4, StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)}}
			}
			repo := &authRepoStub{getByKeyForAuth: func(context.Context, string) (*APIKey, error) {
				return &APIKey{ID: 1, UserID: user.ID, Status: StatusActive, User: user}, nil
			}}
			svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})
			svc.concurrencySettings = NewSettingService(&concurrencyRulesRepo{getErr: errors.New("settings unavailable")}, nil)
			svc.SetUserBalanceReader(&wsCacheOnlyBillingCache{balanceErr: errors.New("balance unavailable")})
			key, err := svc.GetByKey(context.Background(), "cold-subscription-key")
			require.NoError(t, err)
			require.Equal(t, want, key.User.EffectiveConcurrencyAt(now))
			require.Nil(t, svc.concurrencySettings.userConcurrencyRulesCache.Load())
			require.Equal(t, float64(1000), user.Balance)
		})
	}
}
