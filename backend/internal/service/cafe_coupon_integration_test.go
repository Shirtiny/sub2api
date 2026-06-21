//go:build integration

package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/cafecoupon"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const cafeCouponIntegrationPostgresImage = "postgres:18.1-alpine3.23"

func TestClaimCafeCouponPostgresConcurrentClaimsCreateOneCoupon(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponPostgresClient(t)
	user, err := client.User.Create().SetEmail("pg-cafe@example.com").SetPasswordHash("hash").SetUsername("pg-cafe").Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
		userRepo: &cafeCouponIntegrationUserRepo{user: &User{
			ID:             user.ID,
			Email:          user.Email,
			Username:       user.Username,
			Status:         payment.EntityStatusActive,
			TotalRecharged: MembershipLevel1Threshold + 1,
		}},
		configService: &PaymentConfigService{settingRepo: cafeCouponIntegrationSettingsRepo{SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":8,"period":"month"}}}`}},
	}

	const workers = 8
	var wg sync.WaitGroup
	results := make(chan *CafeCouponClaimResult, workers)
	errs := make(chan error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := svc.ClaimCafeCoupon(ctx, user.ID)
			if err != nil {
				errs <- err
				return
			}
			results <- result
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	codes := map[string]struct{}{}
	freshClaims := 0
	alreadyClaimed := 0
	for result := range results {
		require.NotNil(t, result)
		codes[result.Code] = struct{}{}
		if result.AlreadyClaimed {
			alreadyClaimed++
		} else {
			freshClaims++
		}
	}
	require.Len(t, codes, 1)
	require.Equal(t, 1, freshClaims)
	require.Equal(t, workers-1, alreadyClaimed)

	count, err := client.CafeCoupon.Query().Where(cafecoupon.UserIDEQ(user.ID)).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func newCafeCouponPostgresClient(t *testing.T) *dbent.Client {
	t.Helper()
	ctx := context.Background()
	if !cafeCouponIntegrationDockerAvailable(ctx) {
		if os.Getenv("CI") != "" {
			t.Fatal("docker is required for integration tests")
		}
		t.Skip("docker is not available")
	}

	container, err := tcpostgres.Run(
		ctx,
		cafeCouponIntegrationPostgresImage,
		tcpostgres.WithDatabase("sub2api_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(ctx)) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
	require.NoError(t, err)
	db, err := cafeCouponIntegrationOpenSQL(ctx, dsn, 30*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(drv))
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.NoError(t, client.Schema.Create(ctx))
	return client
}

func cafeCouponIntegrationDockerAvailable(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "docker", "info")
	cmd.Env = os.Environ()
	return cmd.Run() == nil
}

func cafeCouponIntegrationOpenSQL(ctx context.Context, dsn string, timeout time.Duration) (*sql.DB, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			lastErr = err
			time.Sleep(250 * time.Millisecond)
			continue
		}
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err = db.PingContext(pingCtx)
		cancel()
		if err == nil {
			return db, nil
		}
		lastErr = err
		_ = db.Close()
		time.Sleep(250 * time.Millisecond)
	}
	return nil, fmt.Errorf("db not ready after %s: %w", timeout, lastErr)
}

type cafeCouponIntegrationSettingsRepo map[string]string

func (r cafeCouponIntegrationSettingsRepo) Get(context.Context, string) (*Setting, error) {
	return nil, nil
}
func (r cafeCouponIntegrationSettingsRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}
func (r cafeCouponIntegrationSettingsRepo) Set(_ context.Context, key, value string) error {
	r[key] = value
	return nil
}
func (r cafeCouponIntegrationSettingsRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = r[key]
	}
	return out, nil
}
func (r cafeCouponIntegrationSettingsRepo) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		r[key] = value
	}
	return nil
}
func (r cafeCouponIntegrationSettingsRepo) GetAll(context.Context) (map[string]string, error) {
	return r, nil
}
func (r cafeCouponIntegrationSettingsRepo) Delete(context.Context, string) error { return nil }

type cafeCouponIntegrationUserRepo struct {
	user *User
}

func (r *cafeCouponIntegrationUserRepo) Create(context.Context, *User) error { return nil }
func (r *cafeCouponIntegrationUserRepo) GetByID(context.Context, int64) (*User, error) {
	if r.user == nil {
		return &User{}, nil
	}
	user := *r.user
	return &user, nil
}
func (r *cafeCouponIntegrationUserRepo) GetByIDIncludeDeleted(ctx context.Context, id int64) (*User, error) {
	return r.GetByID(ctx, id)
}
func (r *cafeCouponIntegrationUserRepo) GetByEmail(context.Context, string) (*User, error) {
	return &User{}, nil
}
func (r *cafeCouponIntegrationUserRepo) GetFirstAdmin(context.Context) (*User, error) {
	return &User{}, nil
}
func (r *cafeCouponIntegrationUserRepo) Update(context.Context, *User) error { return nil }
func (r *cafeCouponIntegrationUserRepo) Delete(context.Context, int64) error { return nil }
func (r *cafeCouponIntegrationUserRepo) GetUserAvatar(context.Context, int64) (*UserAvatar, error) {
	return nil, nil
}
func (r *cafeCouponIntegrationUserRepo) UpsertUserAvatar(context.Context, int64, UpsertUserAvatarInput) (*UserAvatar, error) {
	return nil, nil
}
func (r *cafeCouponIntegrationUserRepo) DeleteUserAvatar(context.Context, int64) error { return nil }
func (r *cafeCouponIntegrationUserRepo) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *cafeCouponIntegrationUserRepo) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *cafeCouponIntegrationUserRepo) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	return nil, nil
}
func (r *cafeCouponIntegrationUserRepo) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
}
func (r *cafeCouponIntegrationUserRepo) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	return nil
}
func (r *cafeCouponIntegrationUserRepo) UpdateBalance(context.Context, int64, float64) error {
	return nil
}
func (r *cafeCouponIntegrationUserRepo) DeductBalance(context.Context, int64, float64) error {
	return nil
}
func (r *cafeCouponIntegrationUserRepo) UpdateConcurrency(context.Context, int64, int) error {
	return nil
}
func (r *cafeCouponIntegrationUserRepo) BatchSetConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
}
func (r *cafeCouponIntegrationUserRepo) BatchAddConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
}
func (r *cafeCouponIntegrationUserRepo) ExistsByEmail(context.Context, string) (bool, error) {
	return false, nil
}
func (r *cafeCouponIntegrationUserRepo) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}
func (r *cafeCouponIntegrationUserRepo) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (r *cafeCouponIntegrationUserRepo) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (r *cafeCouponIntegrationUserRepo) ListUserAuthIdentities(context.Context, int64) ([]UserAuthIdentityRecord, error) {
	return nil, nil
}
func (r *cafeCouponIntegrationUserRepo) UnbindUserAuthProvider(context.Context, int64, string) error {
	return nil
}
func (r *cafeCouponIntegrationUserRepo) UpdateTotpSecret(context.Context, int64, *string) error {
	return nil
}
func (r *cafeCouponIntegrationUserRepo) EnableTotp(context.Context, int64) error  { return nil }
func (r *cafeCouponIntegrationUserRepo) DisableTotp(context.Context, int64) error { return nil }
