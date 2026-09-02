package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	AetherWSAccountExtraKey        = "aether_ws"
	AetherWSAccountSchemaVersion   = 1
	AetherWSMaxClientPayloadBytes  = 16 * 1024 * 1024
	AetherWSMaxRouteFenceOverhead  = 4 * 1024
	AetherWSMaxRoutedMessageBytes  = AetherWSMaxClientPayloadBytes + AetherWSMaxRouteFenceOverhead
	AetherWSControlProtocolRouteV1 = "route-v1"
	AetherWSControlAcceptHeader    = "X-Aether-WS-Control-Accept"
	AetherWSControlSelectedHeader  = "X-Aether-WS-Control"
	AetherWSCapabilitiesHeader     = "X-Aether-WS-Capabilities"
	aetherWSLegacyCapabilities     = "close-after-terminal,client-reconnect"
	aetherWSTurnCancelCapabilities = aetherWSLegacyCapabilities + ",turn-cancel"
	aetherWSTurnCancelPayload      = `{"type":"aether.turn.cancel"}`
)

func validateAetherWSClientPayload(payload []byte) error {
	if len(payload) > AetherWSMaxClientPayloadBytes {
		return fmt.Errorf("aether websocket response.create exceeds %d bytes", AetherWSMaxClientPayloadBytes)
	}
	return nil
}

func validateAetherWSRoutedPayload(payload []byte) error {
	if len(payload) > AetherWSMaxRoutedMessageBytes {
		return fmt.Errorf("aether websocket routed response.create exceeds %d bytes", AetherWSMaxRoutedMessageBytes)
	}
	return nil
}

func (s *OpenAIGatewayService) ValidateAetherWSBindingLease(
	ctx context.Context,
	bound *Account,
	groupID *int64,
	frozenRequestModel string,
) error {
	if s == nil || s.schedulerSnapshot == nil || bound == nil || bound.ID <= 0 {
		return errors.New("aether websocket binding snapshot is unavailable")
	}
	latest, hit, err := s.schedulerSnapshot.GetCachedAccount(ctx, bound.ID)
	if err != nil {
		return fmt.Errorf("read aether websocket binding snapshot: %w", err)
	}
	if !hit || latest == nil {
		return errors.New("aether websocket binding snapshot is missing")
	}
	if err := validateAetherWSBindingLeaseSnapshot(ctx, bound, latest, groupID, frozenRequestModel, s.cfg); err != nil {
		return err
	}
	return nil
}

func validateAetherWSBindingLeaseSnapshot(
	ctx context.Context,
	bound *Account,
	latest *Account,
	groupID *int64,
	frozenRequestModel string,
	cfg *config.Config,
) error {
	if !aetherWSBindingLeaseUnchanged(bound, latest, cfg) {
		return errors.New("aether websocket binding changed")
	}
	if !openAIAccountBelongsToGroup(latest, groupID) {
		return errors.New("aether websocket binding left the request group")
	}
	frozenRequestModel = strings.TrimSpace(frozenRequestModel)
	if frozenRequestModel != "" {
		if !latest.IsSchedulableForModelWithContext(ctx, frozenRequestModel) || !latest.IsModelSupported(frozenRequestModel) {
			return errors.New("aether websocket binding no longer supports the request model")
		}
		boundModel := normalizeOpenAIModelForUpstream(bound, bound.GetMappedModel(frozenRequestModel))
		latestModel := normalizeOpenAIModelForUpstream(latest, latest.GetMappedModel(frozenRequestModel))
		if boundModel != latestModel {
			return errors.New("aether websocket binding model route changed")
		}
	}
	return nil
}

func aetherWSBindingLeaseUnchanged(bound, latest *Account, cfg *config.Config) bool {
	if bound == nil || latest == nil || bound.ID != latest.ID {
		return false
	}
	if !latest.ResolveAetherWSAccountCapability(cfg).Effective {
		return false
	}
	if bound.Status != latest.Status ||
		bound.Schedulable != latest.Schedulable ||
		bound.Concurrency != latest.Concurrency ||
		bound.Platform != latest.Platform ||
		bound.Type != latest.Type ||
		bound.GetOpenAIBaseURL() != latest.GetOpenAIBaseURL() ||
		bound.GetCredential("api_key") != latest.GetCredential("api_key") {
		return false
	}
	return true
}

// AetherWSAccountConfig is the versioned account-level configuration stored in
// accounts.extra.aether_ws. It marks an OpenAI API-key account as a trusted
// Aether middle hop; ordinary custom OpenAI base URLs never receive this trust.
type AetherWSAccountConfig struct {
	SchemaVersion           int
	Enabled                 bool
	RequiredControlProtocol string
}

// AetherWSAccountCapability separates persisted configuration from its current
// effective state. Negotiated is intentionally not represented here because it
// is known only after a successful WebSocket handshake.
type AetherWSAccountCapability struct {
	Configured bool
	Effective  bool
	Reason     string
	Config     AetherWSAccountConfig
}

type AetherWSNegotiatedCapabilities struct {
	ControlProtocol    string
	CloseAfterTerminal bool
	ClientReconnect    bool
	TurnCancel         bool
}

// ResolveAetherWSAccountCapability is the single effective resolver for the
// sub2api -> Aether WebSocket route. Callers must not independently combine the
// legacy mode/enabled fields for an Aether-managed account.
func (a *Account) ResolveAetherWSAccountCapability(cfg *config.Config) AetherWSAccountCapability {
	result := AetherWSAccountCapability{Reason: "aether_ws_not_configured"}
	if a == nil || a.Extra == nil {
		return result
	}
	raw, present := a.Extra[AetherWSAccountExtraKey]
	if !present {
		return result
	}
	result.Configured = true

	object, ok := raw.(map[string]any)
	if !ok {
		result.Reason = "aether_ws_config_invalid"
		return result
	}
	schemaVersion, ok := aetherWSConfigInt(object["schema_version"])
	if !ok || schemaVersion != AetherWSAccountSchemaVersion {
		result.Reason = "aether_ws_schema_unsupported"
		return result
	}
	enabled, ok := object["enabled"].(bool)
	if !ok {
		result.Reason = "aether_ws_enabled_invalid"
		return result
	}
	requiredControlProtocol, ok := object["required_control_protocol"].(string)
	if !ok || strings.TrimSpace(requiredControlProtocol) != AetherWSControlProtocolRouteV1 {
		result.Reason = "aether_ws_control_protocol_unsupported"
		return result
	}
	result.Config = AetherWSAccountConfig{
		SchemaVersion:           schemaVersion,
		Enabled:                 enabled,
		RequiredControlProtocol: AetherWSControlProtocolRouteV1,
	}
	if !enabled {
		result.Reason = "aether_ws_account_disabled"
		return result
	}
	if !a.IsOpenAIApiKey() {
		result.Reason = "aether_ws_requires_openai_apikey"
		return result
	}
	if a.IsOpenAIWSForceHTTPEnabled() {
		result.Reason = "account_force_http"
		return result
	}
	if a.Concurrency <= 0 {
		result.Reason = "account_concurrency_invalid"
		return result
	}

	modeRaw, modeExists := a.Extra["openai_apikey_responses_websockets_v2_mode"]
	mode, modeOK := modeRaw.(string)
	if !modeExists || !modeOK || strings.ToLower(strings.TrimSpace(mode)) != OpenAIWSIngressModePassthrough {
		result.Reason = "account_ws_field_conflict"
		return result
	}
	if compatibilityEnabled, exists := a.Extra["openai_apikey_responses_websockets_v2_enabled"]; exists {
		enabledMirror, valid := compatibilityEnabled.(bool)
		if !valid || !enabledMirror {
			result.Reason = "account_ws_field_conflict"
			return result
		}
	}

	if cfg == nil {
		result.Reason = "aether_ws_global_config_missing"
		return result
	}
	wsCfg := cfg.Gateway.OpenAIWS
	switch {
	case !wsCfg.Enabled:
		result.Reason = "global_disabled"
		return result
	case wsCfg.ForceHTTP:
		result.Reason = "global_force_http"
		return result
	case !wsCfg.ModeRouterV2Enabled:
		result.Reason = "mode_router_v2_disabled"
		return result
	case !wsCfg.ResponsesWebsocketsV2:
		result.Reason = "responses_websockets_v2_disabled"
		return result
	case !wsCfg.APIKeyEnabled:
		result.Reason = "apikey_ws_disabled"
		return result
	case !wsCfg.AetherRouteControlEnabled:
		result.Reason = "aether_route_control_disabled"
		return result
	}

	if !a.IsSchedulable() {
		result.Reason = "account_not_schedulable"
		return result
	}

	result.Effective = true
	result.Reason = "aether_ws_effective"
	return result
}

func (a *Account) IsAetherWSManaged() bool {
	if a == nil || a.Extra == nil {
		return false
	}
	_, ok := a.Extra[AetherWSAccountExtraKey]
	return ok
}

func applyAetherWSHandshakeRequestHeaders(
	headers http.Header,
	capability AetherWSAccountCapability,
) {
	if headers == nil || !capability.Effective {
		return
	}
	headers.Set(AetherWSControlAcceptHeader, capability.Config.RequiredControlProtocol)
}

func validateAetherWSHandshakeResponse(
	headers http.Header,
	capability AetherWSAccountCapability,
) (AetherWSNegotiatedCapabilities, error) {
	negotiated := AetherWSNegotiatedCapabilities{}
	if !capability.Effective {
		return negotiated, nil
	}
	controlValues := headers.Values(AetherWSControlSelectedHeader)
	if len(controlValues) != 1 || strings.TrimSpace(controlValues[0]) != capability.Config.RequiredControlProtocol {
		return negotiated, fmt.Errorf("aether websocket control negotiation failed")
	}
	capabilityValues := headers.Values(AetherWSCapabilitiesHeader)
	if len(capabilityValues) != 1 {
		return negotiated, fmt.Errorf("aether websocket capabilities negotiation failed")
	}
	capabilityList := strings.TrimSpace(capabilityValues[0])
	turnCancel := false
	switch capabilityList {
	case aetherWSLegacyCapabilities:
	case aetherWSTurnCancelCapabilities:
		turnCancel = true
	default:
		return negotiated, fmt.Errorf("aether websocket capabilities are unsupported")
	}
	negotiated = AetherWSNegotiatedCapabilities{
		ControlProtocol:    AetherWSControlProtocolRouteV1,
		CloseAfterTerminal: true,
		ClientReconnect:    true,
		TurnCancel:         turnCancel,
	}
	return negotiated, nil
}

func aetherWSConfigInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		if typed != float64(int(typed)) {
			return 0, false
		}
		return int(typed), true
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		return parsed, err == nil
	default:
		return 0, false
	}
}
