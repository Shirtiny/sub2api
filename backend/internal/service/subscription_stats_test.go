//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"github.com/stretchr/testify/require"
)

// statsUserSubRepoStub 支持 List / ShiftUsageWindows / GetByID / ListUsageDaily，
// 其余方法继承 userSubRepoNoop（panic）。
type statsUserSubRepoStub struct {
	userSubRepoNoop

	listSubs []UserSubscription

	shiftInput ShiftWindowQuery
	shiftRows  ShiftWindowRows

	sub       *UserSubscription
	dailyRows []SubscriptionUsageDaily
}

func (r *statsUserSubRepoStub) List(context.Context, pagination.PaginationParams, *int64, *int64, string, string, string, string) ([]UserSubscription, *pagination.PaginationResult, error) {
	return r.listSubs, &pagination.PaginationResult{Total: int64(len(r.listSubs)), Page: 1, PageSize: 1000, Pages: 1}, nil
}

func (r *statsUserSubRepoStub) ShiftUsageWindows(_ context.Context, input ShiftWindowQuery) (ShiftWindowRows, error) {
	r.shiftInput = input
	return r.shiftRows, nil
}

func (r *statsUserSubRepoStub) GetByID(context.Context, int64) (*UserSubscription, error) {
	return r.sub, nil
}

func (r *statsUserSubRepoStub) ListUsageDaily(context.Context, int64, time.Time, time.Time) ([]SubscriptionUsageDaily, error) {
	return r.dailyRows, nil
}

func TestShiftSubscriptionWindowsRejectsInvalidInput(t *testing.T) {
	svc := &SubscriptionService{userSubRepo: &statsUserSubRepoStub{}}
	ctx := context.Background()

	_, err := svc.ShiftSubscriptionWindows(ctx, &ShiftSubscriptionWindowInput{OffsetHours: 14})
	require.ErrorIs(t, err, ErrInvalidInput, "未勾选任何窗口应被拒绝")

	_, err = svc.ShiftSubscriptionWindows(ctx, &ShiftSubscriptionWindowInput{Weekly: true, OffsetHours: 0})
	require.ErrorIs(t, err, ErrInvalidInput, "偏移量为 0 应被拒绝")

	_, err = svc.ShiftSubscriptionWindows(ctx, &ShiftSubscriptionWindowInput{Weekly: true, OffsetHours: 721})
	require.ErrorIs(t, err, ErrInvalidInput, "超过 720 小时应被拒绝")

	_, err = svc.ShiftSubscriptionWindows(ctx, &ShiftSubscriptionWindowInput{Weekly: true, OffsetHours: -721})
	require.ErrorIs(t, err, ErrInvalidInput, "低于 -720 小时应被拒绝")
}

func TestShiftSubscriptionWindowsCountsSkippedFuture(t *testing.T) {
	repo := &statsUserSubRepoStub{
		shiftRows: ShiftWindowRows{
			Rows: []ShiftWindowRow{
				{ID: 1, UserID: 10, GroupID: 4, Future: false},
				{ID: 2, UserID: 11, GroupID: 4, Future: true},
				{ID: 3, UserID: 12, GroupID: 9, Future: false},
			},
			Updated: 2,
		},
	}
	svc := &SubscriptionService{userSubRepo: repo}

	result, err := svc.ShiftSubscriptionWindows(context.Background(), &ShiftSubscriptionWindowInput{
		Weekly:      true,
		OffsetHours: 14,
		GroupID:     int64Ptr(4),
		Platform:    "openai",
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), result.Matched)
	require.Equal(t, int64(2), result.Updated)
	require.Equal(t, int64(1), result.SkippedFuture)
	require.False(t, result.DryRun)

	// 过滤条件必须原样透传，且状态缺省补成 active。
	require.Equal(t, SubscriptionStatusActive, repo.shiftInput.Status)
	require.Equal(t, "openai", repo.shiftInput.Platform)
	require.Equal(t, int64(4), *repo.shiftInput.GroupID)
	require.Equal(t, 14*time.Hour, repo.shiftInput.Offset)
	require.True(t, repo.shiftInput.Weekly)
	require.False(t, repo.shiftInput.Daily)
}

func TestGetSubscriptionStatsSeparatesDailyAndWeeklyQuotas(t *testing.T) {
	now := time.Now()
	weeklyStart := startOfDay(now).Add(-48 * time.Hour)
	dailyStart := startOfDay(now)

	weeklyGroup := &Group{ID: 4, Name: "Latte (拿铁)", Platform: "openai", WeeklyLimitUSD: float64Ptr(300)}
	dailyGroup := &Group{ID: 5, Name: "Specialty (精选)", Platform: "openai", DailyLimitUSD: float64Ptr(150)}

	repo := &statsUserSubRepoStub{listSubs: []UserSubscription{
		{
			ID: 1, UserID: 10, GroupID: 4, Status: SubscriptionStatusActive,
			StartsAt: now.Add(-72 * time.Hour), ExpiresAt: now.Add(30 * 24 * time.Hour),
			WeeklyWindowStart: &weeklyStart, WeeklyUsageUSD: 120,
			Group:             weeklyGroup, User: &User{ID: 10, Username: "poco", Email: "poco@example.com"},
		},
		{
			ID: 2, UserID: 11, GroupID: 5, Status: SubscriptionStatusActive,
			StartsAt: now.Add(-72 * time.Hour), ExpiresAt: now.Add(30 * 24 * time.Hour),
			DailyWindowStart: &dailyStart, DailyUsageUSD: 50,
			Group:            dailyGroup, User: &User{ID: 11, Username: "naiba", Email: "naiba@example.com"},
		},
	}}
	svc := &SubscriptionService{userSubRepo: repo}

	stats, err := svc.GetSubscriptionStats(context.Background(), 7, 20)
	require.NoError(t, err)

	require.Equal(t, int64(2), stats.Totals.ActiveSubscriptions)
	require.Equal(t, int64(2), stats.Totals.ActiveUsers)
	require.Equal(t, int64(1), stats.Totals.DailyLimitedSubscriptions)
	require.Equal(t, int64(1), stats.Totals.WeeklyLimitedSubscriptions)

	// 日限订阅不进「本周剩余」，周限订阅不进「今日剩余」——两个口径互不折算。
	require.InDelta(t, 100, stats.Totals.RemainingTodayUSD, 0.001, "150 日限 - 50 已用")
	require.InDelta(t, 180, stats.Totals.RemainingWeekUSD, 0.001, "300 周限 - 120 已用")

	// 7 天上限：周限订阅剩余 180 + 一次重置 300；日限订阅剩余 100 + 7 次重置 × 150。
	require.InDelta(t, 480+100+7*150, stats.Totals.HorizonCapacityUSD, 0.001)

	require.Len(t, stats.Ranking.Daily, 1)
	require.Len(t, stats.Ranking.Weekly, 1)
	require.InDelta(t, 50.0/150.0, stats.Ranking.Daily[0].UsageRatio, 0.0001)
	require.InDelta(t, 120.0/300.0, stats.Ranking.Weekly[0].UsageRatio, 0.0001)
	require.Equal(t, "poco", stats.Ranking.Weekly[0].Username)
}

func TestGetSubscriptionStatsTreatsExpiredWindowAsReset(t *testing.T) {
	now := time.Now()
	// 周窗口起点在 8 天前 —— 已过期，用量应被视为已归零。
	staleStart := startOfDay(now).Add(-8 * 24 * time.Hour)
	group := &Group{ID: 4, Name: "Latte (拿铁)", Platform: "openai", WeeklyLimitUSD: float64Ptr(300)}

	repo := &statsUserSubRepoStub{listSubs: []UserSubscription{{
		ID: 1, UserID: 10, GroupID: 4, Status: SubscriptionStatusActive,
		StartsAt: now.Add(-30 * 24 * time.Hour), ExpiresAt: now.Add(24 * time.Hour),
		WeeklyWindowStart: &staleStart, WeeklyUsageUSD: 299,
		Group:             group, User: &User{ID: 10},
	}}}
	svc := &SubscriptionService{userSubRepo: repo}

	stats, err := svc.GetSubscriptionStats(context.Background(), 7, 20)
	require.NoError(t, err)
	require.InDelta(t, 300, stats.Totals.RemainingWeekUSD, 0.001, "过期窗口按满额计")
	require.InDelta(t, 0, stats.Ranking.Weekly[0].UsedUSD, 0.001)
	require.Nil(t, stats.Ranking.Weekly[0].WindowStart, "过期窗口不回报陈旧起点")

	// horizon 被 expires_at 截断到 1 天内，周窗口在此期间不会重置。
	require.InDelta(t, 300, stats.Totals.HorizonCapacityUSD, 0.001)
}

func TestGetSubscriptionStatsHandlesOneTimeDailyQuota(t *testing.T) {
	now := time.Now()
	// starts_at 与 expires_at 相差不到一天 —— 一次性日额度，窗口永不重置。
	startsAt := now.Add(-2 * time.Hour)
	dailyStart := startOfDay(now)
	group := &Group{ID: 20, Name: "一次性日额度", Platform: "openai", DailyLimitUSD: float64Ptr(100)}

	repo := &statsUserSubRepoStub{listSubs: []UserSubscription{{
		ID: 1, UserID: 10, GroupID: 20, Status: SubscriptionStatusActive,
		StartsAt: startsAt, ExpiresAt: startsAt.Add(20 * time.Hour),
		DailyWindowStart: &dailyStart, DailyUsageUSD: 30,
		Group:            group, User: &User{ID: 10},
	}}}
	svc := &SubscriptionService{userSubRepo: repo}

	stats, err := svc.GetSubscriptionStats(context.Background(), 7, 20)
	require.NoError(t, err)

	// 只剩当前窗口的 70，不该按 7 天叠加 7 次重置。
	require.InDelta(t, 70, stats.Totals.RemainingTodayUSD, 0.001)
	require.InDelta(t, 70, stats.Totals.HorizonCapacityUSD, 0.001, "一次性额度不产生后续重置")

	require.Len(t, stats.Ranking.Daily, 1)
	require.NotNil(t, stats.Ranking.Daily[0].WindowResetsAt)
	require.WithinDuration(t, startsAt.Add(20*time.Hour), *stats.Ranking.Daily[0].WindowResetsAt, time.Second,
		"一次性额度的重置时刻是 expires_at，不是窗口起点 +24h")
}

func TestGetSubscriptionUsageSeriesDerivesDailyLimitFromWeekly(t *testing.T) {
	now := time.Now()
	start := startOfDay(now).Add(-3 * 24 * time.Hour)
	weeklyStart := startOfDay(now).Add(-3 * 24 * time.Hour)
	group := &Group{ID: 4, Name: "Latte (拿铁)", WeeklyLimitUSD: float64Ptr(300)}

	repo := &statsUserSubRepoStub{
		sub: &UserSubscription{
			ID: 1, UserID: 10, GroupID: 4,
			StartsAt: start, ExpiresAt: now.Add(27 * 24 * time.Hour),
			WeeklyWindowStart: &weeklyStart,
			Group:             group, User: &User{ID: 10, Username: "poco"},
		},
		dailyRows: []SubscriptionUsageDaily{
			{BucketDate: start, CostUSD: 60, RequestCount: 100, WeeklyLimitUSD: float64Ptr(300)},
			{BucketDate: start.AddDate(0, 0, 1), CostUSD: 90, RequestCount: 150, WeeklyLimitUSD: float64Ptr(300)},
		},
	}
	svc := &SubscriptionService{userSubRepo: repo}

	series, err := svc.GetSubscriptionUsageSeries(context.Background(), 1)
	require.NoError(t, err)

	require.Len(t, series.Daily, 2)
	// 无日限时分母取 周限÷7，并标记为折算值。
	require.True(t, series.Daily[0].LimitIsDerived)
	require.InDelta(t, 300.0/7.0, series.Daily[0].LimitUSD, 0.0001)
	require.InDelta(t, 60.0/(300.0/7.0), series.Daily[0].UsageRatio, 0.0001)

	// 两天落在同一个周窗口内，合并成一个周点，分母是真实周限。
	require.Len(t, series.Weekly, 1)
	require.False(t, series.Weekly[0].LimitIsDerived)
	require.InDelta(t, 150, series.Weekly[0].CostUSD, 0.001)
	require.InDelta(t, 150.0/300.0, series.Weekly[0].UsageRatio, 0.0001)

	require.Equal(t, "weekly", series.Cycle.WindowKind)
	require.InDelta(t, 150, series.Cycle.CostUSD, 0.001)
	require.True(t, series.DataComplete, "汇总表首日不晚于周期起点即视为完整")
}

func TestGetSubscriptionUsageSeriesFlagsIncompleteHistory(t *testing.T) {
	now := time.Now()
	start := startOfDay(now).Add(-30 * 24 * time.Hour)
	group := &Group{ID: 5, Name: "Specialty (精选)", DailyLimitUSD: float64Ptr(150)}

	repo := &statsUserSubRepoStub{
		sub: &UserSubscription{
			ID: 2, UserID: 11, GroupID: 5,
			StartsAt: start, ExpiresAt: now.Add(24 * time.Hour),
			Group:    group, User: &User{ID: 11},
		},
		// 汇总表只有最近 2 天 —— 早于它的历史已随 usage_logs 保留期消失。
		dailyRows: []SubscriptionUsageDaily{
			{BucketDate: startOfDay(now).Add(-24 * time.Hour), CostUSD: 120, DailyLimitUSD: float64Ptr(150)},
			{BucketDate: startOfDay(now), CostUSD: 30, DailyLimitUSD: float64Ptr(150)},
		},
	}
	svc := &SubscriptionService{userSubRepo: repo}

	series, err := svc.GetSubscriptionUsageSeries(context.Background(), 2)
	require.NoError(t, err)

	require.False(t, series.DataComplete)
	require.NotNil(t, series.DataFrom)
	require.Equal(t, startOfDay(now).Add(-24*time.Hour).Format(time.DateOnly), *series.DataFrom)

	require.False(t, series.Daily[0].LimitIsDerived, "有日限时分母就是日限本身")
	require.InDelta(t, 150, series.Daily[0].LimitUSD, 0.001)

	require.Equal(t, "daily", series.Cycle.WindowKind)
	require.Equal(t, int64(31), series.Cycle.WindowsElapsed, "30 天周期已经历 31 个日窗口")
	require.InDelta(t, 150*31, series.Cycle.QuotaUSD, 0.001)
}

func TestWindowResetsWithin(t *testing.T) {
	base := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)

	require.Equal(t, int64(0), windowResetsWithin(base.Add(24*time.Hour), base, dailyWindowPeriod),
		"horizon 早于下次重置时不计")
	require.Equal(t, int64(1), windowResetsWithin(base.Add(24*time.Hour), base.Add(25*time.Hour), dailyWindowPeriod))
	require.Equal(t, int64(7), windowResetsWithin(base.Add(24*time.Hour), base.Add(7*24*time.Hour), dailyWindowPeriod))
	require.Equal(t, int64(1), windowResetsWithin(base.Add(3*24*time.Hour), base.Add(7*24*time.Hour), weeklyWindowPeriod))
}
