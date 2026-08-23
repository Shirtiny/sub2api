package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
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
func (requestControlRepoStub) GetViolationState(context.Context, int64, time.Time) (int, *time.Time, error) {
	return 0, nil, nil
}
func (requestControlRepoStub) UpdateLogSideEffects(context.Context, *RequestControlLog) error {
	return nil
}
func (requestControlRepoStub) ListLogs(context.Context, RequestControlLogFilter) ([]RequestControlLog, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}
func (requestControlRepoStub) CleanupLogs(context.Context, time.Time) (int64, error) { return 0, nil }

type requestControlViolationRepoStub struct {
	requestControlRepoStub
	count   int
	last    *time.Time
	created []*RequestControlLog
	updated []*RequestControlLog
}

func (r *requestControlViolationRepoStub) CreateLog(_ context.Context, log *RequestControlLog) error {
	log.ID = int64(len(r.created) + 1)
	clone := *log
	r.created = append(r.created, &clone)
	return nil
}
func (r *requestControlViolationRepoStub) GetViolationState(context.Context, int64, time.Time) (int, *time.Time, error) {
	return r.count, r.last, nil
}
func (r *requestControlViolationRepoStub) UpdateLogSideEffects(_ context.Context, log *RequestControlLog) error {
	clone := *log
	r.updated = append(r.updated, &clone)
	return nil
}

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

func TestRequestControlConfigDefaultsNotificationAndBanSettings(t *testing.T) {
	cfg := defaultRequestControlConfig()
	require.True(t, cfg.EmailOnHit)
	require.True(t, cfg.AutoBanEnabled)
	require.Equal(t, 4, cfg.BanThreshold)
	require.Equal(t, 720, cfg.ViolationWindowHours)
}

func TestRequestControlUpdateConfigPersistsNotificationAndBanSettings(t *testing.T) {
	cfg := RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	settings := &requestControlSettingStub{values: map[string]string{
		SettingKeyRiskControlEnabled:   "true",
		SettingKeyRequestControlConfig: string(raw),
	}}
	svc := NewRequestControlService(settings, requestControlRepoStub{}, nil)
	emailOnHit := false
	autoBan := true
	threshold := 4
	window := 48
	view, err := svc.UpdateConfig(context.Background(), UpdateRequestControlConfigInput{
		EmailOnHit:           &emailOnHit,
		AutoBanEnabled:       &autoBan,
		BanThreshold:         &threshold,
		ViolationWindowHours: &window,
	})
	require.NoError(t, err)
	require.False(t, view.EmailOnHit)
	require.True(t, view.AutoBanEnabled)
	require.Equal(t, threshold, view.BanThreshold)
	require.Equal(t, window, view.ViolationWindowHours)
}

func TestRequestControlViolationCountUsesFiveMinuteGap(t *testing.T) {
	userID := int64(7)
	now := time.Now()
	last := now
	repo := &requestControlViolationRepoStub{count: 1, last: &last}
	svc := &RequestControlService{repo: repo}
	cfg := defaultRequestControlConfig()
	log := &RequestControlLog{UserID: &userID, Blocked: true, CreatedAt: now.Add(4 * time.Minute)}

	svc.prepareViolationCount(context.Background(), cfg, log)
	require.False(t, log.Counted)
	require.Equal(t, 1, log.ViolationCount)

	// This uncounted request is still the previous hit for the next 5-minute
	// gap decision.
	lastRequest := log.CreatedAt
	repo.last = &lastRequest
	log.CreatedAt = now.Add(8 * time.Minute)
	svc.prepareViolationCount(context.Background(), cfg, log)
	require.False(t, log.Counted)
	require.Equal(t, 1, log.ViolationCount)

	lastRequest = log.CreatedAt
	repo.last = &lastRequest
	log.CreatedAt = now.Add(14*time.Minute + time.Second)
	svc.prepareViolationCount(context.Background(), cfg, log)
	require.True(t, log.Counted)
	require.Equal(t, 2, log.ViolationCount)
}

func TestRequestControlAutoBanDisablesUserAndInvalidatesAuthCache(t *testing.T) {
	userID := int64(42)
	userRepo := &contentModerationTestUserRepo{user: &User{ID: userID, Email: "user@example.com", Status: StatusActive}}
	cache := &contentModerationTestAuthCacheInvalidator{}
	svc := &RequestControlService{userRepo: userRepo, authCacheInvalidator: cache}
	cfg := &RequestControlConfig{AutoBanEnabled: true, BanThreshold: 4}
	log := &RequestControlLog{UserID: &userID, ViolationCount: 4, Counted: true}

	require.True(t, svc.applyRequestControlAutoBan(context.Background(), cfg, log))
	require.True(t, log.AutoBanned)
	require.Equal(t, StatusDisabled, userRepo.user.Status)
	require.Equal(t, []int64{userID}, cache.userIDs)
}

func TestRequestControlQueuedLogPersistsCountedStateAfterHit(t *testing.T) {
	userID := int64(7)
	now := time.Now()
	last := now.Add(-6 * time.Minute)
	raw, err := json.Marshal(RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	require.NoError(t, err)
	settings := &requestControlSettingStub{values: map[string]string{
		SettingKeyRiskControlEnabled:   "true",
		SettingKeyRequestControlConfig: string(raw),
	}}
	repo := &requestControlViolationRepoStub{count: 1, last: &last}
	svc := NewRequestControlService(settings, repo, nil)
	log := &RequestControlLog{UserID: &userID, Blocked: true, CreatedAt: now, RequestID: "req-1"}

	require.NoError(t, svc.processQueuedLog(context.Background(), log))
	require.Len(t, repo.created, 1)
	require.True(t, repo.created[0].Counted)
	require.Equal(t, 2, repo.created[0].ViolationCount)
	require.Len(t, repo.updated, 1)
	require.True(t, repo.updated[0].Counted)
}

func TestRequestControlBlocksAnonymousResponsesBeforeUAClassification(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{Protocol: RequestControlProtocolResponse, Model: "gpt-5", UserID: 1, UserAgent: "curl/8", Headers: http.Header{}, Body: []byte(`{"model":"gpt-5"}`)})
	require.NoError(t, err)
	require.False(t, decision.Allowed)
	require.True(t, decision.Observed)
	require.True(t, decision.Blocked)
	require.Equal(t, "anonymous_response_request", decision.Reason)
	require.Equal(t, "missing", decision.Details["client_session"])
}

func TestRequestControlAllowsNonCodexResponsesWithClientSessionForObservation(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:  RequestControlProtocolResponse,
		Model:     "gpt-5",
		UserID:    1,
		UserAgent: "curl/8",
		Headers:   http.Header{},
		Body:      []byte(`{"model":"gpt-5","prompt_cache_key":"client-session-1"}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.True(t, decision.Observed)
	require.False(t, decision.Blocked)
}

func TestRequestControlAnonymousResponsesCannotUseUAWhitelist(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{
		Enabled:                  true,
		AllGroups:                true,
		AllUsers:                 true,
		GlobalUserAgentWhitelist: []string{"curl/"},
	})
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:  RequestControlProtocolResponse,
		Model:     "gpt-5",
		UserID:    1,
		UserAgent: "curl/8",
		Headers:   http.Header{},
		Body:      []byte(`{"model":"gpt-5"}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, "anonymous_response_request", decision.Reason)
}

func TestRequestControlRecognizesAetherStyleResponseSessionSignals(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	cases := []struct {
		name    string
		headers http.Header
		body    string
	}{
		{name: "aether header", headers: http.Header{"X-Aether-Session-Id": {"aether-session"}}, body: `{"model":"gpt-5"}`},
		{name: "standard header", headers: http.Header{"Session_Id": {"standard-session"}}, body: `{"model":"gpt-5"}`},
		{name: "nested metadata", headers: http.Header{}, body: `{"model":"gpt-5","metadata":{"conversation_id":"nested-session"}}`},
		{name: "codex turn metadata header", headers: http.Header{"X-Codex-Turn-Metadata": {`{"session_id":"turn-session"}`}}, body: `{"model":"gpt-5"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision, err := svc.Check(context.Background(), RequestControlCheckInput{
				Protocol:  RequestControlProtocolResponse,
				Model:     "gpt-5",
				UserID:    1,
				UserAgent: "curl/8",
				Headers:   tc.headers,
				Body:      []byte(tc.body),
			})
			require.NoError(t, err)
			require.True(t, decision.Allowed)
			require.True(t, decision.Observed)
		})
	}
}

func TestRequestControlDoesNotTreatClientRequestIDAsSession(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:  RequestControlProtocolResponse,
		Model:     "gpt-5",
		UserID:    1,
		UserAgent: "curl/8",
		Headers:   http.Header{"X-Client-Request-Id": {"request-only-id"}},
		Body:      []byte(`{"model":"gpt-5"}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, "anonymous_response_request", decision.Reason)
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
	status := svc.GetStatus()
	require.True(t, status.Enabled)
	require.True(t, status.RiskControlEnabled)

	svc.UpdateRiskControlEnabled(false)
	status = svc.GetStatus()
	require.True(t, status.Enabled)
	require.False(t, status.RiskControlEnabled)
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

func TestRequestControlAllowsOfficialCodexThreadOriginatorOverride(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	// Codex can retain the process-level User-Agent while a resumed thread
	// supplies a different first-party originator header.
	headers.Set("originator", "codex-tui")
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
	require.True(t, decision.HeaderMatched)

	headers.Set("originator", "third_party_client")
	decision, err = svc.Check(context.Background(), RequestControlCheckInput{
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

func TestRequestControlBlocksClaudeCodeDuplicateIdentityFields(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers := http.Header{}
	headers.Set("User-Agent", "claude-cli/2.1.234 (external, sdk-cli)")
	headers.Set("X-App", "cli")
	headers.Set("anthropic-beta", "claude-code-20250219")
	headers.Set("anthropic-version", "2023-06-01")
	knownValid := true
	for _, body := range [][]byte{
		[]byte(`{"model":"claude-sonnet-4-6","model":"claude-opus-4-6","metadata":{"user_id":"a"}}`),
		[]byte(`{"model":"claude-sonnet-4-6","metadata":{"user_id":"a","user_\u0069d":"b"}}`),
	} {
		decision, err := svc.Check(context.Background(), RequestControlCheckInput{
			Protocol:        RequestControlProtocolMessages,
			Endpoint:        "/v1/messages",
			Model:           "claude-sonnet-4-6",
			UserID:          1,
			UserAgent:       headers.Get("User-Agent"),
			Headers:         headers,
			Body:            body,
			ClaudeCodeValid: &knownValid,
		})
		require.NoError(t, err)
		require.True(t, decision.Blocked)
	}
}

func TestRequestControlBlocksCodexDuplicateBodyFields(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	body = bytes.Replace(body, []byte(`"model":"gpt-5.6-sol"`), []byte(`"model":"gpt-5.6-sol","mo\u0064el":"gpt-5.4"`), 1)
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
	require.False(t, decision.BodyMatched)
}

func TestParseRequestControlResponsesBodyRejectsOversizedSelectedStrings(t *testing.T) {
	_, err := parseRequestControlResponsesBody([]byte(`{"model":"` + strings.Repeat("m", 1024) + `","input":[]}`))
	require.Error(t, err)
}

func TestBuildRequestControlLogBoundsClientControlledFields(t *testing.T) {
	log := buildRequestControlLog(RequestControlCheckInput{
		RequestID: strings.Repeat("r", 140),
		Model:     strings.Repeat("m", 254) + "界\x00tail",
	}, &RequestControlDecision{
		Action:     RequestControlActionBlock,
		Reason:     "test",
		ClientKind: "test",
	})
	require.Len(t, log.RequestID, 128)
	require.LessOrEqual(t, len(log.Model), 255)
	require.True(t, json.Valid([]byte(strconv.Quote(log.Model))))
	require.NotContains(t, log.Model, "\x00")
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

func BenchmarkValidateCodexResponsesRequest1MB(b *testing.B) {
	const sessionID = "01a02a99-981a-7181-992f-267a960a36a1"
	turnMetadata := `{"installation_id":"ed12c212-f894-4ba5-9f47-22a0999590bc","session_id":"` + sessionID + `","thread_id":"` + sessionID + `","turn_id":"01a02a99-9887-73c0-af4a-af91958ec3ee","request_kind":"turn"}`
	headers := http.Header{}
	headers.Set("User-Agent", "codex_exec/0.149.0 (Debian 13.0.0; x86_64)")
	headers.Set("originator", "codex_exec")
	headers.Set("Accept", "text/event-stream")
	headers.Set("Content-Type", "application/json")
	headers.Set("session-id", sessionID)
	headers.Set("thread-id", sessionID)
	headers.Set("x-client-request-id", sessionID)
	headers.Set("x-codex-turn-metadata", turnMetadata)
	body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"` + strings.Repeat("x", 1024*1024) + `"}]}],"tool_choice":"auto","parallel_tool_calls":false,"reasoning":{"effort":"max"},"store":false,"stream":true,"include":["reasoning.encrypted_content"],"prompt_cache_key":"` + sessionID + `","client_metadata":{"session_id":"` + sessionID + `","thread_id":"` + sessionID + `","x-codex-turn-metadata":` + strconv.Quote(turnMetadata) + `}}`)
	input := RequestControlCheckInput{Endpoint: "/v1/responses", Headers: headers, Body: body}
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		headerOK, bodyOK, _ := validateCodexResponsesRequest(input)
		if !headerOK || !bodyOK {
			b.Fatal("valid request rejected")
		}
	}
}
