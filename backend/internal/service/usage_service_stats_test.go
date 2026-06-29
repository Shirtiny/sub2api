package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type usageStatsRepoStub struct {
	UsageLogRepository
	stats     *usagestats.UsageStats
	gotUserID int64
	gotStart  time.Time
	gotEnd    time.Time
}

func (r *usageStatsRepoStub) GetUserStatsAggregated(_ context.Context, userID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error) {
	r.gotUserID = userID
	r.gotStart = startTime
	r.gotEnd = endTime
	return r.stats, nil
}

func TestUsageServiceGetStatsByUserPreservesCacheBreakdown(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	repo := &usageStatsRepoStub{
		stats: &usagestats.UsageStats{
			TotalRequests:            3,
			TotalInputTokens:         10,
			TotalOutputTokens:        20,
			TotalCacheTokens:         12,
			TotalCacheCreationTokens: 5,
			TotalCacheReadTokens:     7,
			TotalTokens:              42,
			TotalCost:                0.12,
			TotalActualCost:          0.08,
			AverageDurationMs:        123,
		},
	}
	svc := &UsageService{usageRepo: repo}

	got, err := svc.GetStatsByUser(context.Background(), 99, start, end)
	require.NoError(t, err)

	require.Equal(t, int64(99), repo.gotUserID)
	require.Equal(t, start, repo.gotStart)
	require.Equal(t, end, repo.gotEnd)
	require.Equal(t, int64(5), got.TotalCacheCreationTokens)
	require.Equal(t, int64(7), got.TotalCacheReadTokens)
	require.Equal(t, int64(12), got.TotalCacheTokens)
	require.Equal(t, int64(42), got.TotalTokens)
}
