//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

type UserAPIKeyUsageDailySuite struct {
	suite.Suite
	ctx       context.Context
	client    *dbent.Client
	usage     *usageLogRepository
	aggregate *dashboardAggregationRepository
}

func (s *UserAPIKeyUsageDailySuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.client = tx.Client()
	s.usage = newUsageLogRepositoryWithSQL(s.client, tx)
	s.aggregate = newDashboardAggregationRepositoryWithSQL(tx)
}

func TestUserAPIKeyUsageDailySuite(t *testing.T) {
	suite.Run(t, new(UserAPIKeyUsageDailySuite))
}

func (s *UserAPIKeyUsageDailySuite) scanOne(query string, dest any, args ...any) {
	s.T().Helper()
	rows, err := s.client.QueryContext(s.ctx, query, args...)
	s.Require().NoError(err)
	defer func() { _ = rows.Close() }()
	s.Require().True(rows.Next())
	s.Require().NoError(rows.Scan(dest))
	s.Require().NoError(rows.Err())
}

func (s *UserAPIKeyUsageDailySuite) TestHistoricalRollupCombinesWithLiveDayAndSurvivesRecompute() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "durable-usage@example.com"})
	key := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Name: "durable"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "durable"})

	today := timezone.Today()
	yesterday := today.AddDate(0, 0, -1)
	duration := 100
	for _, row := range []struct {
		at                                    time.Time
		input, output, cacheCreate, cacheRead int
		cost                                  float64
	}{
		{yesterday.Add(time.Hour), 10, 20, 3, 4, 1.0},
		{today.Add(time.Hour), 5, 6, 1, 2, 0.5},
	} {
		_, err := s.usage.Create(s.ctx, &service.UsageLog{
			UserID:              user.ID,
			APIKeyID:            key.ID,
			AccountID:           account.ID,
			RequestID:           uuid.NewString(),
			Model:               "gpt-test",
			InputTokens:         row.input,
			OutputTokens:        row.output,
			CacheCreationTokens: row.cacheCreate,
			CacheReadTokens:     row.cacheRead,
			TotalCost:           row.cost,
			ActualCost:          row.cost,
			DurationMs:          &duration,
			CreatedAt:           row.at,
		})
		s.Require().NoError(err)
	}

	s.Require().NoError(s.aggregate.upsertUserAPIKeyUsageDaily(s.ctx, yesterday, today))
	s.Require().NoError(s.aggregate.upsertCompletedUserAPIKeyUsageDaily(s.ctx, today, today.AddDate(0, 0, 1)))
	var currentRollupCount int
	s.scanOne(`
		SELECT COUNT(*) FROM user_api_key_usage_daily
		WHERE user_id = $1 AND bucket_date = $2::date
	`, &currentRollupCount, user.ID, today)
	s.Zero(currentRollupCount, "the live calendar day must stay on usage_logs")

	_, err := s.client.ExecContext(s.ctx,
		"DELETE FROM usage_logs WHERE user_id = $1 AND created_at < $2", user.ID, today)
	s.Require().NoError(err)

	stats, err := s.usage.GetUserStatsAggregated(s.ctx, user.ID, yesterday, today.AddDate(0, 0, 1))
	s.Require().NoError(err)
	s.Equal(int64(2), stats.TotalRequests)
	s.Equal(int64(15), stats.TotalInputTokens)
	s.Equal(int64(26), stats.TotalOutputTokens)
	s.Equal(int64(4), stats.TotalCacheCreationTokens)
	s.Equal(int64(6), stats.TotalCacheReadTokens)
	s.InDelta(1.5, stats.TotalActualCost, 0.0000001)

	trend, err := s.usage.GetAPIKeyDailyUsageTrend(s.ctx, user.ID, key.ID, yesterday, today.AddDate(0, 0, 1))
	s.Require().NoError(err)
	s.Len(trend, 2)
	s.Equal(int64(37), trend[0].TotalTokens)
	s.Equal(int64(14), trend[1].TotalTokens)

	s.Require().NoError(s.aggregate.recomputeRangeInTx(s.ctx, yesterday, today, yesterday, today))
	var historicalCost float64
	s.scanOne(`
		SELECT COALESCE(SUM(actual_cost), 0)
		FROM user_api_key_usage_daily
		WHERE user_id = $1 AND bucket_date = $2::date
	`, &historicalCost, user.ID, yesterday)
	s.InDelta(1.0, historicalCost, 0.0000001)
}

func (s *UserAPIKeyUsageDailySuite) TestCleanupUsesExclusiveCutoff() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "durable-cleanup@example.com"})
	key := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Name: "durable-cleanup"})
	day := timezone.Today().AddDate(0, 0, -60)
	_, err := s.client.ExecContext(s.ctx, `
		INSERT INTO user_api_key_usage_daily (bucket_date, user_id, api_key_id, billing_type, actual_cost)
		VALUES ($1::date, $2, $3, 0, 1)
	`, day, user.ID, key.ID)
	s.Require().NoError(err)

	s.Require().NoError(s.aggregate.CleanupUserAPIKeyUsageDaily(s.ctx, day))
	var count int
	s.scanOne("SELECT COUNT(*) FROM user_api_key_usage_daily WHERE user_id = $1", &count, user.ID)
	s.Equal(1, count, "cutoff day is the oldest retained calendar day")

	s.Require().NoError(s.aggregate.CleanupUserAPIKeyUsageDaily(s.ctx, day.AddDate(0, 0, 1)))
	s.scanOne("SELECT COUNT(*) FROM user_api_key_usage_daily WHERE user_id = $1", &count, user.ID)
	s.Zero(count)
}
