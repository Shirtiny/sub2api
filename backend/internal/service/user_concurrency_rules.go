package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/sync/semaphore"
	"golang.org/x/sync/singleflight"
)

const SettingKeyUserConcurrencyRules = "user_concurrency_rules"

const (
	concurrencyRulesTTL      = 5 * time.Second
	concurrencyRulesMaxStale = 30 * time.Second
	concurrencyRulesTimeout  = 3 * time.Second
	concurrencyRulesErrorTTL = time.Second
)

// Lower bounds are inclusive; each following tier supplies the previous upper
// bound. The first tier also covers negative balances (billing checks are separate).
type BalanceConcurrencyRule struct {
	MinBalance  float64 `json:"min_balance"`
	Concurrency int     `json:"concurrency"`
}

type UserConcurrencyRules struct {
	BalanceTiers []BalanceConcurrencyRule `json:"balance_tiers"`
}

func DefaultBalanceConcurrencyRules() []BalanceConcurrencyRule {
	return []BalanceConcurrencyRule{{0, 1}, {20, 2}, {100, 3}}
}

func (r UserConcurrencyRules) Validate() error {
	invalid := func(message string) error {
		return infraerrors.BadRequest("INVALID_CONCURRENCY_RULES", message)
	}
	if len(r.BalanceTiers) < 1 || len(r.BalanceTiers) > 20 {
		return invalid("Provide between 1 and 20 balance tiers")
	}
	for i, tier := range r.BalanceTiers {
		if math.IsNaN(tier.MinBalance) || math.IsInf(tier.MinBalance, 0) || tier.MinBalance < 0 || tier.MinBalance > 1e12 {
			return invalid("Balance thresholds must be finite numbers between 0 and 1000000000000")
		}
		if (i == 0 && tier.MinBalance != 0) || (i > 0 && tier.MinBalance <= r.BalanceTiers[i-1].MinBalance) {
			return invalid("Balance thresholds must start at 0 and increase strictly")
		}
		if tier.Concurrency < 1 || tier.Concurrency > 1000 {
			return invalid("Concurrency must be an integer between 1 and 1000")
		}
	}
	return nil
}

func balanceConcurrencyWithRules(balance float64, tiers []BalanceConcurrencyRule) int {
	if len(tiers) == 0 {
		tiers = DefaultBalanceConcurrencyRules()
	}
	for i := len(tiers) - 1; i > 0; i-- {
		if balance >= tiers[i].MinBalance {
			return tiers[i].Concurrency
		}
	}
	return tiers[0].Concurrency
}

type cachedUserConcurrencyRules struct {
	tiers    []BalanceConcurrencyRule
	loadedAt time.Time
	err      error
	retryAt  time.Time
}

type concurrencyRulesState struct {
	gate      *semaphore.Weighted
	refresh   singleflight.Group
	ctx       context.Context
	cancel    context.CancelFunc
	startOnce sync.Once
	done      chan struct{}
}

func (s *SettingService) concurrencyRulesState() *concurrencyRulesState {
	s.userConcurrencyRulesInit.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.userConcurrencyRulesState = &concurrencyRulesState{
			gate: semaphore.NewWeighted(1), ctx: ctx, cancel: cancel, done: make(chan struct{}),
		}
	})
	return s.userConcurrencyRulesState
}

// A failed refresh has a short cooldown, but never advances the last successful
// load time: retained WS turns may only use genuinely bounded stale rules.
func (s *SettingService) concurrencyRulesResult() (UserConcurrencyRules, error, bool) {
	if cached := s.userConcurrencyRulesCache.Load(); cached != nil && time.Since(cached.loadedAt) < concurrencyRulesTTL {
		return UserConcurrencyRules{slices.Clone(cached.tiers)}, nil, true
	} else if cached != nil && cached.err != nil && time.Now().Before(cached.retryAt) {
		return UserConcurrencyRules{}, cached.err, true
	}
	return UserConcurrencyRules{}, nil, false
}

// GetUserConcurrencyRules coalesces refreshes without tying the shared read to
// any one caller's lifetime. Each caller can cancel its own wait immediately.
func (s *SettingService) GetUserConcurrencyRules(ctx context.Context) (UserConcurrencyRules, error) {
	if err := ctx.Err(); err != nil {
		return UserConcurrencyRules{}, err
	}
	if s == nil {
		return UserConcurrencyRules{DefaultBalanceConcurrencyRules()}, nil
	}
	if rules, err, ok := s.concurrencyRulesResult(); ok {
		return rules, err
	}
	ctx, cancel := context.WithTimeout(ctx, concurrencyRulesTimeout)
	defer cancel()
	state := s.concurrencyRulesState()
	result := state.refresh.DoChan(SettingKeyUserConcurrencyRules, func() (any, error) {
		return s.refreshUserConcurrencyRules(state)
	})
	select {
	case <-ctx.Done():
		return UserConcurrencyRules{}, ctx.Err()
	case result := <-result:
		if result.Err != nil {
			return UserConcurrencyRules{}, result.Err
		}
		rules, ok := result.Val.(UserConcurrencyRules)
		if !ok {
			return UserConcurrencyRules{}, errors.New("invalid concurrency rules refresh result")
		}
		return UserConcurrencyRules{slices.Clone(rules.BalanceTiers)}, nil
	}
}

func (s *SettingService) refreshUserConcurrencyRules(state *concurrencyRulesState) (UserConcurrencyRules, error) {
	// The timeout covers both waiting behind a save and the actual database read.
	ctx, cancel := context.WithTimeout(state.ctx, concurrencyRulesTimeout)
	defer cancel()
	if err := state.gate.Acquire(ctx, 1); err != nil {
		return UserConcurrencyRules{}, err
	}
	defer state.gate.Release(1)
	if rules, err, ok := s.concurrencyRulesResult(); ok {
		return rules, err
	}
	rules, err := s.loadUserConcurrencyRules(ctx)
	if err != nil {
		cached := &cachedUserConcurrencyRules{err: err, retryAt: time.Now().Add(concurrencyRulesErrorTTL)}
		if previous := s.userConcurrencyRulesCache.Load(); previous != nil {
			cached.tiers, cached.loadedAt = previous.tiers, previous.loadedAt
		}
		s.userConcurrencyRulesCache.Store(cached)
		return UserConcurrencyRules{}, err
	}
	s.userConcurrencyRulesCache.Store(&cachedUserConcurrencyRules{tiers: slices.Clone(rules.BalanceTiers), loadedAt: time.Now()})
	return rules, nil
}

func (s *SettingService) loadUserConcurrencyRules(ctx context.Context) (UserConcurrencyRules, error) {
	if s.settingRepo == nil {
		return UserConcurrencyRules{}, errors.New("concurrency settings repository unavailable")
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyUserConcurrencyRules)
	rules := UserConcurrencyRules{DefaultBalanceConcurrencyRules()}
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return UserConcurrencyRules{}, fmt.Errorf("load concurrency rules: %w", err)
	}
	if err == nil {
		rules = UserConcurrencyRules{}
		if err := json.Unmarshal([]byte(raw), &rules); err != nil {
			return UserConcurrencyRules{}, fmt.Errorf("decode concurrency rules: %w", err)
		}
		if err := rules.Validate(); err != nil {
			return UserConcurrencyRules{}, fmt.Errorf("invalid stored concurrency rules: %w", err)
		}
	}
	return rules, nil
}

func (s *SettingService) SetUserConcurrencyRules(ctx context.Context, rules UserConcurrencyRules) error {
	if err := rules.Validate(); err != nil {
		return err
	}
	rules.BalanceTiers = slices.Clone(rules.BalanceTiers)
	raw, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	if s == nil || s.settingRepo == nil {
		return errors.New("concurrency settings repository unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, concurrencyRulesTimeout)
	defer cancel()
	state := s.concurrencyRulesState()
	if err := state.gate.Acquire(ctx, 1); err != nil {
		return err
	}
	defer state.gate.Release(1)
	if err := s.settingRepo.Set(ctx, SettingKeyUserConcurrencyRules, string(raw)); err != nil {
		return fmt.Errorf("save concurrency rules: %w", err)
	}
	// Read and write share the gate, so an older refresh cannot undo this save.
	s.userConcurrencyRulesCache.Store(&cachedUserConcurrencyRules{tiers: rules.BalanceTiers, loadedAt: time.Now()})
	return nil
}

// Keep rules fresh even when a long-lived WS connection is otherwise idle.
// Otherwise the first turn after 30s of inactivity would fail closed merely
// because no request had triggered a refresh, even with a healthy database.
func (s *SettingService) StartUserConcurrencyRulesRefresh() {
	state := s.concurrencyRulesState()
	state.startOnce.Do(func() {
		go func() {
			defer close(state.done)
			ticker := time.NewTicker(concurrencyRulesTTL / 2)
			defer ticker.Stop()
			for {
				_, _ = s.GetUserConcurrencyRules(state.ctx)
				select {
				case <-state.ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	})
}

func (s *SettingService) StopUserConcurrencyRulesRefresh() {
	if s == nil {
		return
	}
	state := s.concurrencyRulesState()
	state.cancel()
	// Also make stopping an unstarted service safe; it must not start later.
	state.startOnce.Do(func() { close(state.done) })
	<-state.done
}

// CachedUserConcurrencyRules never blocks a retained WS turn on a DB read.
// Cold admission warms this cache. Refresh is asynchronous, deduplicated and
// timeout-bounded. After 30s without a successful refresh admission fails closed.
func (s *SettingService) CachedUserConcurrencyRules() (UserConcurrencyRules, error) {
	if s == nil {
		return UserConcurrencyRules{DefaultBalanceConcurrencyRules()}, nil
	}
	cached := s.userConcurrencyRulesCache.Load()
	if cached == nil || (time.Since(cached.loadedAt) >= concurrencyRulesTTL && !time.Now().Before(cached.retryAt)) {
		if s.userConcurrencyRulesRefreshing.CompareAndSwap(false, true) {
			go func() {
				defer s.userConcurrencyRulesRefreshing.Store(false)
				_, _ = s.GetUserConcurrencyRules(context.Background())
			}()
		}
	}
	if cached == nil || time.Since(cached.loadedAt) >= concurrencyRulesMaxStale {
		return UserConcurrencyRules{}, errors.New("concurrency rules cache unavailable")
	}
	return UserConcurrencyRules{slices.Clone(cached.tiers)}, nil
}
