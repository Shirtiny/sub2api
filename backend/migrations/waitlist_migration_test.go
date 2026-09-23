package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration198WaitlistIsAdditiveAndDeduplicates(t *testing.T) {
	data, err := FS.ReadFile("198_waitlist_entries.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(data)), " ")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS waitlist_entries")
	require.Contains(t, sql, "email VARCHAR(254) NOT NULL UNIQUE")
	require.Contains(t, sql, "created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()")
	require.NotContains(t, strings.ToUpper(sql), "DROP ")
	require.NotContains(t, strings.ToUpper(sql), "ALTER TABLE users")
}

func TestMigration199AddsNullableConfirmationState(t *testing.T) {
	data, err := FS.ReadFile("199_waitlist_confirmation.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(data)), " ")
	require.Contains(t, sql, "ALTER TABLE waitlist_entries")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS confirmation_attempted_at TIMESTAMPTZ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS confirmation_sent_at TIMESTAMPTZ")
	require.NotContains(t, sql, "UPDATE ")
	require.NotContains(t, sql, "DROP ")
	require.NotContains(t, sql, "NOT NULL")
}

func TestMigration200AddsWaitlistApprovalWithoutGrantingAccess(t *testing.T) {
	data, err := FS.ReadFile("200_waitlist_approval.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(data)), " ")
	for _, field := range []string{"approved_at TIMESTAMPTZ", "approved_by BIGINT", "granted_user_id BIGINT", "approval_notice_attempted_at TIMESTAMPTZ", "approval_notice_sent_at TIMESTAMPTZ"} {
		require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS "+field)
	}
	require.NotContains(t, sql, "UPDATE ")
	require.NotContains(t, sql, "DROP ")
	require.NotContains(t, sql, "ON DELETE SET NULL")
}
