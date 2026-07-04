package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type billingCacheWorkerStub struct {
	balanceUpdates          int64
	subscriptionUpdates     int64
	subscriptionInvalidates int64
	subscriptionCache       *SubscriptionCacheData
	subscriptionCacheErr    error
}

func (b *billingCacheWorkerStub) GetUserBalance(ctx context.Context, userID int64) (float64, error) {
	return 0, errors.New("not implemented")
}

func (b *billingCacheWorkerStub) SetUserBalance(ctx context.Context, userID int64, balance float64) error {
	atomic.AddInt64(&b.balanceUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) DeductUserBalance(ctx context.Context, userID int64, amount float64) error {
	atomic.AddInt64(&b.balanceUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) InvalidateUserBalance(ctx context.Context, userID int64) error {
	return nil
}

func (b *billingCacheWorkerStub) GetSubscriptionCache(ctx context.Context, userID, groupID int64) (*SubscriptionCacheData, error) {
	if b.subscriptionCacheErr != nil {
		return nil, b.subscriptionCacheErr
	}
	if b.subscriptionCache != nil {
		return b.subscriptionCache, nil
	}
	return nil, errors.New("not implemented")
}

func (b *billingCacheWorkerStub) SetSubscriptionCache(ctx context.Context, userID, groupID int64, data *SubscriptionCacheData) error {
	atomic.AddInt64(&b.subscriptionUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, cost float64) error {
	atomic.AddInt64(&b.subscriptionUpdates, 1)
	return nil
}

func (b *billingCacheWorkerStub) InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error {
	atomic.AddInt64(&b.subscriptionInvalidates, 1)
	return nil
}

func (b *billingCacheWorkerStub) GetAPIKeyRateLimit(ctx context.Context, keyID int64) (*APIKeyRateLimitCacheData, error) {
	return nil, errors.New("not implemented")
}

func (b *billingCacheWorkerStub) SetAPIKeyRateLimit(ctx context.Context, keyID int64, data *APIKeyRateLimitCacheData) error {
	return nil
}

func (b *billingCacheWorkerStub) UpdateAPIKeyRateLimitUsage(ctx context.Context, keyID int64, cost float64) error {
	return nil
}

func (b *billingCacheWorkerStub) InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error {
	return nil
}

func (b *billingCacheWorkerStub) GetUserPlatformQuotaCache(ctx context.Context, userID int64, platform string) (*UserPlatformQuotaCacheEntry, bool, error) {
	return nil, false, nil
}

func (b *billingCacheWorkerStub) SetUserPlatformQuotaCache(ctx context.Context, userID int64, platform string, entry *UserPlatformQuotaCacheEntry, ttl time.Duration) error {
	return nil
}

func (b *billingCacheWorkerStub) DeleteUserPlatformQuotaCache(ctx context.Context, userID int64, platform string) error {
	return nil
}

func (b *billingCacheWorkerStub) IncrUserPlatformQuotaUsageCache(ctx context.Context, userID int64, platform string, cost float64, ttl time.Duration, markDirty bool) error {
	return nil
}

func (b *billingCacheWorkerStub) PopDirtyUserPlatformQuotaKeys(ctx context.Context, n int) ([]UserPlatformQuotaKey, error) {
	return nil, nil
}

func (b *billingCacheWorkerStub) ReaddDirtyUserPlatformQuotaKeys(ctx context.Context, keys []UserPlatformQuotaKey) error {
	return nil
}

func (b *billingCacheWorkerStub) BatchGetUserPlatformQuotaCache(ctx context.Context, keys []UserPlatformQuotaKey) ([]*UserPlatformQuotaCacheEntry, error) {
	return nil, nil
}

func TestBillingCacheServiceQueueHighLoad(t *testing.T) {
	cache := &billingCacheWorkerStub{}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(svc.Stop)

	start := time.Now()
	for i := 0; i < cacheWriteBufferSize*2; i++ {
		svc.QueueDeductBalance(1, 1)
	}
	require.Less(t, time.Since(start), 2*time.Second)

	svc.QueueUpdateSubscriptionUsage(1, 2, 1.5)

	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&cache.balanceUpdates) > 0
	}, 2*time.Second, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&cache.subscriptionUpdates) > 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestBillingCacheServiceEnqueueAfterStopReturnsFalse(t *testing.T) {
	cache := &billingCacheWorkerStub{}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	svc.Stop()

	enqueued := svc.enqueueCacheWrite(cacheWriteTask{
		kind:   cacheWriteDeductBalance,
		userID: 1,
		amount: 1,
	})
	require.False(t, enqueued)
}

func TestCheckBillingEligibility_InvalidatesStaleDailyUsageCacheAfterWindowReset(t *testing.T) {
	dailyLimit := 300.0
	oldWindowStart := time.Now().Add(-25 * time.Hour)
	expiresAt := time.Now().Add(24 * time.Hour)
	cache := &billingCacheWorkerStub{
		subscriptionCache: &SubscriptionCacheData{
			Status:       SubscriptionStatusActive,
			ExpiresAt:    expiresAt,
			DailyUsage:   dailyLimit,
			WeeklyUsage:  10,
			MonthlyUsage: 10,
			Version:      1,
		},
	}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(svc.Stop)

	err := svc.CheckBillingEligibility(
		context.Background(),
		&User{ID: 43, Status: StatusActive},
		nil,
		&Group{
			ID:               24,
			Status:           StatusActive,
			SubscriptionType: SubscriptionTypeSubscription,
			DailyLimitUSD:    &dailyLimit,
		},
		&UserSubscription{
			ID:               730,
			UserID:           43,
			GroupID:          24,
			Status:           SubscriptionStatusActive,
			ExpiresAt:        expiresAt,
			DailyWindowStart: &oldWindowStart,
			DailyUsageUSD:    0,
		},
		"",
	)

	require.NoError(t, err)
	require.Equal(t, int64(1), atomic.LoadInt64(&cache.subscriptionInvalidates))
}

func TestCheckBillingEligibility_UsesVirtualCustomMultiplierForSubscriptionLimits(t *testing.T) {
	sourceDailyLimit := 250.0
	expiresAt := time.Now().Add(24 * time.Hour)
	customExpiresAt := time.Now().Add(24 * time.Hour)
	windowStart := time.Now()
	cache := &billingCacheWorkerStub{
		subscriptionCache: &SubscriptionCacheData{
			Status:       SubscriptionStatusActive,
			ExpiresAt:    expiresAt,
			DailyUsage:   300,
			WeeklyUsage:  0,
			MonthlyUsage: 0,
			Version:      time.Now().Unix(),
		},
	}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(svc.Stop)

	multiplier := 4
	planID := int64(7)
	sourceGroupID := int64(24)
	customDisplayName := "[4x]Special#43"
	subscription := &UserSubscription{
		ID:                  730,
		UserID:              43,
		GroupID:             sourceGroupID,
		Status:              SubscriptionStatusActive,
		ExpiresAt:           expiresAt,
		DailyWindowStart:    &windowStart,
		DailyUsageUSD:       300,
		CustomMultiplier:    &multiplier,
		CustomSourcePlanID:  &planID,
		CustomSourceGroupID: &sourceGroupID,
		CustomExpiresAt:     &customExpiresAt,
		CustomDisplayName:   customDisplayName,
	}

	err := svc.CheckBillingEligibility(
		context.Background(),
		&User{ID: 43, Status: StatusActive},
		&APIKey{ID: 99, UserID: 43, GroupID: &sourceGroupID, Status: StatusActive},
		&Group{
			ID:               sourceGroupID,
			Status:           StatusActive,
			SubscriptionType: SubscriptionTypeSubscription,
			DailyLimitUSD:    &sourceDailyLimit,
		},
		subscription,
		"openai",
	)

	require.NoError(t, err, "source-group API keys should use the virtual custom 4x quota, not the base source quota")

	cache.subscriptionCache.DailyUsage = 1000
	subscription.DailyUsageUSD = 1000
	err = svc.CheckBillingEligibility(
		context.Background(),
		&User{ID: 43, Status: StatusActive},
		nil,
		&Group{
			ID:               sourceGroupID,
			Status:           StatusActive,
			SubscriptionType: SubscriptionTypeSubscription,
			DailyLimitUSD:    &sourceDailyLimit,
		},
		subscription,
		"openai",
	)

	require.ErrorIs(t, err, ErrDailyLimitExceeded, "effective custom quota should still cap usage at source limit * multiplier")
}

func TestCheckBillingEligibility_RejectsCacheDailyLimitInCurrentWindow(t *testing.T) {
	dailyLimit := 300.0
	windowStart := time.Now()
	expiresAt := time.Now().Add(24 * time.Hour)
	cache := &billingCacheWorkerStub{
		subscriptionCache: &SubscriptionCacheData{
			Status:       SubscriptionStatusActive,
			ExpiresAt:    expiresAt,
			DailyUsage:   dailyLimit,
			WeeklyUsage:  10,
			MonthlyUsage: 10,
			Version:      time.Now().Unix(),
		},
	}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(svc.Stop)

	err := svc.CheckBillingEligibility(
		context.Background(),
		&User{ID: 43, Status: StatusActive},
		nil,
		&Group{
			ID:               24,
			Status:           StatusActive,
			SubscriptionType: SubscriptionTypeSubscription,
			DailyLimitUSD:    &dailyLimit,
		},
		&UserSubscription{
			ID:               730,
			UserID:           43,
			GroupID:          24,
			Status:           SubscriptionStatusActive,
			ExpiresAt:        expiresAt,
			DailyWindowStart: &windowStart,
			DailyUsageUSD:    0,
		},
		"",
	)

	require.ErrorIs(t, err, ErrDailyLimitExceeded)
	require.Equal(t, int64(0), atomic.LoadInt64(&cache.subscriptionInvalidates))
}

func TestCheckBillingEligibility_RejectsCurrentSubscriptionDailyLimit(t *testing.T) {
	dailyLimit := 300.0
	expiresAt := time.Now().Add(24 * time.Hour)
	cache := &billingCacheWorkerStub{}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(svc.Stop)

	err := svc.CheckBillingEligibility(
		context.Background(),
		&User{ID: 43, Status: StatusActive},
		nil,
		&Group{
			ID:               24,
			Status:           StatusActive,
			SubscriptionType: SubscriptionTypeSubscription,
			DailyLimitUSD:    &dailyLimit,
		},
		&UserSubscription{
			ID:            730,
			UserID:        43,
			GroupID:       24,
			Status:        SubscriptionStatusActive,
			ExpiresAt:     expiresAt,
			DailyUsageUSD: dailyLimit + 0.01,
		},
		"",
	)

	require.ErrorIs(t, err, ErrDailyLimitExceeded)
	require.Equal(t, int64(0), atomic.LoadInt64(&cache.subscriptionInvalidates))
}
