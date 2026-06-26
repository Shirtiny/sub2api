//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type rotateAPIKeyRepoStub struct {
	apiKey       *APIKey
	rotated      *APIKey
	rotateGuard  APIKeyRotationGuard
	rotateErr    error
	existsChecks []string
}

func (s *rotateAPIKeyRepoStub) Create(context.Context, *APIKey) error {
	panic("unexpected Create call")
}
func (s *rotateAPIKeyRepoStub) GetByID(ctx context.Context, id int64) (*APIKey, error) {
	if s.apiKey == nil || s.apiKey.ID != id {
		return nil, ErrAPIKeyNotFound
	}
	clone := *s.apiKey
	return &clone, nil
}
func (s *rotateAPIKeyRepoStub) GetKeyAndOwnerID(context.Context, int64) (string, int64, error) {
	panic("unexpected GetKeyAndOwnerID call")
}
func (s *rotateAPIKeyRepoStub) GetByKey(context.Context, string) (*APIKey, error) {
	panic("unexpected GetByKey call")
}
func (s *rotateAPIKeyRepoStub) GetByKeyForAuth(context.Context, string) (*APIKey, error) {
	panic("unexpected GetByKeyForAuth call")
}
func (s *rotateAPIKeyRepoStub) Update(context.Context, *APIKey) error {
	panic("unexpected Update call")
}
func (s *rotateAPIKeyRepoStub) RotateKey(ctx context.Context, key *APIKey, guard APIKeyRotationGuard) error {
	if s.rotateErr != nil {
		return s.rotateErr
	}
	clone := *key
	s.rotated = &clone
	s.rotateGuard = guard
	return nil
}
func (s *rotateAPIKeyRepoStub) Delete(context.Context, int64) error { panic("unexpected Delete call") }
func (s *rotateAPIKeyRepoStub) DeleteWithAudit(context.Context, int64) error {
	panic("unexpected DeleteWithAudit call")
}
func (s *rotateAPIKeyRepoStub) ListByUserID(context.Context, int64, pagination.PaginationParams, APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserID call")
}
func (s *rotateAPIKeyRepoStub) VerifyOwnership(context.Context, int64, []int64) ([]int64, error) {
	panic("unexpected VerifyOwnership call")
}
func (s *rotateAPIKeyRepoStub) CountByUserID(context.Context, int64) (int64, error) {
	panic("unexpected CountByUserID call")
}
func (s *rotateAPIKeyRepoStub) ExistsByKey(ctx context.Context, key string) (bool, error) {
	s.existsChecks = append(s.existsChecks, key)
	return false, nil
}
func (s *rotateAPIKeyRepoStub) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]APIKey, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID call")
}
func (s *rotateAPIKeyRepoStub) SearchAPIKeys(context.Context, int64, string, int) ([]APIKey, error) {
	panic("unexpected SearchAPIKeys call")
}
func (s *rotateAPIKeyRepoStub) ClearGroupIDByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected ClearGroupIDByGroupID call")
}
func (s *rotateAPIKeyRepoStub) UpdateGroupIDByUserAndGroup(context.Context, int64, int64, int64) (int64, error) {
	panic("unexpected UpdateGroupIDByUserAndGroup call")
}
func (s *rotateAPIKeyRepoStub) CountByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected CountByGroupID call")
}
func (s *rotateAPIKeyRepoStub) ListKeysByUserID(context.Context, int64) ([]string, error) {
	panic("unexpected ListKeysByUserID call")
}
func (s *rotateAPIKeyRepoStub) ListKeysByGroupID(context.Context, int64) ([]string, error) {
	panic("unexpected ListKeysByGroupID call")
}
func (s *rotateAPIKeyRepoStub) IncrementQuotaUsed(context.Context, int64, float64) (float64, error) {
	panic("unexpected IncrementQuotaUsed call")
}
func (s *rotateAPIKeyRepoStub) UpdateLastUsed(context.Context, int64, time.Time) error {
	panic("unexpected UpdateLastUsed call")
}
func (s *rotateAPIKeyRepoStub) IncrementRateLimitUsage(context.Context, int64, float64) error {
	panic("unexpected IncrementRateLimitUsage call")
}
func (s *rotateAPIKeyRepoStub) ResetRateLimitWindows(context.Context, int64) error {
	panic("unexpected ResetRateLimitWindows call")
}
func (s *rotateAPIKeyRepoStub) GetRateLimitData(context.Context, int64) (*APIKeyRateLimitData, error) {
	panic("unexpected GetRateLimitData call")
}

func TestAPIKeyService_Rotate_ReplacesKeyMaterialAndInvalidatesCaches(t *testing.T) {
	oldLookup := "oldlookup"
	repo := &rotateAPIKeyRepoStub{
		apiKey: &APIKey{
			ID:            10,
			UserID:        20,
			KeyHash:       strings.Repeat("a", 64),
			KeyHashAlg:    APIKeyHashAlgSHA256,
			KeyLookupHash: oldLookup,
			KeyPrefix:     "sk-old-prefix",
			Name:          "main",
			Status:        StatusActive,
		},
	}
	cache := &authCacheStub{}
	cfg := &config.Config{}
	cfg.Default.APIKeyPrefix = "sk-test-"
	cfg.Security.APIKeyHashSecret = "rotate-test-secret-with-at-least-32-bytes"
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)

	rotated, err := svc.Rotate(context.Background(), 10, 20)
	require.NoError(t, err)
	require.NotEmpty(t, rotated.Key)
	require.True(t, strings.HasPrefix(rotated.Key, "sk-test-"))
	require.Equal(t, APIKeyPrefix(rotated.Key), rotated.KeyPrefix)
	require.Equal(t, rotated.Key, repo.rotated.Key)
	require.Equal(t, APIKeyHashAlgHMACSHA256, repo.rotated.KeyHashAlg)
	require.Len(t, repo.rotated.KeyHash, 64)
	require.Len(t, repo.rotated.KeyLookupHash, 64)
	require.Equal(t, APIKeyRotationGuard{
		KeyHash:       strings.Repeat("a", 64),
		KeyHashAlg:    APIKeyHashAlgSHA256,
		KeyLookupHash: oldLookup,
	}, repo.rotateGuard)
	require.Contains(t, cache.deleteAuthKeys, APIKeyAuthCacheKeyFromHash(APIKeyLookupHash{Alg: APIKeyHashAlgLookupSHA256, Hash: oldLookup}))
	require.NotEmpty(t, repo.existsChecks)
}

func TestAPIKeyService_Rotate_RejectsNonOwner(t *testing.T) {
	repo := &rotateAPIKeyRepoStub{apiKey: &APIKey{ID: 10, UserID: 20}}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})

	_, err := svc.Rotate(context.Background(), 10, 99)
	require.ErrorIs(t, err, ErrInsufficientPerms)
	require.Nil(t, repo.rotated)
}

func TestAPIKeyService_GenerateKey_UsesCafePassDefaultPrefix(t *testing.T) {
	svc := NewAPIKeyService(nil, nil, nil, nil, nil, nil, &config.Config{})

	key, err := svc.GenerateKey()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(key, "cafepass-"))
}

func TestAPIKeyService_Rotate_PropagatesConcurrentRotationConflict(t *testing.T) {
	repo := &rotateAPIKeyRepoStub{
		apiKey: &APIKey{
			ID:            10,
			UserID:        20,
			KeyHash:       strings.Repeat("b", 64),
			KeyHashAlg:    APIKeyHashAlgSHA256,
			KeyLookupHash: strings.Repeat("c", 64),
			Status:        StatusActive,
		},
		rotateErr: ErrAPIKeyRotated,
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})

	_, err := svc.Rotate(context.Background(), 10, 20)
	require.ErrorIs(t, err, ErrAPIKeyRotated)
	require.Nil(t, repo.rotated)
}
