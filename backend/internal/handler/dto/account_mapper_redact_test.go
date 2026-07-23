package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestAccountFromServiceShallow_RedactsSensitiveCredentials(t *testing.T) {
	src := &service.Account{
		ID:       42,
		Name:     "demo",
		Platform: "anthropic",
		Type:     "oauth",
		Credentials: map[string]any{
			"access_token":  "at-secret",
			"refresh_token": "rt-secret",
			"id_token":      "id-secret",
			"api_key":       "sk-secret",
			"base_url":      "https://api.example.com",
			"model_mapping": map[string]any{"foo": "bar"},
		},
	}

	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)

	// 敏感键不在 Credentials 里
	require.NotContains(t, got.Credentials, "access_token")
	require.NotContains(t, got.Credentials, "refresh_token")
	require.NotContains(t, got.Credentials, "id_token")
	require.NotContains(t, got.Credentials, "api_key")
	// 非敏感键保留
	require.Equal(t, "https://api.example.com", got.Credentials["base_url"])
	require.Equal(t, map[string]any{"foo": "bar"}, got.Credentials["model_mapping"])

	// 状态 map 标记敏感键存在
	require.True(t, got.CredentialsStatus["has_access_token"])
	require.True(t, got.CredentialsStatus["has_refresh_token"])
	require.True(t, got.CredentialsStatus["has_id_token"])
	require.True(t, got.CredentialsStatus["has_api_key"])

	// JSON 序列化校验：响应体里不会出现敏感子串
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "rt-secret")
	require.NotContains(t, string(raw), "at-secret")
	require.NotContains(t, string(raw), "sk-secret")
	require.NotContains(t, string(raw), "id-secret")
	// 状态标识应序列化进 JSON
	require.Contains(t, string(raw), "credentials_status")
	require.Contains(t, string(raw), "has_refresh_token")

	// 原始 service.Account 不应被改动
	require.Equal(t, "rt-secret", src.Credentials["refresh_token"])
}

func TestAccountFromServiceShallow_NilCredentialsOmitsStatus(t *testing.T) {
	src := &service.Account{ID: 1, Name: "n", Platform: "anthropic", Type: "oauth"}
	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)
	require.Nil(t, got.Credentials)
	require.Nil(t, got.CredentialsStatus)
}

func TestAccountFromServiceShallow_PoolModeHidesStaleRuntimeState(t *testing.T) {
	now := time.Now()
	resetAt := now.Add(30 * time.Minute)
	src := &service.Account{
		ID:                      40,
		Name:                    "aether-pool",
		Platform:                service.PlatformOpenAI,
		Type:                    service.AccountTypeAPIKey,
		Credentials:             map[string]any{"pool_mode": true},
		RateLimitedAt:           &now,
		RateLimitResetAt:        &resetAt,
		TempUnschedulableUntil:  &resetAt,
		TempUnschedulableReason: "stale temporary state",
		Extra: map[string]any{
			"openai_passthrough": true,
			"model_rate_limits": map[string]any{
				"gpt-5.4": map[string]any{
					"rate_limit_reset_at": resetAt.Format(time.RFC3339),
				},
			},
		},
	}

	got := AccountFromServiceShallow(src)

	require.Nil(t, got.RateLimitedAt)
	require.Nil(t, got.RateLimitResetAt)
	require.Nil(t, got.TempUnschedulableUntil)
	require.Empty(t, got.TempUnschedulableReason)
	require.NotContains(t, got.Extra, "model_rate_limits")
	require.Equal(t, true, got.Extra["openai_passthrough"])
	require.Contains(t, src.Extra, "model_rate_limits", "mapping must not mutate the service account")
}
