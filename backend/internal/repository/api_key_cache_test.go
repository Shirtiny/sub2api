//go:build unit

package repository

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestApiKeyRateLimitKey(t *testing.T) {
	tests := []struct {
		name     string
		userID   int64
		expected string
	}{
		{
			name:     "normal_user_id",
			userID:   123,
			expected: "apikey:ratelimit:123",
		},
		{
			name:     "zero_user_id",
			userID:   0,
			expected: "apikey:ratelimit:0",
		},
		{
			name:     "negative_user_id",
			userID:   -1,
			expected: "apikey:ratelimit:-1",
		},
		{
			name:     "max_int64",
			userID:   math.MaxInt64,
			expected: "apikey:ratelimit:9223372036854775807",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := apiKeyRateLimitKey(tc.userID)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestAPIKeyAuthEpochIncrementSetsAndRenewsTTLAtomically(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })
	cache := &apiKeyCache{rdb: rdb}
	ctx := context.Background()
	cacheKey := "v2:lookup_sha256:test"
	redisKey := apiKeyAuthEpochPrefix + cacheKey
	require.Greater(t, apiKeyAuthEpochTTL, time.Hour, "epoch TTL must exceed the maximum WS idle window")

	require.NoError(t, cache.IncrementAuthCacheEpoch(ctx, cacheKey))
	first, err := cache.GetAuthCacheEpoch(ctx, cacheKey)
	require.NoError(t, err)
	require.NotZero(t, first)
	require.Equal(t, apiKeyAuthEpochTTL, mini.TTL(redisKey))

	mini.FastForward(12 * time.Hour)
	require.NoError(t, cache.IncrementAuthCacheEpoch(ctx, cacheKey))
	second, err := cache.GetAuthCacheEpoch(ctx, cacheKey)
	require.NoError(t, err)
	require.Greater(t, second, first)
	require.Equal(t, apiKeyAuthEpochTTL, mini.TTL(redisKey), "each increment must renew the bounded epoch lifetime")
}

func TestAPIKeyAuthEpochExpiryAllocatesNewGenerationWithoutABA(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })
	cache := &apiKeyCache{rdb: rdb}
	ctx := context.Background()
	cacheKey := "v2:lookup_sha256:aba"

	initial, err := cache.GetAuthCacheEpoch(ctx, cacheKey)
	require.NoError(t, err)
	require.NotZero(t, initial, "a missing generation must be initialized, never represented as zero")
	require.NoError(t, cache.IncrementAuthCacheEpoch(ctx, cacheKey))
	invalidated, err := cache.GetAuthCacheEpoch(ctx, cacheKey)
	require.NoError(t, err)
	require.Greater(t, invalidated, initial)

	mini.FastForward(apiKeyAuthEpochTTL)
	require.False(t, mini.Exists(apiKeyAuthEpochPrefix+cacheKey))
	require.True(t, mini.Exists(apiKeyAuthEpochSequenceKey), "only the singleton sequence remains permanent")
	reinitialized, err := cache.GetAuthCacheEpoch(ctx, cacheKey)
	require.NoError(t, err)
	require.Greater(t, reinitialized, invalidated, "TTL expiry must not let a stale lease observe an old generation again")

	mini.Del(apiKeyAuthEpochPrefix + cacheKey)
	mini.Del(apiKeyAuthEpochSequenceKey)
	mini.FastForward(time.Second)
	afterRestart, err := cache.GetAuthCacheEpoch(ctx, cacheKey)
	require.NoError(t, err)
	require.Greater(t, afterRestart, reinitialized, "sequence recreation must use Redis time and avoid generation reuse after state loss")
}
