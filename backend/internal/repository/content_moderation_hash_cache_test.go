//go:build unit

package repository

import (
	"context"
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
	deleted, err := cache.DeleteFlaggedInputHash(ctx, "abc")
	require.NoError(t, err)
	require.True(t, deleted)

	deleted, err = cache.DeleteFlaggedInputHash(ctx, "abc")
	require.NoError(t, err)
	require.False(t, deleted, "deleting a missing hash reports no deletion")

	require.NoError(t, cache.RecordFlaggedInputHash(ctx, "one", time.Hour))
	require.NoError(t, cache.RecordFlaggedInputHash(ctx, "two", time.Hour))
	require.NoError(t, mr.Set(contentModerationLegacyFlaggedHashKey, "legacy"))

	cleared, err := cache.ClearFlaggedInputHashes(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), cleared)
	require.False(t, mr.Exists(contentModerationFlaggedHashKey))
	require.False(t, mr.Exists(contentModerationLegacyFlaggedHashKey), "clearing also drops the pre-TTL key")
}
