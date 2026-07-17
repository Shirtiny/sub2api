//go:build unit

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newSchedulerMutationCacheTest(t *testing.T) (*schedulerCache, *miniredis.Miniredis) {
	t.Helper()
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache, ok := newSchedulerCacheWithChunkSizes(rdb, 16, 16).(*schedulerCache)
	require.True(t, ok)
	return cache, mini
}

func requireMiniRedisString(t *testing.T, mini *miniredis.Miniredis, key string) string {
	t.Helper()
	value, err := mini.Get(key)
	require.NoError(t, err)
	return value
}

func TestSchedulerAccountProtocolUsesIsolatedV2Keys(t *testing.T) {
	require.Equal(t, "sched:v2:buckets", schedulerBucketSetKey)
	require.Equal(t, "sched:outbox:watermark", schedulerOutboxWatermarkKey)
	require.Equal(t, "sched:v2:acc:1", schedulerAccountKey("1"))
	require.Equal(t, "sched:v2:meta:1", schedulerAccountMetaKey("1"))
	require.Equal(t, "sched:v2:acc:version:1", schedulerAccountVersionKey("1"))
	require.NotEqual(t, "sched:acc:1", schedulerAccountKey("1"), "pre-version writers must not share the full lease key")
}

func TestSchedulerAccountMutationFenceBlocksPendingAndStaleWriters(t *testing.T) {
	cache, mini := newSchedulerMutationCacheTest(t)
	ctx := context.Background()
	oldAccount := &service.Account{ID: 501, Name: "old", Status: service.StatusActive, Schedulable: true}
	require.NoError(t, cache.SetAccount(ctx, oldAccount))

	tokens, err := cache.BeginAccountMutations(ctx, []int64{oldAccount.ID}, time.Minute)
	require.NoError(t, err)
	epoch := tokens[oldAccount.ID]
	require.Positive(t, epoch)
	require.True(t, mini.Exists(schedulerAccountMetaKey("501")), "begin keeps bucket metadata available")
	require.False(t, mini.Exists(schedulerAccountKey("501")), "begin removes the retained lease snapshot")

	// Even a writer bypassing the guarded setter cannot make a pending lease
	// visible because GetAccount checks pending atomically with GET.
	bypassed, err := json.Marshal(oldAccount)
	require.NoError(t, err)
	mini.Set(schedulerAccountKey("501"), string(bypassed))
	got, err := cache.GetAccount(ctx, oldAccount.ID)
	require.NoError(t, err)
	require.Nil(t, got)
	mini.Del(schedulerAccountKey("501"))

	// A normal stale writer is also blocked while pending.
	require.NoError(t, cache.SetAccount(ctx, oldAccount))
	got, err = cache.GetAccount(ctx, oldAccount.ID)
	require.NoError(t, err)
	require.Nil(t, got)

	newAccount := &service.Account{ID: oldAccount.ID, Name: "new", Status: service.StatusActive, Schedulable: true}
	published, err := cache.PublishAccountMutation(ctx, newAccount, epoch)
	require.NoError(t, err)
	require.True(t, published)

	// This models SetSnapshot loading old DB state before Begin and reaching
	// Redis only after the mutation was committed and published.
	require.NoError(t, cache.SetAccount(ctx, oldAccount))
	got, err = cache.GetAccount(ctx, oldAccount.ID)
	require.NoError(t, err)
	require.Equal(t, "new", got.Name)
}

func TestSchedulerAccountMutationVersionAllowsFreshOrdinaryRebuildButRejectsStaleSnapshot(t *testing.T) {
	cache, mini := newSchedulerMutationCacheTest(t)
	ctx := context.Background()
	bucket := service.SchedulerBucket{GroupID: 7, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
	base := time.Date(2026, time.July, 16, 10, 0, 0, 123456000, time.UTC)
	before := service.Account{
		ID:          506,
		Name:        "before",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		UpdatedAt:   base,
	}
	require.NoError(t, cache.SetSnapshot(ctx, bucket, []service.Account{before}))

	tokens, err := cache.BeginAccountMutations(ctx, []int64{before.ID}, time.Minute)
	require.NoError(t, err)
	afterMutation := before
	afterMutation.Name = "fenced"
	afterMutation.UpdatedAt = base.Add(time.Second)
	published, err := cache.PublishAccountMutation(ctx, &afterMutation, tokens[before.ID])
	require.NoError(t, err)
	require.True(t, published)

	// The snapshot loaded before Begin must not overwrite either the full lease
	// value or the slim scheduling metadata after the fenced publish.
	require.NoError(t, cache.SetSnapshot(ctx, bucket, []service.Account{before}))
	full, err := cache.GetAccount(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, "fenced", full.Name)
	snapshot, hit, err := cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.True(t, hit)
	require.Len(t, snapshot, 1)
	require.Equal(t, "fenced", snapshot[0].Name)

	equalVersion := afterMutation
	equalVersion.Name = "equal-version-stale"
	require.NoError(t, cache.SetAccount(ctx, &equalVersion))
	full, err = cache.GetAccount(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, "fenced", full.Name, "an equal source revision cannot cross a mutation epoch")

	// A later authoritative DB revision is comparable and advances both values;
	// the existence of the mutation epoch does not freeze the full key forever.
	afterRebuild := afterMutation
	afterRebuild.Name = "ordinary-rebuild"
	afterRebuild.UpdatedAt = base.Add(2 * time.Second)
	require.NoError(t, cache.SetSnapshot(ctx, bucket, []service.Account{afterRebuild}))
	full, err = cache.GetAccount(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, "ordinary-rebuild", full.Name)
	snapshot, hit, err = cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.True(t, hit)
	require.Equal(t, "ordinary-rebuild", snapshot[0].Name)
	require.Equal(t, schedulerAccountVersion(afterRebuild), requireMiniRedisString(t, mini, schedulerAccountVersionKey("506")))

	// Even after the version advances, the old in-flight write remains stale.
	require.NoError(t, cache.SetAccount(ctx, &before))
	full, err = cache.GetAccount(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, "ordinary-rebuild", full.Name)
}

func TestSchedulerOrdinaryQuotaRefreshUpdatesFullLeaseAfterFencedMutation(t *testing.T) {
	cache, mini := newSchedulerMutationCacheTest(t)
	ctx := context.Background()
	base := time.Date(2026, time.July, 16, 11, 0, 0, 0, time.UTC)
	account := &service.Account{
		ID:          507,
		Name:        "aether-route",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		UpdatedAt:   base,
		Extra: map[string]any{
			"quota_limit": 10.0,
			"quota_used":  9.0,
		},
	}
	require.NoError(t, cache.SetAccount(ctx, account))
	tokens, err := cache.BeginAccountMutations(ctx, []int64{account.ID}, time.Minute)
	require.NoError(t, err)

	published := *account
	published.UpdatedAt = base.Add(time.Second)
	ok, err := cache.PublishAccountMutation(ctx, &published, tokens[account.ID])
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, mini.Exists(schedulerAccountEpochKey("507")))

	bound, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, bound)
	require.True(t, bound.IsSchedulable())

	// IncrementQuotaUsed emits an epoch-less account-change event at the first
	// threshold crossing. SQL NOW and Ent's application clock can produce the
	// same (or an older) UpdatedAt across nodes, but quota exhaustion is a
	// restrictive monotonic transition and must still replace the retained full
	// value read by ValidateAetherWSBindingLease.
	quotaExceeded := published
	quotaExceeded.UpdatedAt = published.UpdatedAt
	quotaExceeded.Extra = map[string]any{
		"quota_limit": 10.0,
		"quota_used":  10.0,
	}
	require.NoError(t, cache.SetAccount(ctx, &quotaExceeded))

	latest, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.Equal(t, 10.0, latest.GetQuotaUsed())
	require.False(t, latest.IsSchedulable(), "the cache-only Aether lease must observe quota exhaustion")
	require.Equal(t, schedulerAccountVersion(quotaExceeded), requireMiniRedisString(t, mini, schedulerAccountVersionKey("507")))

	// A delayed duplicate publish from the already-completed epoch has the same
	// source timestamp but predates the quota event. It must acknowledge without
	// erasing the restrictive merge.
	ok, err = cache.PublishAccountMutation(ctx, &published, tokens[account.ID])
	require.NoError(t, err)
	require.True(t, ok)
	latest, err = cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, 10.0, latest.GetQuotaUsed())
	require.False(t, latest.IsSchedulable())
}

func TestSchedulerExpiringQuotaWindowMergesWithoutOverwritingNewerLease(t *testing.T) {
	cache, _ := newSchedulerMutationCacheTest(t)
	ctx := context.Background()
	base := time.Date(2026, time.July, 16, 11, 30, 0, 0, time.UTC)
	initial := &service.Account{
		ID: 514, Name: "initial", Platform: service.PlatformOpenAI,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive,
		Schedulable: true, UpdatedAt: base,
		Credentials: map[string]any{"api_key": "old-key"},
	}
	require.NoError(t, cache.SetAccount(ctx, initial))
	tokens, err := cache.BeginAccountMutations(ctx, []int64{initial.ID}, time.Minute)
	require.NoError(t, err)

	newer := *initial
	newer.Name = "new-route"
	newer.Credentials = map[string]any{"api_key": "new-key"}
	newer.UpdatedAt = base.Add(2 * time.Second)
	published, err := cache.PublishAccountMutation(ctx, &newer, tokens[initial.ID])
	require.NoError(t, err)
	require.True(t, published)

	staleDailyQuota := *initial
	staleDailyQuota.Name = "stale-daily-quota"
	staleDailyQuota.UpdatedAt = base.Add(time.Second)
	staleDailyQuota.Credentials = map[string]any{"api_key": "old-key"}
	dailyStart := time.Now().UTC()
	staleDailyQuota.Extra = map[string]any{
		"quota_daily_limit": 10.0,
		"quota_daily_used":  10.0,
		"quota_daily_start": dailyStart.Format(time.RFC3339Nano),
	}
	require.True(t, staleDailyQuota.IsQuotaExceeded(), "fixture must exercise the expiring quota branch")
	require.NoError(t, cache.SetAccount(ctx, &staleDailyQuota))

	got, err := cache.GetAccount(ctx, initial.ID)
	require.NoError(t, err)
	require.Equal(t, "new-route", got.Name)
	require.Equal(t, "new-key", got.GetCredential("api_key"))
	require.False(t, got.IsQuotaExceeded(), "expiring stale quota fields must not replace the newer payload")
	require.NotNil(t, got.TempUnschedulableUntil)
	require.True(t, got.TempUnschedulableUntil.Equal(dailyStart.Add(24*time.Hour)))
	require.Equal(t, "quota_window_version_fence", got.TempUnschedulableReason)
	require.False(t, got.IsSchedulable())
}

func TestSchedulerQuotaWindowRestrictionUsesPersistedResetBoundary(t *testing.T) {
	now := time.Date(2026, time.November, 1, 5, 0, 0, 0, time.UTC)
	dailyReset := now.Add(3 * time.Hour)
	weeklyStart := now.Add(-2 * time.Hour)
	account := service.Account{Extra: map[string]any{
		"quota_daily_reset_mode": "fixed",
		"quota_daily_reset_at":   dailyReset.Format(time.RFC3339Nano),
		"quota_weekly_start":     weeklyStart.Format(time.RFC3339Nano),
	}}

	got := schedulerQuotaWindowRestrictionUntil(
		account,
		schedulerQuotaRestrictionDaily|schedulerQuotaRestrictionWeekly,
		now,
	)
	require.True(t, got.Equal(weeklyStart.Add(7*24*time.Hour)), "the later active quota window must bound the synthetic fence")
}

func TestSchedulerAccountVersionSafelyUpgradesLegacyFencedValue(t *testing.T) {
	cache, mini := newSchedulerMutationCacheTest(t)
	ctx := context.Background()
	base := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	legacy := &service.Account{ID: 508, Name: "legacy", UpdatedAt: base}
	require.NoError(t, cache.SetAccount(ctx, legacy))
	mini.Set(schedulerAccountEpochKey("508"), "4")
	mini.Del(schedulerAccountVersionKey("508")) // Simulate a publish from the previous binary.

	equal := *legacy
	equal.Name = "equal-must-not-win"
	require.NoError(t, cache.SetAccount(ctx, &equal))
	got, err := cache.GetAccount(ctx, legacy.ID)
	require.NoError(t, err)
	require.Equal(t, "legacy", got.Name)
	require.False(t, mini.Exists(schedulerAccountVersionKey("508")))

	newer := *legacy
	newer.Name = "versioned"
	newer.UpdatedAt = base.Add(time.Second)
	require.NoError(t, cache.SetAccount(ctx, &newer))
	got, err = cache.GetAccount(ctx, legacy.ID)
	require.NoError(t, err)
	require.Equal(t, "versioned", got.Name)
	require.Equal(t, schedulerAccountVersion(newer), requireMiniRedisString(t, mini, schedulerAccountVersionKey("508")))
}

func TestSchedulerDelayedEpochPublishPreservesNewerOrdinaryRevision(t *testing.T) {
	cache, mini := newSchedulerMutationCacheTest(t)
	ctx := context.Background()
	base := time.Date(2026, time.July, 16, 13, 0, 0, 0, time.UTC)
	initial := &service.Account{ID: 510, Name: "initial", UpdatedAt: base}
	require.NoError(t, cache.SetAccount(ctx, initial))
	tokens, err := cache.BeginAccountMutations(ctx, []int64{initial.ID}, time.Minute)
	require.NoError(t, err)

	// Model a mutation publish delayed beyond its safety fence. A later ordinary
	// DB refresh is allowed to restore the lease with a strictly newer revision.
	mini.Del(schedulerAccountFenceKey("510"))
	newer := *initial
	newer.Name = "ordinary-newer"
	newer.UpdatedAt = base.Add(2 * time.Second)
	require.NoError(t, cache.SetAccount(ctx, &newer))

	delayed := *initial
	delayed.Name = "delayed-epoch"
	delayed.UpdatedAt = base.Add(time.Second)
	published, err := cache.PublishAccountMutation(ctx, &delayed, tokens[initial.ID])
	require.NoError(t, err)
	require.True(t, published, "the matching epoch is acknowledged even when its payload is superseded")

	full, err := cache.GetAccount(ctx, initial.ID)
	require.NoError(t, err)
	require.NotNil(t, full)
	require.Equal(t, "ordinary-newer", full.Name)
	meta, err := decodeCachedAccount(requireMiniRedisString(t, mini, schedulerAccountMetaKey("510")))
	require.NoError(t, err)
	require.Equal(t, "ordinary-newer", meta.Name)
	require.Equal(t, schedulerAccountVersion(newer), requireMiniRedisString(t, mini, schedulerAccountVersionKey("510")))
}

func TestSchedulerDelayedEpochPublishDoesNotRelabelPartialNewerState(t *testing.T) {
	cache, mini := newSchedulerMutationCacheTest(t)
	ctx := context.Background()
	base := time.Date(2026, time.July, 16, 14, 0, 0, 0, time.UTC)
	initial := &service.Account{ID: 512, Name: "initial", UpdatedAt: base.Add(2 * time.Second)}
	require.NoError(t, cache.SetAccount(ctx, initial))
	tokens, err := cache.BeginAccountMutations(ctx, []int64{initial.ID}, time.Minute)
	require.NoError(t, err)
	mini.Del(schedulerAccountFenceKey("512"))

	delayed := *initial
	delayed.Name = "older-payload"
	delayed.UpdatedAt = base.Add(time.Second)
	published, err := cache.PublishAccountMutation(ctx, &delayed, tokens[initial.ID])
	require.NoError(t, err)
	require.True(t, published)

	// Begin removed the full value while retaining the newer watermark. Once the
	// fence has expired, the delayed publisher cannot safely decide whether this
	// partial state came from a newer writer, so it leaves the lease unavailable.
	full, err := cache.GetAccount(ctx, initial.ID)
	require.NoError(t, err)
	require.Nil(t, full)
	require.Equal(t, schedulerAccountVersion(*initial), requireMiniRedisString(t, mini, schedulerAccountVersionKey("512")))
}

func TestSchedulerUpdateLastUsedNeverMovesBackward(t *testing.T) {
	cache, _ := newSchedulerMutationCacheTest(t)
	ctx := context.Background()
	base := time.Date(2026, time.July, 16, 15, 0, 0, 0, time.UTC)
	lastUsed := base.Add(2 * time.Second)
	account := &service.Account{ID: 513, Name: "last-used", UpdatedAt: base, LastUsedAt: &lastUsed}
	require.NoError(t, cache.SetAccount(ctx, account))

	require.NoError(t, cache.UpdateLastUsed(ctx, map[int64]time.Time{account.ID: base.Add(time.Second)}))
	got, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
	require.True(t, got.LastUsedAt.Equal(lastUsed))

	require.NoError(t, cache.UpdateLastUsed(ctx, map[int64]time.Time{account.ID: lastUsed}))
	got, err = cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.True(t, got.LastUsedAt.Equal(lastUsed))

	newest := base.Add(3 * time.Second)
	require.NoError(t, cache.UpdateLastUsed(ctx, map[int64]time.Time{account.ID: newest}))
	got, err = cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.True(t, got.LastUsedAt.Equal(newest))
}

func TestSchedulerAccountMutationFenceSerializesAndCASesEpoch(t *testing.T) {
	cache, _ := newSchedulerMutationCacheTest(t)
	ctx := context.Background()
	account := &service.Account{ID: 502, Name: "initial"}
	require.NoError(t, cache.SetAccount(ctx, account))

	first, err := cache.BeginAccountMutations(ctx, []int64{account.ID}, time.Minute)
	require.NoError(t, err)
	partial, err := cache.BeginAccountMutations(ctx, []int64{account.ID}, time.Minute)
	require.ErrorIs(t, err, service.ErrSchedulerAccountMutationInProgress)
	require.Empty(t, partial)

	published, err := cache.PublishAccountMutation(ctx, &service.Account{ID: account.ID, Name: "first"}, first[account.ID])
	require.NoError(t, err)
	require.True(t, published)
	second, err := cache.BeginAccountMutations(ctx, []int64{account.ID}, time.Minute)
	require.NoError(t, err)
	require.Greater(t, second[account.ID], first[account.ID])

	published, err = cache.PublishAccountMutation(ctx, &service.Account{ID: account.ID, Name: "stale"}, first[account.ID])
	require.NoError(t, err)
	require.False(t, published)
	published, err = cache.PublishAccountMutation(ctx, &service.Account{ID: account.ID, Name: "second"}, second[account.ID])
	require.NoError(t, err)
	require.True(t, published)
	got, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, "second", got.Name)
}

func TestSchedulerAccountMutationDeletionTombstonePreventsResurrection(t *testing.T) {
	cache, mini := newSchedulerMutationCacheTest(t)
	ctx := context.Background()
	account := &service.Account{ID: 503, Name: "deleted"}
	require.NoError(t, cache.SetAccount(ctx, account))
	tokens, err := cache.BeginAccountMutations(ctx, []int64{account.ID}, time.Minute)
	require.NoError(t, err)
	completed, err := cache.CompleteAccountDeletion(ctx, account.ID, tokens[account.ID])
	require.NoError(t, err)
	require.True(t, completed)
	require.False(t, mini.Exists(schedulerAccountVersionKey("503")))

	require.NoError(t, cache.SetAccount(ctx, account))
	got, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, got)
	published, err := cache.PublishAccountMutation(ctx, account, tokens[account.ID])
	require.NoError(t, err)
	require.False(t, published, "a delayed publish cannot reverse this epoch's deletion")
	require.True(t, mini.Exists(schedulerAccountTombstoneKey("503")))
	got, err = cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, got)
	published, err = cache.PublishAccountMutation(ctx, account, tokens[account.ID]-1)
	require.NoError(t, err)
	require.False(t, published)
}

func TestSchedulerAccountDeletionDominatesEarlierPublishInSameEpoch(t *testing.T) {
	cache, mini := newSchedulerMutationCacheTest(t)
	ctx := context.Background()
	account := &service.Account{ID: 511, Name: "published-first", UpdatedAt: time.Now().UTC()}
	require.NoError(t, cache.SetAccount(ctx, account))
	tokens, err := cache.BeginAccountMutations(ctx, []int64{account.ID}, time.Minute)
	require.NoError(t, err)
	published, err := cache.PublishAccountMutation(ctx, account, tokens[account.ID])
	require.NoError(t, err)
	require.True(t, published)

	deleted, err := cache.CompleteAccountDeletion(ctx, account.ID, tokens[account.ID])
	require.NoError(t, err)
	require.True(t, deleted, "the restrictive deletion outcome dominates an earlier publish")
	require.True(t, mini.Exists(schedulerAccountTombstoneKey("511")))
	require.False(t, mini.Exists(schedulerAccountVersionKey("511")))
	got, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestSchedulerDeleteAccountRemovesSourceVersion(t *testing.T) {
	cache, mini := newSchedulerMutationCacheTest(t)
	ctx := context.Background()
	account := &service.Account{ID: 509, Name: "deleted", UpdatedAt: time.Now().UTC()}
	require.NoError(t, cache.SetAccount(ctx, account))
	require.True(t, mini.Exists(schedulerAccountVersionKey("509")))

	require.NoError(t, cache.DeleteAccount(ctx, account.ID))
	require.False(t, mini.Exists(schedulerAccountKey("509")))
	require.False(t, mini.Exists(schedulerAccountMetaKey("509")))
	require.False(t, mini.Exists(schedulerAccountVersionKey("509")))
}

func TestSchedulerAccountMutationBatchReconcilePublishesAndDeletes(t *testing.T) {
	cache, _ := newSchedulerMutationCacheTest(t)
	ctx := context.Background()
	first := &service.Account{ID: 504, Name: "first-old"}
	second := &service.Account{ID: 505, Name: "second-old"}
	require.NoError(t, cache.SetAccount(ctx, first))
	require.NoError(t, cache.SetAccount(ctx, second))
	tokens, err := cache.BeginAccountMutations(ctx, []int64{first.ID, second.ID}, time.Minute)
	require.NoError(t, err)

	results, err := cache.ReconcileAccountMutations(ctx, map[int64]*service.Account{
		first.ID: {ID: first.ID, Name: "first-new"},
	}, tokens)
	require.NoError(t, err)
	require.True(t, results[first.ID])
	require.True(t, results[second.ID])
	got, err := cache.GetAccount(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, "first-new", got.Name)
	got, err = cache.GetAccount(ctx, second.ID)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestSchedulerAccountMutationBeginRejectsInvalidTTL(t *testing.T) {
	cache, _ := newSchedulerMutationCacheTest(t)
	_, err := cache.BeginAccountMutations(context.Background(), []int64{1}, 0)
	require.Error(t, err)
	require.False(t, errors.Is(err, service.ErrSchedulerAccountMutationInProgress))
}

func TestBuildSchedulerMetadataAccount_KeepsOpenAIWSFlags(t *testing.T) {
	account := service.Account{
		ID:       42,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_enabled": true,
			"openai_oauth_responses_websockets_v2_mode":    service.OpenAIWSIngressModePassthrough,
			"openai_ws_force_http":                         true,
			"aether_ws": map[string]any{
				"schema_version":            float64(1),
				"enabled":                   true,
				"required_control_protocol": "route-v1",
				"future_option":             "preserve",
			},
			"openai_responses_mode":      "force_chat_completions",
			"openai_responses_supported": false,
			"mixed_scheduling":           true,
			"unused_large_field":         "drop-me",
		},
	}

	got := buildSchedulerMetadataAccount(account)

	require.Equal(t, true, got.Extra["openai_oauth_responses_websockets_v2_enabled"])
	require.Equal(t, service.OpenAIWSIngressModePassthrough, got.Extra["openai_oauth_responses_websockets_v2_mode"])
	require.Equal(t, true, got.Extra["openai_ws_force_http"])
	require.Equal(t, map[string]any{
		"schema_version":            float64(1),
		"enabled":                   true,
		"required_control_protocol": "route-v1",
		"future_option":             "preserve",
	}, got.Extra["aether_ws"])
	require.Equal(t, "force_chat_completions", got.Extra["openai_responses_mode"])
	require.Equal(t, false, got.Extra["openai_responses_supported"])
	require.Equal(t, true, got.Extra["mixed_scheduling"])
	require.Nil(t, got.Extra["unused_large_field"])
}

func TestBuildSchedulerMetadataAccount_KeepsAetherRouteCredentials(t *testing.T) {
	account := service.Account{
		ID:       43,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":          "aether-admin-key",
			"base_url":         "http://aether:8080/v1",
			"unrelated_secret": "drop-me",
		},
	}

	got := buildSchedulerMetadataAccount(account)

	require.Equal(t, "aether-admin-key", got.Credentials["api_key"])
	require.Equal(t, "http://aether:8080/v1", got.Credentials["base_url"])
	require.NotContains(t, got.Credentials, "unrelated_secret")
}

func TestBuildSchedulerMetadataAccount_KeepsSlimGroupMembership(t *testing.T) {
	account := service.Account{
		ID:       42,
		Platform: service.PlatformAnthropic,
		GroupIDs: []int64{7, 9, 7, 0},
		AccountGroups: []service.AccountGroup{
			{
				AccountID: 42,
				GroupID:   7,
				Priority:  2,
				Account:   &service.Account{ID: 42, Name: "drop-from-metadata"},
				Group:     &service.Group{ID: 7, Name: "drop-from-metadata"},
			},
			{
				AccountID: 42,
				GroupID:   11,
				Priority:  3,
				Group:     &service.Group{ID: 11, Name: "drop-from-metadata"},
			},
			{
				AccountID: 42,
				GroupID:   0,
				Priority:  4,
			},
		},
	}

	got := buildSchedulerMetadataAccount(account)

	require.Equal(t, []int64{7, 9, 11}, got.GroupIDs)
	require.Len(t, got.AccountGroups, 2)
	require.Equal(t, int64(42), got.AccountGroups[0].AccountID)
	require.Equal(t, int64(7), got.AccountGroups[0].GroupID)
	require.Equal(t, 2, got.AccountGroups[0].Priority)
	require.Nil(t, got.AccountGroups[0].Account)
	require.Nil(t, got.AccountGroups[0].Group)
	require.Equal(t, int64(11), got.AccountGroups[1].GroupID)
	require.Nil(t, got.Groups)
}

func TestBuildSchedulerMetadataAccount_KeepsQuotaAutoPauseFields(t *testing.T) {
	account := service.Account{
		ID: 88,
		Extra: map[string]any{
			"codex_5h_used_percent":        12.34,
			"codex_7d_used_percent":        56.78,
			"codex_5h_reset_at":            "2026-05-29T10:00:00Z",
			"codex_7d_reset_at":            "2026-06-01T10:00:00Z",
			"codex_5h_reset_after_seconds": 300,
			"codex_7d_reset_after_seconds": 600,
			"codex_usage_updated_at":       "2026-05-29T09:00:00Z",
			"auto_pause_5h_threshold":      0.95,
			"auto_pause_7d_threshold":      0.96,
			"auto_pause_5h_disabled":       true,
			"auto_pause_7d_disabled":       false,
		},
	}

	got := buildSchedulerMetadataAccount(account)

	require.Equal(t, 12.34, got.Extra["codex_5h_used_percent"])
	require.Equal(t, 56.78, got.Extra["codex_7d_used_percent"])
	require.Equal(t, "2026-05-29T10:00:00Z", got.Extra["codex_5h_reset_at"])
	require.Equal(t, "2026-06-01T10:00:00Z", got.Extra["codex_7d_reset_at"])
	require.Equal(t, 300, got.Extra["codex_5h_reset_after_seconds"])
	require.Equal(t, 600, got.Extra["codex_7d_reset_after_seconds"])
	require.Equal(t, "2026-05-29T09:00:00Z", got.Extra["codex_usage_updated_at"])
	require.Equal(t, 0.95, got.Extra["auto_pause_5h_threshold"])
	require.Equal(t, 0.96, got.Extra["auto_pause_7d_threshold"])
	require.Equal(t, true, got.Extra["auto_pause_5h_disabled"])
	require.Equal(t, false, got.Extra["auto_pause_7d_disabled"])
}

func TestBuildSchedulerMetadataAccount_KeepsModelRateLimits(t *testing.T) {
	account := service.Account{
		ID:       90,
		Platform: service.PlatformAntigravity,
		Extra: map[string]any{
			"model_rate_limits": map[string]any{
				"gemini-3-flash": map[string]any{
					"rate_limit_reset_at": "2026-05-30T10:10:00Z",
				},
				"antigravity:gemini": map[string]any{
					"rate_limit_reset_at": "2026-05-30T10:10:00Z",
				},
			},
			"unused_large_field": "drop-me",
		},
	}

	got := buildSchedulerMetadataAccount(account)

	limits, ok := got.Extra["model_rate_limits"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, limits, "gemini-3-flash")
	require.Contains(t, limits, "antigravity:gemini")
	require.Nil(t, got.Extra["unused_large_field"])
}
