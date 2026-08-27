package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const userAPIKeyUsageDailyTable = "user_api_key_usage_daily"

type durableUsageRange struct {
	startDate string
	endDate   string
	timezone  string
}

func newDurableUsageRange(start, end time.Time) durableUsageRange {
	return newDurableUsageRangeAt(start, end, timezone.Now())
}

func newDurableUsageRangeAt(start, end, now time.Time) durableUsageRange {
	loc := timezone.Location()
	startLocal := start.In(loc)
	endLocal := end.In(loc)

	rollupStart := timezone.StartOfDay(startLocal)
	if startLocal.After(rollupStart) {
		rollupStart = rollupStart.AddDate(0, 0, 1)
	}
	rollupEnd := timezone.StartOfDay(endLocal)
	today := timezone.StartOfDay(now)
	settledEnd := today
	if now.In(loc).Before(today.Add(service.UserAPIKeyUsageDailySettlementDelay)) {
		settledEnd = today.AddDate(0, 0, -1)
	}
	if rollupEnd.After(settledEnd) {
		rollupEnd = settledEnd
	}
	if rollupEnd.Before(rollupStart) {
		rollupEnd = rollupStart
	}

	return durableUsageRange{
		startDate: rollupStart.Format("2006-01-02"),
		endDate:   rollupEnd.Format("2006-01-02"),
		timezone:  timezone.Name(),
	}
}

func durableUsageDimensionColumn(dimension string) (string, error) {
	switch dimension {
	case "user_id", "api_key_id":
		return dimension, nil
	default:
		return "", fmt.Errorf("unsupported durable usage dimension %q", dimension)
	}
}

func (r *usageLogRepository) getDurableUsageStats(
	ctx context.Context,
	dimension string,
	id int64,
	startTime, endTime time.Time,
) (*usagestats.UsageStats, error) {
	column, err := durableUsageDimensionColumn(dimension)
	if err != nil {
		return nil, err
	}
	rangeDays := newDurableUsageRange(startTime, endTime)
	query := fmt.Sprintf(`
		WITH covered_days AS (
			SELECT DISTINCT bucket_date
			FROM %s
			WHERE %s = $1
			  AND bucket_date >= $4::date
			  AND bucket_date < $5::date
		), parts AS (
			SELECT
				COALESCE(SUM(request_count), 0)::bigint AS requests,
				COALESCE(SUM(input_tokens), 0)::bigint AS input_tokens,
				COALESCE(SUM(output_tokens), 0)::bigint AS output_tokens,
				COALESCE(SUM(cache_creation_tokens), 0)::bigint AS cache_creation_tokens,
				COALESCE(SUM(cache_read_tokens), 0)::bigint AS cache_read_tokens,
				COALESCE(SUM(total_cost), 0)::numeric AS total_cost,
				COALESCE(SUM(actual_cost), 0)::numeric AS actual_cost,
				COALESCE(SUM(total_duration_ms), 0)::bigint AS total_duration_ms
			FROM %s
			WHERE %s = $1
			  AND bucket_date >= $4::date
			  AND bucket_date < $5::date

			UNION ALL

			SELECT
				COUNT(*)::bigint,
				COALESCE(SUM(input_tokens), 0)::bigint,
				COALESCE(SUM(output_tokens), 0)::bigint,
				COALESCE(SUM(cache_creation_tokens), 0)::bigint,
				COALESCE(SUM(cache_read_tokens), 0)::bigint,
				COALESCE(SUM(total_cost), 0)::numeric,
				COALESCE(SUM(actual_cost), 0)::numeric,
				COALESCE(SUM(COALESCE(duration_ms, 0)), 0)::bigint
			FROM usage_logs
			WHERE %s = $1
			  AND created_at >= $2
			  AND created_at < $3
			  AND NOT EXISTS (
				SELECT 1
				FROM covered_days covered
				WHERE covered.bucket_date = (created_at AT TIME ZONE $6)::date
			  )
		)
		SELECT
			COALESCE(SUM(requests), 0)::bigint,
			COALESCE(SUM(input_tokens), 0)::bigint,
			COALESCE(SUM(output_tokens), 0)::bigint,
			COALESCE(SUM(cache_creation_tokens), 0)::bigint,
			COALESCE(SUM(cache_read_tokens), 0)::bigint,
			COALESCE(SUM(total_cost), 0)::numeric,
			COALESCE(SUM(actual_cost), 0)::numeric,
			COALESCE(SUM(total_duration_ms), 0)::bigint
		FROM parts
	`, userAPIKeyUsageDailyTable, column, userAPIKeyUsageDailyTable, column, column)

	var stats usagestats.UsageStats
	var totalDurationMs int64
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{id, startTime, endTime, rangeDays.startDate, rangeDays.endDate, rangeDays.timezone},
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheCreationTokens,
		&stats.TotalCacheReadTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&totalDurationMs,
	); err != nil {
		return nil, err
	}
	stats.TotalCacheTokens = stats.TotalCacheCreationTokens + stats.TotalCacheReadTokens
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens
	if stats.TotalRequests > 0 {
		stats.AverageDurationMs = float64(totalDurationMs) / float64(stats.TotalRequests)
	}

	cacheStats, err := r.getDurableCacheGroupTypeStats(ctx, column, id, startTime, endTime, rangeDays)
	if err != nil {
		return nil, err
	}
	stats.CacheByGroupType = cacheStats
	return &stats, nil
}

func (r *usageLogRepository) getDurableCacheGroupTypeStats(
	ctx context.Context,
	column string,
	id int64,
	startTime, endTime time.Time,
	rangeDays durableUsageRange,
) (results []usagestats.CacheGroupTypeStat, err error) {
	query := fmt.Sprintf(`
		WITH covered_days AS (
			SELECT DISTINCT bucket_date
			FROM %s
			WHERE %s = $1
			  AND bucket_date >= $4::date
			  AND bucket_date < $5::date
		), parts AS (
			SELECT
				billing_type,
				COALESCE(SUM(request_count), 0)::bigint AS requests,
				COALESCE(SUM(input_tokens), 0)::bigint AS input_tokens,
				COALESCE(SUM(cache_creation_tokens), 0)::bigint AS cache_creation_tokens,
				COALESCE(SUM(cache_read_tokens), 0)::bigint AS cache_read_tokens
			FROM %s
			WHERE %s = $1
			  AND bucket_date >= $4::date
			  AND bucket_date < $5::date
			GROUP BY billing_type

			UNION ALL

			SELECT
				CASE WHEN billing_type = 1 OR subscription_id IS NOT NULL THEN 1 ELSE 0 END,
				COUNT(*)::bigint,
				COALESCE(SUM(input_tokens), 0)::bigint,
				COALESCE(SUM(cache_creation_tokens), 0)::bigint,
				COALESCE(SUM(cache_read_tokens), 0)::bigint
			FROM usage_logs
			WHERE %s = $1
			  AND created_at >= $2
			  AND created_at < $3
			  AND NOT EXISTS (
				SELECT 1
				FROM covered_days covered
				WHERE covered.bucket_date = (created_at AT TIME ZONE $6)::date
			  )
			GROUP BY 1
		)
		SELECT
			CASE WHEN billing_type = 1 THEN 'subscription' ELSE 'standard' END AS group_type,
			COALESCE(SUM(requests), 0)::bigint,
			COALESCE(SUM(input_tokens), 0)::bigint,
			COALESCE(SUM(cache_creation_tokens), 0)::bigint,
			COALESCE(SUM(cache_read_tokens), 0)::bigint
		FROM parts
		GROUP BY billing_type
		HAVING COALESCE(SUM(input_tokens), 0)
			+ COALESCE(SUM(cache_creation_tokens), 0)
			+ COALESCE(SUM(cache_read_tokens), 0) > 0
		ORDER BY billing_type DESC
	`, userAPIKeyUsageDailyTable, column, userAPIKeyUsageDailyTable, column, column)

	rows, err := r.sql.QueryContext(
		ctx,
		query,
		id,
		startTime,
		endTime,
		rangeDays.startDate,
		rangeDays.endDate,
		rangeDays.timezone,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	for rows.Next() {
		var row usagestats.CacheGroupTypeStat
		if err = rows.Scan(
			&row.GroupType,
			&row.Requests,
			&row.InputTokens,
			&row.CacheCreationTokens,
			&row.CacheReadTokens,
		); err != nil {
			return nil, err
		}
		row.TotalInputTokens = row.InputTokens + row.CacheCreationTokens + row.CacheReadTokens
		row.HitRate = float64(row.CacheReadTokens) / float64(row.TotalInputTokens) * 100
		results = append(results, row)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *usageLogRepository) getDurableDailyUsageTrend(
	ctx context.Context,
	userID, apiKeyID int64,
	startTime, endTime time.Time,
) (results []usagestats.TrendDataPoint, err error) {
	rangeDays := newDurableUsageRange(startTime, endTime)
	rollupKeyFilter := ""
	logKeyFilter := ""
	if apiKeyID > 0 {
		rollupKeyFilter = "AND api_key_id = $2"
		logKeyFilter = "AND api_key_id = $2"
	} else {
		rollupKeyFilter = "AND $2::bigint = 0"
		logKeyFilter = "AND $2::bigint = 0"
	}
	query := fmt.Sprintf(`
		WITH covered_days AS (
			SELECT DISTINCT bucket_date
			FROM %s
			WHERE user_id = $1
			  %s
			  AND bucket_date >= $5::date
			  AND bucket_date < $6::date
		), parts AS (
			SELECT
				bucket_date,
				COALESCE(SUM(request_count), 0)::bigint AS requests,
				COALESCE(SUM(input_tokens), 0)::bigint AS input_tokens,
				COALESCE(SUM(output_tokens), 0)::bigint AS output_tokens,
				COALESCE(SUM(cache_creation_tokens), 0)::bigint AS cache_creation_tokens,
				COALESCE(SUM(cache_read_tokens), 0)::bigint AS cache_read_tokens,
				COALESCE(SUM(total_cost), 0)::numeric AS total_cost,
				COALESCE(SUM(actual_cost), 0)::numeric AS actual_cost
			FROM %s
			WHERE user_id = $1
			  %s
			  AND bucket_date >= $5::date
			  AND bucket_date < $6::date
			GROUP BY bucket_date

			UNION ALL

			SELECT
				(created_at AT TIME ZONE $7)::date,
				COUNT(*)::bigint,
				COALESCE(SUM(input_tokens), 0)::bigint,
				COALESCE(SUM(output_tokens), 0)::bigint,
				COALESCE(SUM(cache_creation_tokens), 0)::bigint,
				COALESCE(SUM(cache_read_tokens), 0)::bigint,
				COALESCE(SUM(total_cost), 0)::numeric,
				COALESCE(SUM(actual_cost), 0)::numeric
			FROM usage_logs
			WHERE user_id = $1
			  %s
			  AND created_at >= $3
			  AND created_at < $4
			  AND NOT EXISTS (
				SELECT 1
				FROM covered_days covered
				WHERE covered.bucket_date = (created_at AT TIME ZONE $7)::date
			  )
			GROUP BY 1
		)
		SELECT
			TO_CHAR(bucket_date, 'YYYY-MM-DD'),
			COALESCE(SUM(requests), 0)::bigint,
			COALESCE(SUM(input_tokens), 0)::bigint,
			COALESCE(SUM(output_tokens), 0)::bigint,
			COALESCE(SUM(cache_creation_tokens), 0)::bigint,
			COALESCE(SUM(cache_read_tokens), 0)::bigint,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0)::bigint,
			COALESCE(SUM(total_cost), 0)::numeric,
			COALESCE(SUM(actual_cost), 0)::numeric
		FROM parts
		GROUP BY bucket_date
		ORDER BY bucket_date
	`, userAPIKeyUsageDailyTable, rollupKeyFilter, userAPIKeyUsageDailyTable, rollupKeyFilter, logKeyFilter)

	rows, err := r.sql.QueryContext(
		ctx,
		query,
		userID,
		apiKeyID,
		startTime,
		endTime,
		rangeDays.startDate,
		rangeDays.endDate,
		rangeDays.timezone,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()
	results, err = scanTrendRows(rows)
	return results, err
}

func (r *usageLogRepository) GetAPIKeyDailyUsageTrend(
	ctx context.Context,
	userID, apiKeyID int64,
	startTime, endTime time.Time,
) ([]usagestats.TrendDataPoint, error) {
	return r.getDurableDailyUsageTrend(ctx, userID, apiKeyID, startTime, endTime)
}

func (r *usageLogRepository) getDurableBatchAPIKeyUsageStats(
	ctx context.Context,
	apiKeyIDs []int64,
	startTime, endTime time.Time,
) (results map[int64]*usagestats.BatchAPIKeyUsageStats, err error) {
	results = make(map[int64]*usagestats.BatchAPIKeyUsageStats, len(apiKeyIDs))
	for _, id := range apiKeyIDs {
		results[id] = &usagestats.BatchAPIKeyUsageStats{APIKeyID: id}
	}
	if len(apiKeyIDs) == 0 {
		return results, nil
	}

	rangeDays := newDurableUsageRange(startTime, endTime)
	today := timezone.Today()
	query := fmt.Sprintf(`
		WITH covered_days AS (
			SELECT DISTINCT api_key_id, bucket_date
			FROM %s
			WHERE api_key_id = ANY($1)
			  AND bucket_date >= $5::date
			  AND bucket_date < $6::date
		), parts AS (
			SELECT
				api_key_id,
				COALESCE(SUM(actual_cost), 0)::numeric AS total_cost,
				0::numeric AS today_cost
			FROM %s
			WHERE api_key_id = ANY($1)
			  AND bucket_date >= $5::date
			  AND bucket_date < $6::date
			GROUP BY api_key_id

			UNION ALL

			SELECT
				api_key_id,
				COALESCE(SUM(actual_cost) FILTER (
					WHERE created_at >= $2 AND created_at < $3
					  AND NOT EXISTS (
						SELECT 1
						FROM covered_days covered
						WHERE covered.api_key_id = usage_logs.api_key_id
						  AND covered.bucket_date = (created_at AT TIME ZONE $7)::date
					  )
				), 0)::numeric,
				COALESCE(SUM(actual_cost) FILTER (WHERE created_at >= $4), 0)::numeric
			FROM usage_logs
			WHERE api_key_id = ANY($1)
			  AND created_at >= LEAST($2, $4)
			GROUP BY api_key_id
		)
		SELECT api_key_id, SUM(total_cost), SUM(today_cost)
		FROM parts
		GROUP BY api_key_id
	`, userAPIKeyUsageDailyTable, userAPIKeyUsageDailyTable)

	rows, err := r.sql.QueryContext(
		ctx,
		query,
		pq.Array(apiKeyIDs),
		startTime,
		endTime,
		today,
		rangeDays.startDate,
		rangeDays.endDate,
		rangeDays.timezone,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	for rows.Next() {
		var apiKeyID int64
		var total, todayCost float64
		if err = rows.Scan(&apiKeyID, &total, &todayCost); err != nil {
			return nil, err
		}
		if row := results[apiKeyID]; row != nil {
			row.TotalActualCost = total
			row.TodayActualCost = todayCost
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func durableUsageDefaultRange() (time.Time, time.Time) {
	today := timezone.Today()
	return today.AddDate(0, 0, -29), timezone.Now()
}

func durableUsageRetentionRange() (time.Time, time.Time) {
	return time.Unix(0, 0).In(timezone.Location()), timezone.Now()
}

func (r *usageLogRepository) getDurableBatchUserCosts(
	ctx context.Context,
	userIDs []int64,
	startTime, endTime time.Time,
) (result map[int64][2]float64, err error) {
	result = make(map[int64][2]float64, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	rangeDays := newDurableUsageRange(startTime, endTime)
	today := timezone.Today()
	query := fmt.Sprintf(`
		WITH covered_days AS (
			SELECT DISTINCT user_id, bucket_date
			FROM %s
			WHERE user_id = ANY($1)
			  AND bucket_date >= $5::date
			  AND bucket_date < $6::date
		), parts AS (
			SELECT
				user_id,
				COALESCE(SUM(actual_cost), 0)::numeric AS total_cost,
				0::numeric AS today_cost
			FROM %s
			WHERE user_id = ANY($1)
			  AND bucket_date >= $5::date
			  AND bucket_date < $6::date
			GROUP BY user_id

			UNION ALL

			SELECT
				user_id,
				COALESCE(SUM(actual_cost) FILTER (
					WHERE created_at >= $2 AND created_at < $3
					  AND NOT EXISTS (
						SELECT 1
						FROM covered_days covered
						WHERE covered.user_id = usage_logs.user_id
						  AND covered.bucket_date = (created_at AT TIME ZONE $7)::date
					  )
				), 0)::numeric,
				COALESCE(SUM(actual_cost) FILTER (WHERE created_at >= $4), 0)::numeric
			FROM usage_logs
			WHERE user_id = ANY($1)
			  AND created_at >= LEAST($2, $4)
			  AND actual_cost > 0
			GROUP BY user_id
		)
		SELECT user_id, SUM(total_cost), SUM(today_cost)
		FROM parts
		GROUP BY user_id
	`, userAPIKeyUsageDailyTable, userAPIKeyUsageDailyTable)

	rows, err := r.sql.QueryContext(
		ctx,
		query,
		pq.Array(userIDs),
		startTime,
		endTime,
		today,
		rangeDays.startDate,
		rangeDays.endDate,
		rangeDays.timezone,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
		}
	}()
	for rows.Next() {
		var userID int64
		var total, todayCost float64
		if err = rows.Scan(&userID, &total, &todayCost); err != nil {
			return nil, err
		}
		result[userID] = [2]float64{total, todayCost}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
