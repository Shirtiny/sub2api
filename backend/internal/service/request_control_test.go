package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type requestControlSettingStub struct {
	values        map[string]string
	getValueCalls int
}

func (s *requestControlSettingStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}
func (s *requestControlSettingStub) GetValue(_ context.Context, key string) (string, error) {
	s.getValueCalls++
	value, ok := s.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}
func (s *requestControlSettingStub) Set(_ context.Context, key, value string) error {
	s.values[key] = value
	return nil
}
func (s *requestControlSettingStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return s.values, nil
}
func (s *requestControlSettingStub) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		s.values[key] = value
	}
	return nil
}
func (s *requestControlSettingStub) GetAll(context.Context) (map[string]string, error) {
	return s.values, nil
}
func (s *requestControlSettingStub) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}

type requestControlRepoStub struct{}

func (requestControlRepoStub) CreateLog(context.Context, *RequestControlLog) error { return nil }
func (requestControlRepoStub) ListLogs(context.Context, RequestControlLogFilter) ([]RequestControlLog, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}
func (requestControlRepoStub) CleanupLogs(context.Context, time.Time) (int64, error) { return 0, nil }

func requestControlTestService(t *testing.T, cfg RequestControlConfig) *RequestControlService {
	t.Helper()
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	settings := &requestControlSettingStub{values: map[string]string{SettingKeyRiskControlEnabled: "true", SettingKeyRequestControlConfig: string(raw)}}
	svc := NewRequestControlService(settings, requestControlRepoStub{}, nil)
	return svc
}

func TestRequestControlCheckChatBlocks(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{Protocol: RequestControlProtocolChat, Model: "gpt-5", UserID: 1, Headers: http.Header{}, Body: []byte(`{"model":"gpt-5"}`)})
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, "openai_chat_completions_blocked", decision.Reason)
}

func TestRequestControlNonCodexResponsesObserved(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{Protocol: RequestControlProtocolResponse, Model: "gpt-5", UserID: 1, UserAgent: "curl/8", Headers: http.Header{}, Body: []byte(`{"model":"gpt-5"}`)})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.True(t, decision.Observed)
	require.False(t, decision.Blocked)
}

func TestRequestControlHotPathUsesRuntimeSnapshot(t *testing.T) {
	cfg := RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	settings := &requestControlSettingStub{values: map[string]string{
		SettingKeyRiskControlEnabled:   "true",
		SettingKeyRequestControlConfig: string(raw),
	}}
	svc := NewRequestControlService(settings, requestControlRepoStub{}, nil)
	readsAfterWarmup := settings.getValueCalls
	for i := 0; i < 100; i++ {
		_, err := svc.Check(context.Background(), RequestControlCheckInput{
			Protocol:  RequestControlProtocolResponse,
			Model:     "gpt-5",
			UserID:    1,
			UserAgent: "curl/8",
			Headers:   http.Header{},
			Body:      []byte(`{"model":"gpt-5"}`),
		})
		require.NoError(t, err)
	}
	require.Equal(t, readsAfterWarmup, settings.getValueCalls)
}

func TestSettingServiceComposesRiskControlCallbacks(t *testing.T) {
	settings := &SettingService{}
	calls := make([]string, 0, 2)
	settings.AddRiskControlUpdateCallback(func(enabled bool) {
		require.True(t, enabled)
		calls = append(calls, "content")
	})
	settings.AddRiskControlUpdateCallback(func(enabled bool) {
		require.True(t, enabled)
		calls = append(calls, "request")
	})
	callback := settings.riskControlUpdate.Load()
	require.NotNil(t, callback)
	(*callback)(true)
	require.Equal(t, []string{"content", "request"}, calls)
}

func TestRequestControlGlobalSwitchUpdatesSnapshotImmediately(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	input := RequestControlCheckInput{Protocol: RequestControlProtocolChat, Model: "gpt-5", UserID: 1, Headers: http.Header{}}

	svc.UpdateRiskControlEnabled(false)
	decision, err := svc.Check(context.Background(), input)
	require.NoError(t, err)
	require.True(t, decision.Allowed)

	svc.UpdateRiskControlEnabled(true)
	decision, err = svc.Check(context.Background(), input)
	require.NoError(t, err)
	require.True(t, decision.Blocked)
}

func TestRequestControlScopesUseCompiledGroupModelAndUserRules(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{
		Enabled:     true,
		AllGroups:   false,
		GroupIDs:    []int64{5},
		ModelFilter: ContentModerationModelFilter{Type: ContentModerationModelFilterInclude, Models: []string{"gpt-5.6-sol"}},
		AllUsers:    false,
		UserRules:   []RequestControlUserRule{{UserID: 7, Participate: true}},
	})
	group5 := int64(5)
	group6 := int64(6)
	cases := []struct {
		name      string
		input     RequestControlCheckInput
		wantBlock bool
	}{
		{name: "all scopes match", input: RequestControlCheckInput{Protocol: RequestControlProtocolChat, Model: "GPT-5.6-SOL", UserID: 7, GroupID: &group5}, wantBlock: true},
		{name: "group excluded", input: RequestControlCheckInput{Protocol: RequestControlProtocolChat, Model: "gpt-5.6-sol", UserID: 7, GroupID: &group6}},
		{name: "source group matches", input: RequestControlCheckInput{Protocol: RequestControlProtocolChat, Model: "gpt-5.6-sol", UserID: 7, GroupID: &group6, EffectiveGroupIDs: []int64{5}}, wantBlock: true},
		{name: "model excluded", input: RequestControlCheckInput{Protocol: RequestControlProtocolChat, Model: "gpt-5.4", UserID: 7, GroupID: &group5}},
		{name: "user excluded", input: RequestControlCheckInput{Protocol: RequestControlProtocolChat, Model: "gpt-5.6-sol", UserID: 8, GroupID: &group5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision, err := svc.Check(context.Background(), tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.wantBlock, decision.Blocked)
		})
	}
}

func TestRequestControlUserUAWhitelistMergesGlobal(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{
		Enabled:                  true,
		AllGroups:                true,
		AllUsers:                 true,
		GlobalUserAgentWhitelist: []string{"trusted-global"},
		UserRules: []RequestControlUserRule{{
			UserID:             7,
			Participate:        true,
			UserAgentWhitelist: []string{"trusted-user"},
		}},
	})
	for _, ua := range []string{"trusted-global/1", "trusted-user/1"} {
		decision, err := svc.Check(context.Background(), RequestControlCheckInput{Protocol: RequestControlProtocolChat, Model: "gpt-5", UserID: 7, UserAgent: ua, Headers: http.Header{}})
		require.NoError(t, err)
		require.True(t, decision.Allowed, ua)
		require.Equal(t, RequestControlActionUABypass, decision.Action)
	}
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{Protocol: RequestControlProtocolChat, Model: "gpt-5", UserID: 7, UserAgent: "curl/8 trusted-user/1", Headers: http.Header{}})
	require.NoError(t, err)
	require.True(t, decision.Blocked, "UA whitelist entries must match from the beginning")
}

func TestRequestControlAcceptsCapturedCodexRequestShape(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:   RequestControlProtocolResponse,
		Endpoint:   "/v1/responses",
		Model:      "gpt-5.6-sol",
		UserID:     1,
		UserAgent:  headers.Get("User-Agent"),
		Originator: headers.Get("originator"),
		Headers:    headers,
		Body:       body,
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.False(t, decision.Blocked)
	require.True(t, decision.HeaderMatched)
	require.True(t, decision.BodyMatched)
}

func TestRequestControlBlocksCodexIdentityMismatch(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	headers.Set("thread-id", "b6e9fe15-c823-4f23-b6b9-738d5858626a")
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:   RequestControlProtocolResponse,
		Endpoint:   "/v1/responses",
		Model:      "gpt-5.6-sol",
		UserID:     1,
		UserAgent:  headers.Get("User-Agent"),
		Originator: headers.Get("originator"),
		Headers:    headers,
		Body:       body,
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.False(t, decision.HeaderMatched)
	require.False(t, decision.BodyMatched)
}

func TestRequestControlAllowsCodexBodyOnlyTurnMetadataExtensions(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	clientMetadata, ok := payload["client_metadata"].(map[string]any)
	require.True(t, ok)
	turnMetadata, ok := clientMetadata["x-codex-turn-metadata"].(string)
	require.True(t, ok)
	var expanded map[string]any
	require.NoError(t, json.Unmarshal([]byte(turnMetadata), &expanded))
	expanded["tool_namespaces_info"] = map[string]any{"tools": []string{"exec_command"}}
	expandedRaw, err := json.Marshal(expanded)
	require.NoError(t, err)
	clientMetadata["x-codex-turn-metadata"] = string(expandedRaw)
	body, err = json.Marshal(payload)
	require.NoError(t, err)
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:   RequestControlProtocolResponse,
		Endpoint:   "/v1/responses",
		Model:      "gpt-5.6-sol",
		UserID:     1,
		UserAgent:  headers.Get("User-Agent"),
		Originator: headers.Get("originator"),
		Headers:    headers,
		Body:       body,
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
}

func TestRequestControlAllowsOfficialCodexCompactShape(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, _ := capturedCodexRequestShape(t)
	headers.Del("Accept")
	headers.Del("x-client-request-id")
	body, err := json.Marshal(map[string]any{
		"model":               "gpt-5.6-sol",
		"input":               []any{},
		"parallel_tool_calls": false,
		"reasoning":           map[string]any{"effort": "max"},
		"prompt_cache_key":    "01a02a99-981a-7181-992f-267a960a36a1",
	})
	require.NoError(t, err)
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:   RequestControlProtocolResponse,
		Endpoint:   "/v1/responses/compact",
		Model:      "gpt-5.6-sol",
		UserID:     1,
		UserAgent:  headers.Get("User-Agent"),
		Originator: headers.Get("originator"),
		Headers:    headers,
		Body:       body,
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
}

func TestRequestControlAllowsCodexCompactOptionalFieldsToBeAbsent(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, _ := capturedCodexRequestShape(t)
	headers.Del("Accept")
	headers.Del("x-client-request-id")
	body, err := json.Marshal(map[string]any{
		"model":               "gpt-5.6-sol",
		"input":               []any{},
		"parallel_tool_calls": false,
	})
	require.NoError(t, err)
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:   RequestControlProtocolResponse,
		Endpoint:   "/v1/responses/compact",
		Model:      "gpt-5.6-sol",
		UserID:     1,
		UserAgent:  headers.Get("User-Agent"),
		Originator: headers.Get("originator"),
		Headers:    headers,
		Body:       body,
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
}

func TestRequestControlAllowsCodexPrewarmWithoutTurnID(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	prewarmMetadata := `{"installation_id":"ed12c212-f894-4ba5-9f47-22a0999590bc","session_id":"01a02a99-981a-7181-992f-267a960a36a1","thread_id":"01a02a99-981a-7181-992f-267a960a36a1","request_kind":"prewarm"}`
	headers.Set("x-codex-turn-metadata", prewarmMetadata)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	clientMetadata, ok := payload["client_metadata"].(map[string]any)
	require.True(t, ok)
	clientMetadata["x-codex-turn-metadata"] = prewarmMetadata
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:   RequestControlProtocolResponse,
		Endpoint:   "/v1/responses",
		Model:      "gpt-5.6-sol",
		UserID:     1,
		UserAgent:  headers.Get("User-Agent"),
		Originator: headers.Get("originator"),
		Headers:    headers,
		Body:       body,
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
}

func TestRequestControlAllowsOfficialCodexWebSocketShape(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	headers.Del("Accept")
	headers.Del("Content-Type")
	headers.Set("OpenAI-Beta", "responses_websockets=2026-02-06")
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	payload["type"] = "response.create"
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:   RequestControlProtocolResponse,
		Endpoint:   "/v1/responses",
		Model:      "gpt-5.6-sol",
		UserID:     1,
		UserAgent:  headers.Get("User-Agent"),
		Originator: headers.Get("originator"),
		Headers:    headers,
		Body:       body,
		WebSocket:  true,
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
}

func TestRequestControlAllowsOfficialCodexMemoryMetadataShape(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	memoryMetadata := `{"request_kind":"memory"}`
	headers.Set("x-codex-turn-metadata", memoryMetadata)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	clientMetadata, ok := payload["client_metadata"].(map[string]any)
	require.True(t, ok)
	clientMetadata["x-codex-turn-metadata"] = memoryMetadata
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:   RequestControlProtocolResponse,
		Endpoint:   "/v1/responses",
		Model:      "gpt-5.6-sol",
		UserID:     1,
		UserAgent:  headers.Get("User-Agent"),
		Originator: headers.Get("originator"),
		Headers:    headers,
		Body:       body,
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
}

func TestRequestControlAcceptsCapturedClaudeCodeShape(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers := http.Header{}
	headers.Set("User-Agent", "claude-cli/2.1.234 (external, sdk-cli)")
	headers.Set("X-App", "cli")
	headers.Set("anthropic-beta", "claude-code-20250219,interleaved-thinking-2025-05-14")
	headers.Set("anthropic-version", "2023-06-01")
	body, err := json.Marshal(map[string]any{
		"model":    "claude-sonnet-4-6",
		"system":   []any{map[string]any{"type": "text", "text": "You are a Claude agent, built on Anthropic's Claude Agent SDK."}},
		"metadata": map[string]any{"user_id": `{"device_id":"16ab94ed13fb8dbae2f6b6dd41c0bf09d4a3d2127c27931a419fc3fc824ac87d","account_uuid":"","session_id":"be8e6743-43fa-4683-b81b-f59a9632206c"}`},
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
	})
	require.NoError(t, err)
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:  RequestControlProtocolMessages,
		Endpoint:  "/v1/messages",
		Model:     "claude-sonnet-4-6",
		UserID:    1,
		UserAgent: headers.Get("User-Agent"),
		Headers:   headers,
		Body:      body,
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
}

func TestRequestControlBlocksClaudeCodeWithoutClaudeBeta(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers := http.Header{}
	headers.Set("User-Agent", "claude-cli/2.1.234 (external, sdk-cli)")
	headers.Set("X-App", "cli")
	headers.Set("anthropic-beta", "interleaved-thinking-2025-05-14")
	headers.Set("anthropic-version", "2023-06-01")
	knownValid := true
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:        RequestControlProtocolMessages,
		Endpoint:        "/v1/messages",
		Model:           "claude-sonnet-4-6",
		UserID:          1,
		UserAgent:       headers.Get("User-Agent"),
		Headers:         headers,
		ClaudeCodeValid: &knownValid,
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.False(t, decision.HeaderMatched)
}

func capturedCodexRequestShape(t *testing.T) (http.Header, []byte) {
	t.Helper()
	const (
		sessionID      = "01a02a99-981a-7181-992f-267a960a36a1"
		turnID         = "01a02a99-9887-73c0-af4a-af91958ec3ee"
		installationID = "ed12c212-f894-4ba5-9f47-22a0999590bc"
	)
	turnMetadata, err := json.Marshal(map[string]any{
		"installation_id": installationID,
		"session_id":      sessionID,
		"thread_id":       sessionID,
		"turn_id":         turnID,
		"request_kind":    "turn",
	})
	require.NoError(t, err)
	headers := http.Header{}
	headers.Set("User-Agent", "codex_exec/0.149.0 (Debian 13.0.0; x86_64) xterm-256color")
	headers.Set("originator", "codex_exec")
	headers.Set("Accept", "text/event-stream")
	headers.Set("Content-Type", "application/json")
	headers.Set("session-id", sessionID)
	headers.Set("thread-id", sessionID)
	headers.Set("x-client-request-id", sessionID)
	headers.Set("x-codex-turn-metadata", string(turnMetadata))
	body, err := json.Marshal(map[string]any{
		"model":               "gpt-5.6-sol",
		"input":               []any{},
		"tool_choice":         "auto",
		"parallel_tool_calls": false,
		"reasoning":           map[string]any{"effort": "max", "summary": "detailed"},
		"store":               false,
		"stream":              true,
		"include":             []string{"reasoning.encrypted_content"},
		"prompt_cache_key":    sessionID,
		"client_metadata": map[string]string{
			"session_id":            sessionID,
			"thread_id":             sessionID,
			"x-codex-turn-metadata": string(turnMetadata),
		},
	})
	require.NoError(t, err)
	return headers, body
}
