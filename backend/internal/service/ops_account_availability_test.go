//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type opsAvailabilityAccountRepoStub struct {
	mockAccountRepoForGemini
	items []Account
}

func (r *opsAvailabilityAccountRepoStub) ListWithFilters(
	_ context.Context,
	params pagination.PaginationParams,
	_, _, _, _ string,
	_ int64,
	_ string,
) ([]Account, *pagination.PaginationResult, error) {
	return r.items, &pagination.PaginationResult{
		Total:    int64(len(r.items)),
		Page:     params.Page,
		PageSize: params.PageSize,
		Pages:    1,
	}, nil
}

func TestOpsAccountAvailability_PoolModeIgnoresStaleRuntimeStateButKeepsManualScheduling(t *testing.T) {
	now := time.Now()
	until := now.Add(time.Hour)
	group := &Group{ID: 91, Name: "pool", Platform: PlatformOpenAI}
	poolCredentials := map[string]any{"pool_mode": "true"}

	repo := &opsAvailabilityAccountRepoStub{items: []Account{
		{
			ID:                     1,
			Name:                   "pool-stale-state",
			Platform:               PlatformOpenAI,
			Type:                   AccountTypeAPIKey,
			Credentials:            poolCredentials,
			Status:                 StatusError,
			Schedulable:            true,
			ErrorMessage:           "stale upstream error",
			RateLimitResetAt:       &until,
			OverloadUntil:          &until,
			TempUnschedulableUntil: &until,
			Groups:                 []*Group{group},
		},
		{
			ID:          2,
			Name:        "pool-manually-disabled",
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Credentials: poolCredentials,
			Status:      StatusActive,
			Schedulable: false,
			Groups:      []*Group{group},
		},
	}}
	svc := &OpsService{
		cfg:         &config.Config{Ops: config.OpsConfig{Enabled: true}},
		accountRepo: repo,
	}

	platforms, groups, accounts, _, err := svc.GetAccountAvailabilityStats(context.Background(), "", nil)
	require.NoError(t, err)

	require.Equal(t, int64(2), platforms[PlatformOpenAI].TotalAccounts)
	require.Equal(t, int64(1), platforms[PlatformOpenAI].AvailableCount)
	require.Zero(t, platforms[PlatformOpenAI].RateLimitCount)
	require.Zero(t, platforms[PlatformOpenAI].ErrorCount)
	require.Equal(t, int64(1), groups[group.ID].AvailableCount)

	stale := accounts[1]
	require.True(t, stale.IsAvailable)
	require.Equal(t, StatusActive, stale.Status)
	require.False(t, stale.IsRateLimited)
	require.False(t, stale.IsOverloaded)
	require.False(t, stale.HasError)
	require.Nil(t, stale.RateLimitResetAt)
	require.Nil(t, stale.OverloadUntil)
	require.Nil(t, stale.TempUnschedulableUntil)
	require.Empty(t, stale.ErrorMessage)

	require.False(t, accounts[2].IsAvailable)
}
