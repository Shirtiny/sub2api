package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestControlSnapshotRolloutRestoresFingerprintAggregation(t *testing.T) {
	snapshotContent, err := FS.ReadFile("192_request_control_request_snapshot.sql")
	require.NoError(t, err)
	require.Contains(t, string(snapshotContent), "request_snapshot JSONB")

	restoreContent, err := FS.ReadFile("193_restore_request_control_fingerprint_aggregation.sql")
	require.NoError(t, err)
	restoreSQL := strings.Join(strings.Fields(string(restoreContent)), " ")
	require.Contains(t, restoreSQL, "SUM(GREATEST(event_count, 1))")
	require.Contains(t, restoreSQL, "ROW_NUMBER() OVER")
	require.Contains(t, restoreSQL, "DELETE FROM request_control_logs AS duplicate")
	require.Contains(t, restoreSQL, "CREATE UNIQUE INDEX IF NOT EXISTS uq_request_control_logs_dedupe")
}
