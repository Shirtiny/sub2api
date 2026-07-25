//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMigration182RebuildsEarlyResetTermsWithoutUnfulfilledOrders(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	now := time.Now().UTC().Truncate(time.Second)
	startsAt := now.Add(-10 * 24 * time.Hour)
	expiresAt := now.Add(25 * 24 * time.Hour)
	suffix := time.Now().UnixNano()

	var userID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash, username)
		VALUES ($1, 'migration-test', 'migration-test')
		RETURNING id
	`, fmt.Sprintf("migration-182-%d@example.com", suffix)).Scan(&userID))

	var groupID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
		INSERT INTO groups (name, platform, subscription_type)
		VALUES ($1, 'openai', 'standard')
		RETURNING id
	`, fmt.Sprintf("migration-182-%d", suffix)).Scan(&groupID))

	var subscriptionID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
		INSERT INTO user_subscriptions (user_id, group_id, starts_at, expires_at, status)
		VALUES ($1, $2, $3, $4, 'active')
		RETURNING id
	`, userID, groupID, startsAt, expiresAt).Scan(&subscriptionID))

	insertOrder := func(status string, days int, paidAt time.Time, completedAt *time.Time) int64 {
		t.Helper()
		var orderID int64
		err := tx.QueryRowContext(ctx, `
			INSERT INTO payment_orders (
				user_id, amount, pay_amount, order_type, subscription_group_id,
				subscription_days, status, expires_at, paid_at, completed_at,
				out_trade_no, subscription_early_reset_enabled,
				subscription_early_reset_duration_days
			)
			VALUES ($1, 1, 1, 'subscription', $2, $3, $4, $5, $6, $7, $8, false, 0)
			RETURNING id
		`, userID, groupID, days, status, now.Add(time.Hour), paidAt, completedAt,
			fmt.Sprintf("migration-182-%d-%s-%d", suffix, status, days)).Scan(&orderID)
		require.NoError(t, err)
		return orderID
	}

	completedAt := now.Add(-9 * 24 * time.Hour)
	completedOrderID := insertOrder("COMPLETED", 30, completedAt, &completedAt)
	rechargingOrderID := insertOrder("RECHARGING", 5, now.Add(-time.Hour), nil)
	unfulfilledOrderID := insertOrder("PAID", 7, now.Add(-30*time.Minute), nil)

	_, err := tx.ExecContext(ctx, `
		INSERT INTO payment_audit_logs (order_id, action)
		VALUES ($1, 'SUBSCRIPTION_ASSIGNED')
	`, fmt.Sprintf("%d", rechargingOrderID))
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `
		UPDATE schema_migrations
		SET applied_at = NOW()
		WHERE filename = '179_subscription_early_reset_entitlements.sql'
	`)
	require.NoError(t, err)

	migration179, err := os.ReadFile(filepath.Join("..", "..", "migrations", "179_subscription_early_reset_entitlements.sql"))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration179))
	require.NoError(t, err)

	var beforeRepair int
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM subscription_early_reset_entitlements
		WHERE subscription_id = $1
	`, subscriptionID).Scan(&beforeRepair))
	require.Equal(t, 3, beforeRepair, "migration 179 should reproduce the unsafe in-flight-order backfill")

	migration182, err := os.ReadFile(filepath.Join("..", "..", "migrations", "182_rebuild_fulfilled_early_reset_entitlements.sql"))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration182))
	require.NoError(t, err)

	type entitlementWindow struct {
		SourceOrderID sql.NullInt64
		StartsAt      time.Time
		ExpiresAt     time.Time
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT source_order_id, starts_at, expires_at
		FROM subscription_early_reset_entitlements
		WHERE subscription_id = $1
		ORDER BY starts_at, id
	`, subscriptionID)
	require.NoError(t, err)
	defer rows.Close()

	windows := make([]entitlementWindow, 0, 2)
	for rows.Next() {
		var window entitlementWindow
		require.NoError(t, rows.Scan(&window.SourceOrderID, &window.StartsAt, &window.ExpiresAt))
		windows = append(windows, window)
	}
	require.NoError(t, rows.Err())
	require.Len(t, windows, 2)

	require.Equal(t, completedOrderID, windows[0].SourceOrderID.Int64)
	require.WithinDuration(t, startsAt, windows[0].StartsAt, time.Microsecond)
	require.WithinDuration(t, expiresAt.Add(-5*24*time.Hour), windows[0].ExpiresAt, time.Microsecond)

	require.Equal(t, rechargingOrderID, windows[1].SourceOrderID.Int64)
	require.WithinDuration(t, expiresAt.Add(-5*24*time.Hour), windows[1].StartsAt, time.Microsecond)
	require.WithinDuration(t, expiresAt, windows[1].ExpiresAt, time.Microsecond)

	for _, window := range windows {
		require.NotEqual(t, unfulfilledOrderID, window.SourceOrderID.Int64)
	}
}
