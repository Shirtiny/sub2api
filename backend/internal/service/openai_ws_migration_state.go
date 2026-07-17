package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrOpenAIWSReconnectMigrationRequested = errors.New("openai websocket reconnect migration requested")
var ErrOpenAIWSLocalAdmission = errors.New("openai websocket local admission rejected")

// OpenAIWSMigrationCacheState is loaded only at physical connection admission
// and modified only by the pre-signal Redis transaction. It is never consulted
// by the frame relay hot path.
type OpenAIWSMigrationCacheState struct {
	WindowStartedAtUnixMilli int64
	ObservedAtUnixMilli      int64
	MigrationCount           int
	BindingGeneration        uint64
	ExcludedUntilUnixMilli   map[int64]int64
}

type OpenAIWSMigrationCacheDecision struct {
	State      OpenAIWSMigrationCacheState
	Admitted   bool
	Idempotent bool
	Exhausted  bool
}

// OpenAIWSMigrationCache is intentionally an optional extension of
// GatewayCache so legacy test doubles do not need frame-unrelated methods.
// Production reconnect migration, however, fails closed when this shared
// implementation is unavailable; a process-local fallback is unsafe across
// gateway nodes.
type OpenAIWSMigrationCache interface {
	LoadOpenAIWSMigrationState(
		ctx context.Context,
		groupID int64,
		sessionHash string,
	) (OpenAIWSMigrationCacheState, error)
	AdmitOpenAIWSReconnectMigration(
		ctx context.Context,
		groupID int64,
		sessionHash string,
		accountID int64,
		controlID string,
		bindingGeneration uint64,
		middleRouteDisposition OpenAIWSMiddleRouteDisposition,
		nowUnixMilli int64,
		window time.Duration,
		exclusionTTL time.Duration,
		maxMigrations int,
	) (OpenAIWSMigrationCacheDecision, error)
}

type OpenAIWSMigrationAdmission struct {
	Active             bool
	MigrationCount     int
	MaxMigrations      int
	Exhausted          bool
	BindingGeneration  uint64
	ExcludedAccountIDs map[int64]struct{}
}

type OpenAIWSReconnectSignalAdmission struct {
	OpenAIWSMigrationAdmission
	Allowed    bool
	Idempotent bool
}

func (s *OpenAIGatewayService) LoadOpenAIWSMigrationAdmission(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
) (OpenAIWSMigrationAdmission, error) {
	limits, active := s.openAIWSMigrationLimits(sessionHash)
	if !active {
		return OpenAIWSMigrationAdmission{}, nil
	}
	cache, ok := s.cache.(OpenAIWSMigrationCache)
	if !ok || cache == nil {
		return OpenAIWSMigrationAdmission{Active: true}, errors.New("shared openai ws migration cache is unavailable")
	}
	state, err := cache.LoadOpenAIWSMigrationState(ctx, derefGroupID(groupID), sessionHash)
	if err != nil {
		return OpenAIWSMigrationAdmission{Active: true}, fmt.Errorf("load openai ws migration state: %w", err)
	}
	return buildOpenAIWSMigrationAdmission(state, time.Now(), limits.maxMigrations), nil
}

// AdmitOpenAIWSReconnectSignal is the only correctness path that may authorize
// a synthetic reconnect event. The backing implementation atomically records
// the control id, advances the binding generation, and increments the bounded
// window counter. Only an explicit exclude disposition removes the current
// middle route and sticky binding; retain reconnects the same Aether route so
// it can select another internal Codex account.
func (s *OpenAIGatewayService) AdmitOpenAIWSReconnectSignal(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	accountID int64,
	controlID string,
	bindingGeneration uint64,
	middleRouteDisposition OpenAIWSMiddleRouteDisposition,
) (OpenAIWSReconnectSignalAdmission, error) {
	limits, active := s.openAIWSMigrationLimits(sessionHash)
	if !active {
		return OpenAIWSReconnectSignalAdmission{}, errors.New("openai ws reconnect migration is inactive")
	}
	controlID = strings.TrimSpace(controlID)
	if accountID <= 0 || !validAetherWSOpaqueID(controlID) || bindingGeneration == 0 || !middleRouteDisposition.Valid() {
		return OpenAIWSReconnectSignalAdmission{}, errors.New("openai ws reconnect migration identity is invalid")
	}
	cache, ok := s.cache.(OpenAIWSMigrationCache)
	if !ok || cache == nil {
		return OpenAIWSReconnectSignalAdmission{}, errors.New("shared openai ws migration cache is unavailable")
	}
	now := time.Now()
	decision, err := cache.AdmitOpenAIWSReconnectMigration(
		ctx,
		derefGroupID(groupID),
		sessionHash,
		accountID,
		controlID,
		bindingGeneration,
		middleRouteDisposition,
		now.UnixMilli(),
		limits.window,
		limits.exclusionTTL,
		limits.maxMigrations,
	)
	if err != nil {
		return OpenAIWSReconnectSignalAdmission{}, fmt.Errorf("admit openai ws reconnect migration: %w", err)
	}
	admission := buildOpenAIWSMigrationAdmission(decision.State, now, limits.maxMigrations)
	admission.Exhausted = decision.Exhausted
	return OpenAIWSReconnectSignalAdmission{
		OpenAIWSMigrationAdmission: admission,
		Allowed:                    decision.Admitted,
		Idempotent:                 decision.Idempotent,
	}, nil
}

type openAIWSMigrationLimits struct {
	maxMigrations int
	window        time.Duration
	exclusionTTL  time.Duration
}

func (s *OpenAIGatewayService) openAIWSMigrationLimits(sessionHash string) (openAIWSMigrationLimits, bool) {
	if s == nil || s.cfg == nil || !s.cfg.Gateway.OpenAIWS.ReconnectMigrationEnabled || strings.TrimSpace(sessionHash) == "" {
		return openAIWSMigrationLimits{}, false
	}
	wsCfg := s.cfg.Gateway.OpenAIWS
	if wsCfg.MaxMigrationsPerSession <= 0 || wsCfg.MigrationWindowSeconds <= 0 {
		return openAIWSMigrationLimits{}, false
	}
	exclusionTTL := time.Duration(wsCfg.RouteMinDwellSeconds) * time.Second
	if exclusionTTL < time.Second {
		exclusionTTL = time.Second
	}
	window := time.Duration(wsCfg.MigrationWindowSeconds) * time.Second
	if exclusionTTL > window {
		exclusionTTL = window
	}
	return openAIWSMigrationLimits{
		maxMigrations: wsCfg.MaxMigrationsPerSession,
		window:        window,
		exclusionTTL:  exclusionTTL,
	}, true
}

func buildOpenAIWSMigrationAdmission(state OpenAIWSMigrationCacheState, now time.Time, maxMigrations int) OpenAIWSMigrationAdmission {
	nowUnixMilli := now.UnixMilli()
	if state.ObservedAtUnixMilli > 0 {
		nowUnixMilli = state.ObservedAtUnixMilli
	}
	excluded := make(map[int64]struct{}, len(state.ExcludedUntilUnixMilli))
	for accountID, until := range state.ExcludedUntilUnixMilli {
		if accountID > 0 && until > nowUnixMilli {
			excluded[accountID] = struct{}{}
		}
	}
	generation := state.BindingGeneration
	if generation == 0 {
		generation = aetherWSBindingGenerationInitial
	}
	return OpenAIWSMigrationAdmission{
		Active:             true,
		MigrationCount:     state.MigrationCount,
		MaxMigrations:      maxMigrations,
		Exhausted:          state.MigrationCount >= maxMigrations,
		BindingGeneration:  generation,
		ExcludedAccountIDs: excluded,
	}
}
