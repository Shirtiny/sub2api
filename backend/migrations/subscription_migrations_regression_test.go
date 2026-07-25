package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration182RebuildsEarlyResetTermsFromFulfilledOrdersOnly(t *testing.T) {
	content, err := FS.ReadFile("182_rebuild_fulfilled_early_reset_entitlements.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "sm.filename = '179_subscription_early_reset_entitlements.sql'")
	require.Contains(t, sql, "other.created_at <> sm.applied_at OR other.updated_at <> other.created_at")
	require.Contains(t, sql, "po.status = 'COMPLETED'")
	require.Contains(t, sql, "po.status IN ('PAID', 'RECHARGING') AND EXISTS")
	require.Contains(t, sql, "FROM payment_audit_logs pal")
	require.Contains(t, sql, "pal.action IN ('SUBSCRIPTION_ASSIGNED', 'SUBSCRIPTION_SUCCESS')")
	require.NotContains(t, sql, "po.status IN ('PAID', 'RECHARGING', 'COMPLETED')")
}
