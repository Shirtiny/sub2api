//go:build unit

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newContentModerationHashCacheForTest(t *testing.T) (*miniredis.Miniredis, *contentModerationHashCache) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return mr, &contentModerationHashCache{rdb: rdb}
}

func TestContentModerationHashCacheRecordAndHas(t *testing.T) {
	_, cache := newContentModerationHashCacheForTest(t)
	ctx := context.Background()

	has, err := cache.HasFlaggedInputHash(ctx, "abc")
	require.NoError(t, err)
	require.False(t, has, "an unrecorded hash must not match")

	require.NoError(t, cache.RecordFlaggedInputHash(ctx, "abc", time.Hour))
	has, err = cache.HasFlaggedInputHash(ctx, "abc")
	require.NoError(t, err)
	require.True(t, has)

	count, err := cache.CountFlaggedInputHashes(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

func TestContentModerationHashCacheClaimsSideEffectsPerSubject(t *testing.T) {
	_, cache := newContentModerationHashCacheForTest(t)
	ctx := context.Background()

	claimed, err := cache.ClaimFlaggedInputSideEffects(ctx, "user:1001", "input-hash", time.Hour)
	require.NoError(t, err)
	require.True(t, claimed)

	claimed, err = cache.ClaimFlaggedInputSideEffects(ctx, "user:1001", "input-hash", time.Hour)
	require.NoError(t, err)
	require.False(t, claimed, "one subject must not repeat notification or violation side effects")

	claimed, err = cache.ClaimFlaggedInputSideEffects(ctx, "user:1002", "input-hash", time.Hour)
	require.NoError(t, err)
	require.True(t, claimed, "a global risk hash must not suppress a different subject")

	claimed, err = cache.ClaimFlaggedInputSideEffects(ctx, "user:1001", "other-input-hash", time.Hour)
	require.NoError(t, err)
	require.True(t, claimed, "the same subject may own side effects for a different input")
}

func TestContentModerationHashCacheSideEffectClaimIsAtomic(t *testing.T) {
	_, cache := newContentModerationHashCacheForTest(t)
	ctx := context.Background()

	const attempts = 32
	type claimResult struct {
		claimed bool
		err     error
	}
	results := make(chan claimResult, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := cache.ClaimFlaggedInputSideEffects(ctx, "user:1001", "input-hash", time.Hour)
			results <- claimResult{claimed: claimed, err: err}
		}()
	}
	wg.Wait()
	close(results)

	claimedCount := 0
	for result := range results {
		require.NoError(t, result.err)
		if result.claimed {
			claimedCount++
		}
	}
	require.Equal(t, 1, claimedCount)
}

func TestContentModerationHashCacheExpiredSideEffectClaimCanBeReclaimed(t *testing.T) {
	_, cache := newContentModerationHashCacheForTest(t)
	ctx := context.Background()
	member := contentModerationSideEffectMember("user:1001", "input-hash")
	require.NoError(t, cache.rdb.ZAdd(ctx, contentModerationSideEffectDedupKey, redis.Z{
		Score:  float64(time.Now().Add(-time.Hour).Unix()),
		Member: member,
	}).Err())

	claimed, err := cache.ClaimFlaggedInputSideEffects(ctx, "user:1001", "input-hash", time.Hour)
	require.NoError(t, err)
	require.True(t, claimed)

	expiresAt, err := cache.rdb.ZScore(ctx, contentModerationSideEffectDedupKey, member).Result()
	require.NoError(t, err)
	require.Greater(t, int64(expiresAt), time.Now().Unix())
}

// expireAt back-dates a member's score. Entries expire against the wall clock in
// the score, so this is how an aged-out entry is reproduced without a fake clock.
func expireAt(t *testing.T, cache *contentModerationHashCache, member string, at time.Time) {
	t.Helper()
	require.NoError(t, cache.rdb.ZAdd(context.Background(), contentModerationFlaggedHashKey, redis.Z{
		Score:  float64(at.Unix()),
		Member: member,
	}).Err())
}

func TestContentModerationHashCacheExpiresEntries(t *testing.T) {
	_, cache := newContentModerationHashCacheForTest(t)
	ctx := context.Background()

	expireAt(t, cache, "stale", time.Now().Add(-time.Hour))
	require.NoError(t, cache.RecordFlaggedInputHash(ctx, "fresh", 72*time.Hour))

	has, err := cache.HasFlaggedInputHash(ctx, "stale")
	require.NoError(t, err)
	require.False(t, has, "an entry past its TTL must not match")

	has, err = cache.HasFlaggedInputHash(ctx, "fresh")
	require.NoError(t, err)
	require.True(t, has)

	count, err := cache.CountFlaggedInputHashes(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), count, "expired entries must not be counted")
}

func TestContentModerationHashCachePrunesExpiredOnWrite(t *testing.T) {
	_, cache := newContentModerationHashCacheForTest(t)
	ctx := context.Background()

	expireAt(t, cache, "stale", time.Now().Add(-time.Hour))
	require.NoError(t, cache.RecordFlaggedInputHash(ctx, "fresh", time.Hour))

	// The set must not grow without bound: writing prunes what has expired.
	members, err := cache.rdb.ZCard(ctx, contentModerationFlaggedHashKey).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), members)
}

func TestContentModerationHashCacheRecordRejectsNonPositiveTTL(t *testing.T) {
	_, cache := newContentModerationHashCacheForTest(t)
	ctx := context.Background()

	require.NoError(t, cache.RecordFlaggedInputHash(ctx, "abc", 0))
	has, err := cache.HasFlaggedInputHash(ctx, "abc")
	require.NoError(t, err)
	require.False(t, has, "a non-positive TTL must not record an immediately-expired entry")
}

func TestContentModerationHashCacheDeleteAndClear(t *testing.T) {
	mr, cache := newContentModerationHashCacheForTest(t)
	ctx := context.Background()

	require.NoError(t, cache.RecordFlaggedInputHash(ctx, "abc", time.Hour))
	claimed, err := cache.ClaimFlaggedInputSideEffects(ctx, "user:1001", "abc", time.Hour)
	require.NoError(t, err)
	require.True(t, claimed)
	deleted, err := cache.DeleteFlaggedInputHash(ctx, "abc")
	require.NoError(t, err)
	require.True(t, deleted)
	claimed, err = cache.ClaimFlaggedInputSideEffects(ctx, "user:1001", "abc", time.Hour)
	require.NoError(t, err)
	require.True(t, claimed, "deleting a false-positive hash must also reset its subject claims")

	deleted, err = cache.DeleteFlaggedInputHash(ctx, "abc")
	require.NoError(t, err)
	require.False(t, deleted, "deleting a missing hash reports no deletion")

	require.NoError(t, cache.RecordFlaggedInputHash(ctx, "one", time.Hour))
	require.NoError(t, cache.RecordFlaggedInputHash(ctx, "two", time.Hour))
	claimed, err = cache.ClaimFlaggedInputSideEffects(ctx, "user:1001", "one", time.Hour)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, mr.Set(contentModerationLegacyFlaggedHashKey, "legacy"))

	cleared, err := cache.ClearFlaggedInputHashes(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), cleared)
	require.False(t, mr.Exists(contentModerationFlaggedHashKey))
	require.False(t, mr.Exists(contentModerationSideEffectDedupKey))
	require.False(t, mr.Exists(contentModerationLegacyFlaggedHashKey), "clearing also drops the pre-TTL key")
}
