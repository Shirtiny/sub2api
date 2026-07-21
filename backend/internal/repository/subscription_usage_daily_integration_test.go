//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type SubscriptionUsageDailySuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client
	repo   *dashboardAggregationRepository
}

func (s *SubscriptionUsageDailySuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.client = tx.Client()
	s.repo = newDashboardAggregationRepositoryWithSQL(tx)
}

func TestSubscriptionUsageDailySuite(t *testing.T) {
	suite.Run(t, new(SubscriptionUsageDailySuite))
}

// seedSubscriptionUsage 建出一条订阅并给它写 n 条 usage_logs，返回订阅 ID 与写入的总金额。
func (s *SubscriptionUsageDailySuite) seedSubscriptionUsage(at time.Time, weeklyLimit float64, costs ...float64) (int64, float64) {
	s.T().Helper()

	user := mustCreateUser(s.T(), s.client, &service.User{Email: "rollup@example.com"})
	group := mustCreateGroup(s.T(), s.client, &service.Group{
		Name:             "Latte (拿铁)",
		Platform:         "openai",
		SubscriptionType: "subscription",
		WeeklyLimitUSD:   &weeklyLimit,
	})
	sub := mustCreateSubscription(s.T(), s.client, &service.UserSubscription{
		UserID:  user.ID,
		GroupID: group.ID,
	})
	proxy := mustCreateProxy(s.T(), s.client, &service.Proxy{Name: "p"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a", ProxyID: &proxy.ID})
	key := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Name: "k"})

	var total float64
	for _, cost := range costs {
		_, err := s.client.ExecContext(s.ctx, `
			INSERT INTO usage_logs (user_id, api_key_id, account_id, subscription_id, group_id,
				model, actual_cost, total_cost, billing_type, created_at)
			VALUES ($1, $2, $3, $4, $5, 'gpt-5.6', $6, $6, 1, $7)
		`, user.ID, key.ID, account.ID, sub.ID, group.ID, cost, at)
		s.Require().NoError(err)
		total += cost
	}
	return sub.ID, total
}

func (s *SubscriptionUsageDailySuite) rollupCost(subID int64) (float64, bool) {
	s.T().Helper()
	rows, err := s.client.QueryContext(s.ctx,
		`SELECT cost_usd FROM subscription_usage_daily WHERE subscription_id = $1`, subID)
	s.Require().NoError(err)
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return 0, false
	}
	var cost float64
	s.Require().NoError(rows.Scan(&cost))
	return cost, true
}

func (s *SubscriptionUsageDailySuite) TestAggregateRangeWritesRollup() {
	at := time.Now().Add(-2 * time.Hour)
	subID, total := s.seedSubscriptionUsage(at, 300, 12.5, 7.25)

	s.Require().NoError(s.repo.upsertSubscriptionUsageDaily(s.ctx,
		truncateToDay(at), truncateToDay(at).Add(24*time.Hour)))

	cost, ok := s.rollupCost(subID)
	s.Require().True(ok, "聚合后应写出订阅日用量行")
	s.InDelta(total, cost, 0.0001)
}

// 这是本表存在意义的回归护栏：usage_logs 被裁掉之后，重算不得抹掉已落库的订阅用量历史。
//
// 触发路径是真实存在的 —— UsageCleanupService 删完某区间的 usage_logs 后，会立刻对
// 同一区间调 TriggerRecomputeRange（见 internal/service/usage_cleanup_service.go）。
// 运维侧的每周清理脚本同样只保留最近一天的 usage_logs。
// 若 recomputeRangeInTx 像对待 usage_dashboard_* 那样先删后建，这段历史将永久消失且无法重建。
func (s *SubscriptionUsageDailySuite) TestRecomputeKeepsRollupAfterUsageLogsPurged() {
	at := time.Now().Add(-2 * time.Hour)
	subID, total := s.seedSubscriptionUsage(at, 300, 40, 60)

	dayStart := truncateToDay(at)
	dayEnd := dayStart.Add(24 * time.Hour)
	s.Require().NoError(s.repo.upsertSubscriptionUsageDaily(s.ctx, dayStart, dayEnd))

	before, ok := s.rollupCost(subID)
	s.Require().True(ok)
	s.InDelta(total, before, 0.0001)

	// 模拟清理：源日志被删光
	_, err := s.client.ExecContext(s.ctx, `DELETE FROM usage_logs WHERE subscription_id = $1`, subID)
	s.Require().NoError(err)

	hourStart := dayStart
	hourEnd := dayEnd
	s.Require().NoError(s.repo.recomputeRangeInTx(s.ctx, hourStart, hourEnd, dayStart, dayEnd))

	after, ok := s.rollupCost(subID)
	s.Require().True(ok, "源日志被清理后，订阅用量历史必须仍然存在")
	s.InDelta(total, after, 0.0001, "重算不得把历史归零")
}

func (s *SubscriptionUsageDailySuite) TestCleanupSubscriptionUsageDailyRespectsCutoff() {
	at := time.Now().Add(-2 * time.Hour)
	subID, _ := s.seedSubscriptionUsage(at, 300, 5)

	dayStart := truncateToDay(at)
	s.Require().NoError(s.repo.upsertSubscriptionUsageDaily(s.ctx, dayStart, dayStart.Add(24*time.Hour)))

	// 保留期截止早于该行 → 不该被删
	s.Require().NoError(s.repo.CleanupSubscriptionUsageDaily(s.ctx, dayStart.AddDate(0, 0, -1)))
	_, ok := s.rollupCost(subID)
	s.Require().True(ok)

	// 保留期截止晚于该行 → 该被删
	s.Require().NoError(s.repo.CleanupSubscriptionUsageDaily(s.ctx, dayStart.AddDate(0, 0, 1)))
	_, ok = s.rollupCost(subID)
	s.Require().False(ok)
}
