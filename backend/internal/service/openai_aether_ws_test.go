package service

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func effectiveAetherWSTestConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS = config.GatewayOpenAIWSConfig{
		Enabled:                   true,
		ModeRouterV2Enabled:       true,
		ResponsesWebsocketsV2:     true,
		APIKeyEnabled:             true,
		AetherRouteControlEnabled: true,
	}
	return cfg
}

func effectiveAetherWSTestAccount() *Account {
	return &Account{
		ID:          1,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "local-secret", "base_url": "http://127.0.0.1:8080/v1"},
		Extra: map[string]any{
			AetherWSAccountExtraKey: map[string]any{
				"schema_version":            float64(AetherWSAccountSchemaVersion),
				"enabled":                   true,
				"required_control_protocol": AetherWSControlProtocolRouteV1,
			},
			"openai_apikey_responses_websockets_v2_mode":    OpenAIWSIngressModePassthrough,
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
}

func TestResolveAetherWSAccountCapabilityRequiresExplicitEffectiveConfiguration(t *testing.T) {
	account := effectiveAetherWSTestAccount()
	capability := account.ResolveAetherWSAccountCapability(effectiveAetherWSTestConfig())
	require.True(t, capability.Configured)
	require.True(t, capability.Effective)
	require.Equal(t, "aether_ws_effective", capability.Reason)
	require.Equal(t, AetherWSControlProtocolRouteV1, capability.Config.RequiredControlProtocol)

	tests := []struct {
		name   string
		mutate func(*Account, *config.Config)
		reason string
	}{
		{name: "account disabled", mutate: func(a *Account, _ *config.Config) {
			a.Extra[AetherWSAccountExtraKey].(map[string]any)["enabled"] = false
		}, reason: "aether_ws_account_disabled"},
		{name: "oauth account", mutate: func(a *Account, _ *config.Config) { a.Type = AccountTypeOAuth }, reason: "aether_ws_requires_openai_apikey"},
		{name: "zero concurrency", mutate: func(a *Account, _ *config.Config) { a.Concurrency = 0 }, reason: "account_concurrency_invalid"},
		{name: "wrong mode", mutate: func(a *Account, _ *config.Config) { a.Extra["openai_apikey_responses_websockets_v2_mode"] = "ctx_pool" }, reason: "account_ws_field_conflict"},
		{name: "not schedulable", mutate: func(a *Account, _ *config.Config) { a.Schedulable = false }, reason: "account_not_schedulable"},
		{name: "global route disabled", mutate: func(_ *Account, cfg *config.Config) { cfg.Gateway.OpenAIWS.AetherRouteControlEnabled = false }, reason: "aether_route_control_disabled"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := effectiveAetherWSTestAccount()
			cfg := effectiveAetherWSTestConfig()
			test.mutate(candidate, cfg)
			resolved := candidate.ResolveAetherWSAccountCapability(cfg)
			require.False(t, resolved.Effective)
			require.Equal(t, test.reason, resolved.Reason)
		})
	}
}

func TestResolveAetherWSAccountCapabilityRejectsMalformedSchema(t *testing.T) {
	account := effectiveAetherWSTestAccount()
	account.Extra[AetherWSAccountExtraKey] = map[string]any{
		"schema_version":            2,
		"enabled":                   true,
		"required_control_protocol": AetherWSControlProtocolRouteV1,
	}
	resolved := account.ResolveAetherWSAccountCapability(effectiveAetherWSTestConfig())
	require.False(t, resolved.Effective)
	require.Equal(t, "aether_ws_schema_unsupported", resolved.Reason)
}

func TestAetherWSHandshakeNegotiationIsExact(t *testing.T) {
	capability := effectiveAetherWSTestAccount().ResolveAetherWSAccountCapability(effectiveAetherWSTestConfig())
	requestHeaders := make(http.Header)
	applyAetherWSHandshakeRequestHeaders(requestHeaders, capability)
	require.Equal(t, AetherWSControlProtocolRouteV1, requestHeaders.Get(AetherWSControlAcceptHeader))

	valid := make(http.Header)
	valid.Set(AetherWSControlSelectedHeader, AetherWSControlProtocolRouteV1)
	valid.Set(AetherWSCapabilitiesHeader, "close-after-terminal,client-reconnect")
	negotiated, err := validateAetherWSHandshakeResponse(valid, capability)
	require.NoError(t, err)
	require.Equal(t, AetherWSControlProtocolRouteV1, negotiated.ControlProtocol)
	require.True(t, negotiated.CloseAfterTerminal)
	require.True(t, negotiated.ClientReconnect)

	for _, invalid := range []string{
		"client-reconnect,close-after-terminal",
		"close-after-terminal",
		"close-after-terminal,client-reconnect,provider-fallback",
	} {
		headers := valid.Clone()
		headers.Set(AetherWSCapabilitiesHeader, invalid)
		_, err := validateAetherWSHandshakeResponse(headers, capability)
		require.Error(t, err)
	}
}

func TestValidateAetherWSClientPayloadBoundary(t *testing.T) {
	require.NoError(t, validateAetherWSClientPayload(bytes.Repeat([]byte{'x'}, AetherWSMaxClientPayloadBytes)))
	require.Error(t, validateAetherWSClientPayload(bytes.Repeat([]byte{'x'}, AetherWSMaxClientPayloadBytes+1)))
	require.NoError(t, validateAetherWSRoutedPayload(bytes.Repeat([]byte{'x'}, AetherWSMaxRoutedMessageBytes)))
	require.Error(t, validateAetherWSRoutedPayload(bytes.Repeat([]byte{'x'}, AetherWSMaxRoutedMessageBytes+1)))
}

func TestAetherWSBindingLeaseDetectsRoutingChanges(t *testing.T) {
	bound := effectiveAetherWSTestAccount()
	bound.UpdatedAt = time.Unix(100, 0)
	latest := *bound
	require.True(t, aetherWSBindingLeaseUnchanged(bound, &latest, effectiveAetherWSTestConfig()))

	latest.UpdatedAt = bound.UpdatedAt.Add(time.Second)
	require.True(t, aetherWSBindingLeaseUnchanged(bound, &latest, effectiveAetherWSTestConfig()), "unrelated persistence timestamps must not churn bindings")
	ignoredProxyID := int64(99)
	latest.ProxyID = &ignoredProxyID
	require.True(t, aetherWSBindingLeaseUnchanged(bound, &latest, effectiveAetherWSTestConfig()), "the direct local Aether route ignores account proxy changes")
	latest.Credentials = map[string]any{"api_key": "rotated", "base_url": bound.GetOpenAIBaseURL()}
	require.False(t, aetherWSBindingLeaseUnchanged(bound, &latest, effectiveAetherWSTestConfig()))
}

func TestAetherWSBindingLeaseSnapshotFencesGroupAndModelRoute(t *testing.T) {
	groupID := int64(10)
	bound := effectiveAetherWSTestAccount()
	bound.GroupIDs = []int64{groupID}
	bound.Credentials = map[string]any{
		"api_key":  "local-secret",
		"base_url": "http://127.0.0.1:8080/v1",
		"model_mapping": map[string]any{
			"gpt-5": "gpt-5-codex",
		},
	}
	latest := *bound
	latest.GroupIDs = append([]int64(nil), bound.GroupIDs...)
	latest.Credentials = map[string]any{
		"api_key":  "local-secret",
		"base_url": "http://127.0.0.1:8080/v1",
		"model_mapping": map[string]any{
			"gpt-5": "gpt-5-codex",
		},
	}

	require.NoError(t, validateAetherWSBindingLeaseSnapshot(
		context.Background(), bound, &latest, &groupID, "gpt-5", effectiveAetherWSTestConfig(),
	))

	latest.GroupIDs = []int64{11}
	require.ErrorContains(t, validateAetherWSBindingLeaseSnapshot(
		context.Background(), bound, &latest, &groupID, "gpt-5", effectiveAetherWSTestConfig(),
	), "left the request group")

	latest.GroupIDs = []int64{groupID}
	latest.Credentials["model_mapping"] = map[string]any{"gpt-5": "gpt-5.1-codex"}
	require.ErrorContains(t, validateAetherWSBindingLeaseSnapshot(
		context.Background(), bound, &latest, &groupID, "gpt-5", effectiveAetherWSTestConfig(),
	), "model route changed")

	latest.Credentials["model_mapping"] = map[string]any{"gpt-4": "gpt-4-codex"}
	require.ErrorContains(t, validateAetherWSBindingLeaseSnapshot(
		context.Background(), bound, &latest, &groupID, "gpt-5", effectiveAetherWSTestConfig(),
	), "no longer supports")
}

func TestAetherWSBindingLeaseSnapshotRejectsQuotaExhaustion(t *testing.T) {
	groupID := int64(10)
	bound := effectiveAetherWSTestAccount()
	bound.GroupIDs = []int64{groupID}
	bound.Extra["quota_limit"] = 10.0
	bound.Extra["quota_used"] = 9.0

	latest := effectiveAetherWSTestAccount()
	latest.GroupIDs = []int64{groupID}
	latest.Extra["quota_limit"] = 10.0
	latest.Extra["quota_used"] = 10.0

	require.True(t, bound.IsSchedulable())
	require.False(t, latest.IsSchedulable())
	require.ErrorContains(t, validateAetherWSBindingLeaseSnapshot(
		context.Background(), bound, latest, &groupID, "gpt-5", effectiveAetherWSTestConfig(),
	), "binding changed")
}
