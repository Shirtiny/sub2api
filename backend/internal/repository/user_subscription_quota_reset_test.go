//go:build unit

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestResetUserQuotaRepositoryIsAtomicAndScoped(t *testing.T) {
	dbName := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	user, err := client.User.Create().SetEmail("quota-reset-user@example.com").SetPasswordHash("hash").SetUsername("quota-reset-user").Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().SetName("quota-reset-group").SetStatus(service.StatusActive).SetPlatform(service.PlatformOpenAI).SetSubscriptionType(service.SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	now := time.Now()
	sub, err := client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(group.ID).SetStartsAt(now.Add(-48 * time.Hour)).SetExpiresAt(now.Add(48 * time.Hour)).SetStatus(service.SubscriptionStatusActive).SetDailyUsageUsd(3).SetWeeklyUsageUsd(4).SetMonthlyUsageUsd(5).SetResetCount(1).Save(ctx)
	require.NoError(t, err)

	repo := NewUserSubscriptionRepository(client).(*userSubscriptionRepository)
	updated, err := repo.ResetUserQuota(ctx, service.UserQuotaResetParams{SubscriptionID: sub.ID, UserID: user.ID, Now: now, WindowStart: now.Truncate(24 * time.Hour)})
	require.NoError(t, err)
	require.Equal(t, 0, updated.ResetCount)
	require.Zero(t, updated.DailyUsageUSD)
	require.Zero(t, updated.WeeklyUsageUSD)
	require.Equal(t, 5.0, updated.MonthlyUsageUSD)

	_, err = repo.ResetUserQuota(ctx, service.UserQuotaResetParams{SubscriptionID: sub.ID, UserID: user.ID, Now: now, WindowStart: now})
	require.ErrorIs(t, err, service.ErrQuotaResetExhausted)
	_, err = repo.ResetUserQuota(ctx, service.UserQuotaResetParams{SubscriptionID: sub.ID, UserID: user.ID + 1, Now: now, WindowStart: now})
	require.ErrorIs(t, err, service.ErrSubscriptionNotFound)

	require.NoError(t, repo.SetResetCount(ctx, sub.ID, 3))
	counted, err := repo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, 3, counted.ResetCount)
	updatedCount, err := repo.BulkSetResetCount(ctx, []int64{sub.ID}, 4)
	require.NoError(t, err)
	require.Equal(t, int64(1), updatedCount)
	counted, err = repo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, 4, counted.ResetCount)
}
