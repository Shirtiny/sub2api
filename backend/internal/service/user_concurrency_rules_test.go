package service

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

type concurrencyRulesRepo struct {
	SettingRepository
	mu       sync.Mutex
	value    string
	getErr   error
	setErr   error
	reads    int
	writes   int
	getStart chan struct{}
	getAllow chan struct{}
	setStart chan struct{}
	setAllow chan struct{}
}

func (r *concurrencyRulesRepo) GetValue(ctx context.Context, _ string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	if r.getStart != nil {
		select {
		case r.getStart <- struct{}{}:
		default:
		}
		select {
		case <-r.getAllow:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if r.getErr != nil {
		return "", r.getErr
	}
	if r.value == "" {
		return "", ErrSettingNotFound
	}
	return r.value, nil
}

func (r *concurrencyRulesRepo) Set(ctx context.Context, _ string, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.setStart != nil {
		select {
		case r.setStart <- struct{}{}:
		default:
		}
		select {
		case <-r.setAllow:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if r.setErr != nil {
		return r.setErr
	}
	r.value = value
	r.writes++
	return nil
}

func TestUserConcurrencyRulesValidationAndBounds(t *testing.T) {
	for _, tiers := range [][]BalanceConcurrencyRule{
		nil, {}, {{1, 1}}, {{0, 0}}, {{0, 1001}}, {{0, -1}},
		{{0, 1}, {0, 2}}, {{0, 1}, {20, 2}, {10, 3}}, {{0, 1}, {-1, 2}},
		{{0, 1}, {math.NaN(), 2}}, {{0, 1}, {math.Inf(1), 2}}, {{0, 1}, {1e12 + 1, 2}},
		make([]BalanceConcurrencyRule, 21),
	} {
		require.Error(t, (UserConcurrencyRules{tiers}).Validate(), "%v", tiers)
	}
	rules := UserConcurrencyRules{[]BalanceConcurrencyRule{{0, 4}, {10.5, 2}, {150, 8}}}
	require.NoError(t, rules.Validate(), "administrators may also choose decreasing limits")
	for _, tc := range []struct {
		balance float64
		want    int
	}{{-1, 4}, {0, 4}, {10.499, 4}, {10.5, 2}, {149.9999, 2}, {150, 8}, {1000000, 8}, {math.NaN(), 4}} {
		user := &User{Balance: tc.balance, BalanceConcurrencyRules: rules.BalanceTiers}
		require.Equal(t, tc.want, user.EffectiveConcurrencyAt(time.Now()))
	}
	user := &User{Balance: 200, BalanceConcurrencyRules: rules.BalanceTiers,
		SubscriptionPeriods: []SubscriptionPeriod{{StartsAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour)}}}
	require.Equal(t, 2, user.EffectiveConcurrencyAt(time.Now()), "active subscription overrides custom balance limits")
}

func TestUserConcurrencyRulesPersistenceIsolationAndErrors(t *testing.T) {
	ctx := context.Background()
	repo := &concurrencyRulesRepo{}
	svc := NewSettingService(repo, nil)
	defaults, err := svc.GetUserConcurrencyRules(ctx)
	require.NoError(t, err)
	require.Equal(t, DefaultBalanceConcurrencyRules(), defaults.BalanceTiers)
	rules := UserConcurrencyRules{[]BalanceConcurrencyRule{{0, 4}, {50, 7}}}
	require.NoError(t, svc.SetUserConcurrencyRules(ctx, rules))
	rules.BalanceTiers[0].Concurrency = 999
	got, err := svc.GetUserConcurrencyRules(ctx)
	require.NoError(t, err)
	require.Equal(t, 4, got.BalanceTiers[0].Concurrency)
	got.BalanceTiers[0].Concurrency = 998
	got, err = svc.CachedUserConcurrencyRules()
	require.NoError(t, err)
	require.Equal(t, 4, got.BalanceTiers[0].Concurrency, "callers cannot mutate shared cache")
	reloaded, err := NewSettingService(repo, nil).GetUserConcurrencyRules(ctx)
	require.NoError(t, err)
	require.Equal(t, got, reloaded, "configuration survives process restart")
	require.Error(t, svc.SetUserConcurrencyRules(ctx, UserConcurrencyRules{}))
	require.Equal(t, 1, repo.writes)
	repo.setErr = errors.New("write failed")
	require.Error(t, svc.SetUserConcurrencyRules(ctx, defaults))
	got, err = svc.GetUserConcurrencyRules(ctx)
	require.NoError(t, err)
	require.Equal(t, 4, got.BalanceTiers[0].Concurrency, "failed save does not change active rules")

	for _, raw := range []string{"not json", "{}", "null", `{"balance_tiers":[]}`} {
		_, err := NewSettingService(&concurrencyRulesRepo{value: raw}, nil).GetUserConcurrencyRules(ctx)
		require.Error(t, err, "corrupted stored policy cannot grant default limits")
	}
}

func TestUserConcurrencyRulesWSTurnsAndBoundedStaleness(t *testing.T) {
	ctx := context.Background()
	repo := &concurrencyRulesRepo{}
	settings := NewSettingService(repo, nil)
	_, err := settings.GetUserConcurrencyRules(ctx)
	require.NoError(t, err)
	cache := &wsCacheOnlyBillingCache{balance: 50}
	svc := newWSCacheOnlyBillingService(cache)
	svc.concurrencySettings = settings
	user := &User{ID: 1, Balance: 1000}
	limit, err := svc.EffectiveConcurrencyCacheOnly(ctx, user, time.Now())
	require.NoError(t, err)
	require.Equal(t, 2, limit)
	require.NoError(t, settings.SetUserConcurrencyRules(ctx, UserConcurrencyRules{[]BalanceConcurrencyRule{{0, 5}, {50, 9}}}))
	limit, err = svc.EffectiveConcurrencyCacheOnly(ctx, user, time.Now())
	require.NoError(t, err)
	require.Equal(t, 9, limit, "already open WS uses new rules, not its connection snapshot")

	// Deliberately block the settings repository. The retained turn must return
	// immediately using bounded stale rules; refresh happens off the admission path.
	repo.getStart, repo.getAllow = make(chan struct{}, 1), make(chan struct{})
	defer close(repo.getAllow)
	settings.userConcurrencyRulesCache.Store(&cachedUserConcurrencyRules{tiers: []BalanceConcurrencyRule{{0, 6}}, loadedAt: time.Now().Add(-6 * time.Second)})
	done := make(chan int, 1)
	go func() { rules, _ := settings.CachedUserConcurrencyRules(); done <- rules.BalanceTiers[0].Concurrency }()
	select {
	case limit := <-done:
		require.Equal(t, 6, limit)
	case <-time.After(time.Second):
		t.Fatal("WS rule read blocked on DB")
	}
	<-repo.getStart
	settings.userConcurrencyRulesCache.Store(&cachedUserConcurrencyRules{tiers: []BalanceConcurrencyRule{{0, 6}}, loadedAt: time.Now().Add(-31 * time.Second)})
	_, err = svc.EffectiveConcurrencyCacheOnly(ctx, user, time.Now())
	require.ErrorIs(t, err, ErrBillingServiceUnavailable, "fail closed after freshness deadline")
}

func TestUserConcurrencyRulesRefreshCannotOverwriteSave(t *testing.T) {
	ctx := context.Background()
	repo := &concurrencyRulesRepo{getStart: make(chan struct{}, 1), getAllow: make(chan struct{})}
	svc := NewSettingService(repo, nil)
	readDone := make(chan error, 1)
	go func() { _, err := svc.GetUserConcurrencyRules(ctx); readDone <- err }()
	<-repo.getStart
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- svc.SetUserConcurrencyRules(ctx, UserConcurrencyRules{[]BalanceConcurrencyRule{{0, 7}}})
	}()
	close(repo.getAllow)
	require.NoError(t, <-readDone)
	require.NoError(t, <-writeDone)
	rules, err := svc.GetUserConcurrencyRules(ctx)
	require.NoError(t, err)
	require.Equal(t, 7, rules.BalanceTiers[0].Concurrency)
}

func TestUserConcurrencyRulesIdleRefreshAndStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := &concurrencyRulesRepo{value: `{"balance_tiers":[{"min_balance":0,"concurrency":9}]}`}
		svc := NewSettingService(repo, nil)
		defer svc.StopUserConcurrencyRulesRefresh()
		svc.StartUserConcurrencyRulesRefresh()
		svc.StartUserConcurrencyRulesRefresh() // Must not create a second worker.
		synctest.Wait()
		require.Equal(t, 9, svc.userConcurrencyRulesCache.Load().tiers[0].Concurrency)

		// Another instance saves a lower limit. Advance a full minute without
		// admission traffic, using Go's test clock rather than real sleeps.
		repo.mu.Lock()
		repo.value = `{"balance_tiers":[{"min_balance":0,"concurrency":1}]}`
		repo.mu.Unlock()
		time.Sleep(2 * concurrencyRulesMaxStale)
		synctest.Wait()
		require.Less(t, time.Since(svc.userConcurrencyRulesCache.Load().loadedAt), concurrencyRulesTTL)

		ws := newWSCacheOnlyBillingService(&wsCacheOnlyBillingCache{balance: 50})
		ws.concurrencySettings = svc
		limit, err := ws.EffectiveConcurrencyCacheOnly(context.Background(), &User{ID: 1}, time.Now())
		require.NoError(t, err, "healthy idle WS turns must not be closed because of traffic-dependent cache expiry")
		require.Equal(t, 1, limit, "an idle instance also observes lower limits saved on another instance")

		svc.StopUserConcurrencyRulesRefresh()
		svc.StopUserConcurrencyRulesRefresh()
		svc.StartUserConcurrencyRulesRefresh() // Cannot restart a stopped service.
		synctest.Wait()
		svc.userConcurrencyRulesCache.Store(nil)
		time.Sleep(2 * concurrencyRulesTTL)
		synctest.Wait()
		require.Nil(t, svc.userConcurrencyRulesCache.Load())

		unstarted := &SettingService{}
		unstarted.StopUserConcurrencyRulesRefresh()
		unstarted.StartUserConcurrencyRulesRefresh()
		synctest.Wait()
		require.Nil(t, unstarted.userConcurrencyRulesCache.Load())
	})
}

func TestUserConcurrencyRulesCanceledWaitersDoNotCancelSharedRefresh(t *testing.T) {
	repo := &concurrencyRulesRepo{getStart: make(chan struct{}, 1), getAllow: make(chan struct{})}
	svc := NewSettingService(repo, nil)
	t.Cleanup(svc.StopUserConcurrencyRulesRefresh)
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	first := make(chan error, 1)
	go func() { _, err := svc.GetUserConcurrencyRules(firstCtx); first <- err }()
	<-repo.getStart

	waitCtx, cancelWait := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelWait()
	start := time.Now()
	_, err := svc.GetUserConcurrencyRules(waitCtx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), time.Second, "a canceled follower cannot wait behind database IO")
	cancelFirst()
	select {
	case err := <-first:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("canceled initiating caller remained blocked")
	}

	// Cancelling every current caller still must not poison the shared refresh.
	close(repo.getAllow)
	rules, err := svc.GetUserConcurrencyRules(context.Background())
	require.NoError(t, err)
	require.Equal(t, DefaultBalanceConcurrencyRules(), rules.BalanceTiers)
	require.Equal(t, 1, repo.reads)
}

func TestUserConcurrencyRulesFailuresAreCoalescedAndBackedOff(t *testing.T) {
	dbErr := errors.New("database unavailable")
	repo := &concurrencyRulesRepo{getErr: dbErr}
	svc := NewSettingService(repo, nil)
	t.Cleanup(svc.StopUserConcurrencyRulesRefresh)
	loadedAt := time.Now().Add(-6 * time.Second)
	svc.userConcurrencyRulesCache.Store(&cachedUserConcurrencyRules{tiers: []BalanceConcurrencyRule{{0, 4}}, loadedAt: loadedAt})

	var wg sync.WaitGroup
	results := make(chan error, 32)
	for range cap(results) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.GetUserConcurrencyRules(context.Background())
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.ErrorIs(t, err, dbErr)
	}
	require.Equal(t, 1, repo.reads, "a failed refresh is shared, including callers arriving just after it finishes")
	cached := *svc.userConcurrencyRulesCache.Load()
	require.Equal(t, loadedAt, cached.loadedAt, "a failure cannot extend the stale-rules safety window")
	rules, err := svc.CachedUserConcurrencyRules()
	require.NoError(t, err)
	require.Equal(t, 4, rules.BalanceTiers[0].Concurrency)

	// Retry after cooldown and recover without restarting the service.
	cached.retryAt = time.Now().Add(-time.Second)
	svc.userConcurrencyRulesCache.Store(&cached)
	repo.mu.Lock()
	repo.getErr = nil
	repo.value = `{"balance_tiers":[{"min_balance":0,"concurrency":2}]}`
	repo.mu.Unlock()
	rules, err = svc.GetUserConcurrencyRules(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, rules.BalanceTiers[0].Concurrency)
	repo.mu.Lock()
	require.Equal(t, 2, repo.reads)
	repo.mu.Unlock()
}

func TestUserConcurrencyRulesSaveRecoversFailedCache(t *testing.T) {
	svc := NewSettingService(&concurrencyRulesRepo{getErr: errors.New("read failed")}, nil)
	_, err := svc.GetUserConcurrencyRules(context.Background())
	require.Error(t, err)
	rules := UserConcurrencyRules{[]BalanceConcurrencyRule{{0, 5}}}
	require.NoError(t, svc.SetUserConcurrencyRules(context.Background(), rules))
	got, err := svc.GetUserConcurrencyRules(context.Background())
	require.NoError(t, err, "save must immediately clear a cached refresh error")
	require.Equal(t, rules, got)
}

func TestUserConcurrencyRulesOutageCannotExtendWSSafetyWindow(t *testing.T) {
	svc := NewSettingService(&concurrencyRulesRepo{getErr: errors.New("database unavailable")}, nil)
	t.Cleanup(svc.StopUserConcurrencyRulesRefresh)
	loadedAt := time.Now().Add(-31 * time.Second)
	svc.userConcurrencyRulesCache.Store(&cachedUserConcurrencyRules{tiers: []BalanceConcurrencyRule{{0, 9}}, loadedAt: loadedAt})
	_, err := svc.GetUserConcurrencyRules(context.Background())
	require.Error(t, err)
	_, err = svc.CachedUserConcurrencyRules()
	require.Error(t, err, "an error cooldown must not authorize admission with indefinitely stale rules")
	require.Equal(t, loadedAt, svc.userConcurrencyRulesCache.Load().loadedAt)

	ws := newWSCacheOnlyBillingService(&wsCacheOnlyBillingCache{balanceErr: errors.New("balance unavailable")})
	ws.concurrencySettings = svc
	now := time.Now()
	user := &User{PlanConcurrencyEntitlements: []PlanConcurrencyEntitlement{{Concurrency: 4, StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)}}}
	limit, err := ws.EffectiveConcurrencyCacheOnly(context.Background(), user, now)
	require.NoError(t, err, "active WS subscriptions remain independent of both balance rules and balance cache")
	require.Equal(t, 4, limit)
	_, err = ws.EffectiveConcurrencyCacheOnly(context.Background(), user, now.Add(time.Hour))
	require.ErrorIs(t, err, ErrBillingServiceUnavailable, "subscription expiry cannot retain its old limit")
}

func TestUserConcurrencyRulesSaveCancellation(t *testing.T) {
	t.Run("waiting behind refresh", func(t *testing.T) {
		repo := &concurrencyRulesRepo{getStart: make(chan struct{}, 1), getAllow: make(chan struct{})}
		svc := NewSettingService(repo, nil)
		t.Cleanup(svc.StopUserConcurrencyRulesRefresh)
		readDone := make(chan error, 1)
		go func() { _, err := svc.GetUserConcurrencyRules(context.Background()); readDone <- err }()
		<-repo.getStart
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		start := time.Now()
		err := svc.SetUserConcurrencyRules(ctx, UserConcurrencyRules{[]BalanceConcurrencyRule{{0, 7}}})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Less(t, time.Since(start), time.Second)
		close(repo.getAllow)
		require.NoError(t, <-readDone)
		require.Zero(t, repo.writes)
	})
	t.Run("database write", func(t *testing.T) {
		repo := &concurrencyRulesRepo{setStart: make(chan struct{}, 1), setAllow: make(chan struct{})}
		svc := NewSettingService(repo, nil)
		_, err := svc.GetUserConcurrencyRules(context.Background())
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		err = svc.SetUserConcurrencyRules(ctx, UserConcurrencyRules{[]BalanceConcurrencyRule{{0, 7}}})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Zero(t, repo.writes)
		rules, err := svc.GetUserConcurrencyRules(context.Background())
		require.NoError(t, err)
		require.Equal(t, DefaultBalanceConcurrencyRules(), rules.BalanceTiers)
	})
}

func TestUserConcurrencyRulesStopCancelsBackgroundRead(t *testing.T) {
	repo := &concurrencyRulesRepo{getStart: make(chan struct{}, 1), getAllow: make(chan struct{})}
	svc := NewSettingService(repo, nil)
	svc.StartUserConcurrencyRulesRefresh()
	<-repo.getStart
	start := time.Now()
	svc.StopUserConcurrencyRulesRefresh()
	require.Less(t, time.Since(start), time.Second)
	// Acquire the gate after shutdown to verify the shared DB read was canceled
	// too, rather than only abandoning the worker's wait for its result.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	state := svc.concurrencyRulesState()
	require.NoError(t, state.gate.Acquire(ctx, 1))
	state.gate.Release(1)
}
