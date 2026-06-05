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

func TestBillingCacheServiceQueueHighLoad(t *testing.T) {
	cache := &billingCacheWorkerStub{}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
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
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
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
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
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
	)

	require.NoError(t, err)
	require.Equal(t, int64(1), atomic.LoadInt64(&cache.subscriptionInvalidates))
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
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
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
	)

	require.ErrorIs(t, err, ErrDailyLimitExceeded)
	require.Equal(t, int64(0), atomic.LoadInt64(&cache.subscriptionInvalidates))
}

func TestCheckBillingEligibility_RejectsCurrentSubscriptionDailyLimit(t *testing.T) {
	dailyLimit := 300.0
	expiresAt := time.Now().Add(24 * time.Hour)
	cache := &billingCacheWorkerStub{}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
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
			DailyUsageUSD: dailyLimit,
		},
	)

	require.ErrorIs(t, err, ErrDailyLimitExceeded)
	require.Equal(t, int64(0), atomic.LoadInt64(&cache.subscriptionInvalidates))
}
