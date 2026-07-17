package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type persistentAuthEpochCacheStub struct {
	APIKeyCache
	mu     sync.Mutex
	epoch  uint64
	getErr error
}

func (s *persistentAuthEpochCacheStub) GetAuthCacheEpoch(context.Context, string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return 0, s.getErr
	}
	return s.epoch, nil
}

func (s *persistentAuthEpochCacheStub) IncrementAuthCacheEpoch(context.Context, string) error {
	s.mu.Lock()
	s.epoch++
	s.mu.Unlock()
	return nil
}

func TestAPIKeyAuthEpochLeaseInvalidatesWithoutCacheIO(t *testing.T) {
	service := NewAPIKeyService(nil, nil, nil, nil, nil, nil, nil)
	key := &APIKey{Key: "cafepass-auth-epoch-test"}
	lease := service.CaptureAuthEpochLease(key)
	require.True(t, service.ValidateAuthEpochLease(lease))

	service.InvalidateAuthCacheByKey(context.Background(), key.Key)
	require.False(t, service.ValidateAuthEpochLease(lease))
	require.True(t, service.ValidateAuthEpochLease(service.CaptureAuthEpochLease(key)))
}

func TestAPIKeyAuthEpochLeaseChecksPersistentGeneration(t *testing.T) {
	cache := &persistentAuthEpochCacheStub{}
	service := NewAPIKeyService(nil, nil, nil, nil, nil, cache, nil)
	key := &APIKey{Key: "cafepass-persistent-auth-epoch"}
	lease, err := service.captureAuthEpochLeaseContext(context.Background(), key)
	require.NoError(t, err)
	valid, err := service.ValidateAuthEpochLeaseContext(context.Background(), lease)
	require.NoError(t, err)
	require.True(t, valid)

	require.NoError(t, cache.IncrementAuthCacheEpoch(context.Background(), "ignored"))
	valid, err = service.ValidateAuthEpochLeaseContext(context.Background(), lease)
	require.NoError(t, err)
	require.False(t, valid)
}

func TestAPIKeyAuthEpochLeaseFailsClosedWhenPersistentGenerationUnavailable(t *testing.T) {
	cache := &persistentAuthEpochCacheStub{}
	service := NewAPIKeyService(nil, nil, nil, nil, nil, cache, nil)
	lease, err := service.captureAuthEpochLeaseContext(context.Background(), &APIKey{Key: "cafepass-epoch-outage"})
	require.NoError(t, err)

	cache.mu.Lock()
	cache.getErr = errors.New("redis unavailable")
	cache.mu.Unlock()
	valid, err := service.ValidateAuthEpochLeaseContext(context.Background(), lease)
	require.ErrorContains(t, err, "redis unavailable")
	require.False(t, valid)
}
