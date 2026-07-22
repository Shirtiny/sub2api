//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type authRepoStub struct {
	getByKeyForAuth   func(ctx context.Context, key string) (*APIKey, error)
	listKeysByUserID  func(ctx context.Context, userID int64) ([]string, error)
	listKeysByGroupID func(ctx context.Context, groupID int64) ([]string, error)
}

func (s *authRepoStub) Create(ctx context.Context, key *APIKey) error {
	panic("unexpected Create call")
}

func (s *authRepoStub) GetByID(ctx context.Context, id int64) (*APIKey, error) {
	panic("unexpected GetByID call")
}

func (s *authRepoStub) GetKeyAndOwnerID(ctx context.Context, id int64) (string, int64, error) {
	panic("unexpected GetKeyAndOwnerID call")
}

func (s *authRepoStub) GetByKey(ctx context.Context, key string) (*APIKey, error) {
	panic("unexpected GetByKey call")
}

func (s *authRepoStub) GetByKeyForAuth(ctx context.Context, key string) (*APIKey, error) {
	if s.getByKeyForAuth == nil {
		panic("unexpected GetByKeyForAuth call")
	}
	return s.getByKeyForAuth(ctx, key)
}

func (s *authRepoStub) Update(ctx context.Context, key *APIKey) error {
	panic("unexpected Update call")
}

func (s *authRepoStub) RotateKey(ctx context.Context, key *APIKey, guard APIKeyRotationGuard) error {
	panic("unexpected RotateKey call")
}

func (s *authRepoStub) Delete(ctx context.Context, id int64) error {
	panic("unexpected Delete call")
}

func (s *authRepoStub) DeleteWithAudit(ctx context.Context, id int64) error {
	panic("unexpected DeleteWithAudit call")
}

func (s *authRepoStub) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserID call")
}

func (s *authRepoStub) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	panic("unexpected VerifyOwnership call")
}

func (s *authRepoStub) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	panic("unexpected CountByUserID call")
}

func (s *authRepoStub) ExistsByKey(ctx context.Context, key string) (bool, error) {
	panic("unexpected ExistsByKey call")
}

func (s *authRepoStub) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]APIKey, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID call")
}

func (s *authRepoStub) SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]APIKey, error) {
	panic("unexpected SearchAPIKeys call")
}

func (s *authRepoStub) ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected ClearGroupIDByGroupID call")
}
func (s *authRepoStub) UpdateGroupIDByUserAndGroup(ctx context.Context, userID, oldGroupID, newGroupID int64) (int64, error) {
	panic("unexpected UpdateGroupIDByUserAndGroup call")
}

func (s *authRepoStub) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected CountByGroupID call")
}

func (s *authRepoStub) ListKeysByUserID(ctx context.Context, userID int64) ([]string, error) {
	if s.listKeysByUserID == nil {
		panic("unexpected ListKeysByUserID call")
	}
	return s.listKeysByUserID(ctx, userID)
}

func (s *authRepoStub) ListKeysByGroupID(ctx context.Context, groupID int64) ([]string, error) {
	if s.listKeysByGroupID == nil {
		panic("unexpected ListKeysByGroupID call")
	}
	return s.listKeysByGroupID(ctx, groupID)
}

func (s *authRepoStub) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) (float64, error) {
	panic("unexpected IncrementQuotaUsed call")
}

func (s *authRepoStub) UpdateLastUsed(ctx context.Context, id int64, usedAt time.Time) error {
	panic("unexpected UpdateLastUsed call")
}
func (s *authRepoStub) IncrementRateLimitUsage(ctx context.Context, id int64, cost float64) error {
	panic("unexpected IncrementRateLimitUsage call")
}
func (s *authRepoStub) ResetRateLimitWindows(ctx context.Context, id int64) error {
	panic("unexpected ResetRateLimitWindows call")
}
func (s *authRepoStub) GetRateLimitData(ctx context.Context, id int64) (*APIKeyRateLimitData, error) {
	panic("unexpected GetRateLimitData call")
}

type authCacheStub struct {
	getAuthCache   func(ctx context.Context, key string) (*APIKeyAuthCacheEntry, error)
	setAuthKeys    []string
	deleteAuthKeys []string
}

type epochAwareAuthCacheStub struct {
	*authCacheStub
	epoch        uint64
	epochGets    int32
	incrementErr error
	mu           sync.Mutex
	operations   []string
}

func (s *epochAwareAuthCacheStub) GetAuthCacheEpoch(context.Context, string) (uint64, error) {
	atomic.AddInt32(&s.epochGets, 1)
	return atomic.LoadUint64(&s.epoch), nil
}

func (s *epochAwareAuthCacheStub) IncrementAuthCacheEpoch(context.Context, string) error {
	s.recordOperation("epoch")
	if s.incrementErr != nil {
		return s.incrementErr
	}
	atomic.AddUint64(&s.epoch, 1)
	return nil
}

func (s *epochAwareAuthCacheStub) DeleteAuthCache(ctx context.Context, key string) error {
	s.recordOperation("delete")
	return s.authCacheStub.DeleteAuthCache(ctx, key)
}

func (s *epochAwareAuthCacheStub) PublishAuthCacheInvalidation(context.Context, string) error {
	s.recordOperation("publish")
	return nil
}

func (s *epochAwareAuthCacheStub) recordOperation(operation string) {
	s.mu.Lock()
	s.operations = append(s.operations, operation)
	s.mu.Unlock()
}

func (s *epochAwareAuthCacheStub) recordedOperations() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.operations...)
}

type wsAuthLeaseRateRepoStub struct {
	UserGroupRateRepository
	override *int
	err      error
	calls    int32
}

func (s *wsAuthLeaseRateRepoStub) GetRPMOverrideByUserAndGroup(context.Context, int64, int64) (*int, error) {
	atomic.AddInt32(&s.calls, 1)
	return s.override, s.err
}

func (s *authCacheStub) GetCreateAttemptCount(ctx context.Context, userID int64) (int, error) {
	return 0, nil
}

func (s *authCacheStub) IncrementCreateAttemptCount(ctx context.Context, userID int64) error {
	return nil
}

func (s *authCacheStub) DeleteCreateAttemptCount(ctx context.Context, userID int64) error {
	return nil
}

func (s *authCacheStub) IncrementDailyUsage(ctx context.Context, apiKey string) error {
	return nil
}

func (s *authCacheStub) SetDailyUsageExpiry(ctx context.Context, apiKey string, ttl time.Duration) error {
	return nil
}

func (s *authCacheStub) GetAuthCache(ctx context.Context, key string) (*APIKeyAuthCacheEntry, error) {
	if s.getAuthCache == nil {
		return nil, redis.Nil
	}
	return s.getAuthCache(ctx, key)
}

func (s *authCacheStub) SetAuthCache(ctx context.Context, key string, entry *APIKeyAuthCacheEntry, ttl time.Duration) error {
	s.setAuthKeys = append(s.setAuthKeys, key)
	return nil
}

func (s *authCacheStub) DeleteAuthCache(ctx context.Context, key string) error {
	s.deleteAuthKeys = append(s.deleteAuthKeys, key)
	return nil
}

func (s *authCacheStub) PublishAuthCacheInvalidation(ctx context.Context, cacheKey string) error {
	return nil
}

func (s *authCacheStub) SubscribeAuthCacheInvalidation(ctx context.Context, handler func(cacheKey string)) error {
	return nil
}

func expectedPlaintextAuthCacheKeys(key string, cfg *config.Config) []string {
	hashes := APIKeyLookupHashes(key, cfg)
	out := make([]string, 0, len(hashes)+1)
	for _, hash := range hashes {
		if cacheKey := APIKeyAuthCacheKeyFromHash(hash); cacheKey != "" {
			out = append(out, cacheKey)
		}
	}
	if legacyCacheKey := apiKeyLegacyAuthCacheKey(key); legacyCacheKey != "" {
		out = append(out, legacyCacheKey)
	}
	return out
}

func TestAPIKeyService_GetByKey_DoesNotReadPersistentEpoch(t *testing.T) {
	baseCache := &authCacheStub{}
	cache := &epochAwareAuthCacheStub{authCacheStub: baseCache}
	baseCache.getAuthCache = func(context.Context, string) (*APIKeyAuthCacheEntry, error) {
		return &APIKeyAuthCacheEntry{Snapshot: &APIKeyAuthSnapshot{
			Version:  apiKeyAuthSnapshotVersion,
			APIKeyID: 1,
			UserID:   2,
			Status:   StatusActive,
			User: APIKeyAuthUserSnapshot{
				ID:     2,
				Status: StatusActive,
			},
		}}, nil
	}
	svc := NewAPIKeyService(
		&authRepoStub{getByKeyForAuth: func(context.Context, string) (*APIKey, error) {
			return nil, errors.New("unexpected repository call")
		}},
		nil, nil, nil, nil, cache,
		&config.Config{APIKeyAuth: config.APIKeyAuthCacheConfig{L2TTLSeconds: 60}},
	)

	key, err := svc.GetByKey(context.Background(), "ordinary-http-key")
	require.NoError(t, err)
	require.Equal(t, int64(1), key.ID)
	require.Zero(t, atomic.LoadInt32(&cache.epochGets), "ordinary HTTP auth must not add a Redis generation RTT")
}

func TestAPIKeyService_ApplyAuthCacheEntryRejectsPreviousSnapshotVersion(t *testing.T) {
	svc := &APIKeyService{}
	apiKey, hit, err := svc.applyAuthCacheEntry("cached-key", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{Version: apiKeyAuthSnapshotVersion - 1},
	})

	require.NoError(t, err)
	require.False(t, hit)
	require.Nil(t, apiKey)
}

func TestAPIKeyService_GetByKeyWithAuthEpochLease_HydratesCompleteSnapshot(t *testing.T) {
	plaintext := "ws-complete-auth-snapshot"
	groupID := int64(8)
	override := 17
	rateRepo := &wsAuthLeaseRateRepoStub{override: &override}
	cache := &epochAwareAuthCacheStub{authCacheStub: &authCacheStub{}}
	repo := &authRepoStub{getByKeyForAuth: func(_ context.Context, token string) (*APIKey, error) {
		lookups, ok := DecodeAPIKeyLookupToken(token)
		require.True(t, ok)
		require.NotEmpty(t, lookups)
		return &APIKey{
			ID:            11,
			UserID:        22,
			KeyLookupHash: APIKeyLookupHashValue(plaintext),
			GroupID:       &groupID,
			Status:        StatusActive,
			IPWhitelist:   []string{"127.0.0.1", "10.0.0.0/8"},
			IPBlacklist:   []string{"192.0.2.1"},
			User: &User{
				ID:          22,
				Status:      StatusActive,
				Concurrency: 4,
				RPMLimit:    100,
			},
			Group: &Group{
				ID:               groupID,
				Name:             "openai",
				Platform:         PlatformOpenAI,
				Status:           StatusActive,
				SubscriptionType: SubscriptionTypeStandard,
				RateMultiplier:   1,
				RPMLimit:         50,
			},
		}, nil
	}}
	svc := NewAPIKeyService(repo, nil, nil, nil, rateRepo, cache, &config.Config{})

	key, err := svc.GetByKeyWithAuthEpochLease(context.Background(), plaintext)
	require.NoError(t, err)
	require.Equal(t, plaintext, key.Key)
	require.NotNil(t, key.User)
	require.True(t, key.User.UserGroupRPMOverrideResolved)
	require.Equal(t, override, *key.User.UserGroupRPMOverride)
	require.NotNil(t, key.Group)
	require.True(t, key.Group.Hydrated)
	require.Equal(t, 50, key.Group.RPMLimit)
	require.NotNil(t, key.CompiledIPWhitelist)
	require.NotNil(t, key.CompiledIPBlacklist)
	require.EqualValues(t, 1, atomic.LoadInt32(&rateRepo.calls), "RPM override must be resolved once at admission")
	require.EqualValues(t, 2, atomic.LoadInt32(&cache.epochGets), "WS admission fences both sides of the repository snapshot")
}

func TestAPIKeyService_GetByKeyWithAuthEpochLease_RejectsUnresolvedRPMOverride(t *testing.T) {
	groupID := int64(8)
	rateRepo := &wsAuthLeaseRateRepoStub{err: errors.New("database unavailable")}
	cache := &epochAwareAuthCacheStub{authCacheStub: &authCacheStub{}}
	repo := &authRepoStub{getByKeyForAuth: func(context.Context, string) (*APIKey, error) {
		return &APIKey{
			ID:      11,
			UserID:  22,
			GroupID: &groupID,
			Status:  StatusActive,
			User:    &User{ID: 22, Status: StatusActive},
			Group:   &Group{ID: groupID, Status: StatusActive},
		}, nil
	}}
	svc := NewAPIKeyService(repo, nil, nil, nil, rateRepo, cache, &config.Config{})

	_, err := svc.GetByKeyWithAuthEpochLease(context.Background(), "ws-unresolved-rpm")
	require.ErrorContains(t, err, "resolve user/group RPM override")
	require.EqualValues(t, 1, atomic.LoadInt32(&rateRepo.calls))
}

func TestAPIKeyService_InvalidateAuthCache_AdvancesEpochBeforeEviction(t *testing.T) {
	cache := &epochAwareAuthCacheStub{
		authCacheStub: &authCacheStub{},
		incrementErr:  errors.New("redis unavailable"),
	}
	svc := NewAPIKeyService(nil, nil, nil, nil, nil, cache, &config.Config{})
	hash := APIKeyLookupHash{Alg: APIKeyHashAlgLookupSHA256, Hash: strings.Repeat("a", 64)}
	cacheKey := APIKeyAuthCacheKeyFromHash(hash)
	localLease := APIKeyAuthEpochLease{cacheKey: cacheKey, epoch: 0}
	require.True(t, svc.ValidateAuthEpochLease(localLease))

	svc.InvalidateAuthCacheByHash(context.Background(), hash.Alg, hash.Hash)

	require.Equal(t, []string{"epoch", "delete", "publish"}, cache.recordedOperations())
	require.False(t, svc.ValidateAuthEpochLease(localLease), "a Redis failure must still fail closed for leases in this process")
}

func TestAPIKeyService_GetByKey_UsesL2Cache(t *testing.T) {
	cache := &authCacheStub{}
	repo := &authRepoStub{
		getByKeyForAuth: func(ctx context.Context, key string) (*APIKey, error) {
			return nil, errors.New("unexpected repo call")
		},
	}
	cfg := &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			L2TTLSeconds:       60,
			NegativeTTLSeconds: 30,
		},
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)

	groupID := int64(9)
	cacheEntry := &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{
			Version:  apiKeyAuthSnapshotVersion,
			APIKeyID: 1,
			UserID:   2,
			GroupID:  &groupID,
			Status:   StatusActive,
			User: APIKeyAuthUserSnapshot{
				ID:          2,
				Status:      StatusActive,
				Role:        RoleUser,
				Balance:     10,
				Concurrency: 3,
			},
			Group: &APIKeyAuthGroupSnapshot{
				ID:                  groupID,
				Name:                "g",
				Platform:            PlatformAnthropic,
				Status:              StatusActive,
				SubscriptionType:    SubscriptionTypeStandard,
				RateMultiplier:      1,
				ModelRoutingEnabled: true,
				ModelRouting: map[string][]int64{
					"claude-opus-*": {1, 2},
				},
			},
		},
	}
	cache.getAuthCache = func(ctx context.Context, key string) (*APIKeyAuthCacheEntry, error) {
		return cacheEntry, nil
	}

	apiKey, err := svc.GetByKey(context.Background(), "k1")
	require.NoError(t, err)
	require.Equal(t, int64(1), apiKey.ID)
	require.Equal(t, int64(2), apiKey.User.ID)
	require.Equal(t, groupID, apiKey.Group.ID)
	require.True(t, apiKey.Group.ModelRoutingEnabled)
	require.Equal(t, map[string][]int64{"claude-opus-*": {1, 2}}, apiKey.Group.ModelRouting)
}

func TestAPIKeyService_SnapshotRoundTrip_PreservesMessagesDispatchModelConfig(t *testing.T) {
	svc := NewAPIKeyService(nil, nil, nil, nil, nil, nil, &config.Config{})
	groupID := int64(9)
	apiKey := &APIKey{
		ID:      1,
		UserID:  2,
		GroupID: &groupID,
		Key:     "k-roundtrip",
		Name:    "Audit Key",
		Status:  StatusActive,
		User: &User{
			ID:          2,
			Status:      StatusActive,
			Role:        RoleUser,
			Balance:     10,
			Concurrency: 3,
		},
		Group: &Group{
			ID:                    groupID,
			Name:                  "openai",
			Platform:              PlatformOpenAI,
			Status:                StatusActive,
			SubscriptionType:      SubscriptionTypeStandard,
			RateMultiplier:        1,
			AllowMessagesDispatch: true,
			DefaultMappedModel:    "gpt-5.4",
			MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
				OpusMappedModel:   "gpt-5.4-nano",
				SonnetMappedModel: "gpt-5.3-codex",
				HaikuMappedModel:  "gpt-5.4-mini",
				ExactModelMappings: map[string]string{
					"claude-sonnet-4.5": "gpt-5.4-nano",
				},
			},
		},
	}

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	roundTrip := svc.snapshotToAPIKey(apiKey.Key, snapshot)

	require.NotNil(t, roundTrip)
	require.Equal(t, apiKey.Name, roundTrip.Name)
	require.NotNil(t, roundTrip.Group)
	require.Equal(t, apiKey.Group.MessagesDispatchModelConfig, roundTrip.Group.MessagesDispatchModelConfig)
}

func TestAPIKeyService_SnapshotRoundTripPreservesBaseConcurrencyAcrossPlanExpiry(t *testing.T) {
	svc := NewAPIKeyService(nil, nil, nil, nil, nil, nil, &config.Config{})
	now := time.Now()
	apiKey := &APIKey{
		Key:    "k-plan-concurrency-roundtrip",
		Status: StatusActive,
		User: &User{
			ID:          2,
			Status:      StatusActive,
			Concurrency: 5,
			PlanConcurrencyEntitlements: []PlanConcurrencyEntitlement{
				{SubscriptionID: 42, Concurrency: 16, StartsAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute)},
			},
		},
	}

	roundTrip := svc.snapshotToAPIKey(apiKey.Key, svc.snapshotFromAPIKey(context.Background(), apiKey))

	require.NotNil(t, roundTrip)
	require.Equal(t, 5, roundTrip.User.Concurrency)
	require.Equal(t, int64(42), roundTrip.User.PlanConcurrencyEntitlements[0].SubscriptionID)
	require.Equal(t, 16, roundTrip.User.EffectiveConcurrencyAt(now))
	require.Equal(t, 5, roundTrip.User.EffectiveConcurrencyAt(now.Add(2*time.Minute)))
}

func TestAPIKeyService_GetByKey_IgnoresLegacyAuthCacheSnapshotWithoutMessagesDispatchConfig(t *testing.T) {
	cache := &authCacheStub{}
	var repoCalls int32
	repo := &authRepoStub{
		getByKeyForAuth: func(ctx context.Context, key string) (*APIKey, error) {
			atomic.AddInt32(&repoCalls, 1)
			groupID := int64(9)
			return &APIKey{
				ID:      1,
				UserID:  2,
				GroupID: &groupID,
				Status:  StatusActive,
				User: &User{
					ID:          2,
					Status:      StatusActive,
					Role:        RoleUser,
					Balance:     10,
					Concurrency: 3,
				},
				Group: &Group{
					ID:                    groupID,
					Name:                  "openai",
					Platform:              PlatformOpenAI,
					Status:                StatusActive,
					Hydrated:              true,
					SubscriptionType:      SubscriptionTypeStandard,
					RateMultiplier:        1,
					AllowMessagesDispatch: true,
					DefaultMappedModel:    "gpt-5.4",
					MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
						OpusMappedModel: "gpt-5.4-nano",
					},
				},
			}, nil
		},
	}
	cfg := &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			L2TTLSeconds: 60,
		},
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)

	groupID := int64(9)
	cache.getAuthCache = func(ctx context.Context, key string) (*APIKeyAuthCacheEntry, error) {
		return &APIKeyAuthCacheEntry{
			Snapshot: &APIKeyAuthSnapshot{
				APIKeyID: 1,
				UserID:   2,
				GroupID:  &groupID,
				Status:   StatusActive,
				User: APIKeyAuthUserSnapshot{
					ID:          2,
					Status:      StatusActive,
					Role:        RoleUser,
					Balance:     10,
					Concurrency: 3,
				},
				Group: &APIKeyAuthGroupSnapshot{
					ID:                    groupID,
					Name:                  "openai",
					Platform:              PlatformOpenAI,
					Status:                StatusActive,
					SubscriptionType:      SubscriptionTypeStandard,
					RateMultiplier:        1,
					AllowMessagesDispatch: true,
					DefaultMappedModel:    "gpt-5.4",
				},
			},
		}, nil
	}

	apiKey, err := svc.GetByKey(context.Background(), "k-legacy")
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&repoCalls))
	require.NotNil(t, apiKey.Group)
	require.Equal(t, "gpt-5.4-nano", apiKey.Group.MessagesDispatchModelConfig.OpusMappedModel)
}

func TestAPIKeyService_GetByKey_NegativeCache(t *testing.T) {
	cache := &authCacheStub{}
	repo := &authRepoStub{
		getByKeyForAuth: func(ctx context.Context, key string) (*APIKey, error) {
			return nil, errors.New("unexpected repo call")
		},
	}
	cfg := &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			L2TTLSeconds:       60,
			NegativeTTLSeconds: 30,
		},
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)
	cache.getAuthCache = func(ctx context.Context, key string) (*APIKeyAuthCacheEntry, error) {
		return &APIKeyAuthCacheEntry{NotFound: true}, nil
	}

	_, err := svc.GetByKey(context.Background(), "missing")
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
}

func TestAPIKeyService_GetByKey_CacheMissStoresL2(t *testing.T) {
	cache := &authCacheStub{}
	repo := &authRepoStub{
		getByKeyForAuth: func(ctx context.Context, key string) (*APIKey, error) {
			return &APIKey{
				ID:     5,
				UserID: 7,
				Status: StatusActive,
				User: &User{
					ID:          7,
					Status:      StatusActive,
					Role:        RoleUser,
					Balance:     12,
					Concurrency: 2,
				},
			}, nil
		},
	}
	cfg := &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			L2TTLSeconds:       60,
			NegativeTTLSeconds: 30,
		},
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)
	cache.getAuthCache = func(ctx context.Context, key string) (*APIKeyAuthCacheEntry, error) {
		return nil, redis.Nil
	}

	apiKey, err := svc.GetByKey(context.Background(), "k2")
	require.NoError(t, err)
	require.Equal(t, int64(5), apiKey.ID)
	require.Len(t, cache.setAuthKeys, 1)
}

func TestAPIKeyService_GetByKey_UsesL1Cache(t *testing.T) {
	var calls int32
	cache := &authCacheStub{}
	repo := &authRepoStub{
		getByKeyForAuth: func(ctx context.Context, key string) (*APIKey, error) {
			atomic.AddInt32(&calls, 1)
			return &APIKey{
				ID:     21,
				UserID: 3,
				Status: StatusActive,
				User: &User{
					ID:          3,
					Status:      StatusActive,
					Role:        RoleUser,
					Balance:     5,
					Concurrency: 2,
				},
			}, nil
		},
	}
	cfg := &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			L1Size:       1000,
			L1TTLSeconds: 60,
		},
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)
	require.NotNil(t, svc.authCacheL1)

	_, err := svc.GetByKey(context.Background(), "k-l1")
	require.NoError(t, err)
	svc.authCacheL1.Wait()
	hashes := APIKeyLookupHashes("k-l1", cfg)
	require.NotEmpty(t, hashes)
	cacheKey := APIKeyAuthCacheKeyFromHash(hashes[0])
	require.NotEmpty(t, cacheKey)
	_, ok := svc.authCacheL1.Get(cacheKey)
	require.True(t, ok)
	_, err = svc.GetByKey(context.Background(), "k-l1")
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestAPIKeyService_InvalidateAuthCacheByUserID(t *testing.T) {
	cache := &authCacheStub{}
	repo := &authRepoStub{
		listKeysByUserID: func(ctx context.Context, userID int64) ([]string, error) {
			return []string{"k1", "k2"}, nil
		},
	}
	cfg := &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			L2TTLSeconds:       60,
			NegativeTTLSeconds: 30,
		},
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)

	svc.InvalidateAuthCacheByUserID(context.Background(), 7)
	expected := append(expectedPlaintextAuthCacheKeys("k1", cfg), expectedPlaintextAuthCacheKeys("k2", cfg)...)
	require.Equal(t, expected, cache.deleteAuthKeys)
}

func TestAPIKeyService_InvalidateAuthCacheByGroupID(t *testing.T) {
	cache := &authCacheStub{}
	repo := &authRepoStub{
		listKeysByGroupID: func(ctx context.Context, groupID int64) ([]string, error) {
			return []string{"k1", "k2"}, nil
		},
	}
	cfg := &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			L2TTLSeconds: 60,
		},
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)

	svc.InvalidateAuthCacheByGroupID(context.Background(), 9)
	expected := append(expectedPlaintextAuthCacheKeys("k1", cfg), expectedPlaintextAuthCacheKeys("k2", cfg)...)
	require.Equal(t, expected, cache.deleteAuthKeys)
}

func TestAPIKeyService_InvalidateAuthCacheByKey(t *testing.T) {
	cache := &authCacheStub{}
	repo := &authRepoStub{
		listKeysByUserID: func(ctx context.Context, userID int64) ([]string, error) {
			return nil, nil
		},
	}
	cfg := &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			L2TTLSeconds: 60,
		},
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)

	svc.InvalidateAuthCacheByKey(context.Background(), "k1")
	require.Equal(t, expectedPlaintextAuthCacheKeys("k1", cfg), cache.deleteAuthKeys)
}

func TestAPIKeyService_GetByKey_CachesNegativeOnRepoMiss(t *testing.T) {
	cache := &authCacheStub{}
	repo := &authRepoStub{
		getByKeyForAuth: func(ctx context.Context, key string) (*APIKey, error) {
			return nil, ErrAPIKeyNotFound
		},
	}
	cfg := &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			L2TTLSeconds:       60,
			NegativeTTLSeconds: 30,
		},
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)
	cache.getAuthCache = func(ctx context.Context, key string) (*APIKeyAuthCacheEntry, error) {
		return nil, redis.Nil
	}

	_, err := svc.GetByKey(context.Background(), "missing")
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
	require.Equal(t, expectedPlaintextAuthCacheKeys("missing", cfg), cache.setAuthKeys)
}

func TestAPIKeyService_GetByKey_SingleflightCollapses(t *testing.T) {
	var calls int32
	cache := &authCacheStub{}
	repo := &authRepoStub{
		getByKeyForAuth: func(ctx context.Context, key string) (*APIKey, error) {
			atomic.AddInt32(&calls, 1)
			time.Sleep(50 * time.Millisecond)
			return &APIKey{
				ID:     11,
				UserID: 2,
				Status: StatusActive,
				User: &User{
					ID:          2,
					Status:      StatusActive,
					Role:        RoleUser,
					Balance:     1,
					Concurrency: 1,
				},
			}, nil
		},
	}
	cfg := &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			Singleflight: true,
		},
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)

	start := make(chan struct{})
	wg := sync.WaitGroup{}
	errs := make([]error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, err := svc.GetByKey(context.Background(), "k1")
			errs[idx] = err
		}(i)
	}
	close(start)
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestAPIKeyService_GetByKey_RejectsHashBackedTombstoneInLegacyFallback(t *testing.T) {
	cache := &authCacheStub{}
	repo := &authRepoStub{
		getByKeyForAuth: func(ctx context.Context, key string) (*APIKey, error) {
			if key != "__hashed__stored_tombstone" {
				return nil, ErrAPIKeyNotFound
			}
			return &APIKey{
				ID:            88,
				UserID:        9,
				KeyHash:       strings.Repeat("a", 64),
				KeyHashAlg:    APIKeyHashAlgSHA256,
				KeyLookupHash: strings.Repeat("b", 64),
				Status:        StatusActive,
				User: &User{
					ID:          9,
					Status:      StatusActive,
					Role:        RoleUser,
					Balance:     8,
					Concurrency: 2,
				},
			}, nil
		},
	}
	cfg := &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			L2TTLSeconds:       60,
			NegativeTTLSeconds: 30,
		},
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)
	cache.getAuthCache = func(ctx context.Context, key string) (*APIKeyAuthCacheEntry, error) {
		return nil, redis.Nil
	}

	_, err := svc.GetByKey(context.Background(), "__hashed__stored_tombstone")
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
	require.Equal(t, expectedPlaintextAuthCacheKeys("__hashed__stored_tombstone", cfg), cache.setAuthKeys)
}

func TestAPIKeyService_GetByKey_AuthenticatesGeneratedCafePassHashRow(t *testing.T) {
	cache := &authCacheStub{}
	cfg := &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			L2TTLSeconds:       60,
			NegativeTTLSeconds: 30,
		},
	}
	svc := NewAPIKeyService(nil, nil, nil, nil, nil, cache, cfg)
	key, err := svc.GenerateKey()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(key, "cafepass-"))

	storageHash := APIKeyStorageHash(key, cfg)
	lookupHash := APIKeyLookupHashValue(key)
	var calls []string
	repo := &authRepoStub{
		getByKeyForAuth: func(ctx context.Context, lookupToken string) (*APIKey, error) {
			calls = append(calls, lookupToken)
			hashes, ok := DecodeAPIKeyLookupToken(lookupToken)
			require.True(t, ok)
			require.Len(t, hashes, 1)
			if hashes[0].Alg != APIKeyHashAlgLookupSHA256 || hashes[0].Hash != lookupHash {
				return nil, ErrAPIKeyNotFound
			}
			return &APIKey{
				ID:            78,
				UserID:        9,
				KeyHash:       storageHash.Hash,
				KeyHashAlg:    storageHash.Alg,
				KeyLookupHash: lookupHash,
				KeyPrefix:     APIKeyPrefix(key),
				Status:        StatusActive,
				User: &User{
					ID:          9,
					Status:      StatusActive,
					Role:        RoleUser,
					Balance:     8,
					Concurrency: 2,
				},
			}, nil
		},
	}
	svc.apiKeyRepo = repo
	cache.getAuthCache = func(ctx context.Context, key string) (*APIKeyAuthCacheEntry, error) {
		return nil, redis.Nil
	}

	apiKey, err := svc.GetByKey(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, int64(78), apiKey.ID)
	require.Equal(t, key, apiKey.Key)
	require.Len(t, calls, 1)
	require.NotContains(t, calls[0], key)
	require.Contains(t, cache.setAuthKeys, APIKeyAuthCacheKeyFromHash(APIKeyLookupHash{Alg: APIKeyHashAlgLookupSHA256, Hash: lookupHash}))
}

func TestAPIKeyService_GetByKey_AuthenticatesLegacySKHashRow(t *testing.T) {
	cache := &authCacheStub{}
	cfg := &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			L2TTLSeconds:       60,
			NegativeTTLSeconds: 30,
		},
	}
	key := "sk-legacy-hash-key-123456"
	storageHash := APIKeyStorageHash(key, cfg)
	lookupHash := APIKeyLookupHashValue(key)
	var calls []string
	repo := &authRepoStub{
		getByKeyForAuth: func(ctx context.Context, lookupToken string) (*APIKey, error) {
			calls = append(calls, lookupToken)
			hashes, ok := DecodeAPIKeyLookupToken(lookupToken)
			require.True(t, ok)
			require.Len(t, hashes, 1)
			if hashes[0].Alg != APIKeyHashAlgLookupSHA256 || hashes[0].Hash != lookupHash {
				return nil, ErrAPIKeyNotFound
			}
			return &APIKey{
				ID:            79,
				UserID:        9,
				KeyHash:       storageHash.Hash,
				KeyHashAlg:    storageHash.Alg,
				KeyLookupHash: lookupHash,
				KeyPrefix:     APIKeyPrefix(key),
				Status:        StatusActive,
				User: &User{
					ID:          9,
					Status:      StatusActive,
					Role:        RoleUser,
					Balance:     8,
					Concurrency: 2,
				},
			}, nil
		},
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)
	cache.getAuthCache = func(ctx context.Context, key string) (*APIKeyAuthCacheEntry, error) {
		return nil, redis.Nil
	}

	apiKey, err := svc.GetByKey(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, int64(79), apiKey.ID)
	require.Equal(t, key, apiKey.Key)
	require.Len(t, calls, 1)
	require.NotContains(t, calls[0], key)
	require.Contains(t, cache.setAuthKeys, APIKeyAuthCacheKeyFromHash(APIKeyLookupHash{Alg: APIKeyHashAlgLookupSHA256, Hash: lookupHash}))
}
func TestAPIKeyService_GetByKey_FallsBackToLegacyPlaintextRow(t *testing.T) {
	cache := &authCacheStub{}
	var calls []string
	repo := &authRepoStub{
		getByKeyForAuth: func(ctx context.Context, key string) (*APIKey, error) {
			calls = append(calls, key)
			if key != "sk-legacy-plaintext-key" {
				return nil, ErrAPIKeyNotFound
			}
			return &APIKey{
				ID:     77,
				UserID: 9,
				Key:    key,
				Status: StatusActive,
				User: &User{
					ID:          9,
					Status:      StatusActive,
					Role:        RoleUser,
					Balance:     8,
					Concurrency: 2,
				},
			}, nil
		},
	}
	cfg := &config.Config{
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			L2TTLSeconds:       60,
			NegativeTTLSeconds: 30,
		},
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)
	cache.getAuthCache = func(ctx context.Context, key string) (*APIKeyAuthCacheEntry, error) {
		return nil, redis.Nil
	}

	apiKey, err := svc.GetByKey(context.Background(), "sk-legacy-plaintext-key")
	require.NoError(t, err)
	require.Equal(t, int64(77), apiKey.ID)
	require.Greater(t, len(calls), 1)
	require.Equal(t, "sk-legacy-plaintext-key", calls[len(calls)-1])
	require.NotEmpty(t, cache.setAuthKeys)
}
