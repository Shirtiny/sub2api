//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRateLimitService_PoolModeSkipsTemporaryStatePaths(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := &RateLimitService{accountRepo: repo}
	account := &Account{
		ID:       701,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"pool_mode":                  true,
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{map[string]any{
				"error_code":       float64(http.StatusServiceUnavailable),
				"duration_minutes": float64(5),
			}},
		},
	}

	require.False(t, svc.HandleTempUnschedulable(context.Background(), account, http.StatusServiceUnavailable, []byte("unavailable")))
	require.False(t, svc.HandleStreamTimeout(context.Background(), account, "gpt-5.4"))
	require.Zero(t, repo.tempCalls)
	require.Zero(t, repo.setErrorCalls)
}

func TestRateLimitService_GetTempUnschedStatusPoolModeHidesPersistedState(t *testing.T) {
	until := time.Now().Add(10 * time.Minute)
	account := &Account{
		ID:                      702,
		Platform:                PlatformOpenAI,
		Type:                    AccountTypeAPIKey,
		Credentials:             map[string]any{"pool_mode": true},
		TempUnschedulableUntil:  &until,
		TempUnschedulableReason: "stale transport failure",
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	svc := &RateLimitService{accountRepo: repo}

	state, err := svc.GetTempUnschedStatus(context.Background(), account.ID)

	require.NoError(t, err)
	require.Nil(t, state)
}
