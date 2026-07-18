//go:build unit

package repository

import (
	"context"
	"fmt"
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

func TestContentModerationHashCacheReservesAndFinalizesSideEffectsPerSubject(t *testing.T) {
	_, cache := newContentModerationHashCacheForTest(t)
	ctx := context.Background()
	const reservationTTL = time.Minute
	const retentionTTL = time.Hour

	reserved, err := cache.ReserveFlaggedInputSideEffect(ctx, "user:1001", "violation", "input-hash", "token-1", reservationTTL, retentionTTL)
	require.NoError(t, err)
	require.True(t, reserved)
	finalized, err := cache.FinalizeFlaggedInputSideEffect(ctx, "user:1001", "violation", "input-hash", "token-1", retentionTTL)
	require.NoError(t, err)
	require.True(t, finalized)

	reserved, err = cache.ReserveFlaggedInputSideEffect(ctx, "user:1001", "violation", "input-hash", "token-2", reservationTTL, retentionTTL)
	require.NoError(t, err)
	require.False(t, reserved, "one subject must not repeat finalized violation side effects")

	reserved, err = cache.ReserveFlaggedInputSideEffect(ctx, "user:1002", "violation", "input-hash", "token-3", reservationTTL, retentionTTL)
	require.NoError(t, err)
	require.True(t, reserved, "a global risk hash must not suppress a different subject")

	reserved, err = cache.ReserveFlaggedInputSideEffect(ctx, "user:1001", "violation", "other-input-hash", "token-4", reservationTTL, retentionTTL)
	require.NoError(t, err)
	require.True(t, reserved, "the same subject may own side effects for a different input")
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
	for index := range attempts {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			reserved, err := cache.ReserveFlaggedInputSideEffect(ctx, "user:1001", "violation", "input-hash", fmt.Sprintf("token-%d", index), time.Minute, time.Hour)
			results <- claimResult{claimed: reserved, err: err}
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

func TestContentModerationHashCacheSideEffectReservationOwnershipAndRelease(t *testing.T) {
	_, cache := newContentModerationHashCacheForTest(t)
	ctx := context.Background()

	reserved, err := cache.ReserveFlaggedInputSideEffect(ctx, "user:1001", "violation_email", "input-hash", "owner", time.Minute, time.Hour)
	require.NoError(t, err)
	require.True(t, reserved)

	finalized, err := cache.FinalizeFlaggedInputSideEffect(ctx, "user:1001", "violation_email", "input-hash", "other", time.Hour)
	require.NoError(t, err)
	require.False(t, finalized, "a different worker must not finalize the reservation")

	released, err := cache.ReleaseFlaggedInputSideEffect(ctx, "user:1001", "violation_email", "input-hash", "other")
	require.NoError(t, err)
	require.False(t, released, "a different worker must not release the reservation")

	released, err = cache.ReleaseFlaggedInputSideEffect(ctx, "user:1001", "violation_email", "input-hash", "owner")
	require.NoError(t, err)
	require.True(t, released)

	reserved, err = cache.ReserveFlaggedInputSideEffect(ctx, "user:1001", "violation_email", "input-hash", "retry", time.Minute, time.Hour)
	require.NoError(t, err)
	require.True(t, reserved, "releasing a failed effect must permit a retry")
}

func TestContentModerationHashCacheSideEffectTypesAreIndependent(t *testing.T) {
	_, cache := newContentModerationHashCacheForTest(t)
	ctx := context.Background()

	for _, effectType := range []string{"violation", "violation_email", "disabled_email"} {
		reserved, err := cache.ReserveFlaggedInputSideEffect(ctx, "user:1001", effectType, "input-hash", effectType+"-token", time.Minute, time.Hour)
		require.NoError(t, err)
		require.True(t, reserved, effectType)
	}
}

func TestContentModerationHashCacheDoesNotShortenExistingSideEffectRetention(t *testing.T) {
	mr, cache := newContentModerationHashCacheForTest(t)
	ctx := context.Background()
	const longRetention = 48 * time.Hour

	reserved, err := cache.ReserveFlaggedInputSideEffect(ctx, "user:1001", "violation", "input-hash", "token-1", time.Minute, longRetention)
	require.NoError(t, err)
	require.True(t, reserved)
	finalized, err := cache.FinalizeFlaggedInputSideEffect(ctx, "user:1001", "violation", "input-hash", "token-1", longRetention)
	require.NoError(t, err)
	require.True(t, finalized)

	reserved, err = cache.ReserveFlaggedInputSideEffect(ctx, "user:1002", "violation", "input-hash", "token-2", time.Minute, time.Hour)
	require.NoError(t, err)
	require.True(t, reserved)
	require.Greater(t, mr.TTL(contentModerationSideEffectKey("input-hash")), 47*time.Hour)
}

func TestContentModerationHashCacheExpiredSideEffectClaimCanBeReclaimed(t *testing.T) {
	_, cache := newContentModerationHashCacheForTest(t)
	ctx := context.Background()
	key := contentModerationSideEffectKey("input-hash")
	field := contentModerationSideEffectField("user:1001", "violation")
	require.NoError(t, cache.rdb.HSet(ctx, key, field, contentModerationSideEffectReservationValue("expired-token", time.Now().Add(-time.Hour))).Err())

	reserved, err := cache.ReserveFlaggedInputSideEffect(ctx, "user:1001", "violation", "input-hash", "fresh-token", time.Minute, time.Hour)
	require.NoError(t, err)
	require.True(t, reserved)

	value, err := cache.rdb.HGet(ctx, key, field).Result()
	require.NoError(t, err)
	require.Contains(t, value, "r:fresh-token:")
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
	reserved, err := cache.ReserveFlaggedInputSideEffect(ctx, "user:1001", "violation", "abc", "token-1", time.Minute, time.Hour)
	require.NoError(t, err)
	require.True(t, reserved)
	finalized, err := cache.FinalizeFlaggedInputSideEffect(ctx, "user:1001", "violation", "abc", "token-1", time.Hour)
	require.NoError(t, err)
	require.True(t, finalized)
	deleted, err := cache.DeleteFlaggedInputHash(ctx, "abc")
	require.NoError(t, err)
	require.True(t, deleted)
	reserved, err = cache.ReserveFlaggedInputSideEffect(ctx, "user:1001", "violation", "abc", "token-2", time.Minute, time.Hour)
	require.NoError(t, err)
	require.True(t, reserved, "deleting a false-positive hash must also reset its subject claims")

	deleted, err = cache.DeleteFlaggedInputHash(ctx, "abc")
	require.NoError(t, err)
	require.False(t, deleted, "deleting a missing hash reports no deletion")

	require.NoError(t, cache.RecordFlaggedInputHash(ctx, "one", time.Hour))
	require.NoError(t, cache.RecordFlaggedInputHash(ctx, "two", time.Hour))
	reserved, err = cache.ReserveFlaggedInputSideEffect(ctx, "user:1001", "violation", "one", "token-3", time.Minute, time.Hour)
	require.NoError(t, err)
	require.True(t, reserved)
	finalized, err = cache.FinalizeFlaggedInputSideEffect(ctx, "user:1001", "violation", "one", "token-3", time.Hour)
	require.NoError(t, err)
	require.True(t, finalized)
	require.NoError(t, mr.Set(contentModerationLegacyFlaggedHashKey, "legacy"))
	require.NoError(t, mr.Set(contentModerationLegacySideEffectDedupKey, "legacy-side-effect"))

	cleared, err := cache.ClearFlaggedInputHashes(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), cleared)
	require.False(t, mr.Exists(contentModerationFlaggedHashKey))
	require.False(t, mr.Exists(contentModerationSideEffectKey("one")))
	require.False(t, mr.Exists(contentModerationLegacySideEffectDedupKey))
	require.False(t, mr.Exists(contentModerationLegacyFlaggedHashKey), "clearing also drops the pre-TTL key")
}
