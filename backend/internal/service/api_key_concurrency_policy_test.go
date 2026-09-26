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
	for _, balance := range []float64{100, 20, 19, 100} {
		reader.balance = balance
		key, err := svc.GetByKey(ctx, "test-concurrency")
		require.NoError(t, err)
		require.Equal(t, BalanceConcurrency(balance), key.User.EffectiveConcurrencyAt(time.Now()))
		require.Equal(t, float64(999), entry.Snapshot.User.Balance, "immutable auth cache")
	}
	reader.balanceErr = errors.New("cache and database unavailable")
	_, err := svc.GetByKey(ctx, "test-concurrency")
	require.ErrorContains(t, err, "resolve concurrency balance")

	now := time.Now()
	entry.Snapshot.User.SubscriptionPeriods = []SubscriptionPeriod{{StartsAt: now, ExpiresAt: now.Add(time.Hour)}}
	key, err := svc.GetByKey(ctx, "test-concurrency")
	require.NoError(t, err)
	require.Equal(t, 2, key.User.EffectiveConcurrencyAt(now), "active subscriptions skip balance tiers")
	require.Equal(t, entry.Snapshot.User.SubscriptionPeriods, key.User.SubscriptionPeriods)
}
