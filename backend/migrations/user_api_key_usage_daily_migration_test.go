package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration195CreatesStableUserAPIKeyDailyTotals(t *testing.T) {
	content, err := FS.ReadFile("195_user_api_key_usage_daily.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "PRIMARY KEY (bucket_date, user_id, api_key_id, billing_type)")
	require.Contains(t, sql, "AT TIME ZONE CURRENT_SETTING('TimeZone')")
	require.Contains(t, sql, "WHERE ul.created_at < date_trunc('day', NOW())")
	require.Contains(t, sql, "WHERE EXCLUDED.request_count > user_api_key_usage_daily.request_count")
	require.Contains(t, sql, "input_tokens BIGINT NOT NULL")
	require.Contains(t, sql, "actual_cost DECIMAL(20, 10) NOT NULL")
	require.NotContains(t, sql, "platform VARCHAR")
	require.NotContains(t, sql, "model VARCHAR")
}
