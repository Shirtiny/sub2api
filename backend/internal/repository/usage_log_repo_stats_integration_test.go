//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLog_GetStatsWithFilters_AggregatesAndEndpoints(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	user := mustCreateUser(t, client, &service.User{Email: "stats@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-stats-1", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-stats"})
	standardGroup := mustCreateGroup(t, client, &service.Group{Name: "g-stats-standard", SubscriptionType: service.SubscriptionTypeStandard})
	subscriptionGroup := mustCreateGroup(t, client, &service.Group{Name: "g-stats-subscription", SubscriptionType: service.SubscriptionTypeSubscription})

	now := time.Now().UTC()
	inboundEndpoint := "/v1/messages"
	upstreamEndpoint := "/v1/responses"
	for i := 0; i < 3; i++ {
		groupID := standardGroup.ID
		billingType := service.BillingTypeBalance
		if i == 2 {
			groupID = subscriptionGroup.ID
			billingType = service.BillingTypeSubscription
		}
		_, err := repo.Create(ctx, &service.UsageLog{
			UserID: user.ID, APIKeyID: apiKey.ID, AccountID: account.ID,
			GroupID: &groupID, BillingType: billingType, Model: "claude-3", InputTokens: 2, OutputTokens: 3, CacheCreationTokens: 4, CacheReadTokens: 5,
			TotalCost: 0.5, ActualCost: 0.4, CreatedAt: now,
			InboundEndpoint: &inboundEndpoint, UpstreamEndpoint: &upstreamEndpoint,
		})
		require.NoError(t, err)
	}

	start := now.Add(-1 * time.Hour)
	end := now.Add(1 * time.Hour)
	// 按本测试创建的 user 维度过滤:集成库为共享实例,其它用 testEntClient 的兄弟测试会留下
	// 已提交的 usage_log 行(含零 token 的失败请求),不限定 user 会把它们计入 TotalRequests。
	stats, err := repo.GetStatsWithFilters(ctx, usagestats.UsageLogFilters{UserID: user.ID, StartTime: &start, EndTime: &end})
	require.NoError(t, err)
	require.Equal(t, int64(3), stats.TotalRequests)
	require.Equal(t, int64(6), stats.TotalInputTokens)
	require.Equal(t, int64(9), stats.TotalOutputTokens)
	require.Equal(t, int64(12), stats.TotalCacheCreationTokens)
	require.Equal(t, int64(15), stats.TotalCacheReadTokens)
	require.Equal(t, int64(27), stats.TotalCacheTokens)
	require.Equal(t, int64(42), stats.TotalTokens)
	require.Len(t, stats.CacheByGroupType, 2)
	require.Equal(t, service.SubscriptionTypeSubscription, stats.CacheByGroupType[0].GroupType)
	require.Equal(t, int64(5), stats.CacheByGroupType[0].CacheReadTokens)
	require.InDelta(t, 5.0/11.0*100, stats.CacheByGroupType[0].HitRate, 1e-9)
	require.Equal(t, service.SubscriptionTypeStandard, stats.CacheByGroupType[1].GroupType)
	require.Equal(t, int64(10), stats.CacheByGroupType[1].CacheReadTokens)
	require.InDelta(t, 10.0/22.0*100, stats.CacheByGroupType[1].HitRate, 1e-9)
	require.InDelta(t, 1.2, stats.TotalActualCost, 1e-9)
	require.NotEmpty(t, stats.Endpoints)
	require.NotEmpty(t, stats.UpstreamEndpoints)
	require.NotEmpty(t, stats.EndpointPaths)
}
