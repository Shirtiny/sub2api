package repository

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCacheOpenAIWSMigrationAtomicAdmissionAndIdempotency(t *testing.T) {
	cache, rdb := newOpenAIWSMigrationTestCache(t)
	ctx := context.Background()
	const groupID = int64(7)
	const sessionHash = "session-hash"
	now := time.Now().UnixMilli()

	require.NoError(t, cache.SetSessionAccountID(ctx, groupID, sessionHash, 101, time.Minute))
	first, err := cache.AdmitOpenAIWSReconnectMigration(
		ctx, groupID, sessionHash, 101, "control-1", 1, service.OpenAIWSMiddleRouteDispositionExclude, now, 10*time.Minute, 30*time.Second, 2,
	)
	require.NoError(t, err)
	require.True(t, first.Admitted)
	require.False(t, first.Idempotent)
	require.Equal(t, 1, first.State.MigrationCount)
	require.Equal(t, uint64(2), first.State.BindingGeneration)
	_, err = rdb.Get(ctx, buildSessionKey(groupID, sessionHash)).Result()
	require.ErrorIs(t, err, redis.Nil, "sticky deletion must commit in the admission transaction")

	duplicate, err := cache.AdmitOpenAIWSReconnectMigration(
		ctx, groupID, sessionHash, 101, "control-1", 1, service.OpenAIWSMiddleRouteDispositionExclude, now+1, 10*time.Minute, 30*time.Second, 2,
	)
	require.NoError(t, err)
	require.True(t, duplicate.Admitted)
	require.True(t, duplicate.Idempotent)
	require.Equal(t, 1, duplicate.State.MigrationCount)
	require.Equal(t, uint64(2), duplicate.State.BindingGeneration)
	require.Equal(t, first.State.ExcludedUntilUnixMilli, duplicate.State.ExcludedUntilUnixMilli)

	_, err = cache.AdmitOpenAIWSReconnectMigration(
		ctx, groupID, sessionHash, 102, "control-1", 1, service.OpenAIWSMiddleRouteDispositionExclude, now+2, 10*time.Minute, 30*time.Second, 2,
	)
	require.ErrorContains(t, err, "control id was reused")

	second, err := cache.AdmitOpenAIWSReconnectMigration(
		ctx, groupID, sessionHash, 102, "control-2", 2, service.OpenAIWSMiddleRouteDispositionExclude, now+3, 10*time.Minute, 30*time.Second, 2,
	)
	require.NoError(t, err)
	require.True(t, second.Admitted)
	require.Equal(t, 2, second.State.MigrationCount)
	require.Equal(t, uint64(3), second.State.BindingGeneration)

	require.NoError(t, cache.SetSessionAccountID(ctx, groupID, sessionHash, 103, time.Minute))
	exhausted, err := cache.AdmitOpenAIWSReconnectMigration(
		ctx, groupID, sessionHash, 103, "control-3", 3, service.OpenAIWSMiddleRouteDispositionExclude, now+4, 10*time.Minute, 30*time.Second, 2,
	)
	require.NoError(t, err)
	require.False(t, exhausted.Admitted)
	require.True(t, exhausted.Exhausted)
	require.Equal(t, 2, exhausted.State.MigrationCount)
	require.Equal(t, int64(103), mustOpenAIWSMigrationStickyAccount(t, ctx, rdb, groupID, sessionHash), "denied admission must not mutate sticky state")

	loaded, err := cache.LoadOpenAIWSMigrationState(ctx, groupID, sessionHash)
	require.NoError(t, err)
	require.Equal(t, 2, loaded.MigrationCount)
	require.Equal(t, uint64(3), loaded.BindingGeneration)
	require.Len(t, loaded.ExcludedUntilUnixMilli, 2)
}

func TestGatewayCacheOpenAIWSMigrationRetainsMiddleRouteAtomically(t *testing.T) {
	cache, rdb := newOpenAIWSMigrationTestCache(t)
	ctx := context.Background()
	const groupID = int64(7)
	const sessionHash = "retain-middle-route"
	const accountID = int64(101)

	require.NoError(t, cache.SetSessionAccountID(ctx, groupID, sessionHash, accountID, time.Minute))
	decision, err := cache.AdmitOpenAIWSReconnectMigration(
		ctx, groupID, sessionHash, accountID, "retain-control", 1,
		service.OpenAIWSMiddleRouteDispositionRetain, time.Now().UnixMilli(),
		10*time.Minute, 30*time.Second, 3,
	)
	require.NoError(t, err)
	require.True(t, decision.Admitted)
	require.Equal(t, uint64(2), decision.State.BindingGeneration)
	require.Empty(t, decision.State.ExcludedUntilUnixMilli)
	require.Equal(t, accountID, mustOpenAIWSMigrationStickyAccount(t, ctx, rdb, groupID, sessionHash))

	loaded, err := cache.LoadOpenAIWSMigrationState(ctx, groupID, sessionHash)
	require.NoError(t, err)
	require.Empty(t, loaded.ExcludedUntilUnixMilli)

	_, err = cache.AdmitOpenAIWSReconnectMigration(
		ctx, groupID, sessionHash, accountID, "retain-control", 1,
		service.OpenAIWSMiddleRouteDispositionExclude, time.Now().UnixMilli(),
		10*time.Minute, 30*time.Second, 3,
	)
	require.ErrorContains(t, err, "control id was reused")
}

func TestGatewayCacheOpenAIWSMigrationReplayKeepsOriginalExclusionDeadline(t *testing.T) {
	cache, _ := newOpenAIWSMigrationTestCache(t)
	ctx := context.Background()
	const groupID = int64(7)
	const sessionHash = "stable-exclusion-deadline"
	const accountID = int64(101)

	first, err := cache.AdmitOpenAIWSReconnectMigration(
		ctx, groupID, sessionHash, accountID, "control-1", 1,
		service.OpenAIWSMiddleRouteDispositionExclude, time.Now().UnixMilli(),
		10*time.Minute, 30*time.Second, 3,
	)
	require.NoError(t, err)
	firstDeadline := first.State.ExcludedUntilUnixMilli[accountID]
	require.Positive(t, firstDeadline)

	second, err := cache.AdmitOpenAIWSReconnectMigration(
		ctx, groupID, sessionHash, accountID, "control-2", 2,
		service.OpenAIWSMiddleRouteDispositionExclude, time.Now().UnixMilli(),
		10*time.Minute, 90*time.Second, 3,
	)
	require.NoError(t, err)
	require.NotEqual(t, firstDeadline, second.State.ExcludedUntilUnixMilli[accountID])

	replayed, err := cache.AdmitOpenAIWSReconnectMigration(
		ctx, groupID, sessionHash, accountID, "control-1", 1,
		service.OpenAIWSMiddleRouteDispositionExclude, time.Now().UnixMilli(),
		10*time.Minute, 5*time.Minute, 3,
	)
	require.NoError(t, err)
	require.True(t, replayed.Idempotent)
	require.Equal(t, firstDeadline, replayed.State.ExcludedUntilUnixMilli[accountID])
}

func TestGatewayCacheOpenAIWSMigrationConcurrentSameControlCountsOnce(t *testing.T) {
	cache, _ := newOpenAIWSMigrationTestCache(t)
	ctx := context.Background()
	const goroutines = 24
	start := make(chan struct{})
	var wg sync.WaitGroup
	var admitted atomic.Int64
	var idempotent atomic.Int64
	errs := make(chan error, goroutines)

	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			decision, err := cache.AdmitOpenAIWSReconnectMigration(
				ctx, 7, "concurrent-same", 101, "same-control", 1,
				service.OpenAIWSMiddleRouteDispositionExclude, time.Now().UnixMilli(), 10*time.Minute, 30*time.Second, 10,
			)
			if err != nil {
				errs <- err
				return
			}
			if decision.Admitted {
				admitted.Add(1)
			}
			if decision.Idempotent {
				idempotent.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, int64(goroutines), admitted.Load())
	require.Equal(t, int64(goroutines-1), idempotent.Load())
	loaded, err := cache.LoadOpenAIWSMigrationState(ctx, 7, "concurrent-same")
	require.NoError(t, err)
	require.Equal(t, 1, loaded.MigrationCount)
	require.Equal(t, uint64(2), loaded.BindingGeneration)
}

func TestGatewayCacheOpenAIWSMigrationConcurrentGenerationFence(t *testing.T) {
	cache, _ := newOpenAIWSMigrationTestCache(t)
	ctx := context.Background()
	const goroutines = 24
	start := make(chan struct{})
	var wg sync.WaitGroup
	var admitted atomic.Int64
	var rejected atomic.Int64

	for index := range goroutines {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			decision, err := cache.AdmitOpenAIWSReconnectMigration(
				ctx, 7, "concurrent-generation", int64(100+index), fmt.Sprintf("control-%d", index), 1,
				service.OpenAIWSMiddleRouteDispositionExclude, time.Now().UnixMilli(), 10*time.Minute, 30*time.Second, 10,
			)
			if err != nil {
				rejected.Add(1)
				return
			}
			if decision.Admitted {
				admitted.Add(1)
			}
		}(index)
	}
	close(start)
	wg.Wait()
	require.Equal(t, int64(1), admitted.Load())
	require.Equal(t, int64(goroutines-1), rejected.Load())
	loaded, err := cache.LoadOpenAIWSMigrationState(ctx, 7, "concurrent-generation")
	require.NoError(t, err)
	require.Equal(t, 1, loaded.MigrationCount)
	require.Equal(t, uint64(2), loaded.BindingGeneration)
}

func TestGatewayCacheOpenAIWSMigrationLoadFailsClosedOnCorruptSharedState(t *testing.T) {
	cache, rdb := newOpenAIWSMigrationTestCache(t)
	ctx := context.Background()
	key := buildOpenAIWSMigrationKey(7, "corrupt")
	require.NoError(t, rdb.HSet(ctx, key, "migration_count", "not-a-number").Err())

	_, err := cache.LoadOpenAIWSMigrationState(ctx, 7, "corrupt")
	require.ErrorContains(t, err, "invalid")
}

func newOpenAIWSMigrationTestCache(t *testing.T) (*gatewayCache, *redis.Client) {
	t.Helper()
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return &gatewayCache{rdb: rdb}, rdb
}

func mustOpenAIWSMigrationStickyAccount(t *testing.T, ctx context.Context, rdb *redis.Client, groupID int64, sessionHash string) int64 {
	t.Helper()
	accountID, err := rdb.Get(ctx, buildSessionKey(groupID, sessionHash)).Int64()
	require.NoError(t, err)
	return accountID
}
