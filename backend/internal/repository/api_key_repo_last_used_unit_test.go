package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newAPIKeyRepoSQLite(t *testing.T) (*apiKeyRepository, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:api_key_repo_last_used?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return &apiKeyRepository{client: client}, client
}

func mustCreateAPIKeyRepoUser(t *testing.T, ctx context.Context, client *dbent.Client, email string) *service.User {
	t.Helper()
	u, err := client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-password-hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	return userEntityToService(u)
}

func TestAPIKeyRepository_CreateWithLastUsedAt(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "create-last-used@test.com")

	lastUsed := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	key := &service.APIKey{
		UserID:     user.ID,
		Key:        "sk-create-last-used",
		Name:       "CreateWithLastUsed",
		Status:     service.StatusActive,
		LastUsedAt: &lastUsed,
	}

	require.NoError(t, repo.Create(ctx, key))
	require.NotNil(t, key.LastUsedAt)
	require.WithinDuration(t, lastUsed, *key.LastUsedAt, time.Second)

	got, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
	require.WithinDuration(t, lastUsed, *got.LastUsedAt, time.Second)
}

func TestAPIKeyRepository_UpdateLastUsed(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "update-last-used@test.com")

	key := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-update-last-used",
		Name:   "UpdateLastUsed",
		Status: service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))

	before, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Nil(t, before.LastUsedAt)

	target := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)
	require.NoError(t, repo.UpdateLastUsed(ctx, key.ID, target))

	after, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, after.LastUsedAt)
	require.WithinDuration(t, target, *after.LastUsedAt, time.Second)
	require.WithinDuration(t, target, after.UpdatedAt, time.Second)
}

func TestAPIKeyRepository_UpdateLastUsedDeletedKey(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "deleted-last-used@test.com")

	key := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-update-last-used-deleted",
		Name:   "UpdateLastUsedDeleted",
		Status: service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))
	require.NoError(t, repo.Delete(ctx, key.ID))

	err := repo.UpdateLastUsed(ctx, key.ID, time.Now().UTC())
	require.ErrorIs(t, err, service.ErrAPIKeyNotFound)
}

func TestAPIKeyRepository_UpdateLastUsedDBError(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "db-error-last-used@test.com")

	key := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-update-last-used-db-error",
		Name:   "UpdateLastUsedDBError",
		Status: service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))

	require.NoError(t, client.Close())
	err := repo.UpdateLastUsed(ctx, key.ID, time.Now().UTC())
	require.Error(t, err)
}

func TestAPIKeyRepository_CreateDuplicateKey(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "duplicate-key@test.com")

	first := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-duplicate",
		Name:   "first",
		Status: service.StatusActive,
	}
	second := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-duplicate",
		Name:   "second",
		Status: service.StatusActive,
	}

	require.NoError(t, repo.Create(ctx, first))
	err := repo.Create(ctx, second)
	require.ErrorIs(t, err, service.ErrAPIKeyExists)
}

func TestAPIKeyRepository_RotateKeyUsesGuardForHashBackedRows(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "rotate-hash@test.com")

	oldKey := "cafepass-rotate-old-hash"
	oldStorage := service.APIKeyStorageHash(oldKey, nil)
	oldLookup := service.APIKeyLookupHashValue(oldKey)
	key := &service.APIKey{
		UserID:        user.ID,
		Key:           oldKey,
		KeyHash:       oldStorage.Hash,
		KeyHashAlg:    oldStorage.Alg,
		KeyLookupHash: oldLookup,
		KeyPrefix:     service.APIKeyPrefix(oldKey),
		Name:          "RotateHash",
		Status:        service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))

	newKey := "cafepass-rotate-new-hash"
	newStorage := service.APIKeyStorageHash(newKey, nil)
	rotated := *key
	rotated.Key = newKey
	rotated.KeyHash = newStorage.Hash
	rotated.KeyHashAlg = newStorage.Alg
	rotated.KeyLookupHash = service.APIKeyLookupHashValue(newKey)
	rotated.KeyPrefix = service.APIKeyPrefix(newKey)
	guard := service.APIKeyRotationGuard{
		KeyHash:       oldStorage.Hash,
		KeyHashAlg:    oldStorage.Alg,
		KeyLookupHash: oldLookup,
	}
	require.NoError(t, repo.RotateKey(ctx, &rotated, guard))

	stale := rotated
	stale.Key = "cafepass-rotate-stale"
	staleStorage := service.APIKeyStorageHash(stale.Key, nil)
	stale.KeyHash = staleStorage.Hash
	stale.KeyHashAlg = staleStorage.Alg
	stale.KeyLookupHash = service.APIKeyLookupHashValue(stale.Key)
	stale.KeyPrefix = service.APIKeyPrefix(stale.Key)
	err := repo.RotateKey(ctx, &stale, guard)
	require.ErrorIs(t, err, service.ErrAPIKeyRotated)
}

func TestAPIKeyRepository_RotateKeyUsesGuardForLegacyPlaintextRows(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "rotate-legacy@test.com")

	legacyKey := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-legacy-rotate-old",
		Name:   "RotateLegacy",
		Status: service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, legacyKey))

	newKey := "cafepass-legacy-rotate-new"
	newStorage := service.APIKeyStorageHash(newKey, nil)
	rotated := *legacyKey
	rotated.Key = newKey
	rotated.KeyHash = newStorage.Hash
	rotated.KeyHashAlg = newStorage.Alg
	rotated.KeyLookupHash = service.APIKeyLookupHashValue(newKey)
	rotated.KeyPrefix = service.APIKeyPrefix(newKey)
	guard := service.APIKeyRotationGuard{Key: legacyKey.Key}
	require.NoError(t, repo.RotateKey(ctx, &rotated, guard))

	stale := rotated
	stale.Key = "cafepass-legacy-rotate-stale"
	staleStorage := service.APIKeyStorageHash(stale.Key, nil)
	stale.KeyHash = staleStorage.Hash
	stale.KeyHashAlg = staleStorage.Alg
	stale.KeyLookupHash = service.APIKeyLookupHashValue(stale.Key)
	stale.KeyPrefix = service.APIKeyPrefix(stale.Key)
	err := repo.RotateKey(ctx, &stale, guard)
	require.ErrorIs(t, err, service.ErrAPIKeyRotated)
}
