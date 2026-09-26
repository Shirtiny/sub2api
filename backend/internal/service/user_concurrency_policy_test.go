package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBalanceConcurrencyBoundaries(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		balance float64
		want    int
	}{{-1, 1}, {0, 1}, {19.99999999, 1}, {20, 2}, {99.99999999, 2}, {100, 3}, {1000, 3}, {math.NaN(), 1}} {
		user := &User{Concurrency: 32, Balance: tc.balance}
		require.Equal(t, tc.want, user.EffectiveConcurrencyAt(now), "balance %v", tc.balance)
	}
}

func TestSubscriptionConcurrencyPriorityAndWindows(t *testing.T) {
	now := time.Now()
	user := &User{Concurrency: 32, Balance: 1000}
	user.SubscriptionPeriods = []SubscriptionPeriod{{SubscriptionID: 1, StartsAt: now, ExpiresAt: now.Add(time.Hour)}}
	require.Equal(t, 3, user.EffectiveConcurrencyAt(now.Add(-time.Nanosecond)), "not yet effective")
	require.Equal(t, 2, user.EffectiveConcurrencyAt(now), "legacy active subscription defaults to 2")
	user.PlanConcurrencyEntitlements = []PlanConcurrencyEntitlement{
		{SubscriptionID: 1, Concurrency: 1, StartsAt: now, ExpiresAt: now.Add(time.Hour)},
		{SubscriptionID: 2, Concurrency: 20, StartsAt: now.Add(time.Hour), ExpiresAt: now.Add(2 * time.Hour)},
	}
	require.Equal(t, 1, user.EffectiveConcurrencyAt(now), "subscription may be lower than balance tier")
	require.Equal(t, 20, user.EffectiveConcurrencyAt(now.Add(time.Hour)), "new term applies at its start")
	require.Equal(t, 3, user.EffectiveConcurrencyAt(now.Add(2*time.Hour)), "expiry is exclusive")
	user.Balance = 0
	require.Equal(t, 1, user.EffectiveConcurrencyAt(now.Add(2*time.Hour)))
}

func TestEffectiveConcurrencyCacheOnlyTracksBalanceAcrossWSTurns(t *testing.T) {
	cache := &wsCacheOnlyBillingCache{balance: 100}
	svc := newWSCacheOnlyBillingService(cache) // Panics if any DB fallback occurs.
	user := &User{ID: 10, Balance: 999, Concurrency: 32}
	for _, balance := range []float64{100, 99, 20, 19, 100} {
		cache.balance = balance
		got, err := svc.EffectiveConcurrencyCacheOnly(context.Background(), user, time.Now())
		require.NoError(t, err)
		require.Equal(t, BalanceConcurrency(balance), got)
	}
	require.Equal(t, float64(999), user.Balance, "do not mutate a shared connection snapshot")
	cache.balanceErr = errors.New("cache unavailable")
	_, err := svc.EffectiveConcurrencyCacheOnly(context.Background(), user, time.Now())
	require.ErrorIs(t, err, ErrBillingServiceUnavailable)
	now := time.Now()
	user.PlanConcurrencyEntitlements = []PlanConcurrencyEntitlement{{Concurrency: 6, StartsAt: now, ExpiresAt: now.Add(time.Minute)}}
	got, err := svc.EffectiveConcurrencyCacheOnly(context.Background(), user, now)
	require.NoError(t, err, "active subscriptions do not depend on the balance cache")
	require.Equal(t, 6, got)
	_, err = svc.EffectiveConcurrencyCacheOnly(context.Background(), user, now.Add(time.Minute))
	require.ErrorIs(t, err, ErrBillingServiceUnavailable, "expired subscription cannot keep its old limit")
}
