package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type stubOpenAIWSMigrationCache struct {
	loadState OpenAIWSMigrationCacheState
	loadErr   error
	decision  OpenAIWSMigrationCacheDecision
	admitErr  error
}

func (c *stubOpenAIWSMigrationCache) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return 0, nil
}

func (c *stubOpenAIWSMigrationCache) SetSessionAccountID(context.Context, int64, string, int64, time.Duration) error {
	return nil
}

func (c *stubOpenAIWSMigrationCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (c *stubOpenAIWSMigrationCache) DeleteSessionAccountID(context.Context, int64, string) error {
	return nil
}

func (c *stubOpenAIWSMigrationCache) LoadOpenAIWSMigrationState(context.Context, int64, string) (OpenAIWSMigrationCacheState, error) {
	return c.loadState, c.loadErr
}

func (c *stubOpenAIWSMigrationCache) AdmitOpenAIWSReconnectMigration(
	context.Context,
	int64,
	string,
	int64,
	string,
	uint64,
	OpenAIWSMiddleRouteDisposition,
	int64,
	time.Duration,
	time.Duration,
	int,
) (OpenAIWSMigrationCacheDecision, error) {
	return c.decision, c.admitErr
}

func TestOpenAIWSMigrationStateLoadsSharedGenerationAndExclusions(t *testing.T) {
	cfg := openAIWSMigrationTestConfig()
	now := time.Now()
	cache := &stubOpenAIWSMigrationCache{loadState: OpenAIWSMigrationCacheState{
		MigrationCount:         1,
		BindingGeneration:      2,
		ExcludedUntilUnixMilli: map[int64]int64{101: now.Add(time.Minute).UnixMilli()},
	}}
	svc := &OpenAIGatewayService{cfg: cfg, cache: cache}
	groupID := int64(7)

	admission, err := svc.LoadOpenAIWSMigrationAdmission(context.Background(), &groupID, "session-a")
	require.NoError(t, err)
	require.True(t, admission.Active)
	require.Equal(t, 1, admission.MigrationCount)
	require.Equal(t, uint64(2), admission.BindingGeneration)
	require.Contains(t, admission.ExcludedAccountIDs, int64(101))
}

func TestOpenAIWSMigrationStateFailsClosedWithoutSharedCache(t *testing.T) {
	cfg := openAIWSMigrationTestConfig()
	svc := &OpenAIGatewayService{cfg: cfg}

	admission, err := svc.LoadOpenAIWSMigrationAdmission(context.Background(), nil, "session-a")
	require.ErrorContains(t, err, "shared")
	require.True(t, admission.Active)

	cache := &stubOpenAIWSMigrationCache{loadErr: errors.New("redis unavailable")}
	svc.cache = cache
	_, err = svc.LoadOpenAIWSMigrationAdmission(context.Background(), nil, "session-a")
	require.ErrorContains(t, err, "redis unavailable")
}

func TestOpenAIWSReconnectSignalAdmissionPropagatesAtomicDecision(t *testing.T) {
	cfg := openAIWSMigrationTestConfig()
	cache := &stubOpenAIWSMigrationCache{decision: OpenAIWSMigrationCacheDecision{
		State: OpenAIWSMigrationCacheState{
			MigrationCount:         2,
			BindingGeneration:      3,
			ExcludedUntilUnixMilli: map[int64]int64{101: time.Now().Add(time.Minute).UnixMilli()},
		},
		Admitted:   true,
		Idempotent: true,
	}}
	svc := &OpenAIGatewayService{cfg: cfg, cache: cache}

	admission, err := svc.AdmitOpenAIWSReconnectSignal(context.Background(), nil, "session-a", 101, "control-a", 2, OpenAIWSMiddleRouteDispositionExclude)
	require.NoError(t, err)
	require.True(t, admission.Allowed)
	require.True(t, admission.Idempotent)
	require.Equal(t, uint64(3), admission.BindingGeneration)
	require.Contains(t, admission.ExcludedAccountIDs, int64(101))
}

func TestOpenAIWSMigrationStateIsInactiveWhenFeatureIsDisabled(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	admission, err := svc.LoadOpenAIWSMigrationAdmission(context.Background(), nil, "session-a")
	require.NoError(t, err)
	require.False(t, admission.Active)
}

func openAIWSMigrationTestConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.ReconnectMigrationEnabled = true
	cfg.Gateway.OpenAIWS.MaxMigrationsPerSession = 2
	cfg.Gateway.OpenAIWS.MigrationWindowSeconds = 600
	cfg.Gateway.OpenAIWS.RouteMinDwellSeconds = 30
	return cfg
}
