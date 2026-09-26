//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestUserConcurrencyPolicyMigration(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	var id int64
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO users (email, password_hash, balance, concurrency)
		VALUES ($1, 'hash', 123.456789, 32) RETURNING id`, fmt.Sprintf("concurrency-migration-%d@example.com", time.Now().UnixNano())).Scan(&id))
	_, err = tx.ExecContext(ctx, "UPDATE settings SET value='9' WHERE key='default_concurrency'")
	require.NoError(t, err)
	sql, err := migrations.FS.ReadFile("207_user_concurrency_policy.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(sql))
	require.NoError(t, err)
	var concurrency int
	var balance float64
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT concurrency, balance FROM users WHERE id=$1", id).Scan(&concurrency, &balance))
	require.Equal(t, 2, concurrency)
	require.Equal(t, 123.456789, balance)
	var defaultValue string
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='default_concurrency'").Scan(&defaultValue))
	require.Equal(t, "2", defaultValue)
	var notNormalized int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE deleted_at IS NULL AND concurrency <> 2").Scan(&notNormalized))
	require.Zero(t, notNormalized)
}

func TestUserConcurrencyPolicyPostgresAndRedis(t *testing.T) {
	parent := t
	ctx := context.Background()
	now := time.Now()
	client := integrationEntClient
	repo := newUserRepositoryWithSQL(client, integrationDB)
	keyRepo := newAPIKeyRepositoryWithSQL(client, integrationDB)
	prefix := fmt.Sprintf("concurrency-policy-%d", now.UnixNano())
	group, err := client.Group.Create().SetName(prefix).SetPlatform("openai").SetSubscriptionType("subscription").Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(ctx, "DELETE FROM groups WHERE id=$1", group.ID) })

	for i, tc := range []struct {
		balance float64
		kind    string
		want    int
	}{{19, "none", 1}, {20, "none", 2}, {100, "none", 3}, {100, "pending", 3}, {0, "legacy", 2}, {100, "plan", 1}, {100, "expired", 3}, {100, "cancelled", 3}, {0, "renewal", 4}} {
		t.Run(tc.kind+fmt.Sprint(i), func(t *testing.T) {
			user, err := client.User.Create().SetEmail(fmt.Sprintf("%s-%d@example.com", prefix, i)).SetPasswordHash("hash").SetConcurrency(32).SetBalance(tc.balance).Save(ctx)
			require.NoError(t, err)
			parent.Cleanup(func() { _, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id=$1", user.ID) })
			if tc.kind != "none" {
				starts, expires, status := now.Add(-time.Hour), now.Add(time.Hour), "active"
				if tc.kind == "pending" {
					starts = now.Add(time.Minute)
				}
				if tc.kind == "expired" {
					expires = now.Add(-time.Minute)
				}
				if tc.kind == "cancelled" {
					status = "cancelled"
				}
				sub, err := client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(group.ID).SetStartsAt(starts).SetExpiresAt(expires).SetStatus(status).Save(ctx)
				require.NoError(t, err)
				if tc.kind != "legacy" {
					grantStart := starts
					if tc.kind == "pending" {
						grantStart = now.Add(-time.Minute)
					}
					_, err = client.SubscriptionConcurrencyEntitlement.Create().SetUserID(user.ID).SetSubscriptionID(sub.ID).SetConcurrency(1).SetStartsAt(grantStart).SetExpiresAt(expires).Save(ctx)
					require.NoError(t, err)
				}
				if tc.kind == "renewal" {
					_, err = client.UserSubscription.UpdateOneID(sub.ID).SetPlanConcurrency(32).Save(ctx)
					require.NoError(t, err)
					_, err = client.SubscriptionConcurrencyEntitlement.Create().SetUserID(user.ID).SetSubscriptionID(sub.ID).SetConcurrency(4).SetStartsAt(now.Add(-time.Minute)).SetExpiresAt(expires).Save(ctx)
					require.NoError(t, err)
				}
			}
			key := &service.APIKey{UserID: user.ID, Key: prefix + fmt.Sprint(i), Name: "concurrency test", Status: service.StatusActive}
			require.NoError(t, keyRepo.Create(ctx, key))
			auth, err := keyRepo.GetByKeyForAuth(ctx, key.Key)
			require.NoError(t, err)
			detail, err := repo.GetByID(ctx, user.ID)
			require.NoError(t, err)
			require.Equal(t, tc.want, detail.EffectiveConcurrencyAt(now))
			require.Equal(t, tc.want, auth.User.EffectiveConcurrencyAt(now), "auth/profile parity")

			cache := NewConcurrencyCache(integrationRedis, 1, 60)
			for slot := 0; slot < tc.want; slot++ {
				req := fmt.Sprintf("%s-%d-%d", prefix, i, slot)
				ok, err := cache.AcquireUserSlot(ctx, user.ID, auth.User.EffectiveConcurrencyAt(now), req)
				require.NoError(t, err)
				require.True(t, ok)
				t.Cleanup(func() { _ = cache.ReleaseUserSlot(ctx, user.ID, req) })
			}
			ok, err := cache.AcquireUserSlot(ctx, user.ID, tc.want, "one-too-many")
			require.NoError(t, err)
			require.False(t, ok, "Redis must enforce the actual policy, not the stored 32")

			// Exercise the PostgreSQL sort expression, not just Go calculation.
			users, _, err := repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 100, SortBy: "concurrency", SortOrder: "asc"}, service.UserListFilters{Search: prefix})
			require.NoError(t, err)
			for j := 1; j < len(users); j++ {
				require.LessOrEqual(t, users[j-1].EffectiveConcurrencyAt(now), users[j].EffectiveConcurrencyAt(now))
			}
		})
	}
}

func TestConfiguredConcurrencyRulesPostgresSortAndHydration(t *testing.T) {
	ctx := context.Background()
	repo := newUserRepositoryWithSQL(integrationEntClient, integrationDB)
	settings := service.NewSettingService(NewSettingRepository(integrationEntClient), nil)
	repo.concurrencySettings = settings
	previous, previousErr := NewSettingRepository(integrationEntClient).GetValue(ctx, service.SettingKeyUserConcurrencyRules)
	t.Cleanup(func() {
		if previousErr == nil {
			_ = NewSettingRepository(integrationEntClient).Set(ctx, service.SettingKeyUserConcurrencyRules, previous)
		} else {
			_ = NewSettingRepository(integrationEntClient).Delete(ctx, service.SettingKeyUserConcurrencyRules)
		}
	})
	rules := service.UserConcurrencyRules{BalanceTiers: []service.BalanceConcurrencyRule{{MinBalance: 0, Concurrency: 6}, {MinBalance: 20, Concurrency: 2}, {MinBalance: 100, Concurrency: 4}}}
	require.NoError(t, settings.SetUserConcurrencyRules(ctx, rules))
	prefix := fmt.Sprintf("configured-policy-%d", time.Now().UnixNano())
	for i, balance := range []float64{0, 19.99, 20, 99.99, 100} {
		user := &service.User{Email: fmt.Sprintf("%s-%d@example.com", prefix, i), PasswordHash: "hash", Role: service.RoleUser, Status: service.StatusActive, Balance: balance, Concurrency: 32}
		require.NoError(t, repo.Create(ctx, user))
		t.Cleanup(func() { _, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id=$1", user.ID) })
		require.Equal(t, rules.BalanceTiers, user.BalanceConcurrencyRules)
		got, err := repo.GetByID(ctx, user.ID)
		require.NoError(t, err)
		want := []int{6, 6, 2, 2, 4}[i]
		require.Equal(t, want, got.EffectiveConcurrencyAt(time.Now()))
		require.Equal(t, rules.BalanceTiers, got.BalanceConcurrencyRules)
	}
	for _, direction := range []string{"asc", "desc"} {
		users, _, err := repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 100, SortBy: "concurrency", SortOrder: direction}, service.UserListFilters{Search: prefix})
		require.NoError(t, err)
		require.Len(t, users, 5)
		for i := 1; i < len(users); i++ {
			a, b := users[i-1].EffectiveConcurrencyAt(time.Now()), users[i].EffectiveConcurrencyAt(time.Now())
			if direction == "asc" {
				require.LessOrEqual(t, a, b)
			} else {
				require.GreaterOrEqual(t, a, b)
			}
		}
	}
}

func TestSingleConfiguredConcurrencyTierSort(t *testing.T) {
	ctx := context.Background()
	settings := service.NewSettingService(NewSettingRepository(integrationEntClient), nil)
	repo := newUserRepositoryWithSQL(integrationEntClient, integrationDB)
	repo.concurrencySettings = settings
	previous, err := NewSettingRepository(integrationEntClient).GetValue(ctx, service.SettingKeyUserConcurrencyRules)
	t.Cleanup(func() {
		if err == nil {
			_ = NewSettingRepository(integrationEntClient).Set(ctx, service.SettingKeyUserConcurrencyRules, previous)
		} else {
			_ = NewSettingRepository(integrationEntClient).Delete(ctx, service.SettingKeyUserConcurrencyRules)
		}
	})
	require.NoError(t, settings.SetUserConcurrencyRules(ctx, service.UserConcurrencyRules{BalanceTiers: []service.BalanceConcurrencyRule{{MinBalance: 0, Concurrency: 3}}}))
	_, _, sortErr := repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 1, SortBy: "concurrency"}, service.UserListFilters{})
	require.NoError(t, sortErr)
}
