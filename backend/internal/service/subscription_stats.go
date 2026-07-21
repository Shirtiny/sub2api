package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	dailyWindowPeriod   = 24 * time.Hour
	weeklyWindowPeriod  = 7 * 24 * time.Hour
	monthlyWindowPeriod = 30 * 24 * time.Hour

	// statsSubscriptionPageSize 与 AdminBulkResetQuota 保持一致的分页拉取粒度。
	statsSubscriptionPageSize = 1000

	maxShiftWindowOffsetHours = 720
)

// ---------- 批量平移重置窗口 ----------

// ShiftSubscriptionWindowInput 是管理端「统一延长重置窗口」的入参。
type ShiftSubscriptionWindowInput struct {
	Daily       bool
	Weekly      bool
	Monthly     bool
	OffsetHours int
	DryRun      bool

	Status   string
	UserID   *int64
	GroupID  *int64
	Platform string
}

// ShiftSubscriptionWindowResult 汇报一次平移的影响面。
type ShiftSubscriptionWindowResult struct {
	Matched       int64 `json:"matched"`
	Updated       int64 `json:"updated"`
	SkippedFuture int64 `json:"skipped_future"`
	DryRun        bool  `json:"dry_run"`
}

// ShiftSubscriptionWindows 按过滤条件批量平移订阅窗口的重置时间。
//
// 注意这是一次性平移：窗口长度本身没变，下一轮重置触发时 CheckAndResetWindows 会把
// 新窗口起点写成 startOfDay(now)，偏移量随之消失。需要长期偏移得改重置算法而非改数据。
func (s *SubscriptionService) ShiftSubscriptionWindows(ctx context.Context, input *ShiftSubscriptionWindowInput) (*ShiftSubscriptionWindowResult, error) {
	if input == nil {
		return nil, ErrInvalidInput
	}
	if !input.Daily && !input.Weekly && !input.Monthly {
		return nil, ErrInvalidInput
	}
	if input.OffsetHours == 0 {
		return nil, ErrInvalidInput
	}
	if input.OffsetHours > maxShiftWindowOffsetHours || input.OffsetHours < -maxShiftWindowOffsetHours {
		return nil, ErrInvalidInput
	}

	status := input.Status
	if status == "" {
		status = SubscriptionStatusActive
	}

	rows, err := s.userSubRepo.ShiftUsageWindows(ctx, ShiftWindowQuery{
		Daily:    input.Daily,
		Weekly:   input.Weekly,
		Monthly:  input.Monthly,
		Offset:   time.Duration(input.OffsetHours) * time.Hour,
		DryRun:   input.DryRun,
		Status:   status,
		UserID:   input.UserID,
		GroupID:  input.GroupID,
		Platform: input.Platform,
	})
	if err != nil {
		return nil, fmt.Errorf("shift subscription windows: %w", err)
	}

	result := &ShiftSubscriptionWindowResult{
		Matched: int64(len(rows.Rows)),
		Updated: rows.Updated,
		DryRun:  input.DryRun,
	}
	for i := range rows.Rows {
		if rows.Rows[i].Future {
			result.SkippedFuture++
			continue
		}
		if !input.DryRun {
			s.invalidateSubscriptionCaches(ctx, rows.Rows[i].UserID, rows.Rows[i].GroupID)
		}
	}
	return result, nil
}

// ---------- 订阅统计总览 ----------

// SubscriptionStatsTotals 是统计面板顶部的三张概览卡片。
type SubscriptionStatsTotals struct {
	ActiveSubscriptions        int64   `json:"active_subscriptions"`
	ActiveUsers                int64   `json:"active_users"`
	DailyLimitedSubscriptions  int64   `json:"daily_limited_subscriptions"`
	WeeklyLimitedSubscriptions int64   `json:"weekly_limited_subscriptions"`
	RemainingTodayUSD          float64 `json:"remaining_today_usd"`
	RemainingWeekUSD           float64 `json:"remaining_week_usd"`
	HorizonCapacityUSD         float64 `json:"horizon_capacity_usd"`
}

// SubscriptionPlanStat 是按分组（套餐）汇总的一行。
type SubscriptionPlanStat struct {
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	Platform  string `json:"platform"`

	Subscriptions int64 `json:"subscriptions"`
	Users         int64 `json:"users"`

	DailyLimitUSD   float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD  float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD float64 `json:"monthly_limit_usd"`

	DailyQuotaUSD     float64 `json:"daily_quota_usd"`
	DailyUsedUSD      float64 `json:"daily_used_usd"`
	RemainingTodayUSD float64 `json:"remaining_today_usd"`

	WeeklyQuotaUSD   float64 `json:"weekly_quota_usd"`
	WeeklyUsedUSD    float64 `json:"weekly_used_usd"`
	RemainingWeekUSD float64 `json:"remaining_week_usd"`

	MonthlyQuotaUSD   float64 `json:"monthly_quota_usd"`
	MonthlyUsedUSD    float64 `json:"monthly_used_usd"`
	RemainingMonthUSD float64 `json:"remaining_month_usd"`

	// UsedUSD / QuotaUSD / UsageRatio 是该套餐的主窗口口径汇总，优先级 日 > 周 > 月。
	// 由后端给出而不是让前端自行推导，避免同时设了两种限额的分组被静默丢掉一个维度。
	UsedUSD    float64 `json:"used_usd"`
	QuotaUSD   float64 `json:"quota_usd"`
	UsageRatio float64 `json:"usage_ratio"`

	HorizonCapacityUSD float64 `json:"horizon_capacity_usd"`
}

// SubscriptionRankingItem 是使用率排行的一行。
type SubscriptionRankingItem struct {
	SubscriptionID int64  `json:"subscription_id"`
	UserID         int64  `json:"user_id"`
	Username       string `json:"username"`
	Email          string `json:"email"`
	GroupID        int64  `json:"group_id"`
	GroupName      string `json:"group_name"`

	LimitUSD     float64 `json:"limit_usd"`
	UsedUSD      float64 `json:"used_usd"`
	RemainingUSD float64 `json:"remaining_usd"`
	UsageRatio   float64 `json:"usage_ratio"`

	WindowStart    *time.Time `json:"window_start"`
	WindowResetsAt *time.Time `json:"window_resets_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
}

// SubscriptionStatsRanking 分日窗口 / 周窗口两个榜单。
type SubscriptionStatsRanking struct {
	Daily  []SubscriptionRankingItem `json:"daily"`
	Weekly []SubscriptionRankingItem `json:"weekly"`
}

// SubscriptionStats 是 GET /admin/subscriptions/stats 的响应体。
type SubscriptionStats struct {
	GeneratedAt time.Time                `json:"generated_at"`
	HorizonDays int                      `json:"horizon_days"`
	Totals      SubscriptionStatsTotals  `json:"totals"`
	Plans       []SubscriptionPlanStat   `json:"plans"`
	Ranking     SubscriptionStatsRanking `json:"ranking"`
}

// GetSubscriptionStats 汇总全部生效中订阅的额度与用量。
//
// 口径：
//   - 窗口已过期的订阅按「已重置」处理，用量计 0、剩余计满额，与计费侧
//     CheckAndResetWindows 的惰性重置语义一致；
//   - horizonDays 天可消耗上限 = 当前窗口剩余 + 限额 × 该区间内还会发生的重置次数，
//     区间终点被 expires_at 截断；
//   - 限额取 EffectiveSubscriptionGroup 的结果，即已应用 custom_multiplier 倍率。
func (s *SubscriptionService) GetSubscriptionStats(ctx context.Context, horizonDays, rankingLimit int) (*SubscriptionStats, error) {
	if horizonDays <= 0 {
		horizonDays = 7
	}
	if rankingLimit <= 0 {
		rankingLimit = 20
	}

	subs, err := s.listAllActiveSubscriptions(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	stats := &SubscriptionStats{
		GeneratedAt: now,
		HorizonDays: horizonDays,
		Plans:       []SubscriptionPlanStat{},
		Ranking: SubscriptionStatsRanking{
			Daily:  []SubscriptionRankingItem{},
			Weekly: []SubscriptionRankingItem{},
		},
	}

	planIndex := make(map[int64]int)
	planUsers := make(map[int64]map[int64]struct{})
	activeUsers := make(map[int64]struct{})

	for i := range subs {
		sub := &subs[i]
		group := EffectiveSubscriptionGroup(sub, sub.Group)
		if group == nil {
			continue
		}

		stats.Totals.ActiveSubscriptions++
		activeUsers[sub.UserID] = struct{}{}

		idx, ok := planIndex[group.ID]
		if !ok {
			idx = len(stats.Plans)
			planIndex[group.ID] = idx
			stats.Plans = append(stats.Plans, SubscriptionPlanStat{
				GroupID:   group.ID,
				GroupName: group.Name,
				Platform:  group.Platform,
			})
			planUsers[group.ID] = make(map[int64]struct{})
		}
		plan := &stats.Plans[idx]
		plan.Subscriptions++
		planUsers[group.ID][sub.UserID] = struct{}{}

		horizonEnd := now.Add(time.Duration(horizonDays) * 24 * time.Hour)
		if !sub.ExpiresAt.IsZero() && sub.ExpiresAt.Before(horizonEnd) {
			horizonEnd = sub.ExpiresAt
		}

		if group.HasDailyLimit() {
			limit := *group.DailyLimitUSD
			used := effectiveWindowUsage(sub.DailyUsageUSD, sub.NeedsDailyReset())
			remaining := math.Max(limit-used, 0)

			stats.Totals.DailyLimitedSubscriptions++
			stats.Totals.RemainingTodayUSD += remaining
			plan.DailyLimitUSD = limit
			plan.DailyQuotaUSD += limit
			plan.DailyUsedUSD += used
			plan.RemainingTodayUSD += remaining

			capacity := remaining
			// 一次性日额度订阅（starts_at 与 expires_at 相差不超过一天）永不重置。
			if !sub.HasOneTimeDailyQuota() {
				capacity += limit * float64(windowResetsWithin(
					nextWindowReset(sub.DailyWindowStart, dailyWindowPeriod, now, sub.NeedsDailyReset()),
					horizonEnd, dailyWindowPeriod))
			}
			plan.HorizonCapacityUSD += capacity
			stats.Totals.HorizonCapacityUSD += capacity

			// DailyResetTime 对一次性日额度订阅返回 expires_at 而非 +24h，直接复用它而不是自己加周期。
			stats.Ranking.Daily = append(stats.Ranking.Daily, buildRankingItem(sub, group, limit, used, remaining,
				sub.DailyWindowStart, sub.DailyResetTime(), sub.NeedsDailyReset()))
		}

		if group.HasWeeklyLimit() {
			limit := *group.WeeklyLimitUSD
			used := effectiveWindowUsage(sub.WeeklyUsageUSD, sub.NeedsWeeklyReset())
			remaining := math.Max(limit-used, 0)

			stats.Totals.WeeklyLimitedSubscriptions++
			stats.Totals.RemainingWeekUSD += remaining
			plan.WeeklyLimitUSD = limit
			plan.WeeklyQuotaUSD += limit
			plan.WeeklyUsedUSD += used
			plan.RemainingWeekUSD += remaining

			capacity := remaining + limit*float64(windowResetsWithin(
				nextWindowReset(sub.WeeklyWindowStart, weeklyWindowPeriod, now, sub.NeedsWeeklyReset()),
				horizonEnd, weeklyWindowPeriod))
			plan.HorizonCapacityUSD += capacity
			stats.Totals.HorizonCapacityUSD += capacity

			stats.Ranking.Weekly = append(stats.Ranking.Weekly, buildRankingItem(sub, group, limit, used, remaining,
				sub.WeeklyWindowStart, sub.WeeklyResetTime(), sub.NeedsWeeklyReset()))
		}

		if group.HasMonthlyLimit() {
			limit := *group.MonthlyLimitUSD
			used := effectiveWindowUsage(sub.MonthlyUsageUSD, sub.NeedsMonthlyReset())
			remaining := math.Max(limit-used, 0)

			plan.MonthlyLimitUSD = limit
			plan.MonthlyQuotaUSD += limit
			plan.MonthlyUsedUSD += used
			plan.RemainingMonthUSD += remaining

			capacity := remaining + limit*float64(windowResetsWithin(
				nextWindowReset(sub.MonthlyWindowStart, monthlyWindowPeriod, now, sub.NeedsMonthlyReset()),
				horizonEnd, monthlyWindowPeriod))
			plan.HorizonCapacityUSD += capacity
			stats.Totals.HorizonCapacityUSD += capacity
		}
	}

	stats.Totals.ActiveUsers = int64(len(activeUsers))
	for i := range stats.Plans {
		plan := &stats.Plans[i]
		plan.Users = int64(len(planUsers[plan.GroupID]))
		plan.UsedUSD, plan.QuotaUSD = primaryWindowUsage(plan)
		if plan.QuotaUSD > 0 {
			plan.UsageRatio = plan.UsedUSD / plan.QuotaUSD
		}
	}
	sort.Slice(stats.Plans, func(i, j int) bool {
		if stats.Plans[i].HorizonCapacityUSD != stats.Plans[j].HorizonCapacityUSD {
			return stats.Plans[i].HorizonCapacityUSD > stats.Plans[j].HorizonCapacityUSD
		}
		return stats.Plans[i].GroupID < stats.Plans[j].GroupID
	})

	stats.Ranking.Daily = topByUsageRatio(stats.Ranking.Daily, rankingLimit)
	stats.Ranking.Weekly = topByUsageRatio(stats.Ranking.Weekly, rankingLimit)
	return stats, nil
}

func (s *SubscriptionService) listAllActiveSubscriptions(ctx context.Context) ([]UserSubscription, error) {
	params := pagination.PaginationParams{Page: 1, PageSize: statsSubscriptionPageSize}
	subs, pag, err := s.userSubRepo.List(ctx, params, nil, nil, SubscriptionStatusActive, "", "created_at", "asc")
	if err != nil {
		return nil, fmt.Errorf("list active subscriptions: %w", err)
	}
	if pag != nil {
		for page := 2; page <= pag.Pages; page++ {
			params.Page = page
			pageSubs, _, pageErr := s.userSubRepo.List(ctx, params, nil, nil, SubscriptionStatusActive, "", "created_at", "asc")
			if pageErr != nil {
				return nil, fmt.Errorf("list active subscriptions page %d: %w", page, pageErr)
			}
			subs = append(subs, pageSubs...)
		}
	}
	return subs, nil
}

// primaryWindowUsage 选出套餐的主窗口口径，优先级与 buildCycle 保持一致：日 > 周 > 月。
func primaryWindowUsage(plan *SubscriptionPlanStat) (used, quota float64) {
	switch {
	case plan.DailyLimitUSD > 0:
		return plan.DailyUsedUSD, plan.DailyQuotaUSD
	case plan.WeeklyLimitUSD > 0:
		return plan.WeeklyUsedUSD, plan.WeeklyQuotaUSD
	case plan.MonthlyLimitUSD > 0:
		return plan.MonthlyUsedUSD, plan.MonthlyQuotaUSD
	default:
		return 0, 0
	}
}

func buildRankingItem(sub *UserSubscription, group *Group, limit, used, remaining float64,
	windowStart, resetAt *time.Time, needsReset bool) SubscriptionRankingItem {
	item := SubscriptionRankingItem{
		SubscriptionID: sub.ID,
		UserID:         sub.UserID,
		GroupID:        group.ID,
		GroupName:      group.Name,
		LimitUSD:       limit,
		UsedUSD:        used,
		RemainingUSD:   remaining,
		ExpiresAt:      sub.ExpiresAt,
	}
	if limit > 0 {
		item.UsageRatio = used / limit
	}
	if sub.User != nil {
		item.Username = sub.User.Username
		item.Email = sub.User.Email
	}
	// 窗口已过期时不再回报旧起点，避免前端把陈旧窗口当成当前窗口渲染。
	if windowStart != nil && !needsReset {
		start := *windowStart
		item.WindowStart = &start
		if resetAt != nil {
			reset := *resetAt
			item.WindowResetsAt = &reset
		}
	}
	return item
}

func topByUsageRatio(items []SubscriptionRankingItem, limit int) []SubscriptionRankingItem {
	sort.Slice(items, func(i, j int) bool {
		if items[i].UsageRatio != items[j].UsageRatio {
			return items[i].UsageRatio > items[j].UsageRatio
		}
		return items[i].SubscriptionID < items[j].SubscriptionID
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

// effectiveWindowUsage 把「窗口已过期」折算成「已重置」，与计费侧惰性重置口径一致。
func effectiveWindowUsage(usage float64, needsReset bool) float64 {
	if needsReset {
		return 0
	}
	return usage
}

// nextWindowReset 返回窗口下一次归零的时刻。
// 窗口未激活或已过期时，下一次实际写库都会把起点落到当天零点，故按 startOfDay(now)+period 推算。
func nextWindowReset(windowStart *time.Time, period time.Duration, now time.Time, needsReset bool) time.Time {
	if windowStart == nil || needsReset {
		return startOfDay(now).Add(period)
	}
	return windowStart.Add(period)
}

// windowResetsWithin 统计 (now, horizonEnd] 区间内 nextReset 起按 period 周期还会发生几次重置。
func windowResetsWithin(nextReset, horizonEnd time.Time, period time.Duration) int64 {
	if period <= 0 || !horizonEnd.After(nextReset) {
		return 0
	}
	return int64(horizonEnd.Sub(nextReset)/period) + 1
}

// ---------- 单订阅使用率明细 ----------

// SubscriptionUsageDailyPoint 是每天使用率的一个点。
type SubscriptionUsageDailyPoint struct {
	Date           string  `json:"date"`
	CostUSD        float64 `json:"cost_usd"`
	Requests       int64   `json:"requests"`
	LimitUSD       float64 `json:"limit_usd"`
	LimitIsDerived bool    `json:"limit_is_derived"`
	UsageRatio     float64 `json:"usage_ratio"`
}

// SubscriptionUsageWeeklyPoint 是每周使用率的一个点，按订阅自身的周窗口锚点切分。
type SubscriptionUsageWeeklyPoint struct {
	WeekStart      string  `json:"week_start"`
	WeekEnd        string  `json:"week_end"`
	CostUSD        float64 `json:"cost_usd"`
	Requests       int64   `json:"requests"`
	LimitUSD       float64 `json:"limit_usd"`
	LimitIsDerived bool    `json:"limit_is_derived"`
	UsageRatio     float64 `json:"usage_ratio"`
}

// SubscriptionUsageCycle 是整个订阅周期的累计使用率。
type SubscriptionUsageCycle struct {
	Start          string  `json:"start"`
	End            string  `json:"end"`
	CostUSD        float64 `json:"cost_usd"`
	QuotaUSD       float64 `json:"quota_usd"`
	UsageRatio     float64 `json:"usage_ratio"`
	WindowsElapsed int64   `json:"windows_elapsed"`
	WindowKind     string  `json:"window_kind"`
}

// SubscriptionUsageSeries 是 GET /admin/subscriptions/:id/usage-series 的响应体。
type SubscriptionUsageSeries struct {
	SubscriptionID int64  `json:"subscription_id"`
	UserID         int64  `json:"user_id"`
	Username       string `json:"username"`
	GroupID        int64  `json:"group_id"`
	GroupName      string `json:"group_name"`

	StartsAt  time.Time `json:"starts_at"`
	ExpiresAt time.Time `json:"expires_at"`

	DailyLimitUSD   float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD  float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD float64 `json:"monthly_limit_usd"`

	DataFrom     *string `json:"data_from"`
	DataComplete bool    `json:"data_complete"`

	Daily  []SubscriptionUsageDailyPoint  `json:"daily"`
	Weekly []SubscriptionUsageWeeklyPoint `json:"weekly"`
	// Cycle 在分组完全没有配置任何窗口限额时为 null —— 此时「整周期使用率」没有分母，
	// 返回 0 会被误读成「用了 0%」。用量本身仍可从 Daily/Weekly 看到。
	Cycle *SubscriptionUsageCycle `json:"cycle"`
}

// GetSubscriptionUsageSeries 返回单个订阅在其周期内的每天 / 每周 / 整周期使用率。
//
// 数据源是 subscription_usage_daily 汇总表而非 usage_logs —— 后者受保留期裁剪，
// 覆盖不了一个完整订阅周期。汇总表启用之前的日期没有数据，通过 DataFrom/DataComplete 显式告知前端。
func (s *SubscriptionService) GetSubscriptionUsageSeries(ctx context.Context, subscriptionID int64) (*SubscriptionUsageSeries, error) {
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	group := EffectiveSubscriptionGroup(sub, sub.Group)

	now := time.Now()
	cycleStart := startOfDay(sub.StartsAt)
	cycleEnd := startOfDay(now)
	if !sub.ExpiresAt.IsZero() && sub.ExpiresAt.Before(now) {
		cycleEnd = startOfDay(sub.ExpiresAt)
	}
	if cycleEnd.Before(cycleStart) {
		cycleEnd = cycleStart
	}

	rows, err := s.userSubRepo.ListUsageDaily(ctx, subscriptionID, cycleStart, cycleEnd)
	if err != nil {
		return nil, fmt.Errorf("list subscription usage daily: %w", err)
	}

	out := &SubscriptionUsageSeries{
		SubscriptionID: sub.ID,
		UserID:         sub.UserID,
		StartsAt:       sub.StartsAt,
		ExpiresAt:      sub.ExpiresAt,
		Daily:          []SubscriptionUsageDailyPoint{},
		Weekly:         []SubscriptionUsageWeeklyPoint{},
	}
	if sub.User != nil {
		out.Username = sub.User.Username
	}
	if group != nil {
		out.GroupID = group.ID
		out.GroupName = group.Name
		if group.DailyLimitUSD != nil {
			out.DailyLimitUSD = *group.DailyLimitUSD
		}
		if group.WeeklyLimitUSD != nil {
			out.WeeklyLimitUSD = *group.WeeklyLimitUSD
		}
		if group.MonthlyLimitUSD != nil {
			out.MonthlyLimitUSD = *group.MonthlyLimitUSD
		}
	}

	if len(rows) > 0 {
		from := rows[0].BucketDate.Format(time.DateOnly)
		out.DataFrom = &from
		out.DataComplete = !rows[0].BucketDate.After(cycleStart)
	}

	// 每日：优先用当天快照的日限；没有日限的套餐用「周限÷7」或「月限÷30」折算，并标注为派生值。
	var totalCost float64
	for i := range rows {
		row := rows[i]
		limit, derived := derivedDailyLimit(row)
		point := SubscriptionUsageDailyPoint{
			Date:           row.BucketDate.Format(time.DateOnly),
			CostUSD:        row.CostUSD,
			Requests:       row.RequestCount,
			LimitUSD:       limit,
			LimitIsDerived: derived,
		}
		if limit > 0 {
			point.UsageRatio = row.CostUSD / limit
		}
		out.Daily = append(out.Daily, point)
		totalCost += row.CostUSD
	}

	out.Weekly = buildWeeklyPoints(rows, weeklyAnchor(sub, cycleStart))
	out.Cycle = buildCycle(group, cycleStart, cycleEnd, totalCost)
	return out, nil
}

// derivedDailyLimit 给出某一天的日额度分母，第二个返回值表示它是否为折算值。
func derivedDailyLimit(row SubscriptionUsageDaily) (float64, bool) {
	if row.DailyLimitUSD != nil && *row.DailyLimitUSD > 0 {
		return *row.DailyLimitUSD, false
	}
	if row.WeeklyLimitUSD != nil && *row.WeeklyLimitUSD > 0 {
		return *row.WeeklyLimitUSD / 7, true
	}
	if row.MonthlyLimitUSD != nil && *row.MonthlyLimitUSD > 0 {
		return *row.MonthlyLimitUSD / 30, true
	}
	return 0, false
}

// weeklyAnchor 以订阅自身的周窗口起点作为周切分锚点（而非自然周），
// 这样每周使用率的分桶边界与计费实际的重置边界一致。
func weeklyAnchor(sub *UserSubscription, cycleStart time.Time) time.Time {
	if sub.WeeklyWindowStart == nil {
		return cycleStart
	}
	anchor := startOfDay(*sub.WeeklyWindowStart)
	for anchor.After(cycleStart) {
		anchor = anchor.AddDate(0, 0, -7)
	}
	return anchor
}

func buildWeeklyPoints(rows []SubscriptionUsageDaily, anchor time.Time) []SubscriptionUsageWeeklyPoint {
	points := []SubscriptionUsageWeeklyPoint{}
	if len(rows) == 0 {
		return points
	}

	type bucket struct {
		cost     float64
		requests int64
		limit    float64
		derived  bool
	}
	order := []time.Time{}
	buckets := map[time.Time]*bucket{}

	for i := range rows {
		row := rows[i]
		offsetDays := int(row.BucketDate.Sub(anchor).Hours() / 24)
		if offsetDays < 0 {
			offsetDays = 0
		}
		weekStart := anchor.AddDate(0, 0, (offsetDays/7)*7)

		b, ok := buckets[weekStart]
		if !ok {
			b = &bucket{}
			buckets[weekStart] = b
			order = append(order, weekStart)
		}
		b.cost += row.CostUSD
		b.requests += row.RequestCount
		if limit, derived := derivedWeeklyLimit(row); limit > 0 {
			// 同一周内限额若变动，取最后一天的快照为准。
			b.limit = limit
			b.derived = derived
		}
	}

	sort.Slice(order, func(i, j int) bool { return order[i].Before(order[j]) })
	for _, weekStart := range order {
		b := buckets[weekStart]
		point := SubscriptionUsageWeeklyPoint{
			WeekStart:      weekStart.Format(time.DateOnly),
			WeekEnd:        weekStart.AddDate(0, 0, 6).Format(time.DateOnly),
			CostUSD:        b.cost,
			Requests:       b.requests,
			LimitUSD:       b.limit,
			LimitIsDerived: b.derived,
		}
		if b.limit > 0 {
			point.UsageRatio = b.cost / b.limit
		}
		points = append(points, point)
	}
	return points
}

// derivedWeeklyLimit 给出某一周的额度分母，第二个返回值表示它是否为折算值。
func derivedWeeklyLimit(row SubscriptionUsageDaily) (float64, bool) {
	if row.WeeklyLimitUSD != nil && *row.WeeklyLimitUSD > 0 {
		return *row.WeeklyLimitUSD, false
	}
	if row.DailyLimitUSD != nil && *row.DailyLimitUSD > 0 {
		return *row.DailyLimitUSD * 7, true
	}
	if row.MonthlyLimitUSD != nil && *row.MonthlyLimitUSD > 0 {
		return *row.MonthlyLimitUSD / 30 * 7, true
	}
	return 0, false
}

// buildCycle 用「限额 × 周期内已经历的窗口数」作为整周期总额度。
// 分组没有任何窗口限额时返回 nil：没有分母的「使用率」不该被渲染成 0%。
func buildCycle(group *Group, cycleStart, cycleEnd time.Time, totalCost float64) *SubscriptionUsageCycle {
	if group == nil {
		return nil
	}

	cycle := &SubscriptionUsageCycle{
		Start:   cycleStart.Format(time.DateOnly),
		End:     cycleEnd.Format(time.DateOnly),
		CostUSD: totalCost,
	}

	elapsed := cycleEnd.Sub(cycleStart)
	if elapsed < 0 {
		elapsed = 0
	}

	var limit float64
	var period time.Duration
	switch {
	case group.HasDailyLimit():
		limit, period, cycle.WindowKind = *group.DailyLimitUSD, dailyWindowPeriod, "daily"
	case group.HasWeeklyLimit():
		limit, period, cycle.WindowKind = *group.WeeklyLimitUSD, weeklyWindowPeriod, "weekly"
	case group.HasMonthlyLimit():
		limit, period, cycle.WindowKind = *group.MonthlyLimitUSD, monthlyWindowPeriod, "monthly"
	default:
		return nil
	}

	cycle.WindowsElapsed = int64(elapsed/period) + 1
	cycle.QuotaUSD = limit * float64(cycle.WindowsElapsed)
	if cycle.QuotaUSD > 0 {
		cycle.UsageRatio = totalCost / cycle.QuotaUSD
	}
	return cycle
}
