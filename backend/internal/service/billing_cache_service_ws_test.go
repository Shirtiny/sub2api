package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type wsCacheOnlyBillingCache struct {
	BillingCache
	balance         float64
	balanceErr      error
	setBalanceCalls int
	subscription    *SubscriptionCacheData
	subscriptionErr error
	rateLimit       *APIKeyRateLimitCacheData
	rateLimitErr    error
	quota           *UserPlatformQuotaCacheEntry
	quotaHit        bool
	quotaErr        error
}

func (c *wsCacheOnlyBillingCache) GetUserBalance(context.Context, int64) (float64, error) {
	return c.balance, c.balanceErr
}

func (c *wsCacheOnlyBillingCache) SetUserBalance(_ context.Context, _ int64, balance float64) error {
	c.balance = balance
	c.balanceErr = nil
	c.setBalanceCalls++
	return nil
}

func (c *wsCacheOnlyBillingCache) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	return c.subscription, c.subscriptionErr
}

func (c *wsCacheOnlyBillingCache) GetAPIKeyRateLimit(context.Context, int64) (*APIKeyRateLimitCacheData, error) {
	return c.rateLimit, c.rateLimitErr
}

func (c *wsCacheOnlyBillingCache) GetUserPlatformQuotaCache(context.Context, int64, string) (*UserPlatformQuotaCacheEntry, bool, error) {
	return c.quota, c.quotaHit, c.quotaErr
}

type wsPanicUserRepository struct{ UserRepository }

func (wsPanicUserRepository) GetByID(context.Context, int64) (*User, error) {
	panic("cache-only billing must not load a user")
}

type wsPanicSubscriptionRepository struct{ UserSubscriptionRepository }

func (wsPanicSubscriptionRepository) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	panic("cache-only billing must not load a subscription")
}

type wsPanicAPIKeyRateLimitLoader struct{}

func (wsPanicAPIKeyRateLimitLoader) GetRateLimitData(context.Context, int64) (*APIKeyRateLimitData, error) {
	panic("cache-only billing must not load API key rate limits")
}

type wsPanicPlatformQuotaRepository struct{ UserPlatformQuotaRepository }

func (wsPanicPlatformQuotaRepository) GetByUserPlatform(context.Context, int64, string) (*UserPlatformQuotaRecord, error) {
	panic("cache-only billing must not load platform quota")
}

type wsBalanceUserRepository struct {
	UserRepository
	balance float64
	calls   int
}

func (r *wsBalanceUserRepository) GetByID(context.Context, int64) (*User, error) {
	r.calls++
	return &User{Balance: r.balance}, nil
}

func newWSCacheOnlyBillingService(cache BillingCache) *BillingCacheService {
	return &BillingCacheService{
		cache:                 cache,
		userRepo:              wsPanicUserRepository{},
		subRepo:               wsPanicSubscriptionRepository{},
		apiKeyRateLimitLoader: wsPanicAPIKeyRateLimitLoader{},
		userPlatformQuotaRepo: wsPanicPlatformQuotaRepository{},
		cfg:                   &config.Config{},
	}
}

func TestCheckBillingEligibilityCacheOnlyFailsClosedWithoutDBFallback(t *testing.T) {
	svc := newWSCacheOnlyBillingService(&wsCacheOnlyBillingCache{balanceErr: errors.New("cache miss")})
	user := &User{ID: 10, UserGroupRPMOverrideResolved: true}

	err := svc.CheckBillingEligibilityCacheOnly(context.Background(), user, nil, nil, nil, PlatformOpenAI)
	require.ErrorIs(t, err, ErrBillingServiceUnavailable)
}

func TestCheckBillingEligibilityCacheOnlyUsesOnlyWarmEntries(t *testing.T) {
	now := time.Now()
	cache := &wsCacheOnlyBillingCache{
		balance: 12,
		rateLimit: &APIKeyRateLimitCacheData{
			Usage5h:  1,
			Window5h: now.Unix(),
		},
		quota: &UserPlatformQuotaCacheEntry{
			SchemaVersion:      UserPlatformQuotaCacheSchemaV1,
			DailyWindowStart:   &now,
			WeeklyWindowStart:  &now,
			MonthlyWindowStart: &now,
		},
		quotaHit: true,
	}
	svc := newWSCacheOnlyBillingService(cache)
	user := &User{ID: 10, UserGroupRPMOverrideResolved: true}
	apiKey := &APIKey{ID: 20, RateLimit5h: 10}

	require.NoError(t, svc.CheckBillingEligibilityCacheOnly(
		context.Background(), user, apiKey, nil, nil, PlatformOpenAI,
	))
}

func TestCheckBillingEligibilityCacheOnlySubscriptionMissDoesNotLoadRepository(t *testing.T) {
	svc := newWSCacheOnlyBillingService(&wsCacheOnlyBillingCache{subscriptionErr: errors.New("cache miss")})
	user := &User{ID: 10, UserGroupRPMOverrideResolved: true}
	group := &Group{ID: 30, SubscriptionType: SubscriptionTypeSubscription}
	subscription := &UserSubscription{
		Status:    SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	err := svc.CheckBillingEligibilityCacheOnly(context.Background(), user, nil, group, subscription, PlatformOpenAI)
	require.ErrorIs(t, err, ErrBillingServiceUnavailable)
}

func TestPrimeLongLivedBillingEligibilitySettlesBalanceBeforeHotPath(t *testing.T) {
	cache := &wsCacheOnlyBillingCache{balanceErr: errors.New("cache miss")}
	repo := &wsBalanceUserRepository{balance: 12}
	svc := &BillingCacheService{
		cache:    cache,
		userRepo: repo,
		cfg:      &config.Config{},
	}
	user := &User{ID: 10, UserGroupRPMOverrideResolved: true}

	require.NoError(t, svc.PrimeLongLivedBillingEligibility(
		context.Background(), user, nil, nil, nil, "",
	))
	require.Equal(t, 1, repo.calls)
	require.Equal(t, 1, cache.setBalanceCalls)
	require.NoError(t, svc.CheckBillingEligibilityCacheOnly(
		context.Background(), user, nil, nil, nil, "",
	))
	require.Equal(t, 1, repo.calls, "hot path must not return to the repository")
}
