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
func (requestControlRepoStub) GetLog(context.Context, int64) (*RequestControlLogDetail, error) {
	return nil, ErrRequestControlLogNotFound
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
	if !cfg.BlockOpenAIChat && !cfg.BlockClaudeMessages && !cfg.BlockOpenAIResponses {
		cfg.BlockOpenAIChat = true
		cfg.BlockClaudeMessages = true
		cfg.BlockOpenAIResponses = true
	}
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

func TestRequestControlProtocolSwitchObservesExpectedBlock(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{
		Enabled:              true,
		BlockOpenAIChat:      false,
		BlockClaudeMessages:  true,
		BlockOpenAIResponses: true,
		AllGroups:            true,
		AllUsers:             true,
	})
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolChat,
		Model:    "gpt-5",
		UserID:   1,
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.False(t, decision.Blocked)
	require.True(t, decision.Observed)
	require.Equal(t, RequestControlActionObserve, decision.Action)
	require.Equal(t, "openai_chat_completions_blocked", decision.ExpectedReason)
	require.True(t, decision.ExpectedBlocked)
	require.Equal(t, http.StatusForbidden, decision.ExpectedStatusCode)
}

func TestRequestControlProtocolSwitchesApplyIndependently(t *testing.T) {
	claudeSvc := requestControlTestService(t, RequestControlConfig{
		Enabled: true, BlockOpenAIChat: true, BlockClaudeMessages: false, BlockOpenAIResponses: true,
		AllGroups: true, AllUsers: true,
	})
	claudeDecision, err := claudeSvc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolMessages, Model: "claude-sonnet-4-6", UserID: 1,
		Headers: http.Header{}, Body: []byte(`{"model":"claude-sonnet-4-6","messages":[]}`),
	})
	require.NoError(t, err)
	require.True(t, claudeDecision.Allowed)
	require.Equal(t, "claude_code_signature_mismatch", claudeDecision.ExpectedReason)

	responseSvc := requestControlTestService(t, RequestControlConfig{
		Enabled: true, BlockOpenAIChat: true, BlockClaudeMessages: true, BlockOpenAIResponses: false,
		AllGroups: true, AllUsers: true,
	})
	headers, body := capturedCodexRequestShape(t)
	headers.Del("Accept")
	responseDecision, err := responseSvc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolResponse, Endpoint: "/v1/responses", Model: "gpt-5.6-sol", UserID: 1,
		UserAgent: headers.Get("User-Agent"), Originator: headers.Get("originator"), Headers: headers, Body: body,
	})
	require.NoError(t, err)
	require.True(t, responseDecision.Allowed)
	require.Equal(t, "codex_request_signature_mismatch", responseDecision.ExpectedReason)
}

func TestRequestControlLegacyConfigDefaultsProtocolSwitchesToBlock(t *testing.T) {
	var cfg RequestControlConfig
	require.NoError(t, json.Unmarshal([]byte(`{"enabled":true,"all_groups":true,"all_users":true}`), &cfg))
	require.True(t, cfg.BlockOpenAIChat)
	require.True(t, cfg.BlockClaudeMessages)
	require.True(t, cfg.BlockOpenAIResponses)
	require.False(t, cfg.RequestSnapshotEnabled)
}

func TestRequestControlLegacyConfigPreservesExistingDefaults(t *testing.T) {
	cfg := defaultRequestControlConfig()
	require.NoError(t, json.Unmarshal([]byte(`{"enabled":true}`), cfg))
	cfg.normalize()
	require.True(t, cfg.AllGroups)
	require.True(t, cfg.AllUsers)
	require.True(t, cfg.EmailOnHit)
	require.True(t, cfg.AutoBanEnabled)
	require.Equal(t, http.StatusForbidden, cfg.BlockStatus)
	require.Equal(t, requestControlDefaultBlockMessage, cfg.BlockMessage)
	require.True(t, cfg.BlockOpenAIChat)
	require.True(t, cfg.BlockClaudeMessages)
	require.True(t, cfg.BlockOpenAIResponses)
}

func TestRequestControlConfigPreservesExplicitProtocolSwitchOff(t *testing.T) {
	var cfg RequestControlConfig
	require.NoError(t, json.Unmarshal([]byte(`{"block_openai_chat":false,"block_claude_messages":true,"block_openai_responses":false}`), &cfg))
	require.False(t, cfg.BlockOpenAIChat)
	require.True(t, cfg.BlockClaudeMessages)
	require.False(t, cfg.BlockOpenAIResponses)
}

func TestRequestControlDedupHashesBoundMetadata(t *testing.T) {
	base := RequestControlCheckInput{
		Protocol: RequestControlProtocolChat,
		Headers: http.Header{
			"Content-Type": {"application/json"},
			"User-Agent":   {"stable-client/1.0.0"},
			"originator":   {"stable-client"},
		},
		MetadataHeaders: http.Header{
			"Authorization":     {"Bearer first"},
			"X-Forwarded-For":   {"1.2.3.4"},
			"Sec-WebSocket-Key": {"first-random-key"},
		},
		Body: []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"hidden"}]}`),
	}
	changedNoise := base
	changedNoise.MetadataHeaders = base.MetadataHeaders.Clone()
	changedNoise.MetadataHeaders.Set("Authorization", "Bearer second")
	changedNoise.MetadataHeaders.Set("X-Forwarded-For", "9.9.9.9")
	changedNoise.MetadataHeaders.Set("Sec-WebSocket-Key", "second-random-key")
	firstHeader, firstBody := requestControlDedupHashes(base)
	secondHeader, secondBody := requestControlDedupHashes(changedNoise)
	require.Equal(t, firstHeader, secondHeader)
	require.Equal(t, firstBody, secondBody)

	changedClient := base
	changedClient.Headers = base.Headers.Clone()
	changedClient.Headers.Set("User-Agent", "other-client/2.0.0")
	changedClientHash, _ := requestControlDedupHashes(changedClient)
	require.Equal(t, firstHeader, changedClientHash)

	knownClient := base
	knownClient.Headers = base.Headers.Clone()
	knownClient.Headers.Set("User-Agent", "curl/8.0.0")
	knownClientHash, _ := requestControlDedupHashes(knownClient)
	require.NotEqual(t, firstHeader, knownClientHash)

	changedSessionHeader := base
	changedSessionHeader.Headers = base.Headers.Clone()
	changedSessionHeader.Headers.Set("session-id", "session-b")
	firstSessionHeader := base
	firstSessionHeader.Headers = base.Headers.Clone()
	firstSessionHeader.Headers.Set("session-id", "session-a")
	firstSessionHash, _ := requestControlDedupHashes(firstSessionHeader)
	secondSessionHash, _ := requestControlDedupHashes(changedSessionHeader)
	require.Equal(t, firstSessionHash, secondSessionHash)

	changedBody := base
	changedBody.Body = []byte(`{"model":"gpt-5","messages":[{"role":"assistant","content":"hidden"}]}`)
	_, thirdBody := requestControlDedupHashes(changedBody)
	require.Equal(t, firstBody, thirdBody)
	changedContentSameLength := base
	changedContentSameLength.Body = []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"secret"}]}`)
	_, sameShapeBody := requestControlDedupHashes(changedContentSameLength)
	require.Equal(t, firstBody, sameShapeBody)

	changedPromptLength := base
	changedPromptLength.Body = []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"a much longer but still private prompt"}]}`)
	_, promptLengthHash := requestControlDedupHashes(changedPromptLength)
	require.Equal(t, firstBody, promptLengthHash)

	changedModel := base
	changedModel.Body = []byte(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"hidden"}]}`)
	_, modelHash := requestControlDedupHashes(changedModel)
	require.Equal(t, firstBody, modelHash)

	sessionA := base
	sessionA.Body = []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"hidden"}],"client_metadata":{"session_id":"session-a"}}`)
	sessionB := sessionA
	sessionB.Body = []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"hidden"}],"client_metadata":{"session_id":"session-b"}}`)
	_, sessionHashA := requestControlDedupHashes(sessionA)
	_, sessionHashB := requestControlDedupHashes(sessionB)
	require.Equal(t, sessionHashA, sessionHashB)

	changedFormat := base
	changedFormat.Body = []byte(`{"model":"gpt-5","messages":{"role":"user"}}`)
	_, changedFormatHash := requestControlDedupHashes(changedFormat)
	require.NotEqual(t, firstBody, changedFormatHash)

	changedStaticControl := base
	changedStaticControl.Body = []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"hidden"}],"stream":false}`)
	_, changedStaticControlHash := requestControlDedupHashes(changedStaticControl)
	require.NotEqual(t, firstBody, changedStaticControlHash)

	reorderedBody := base
	reorderedBody.Body = []byte(`{"messages":[{"content":"hidden","role":"user"}],"model":"gpt-5"}`)
	_, reorderedHash := requestControlDedupHashes(reorderedBody)
	require.Equal(t, firstBody, reorderedHash)

	fields := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		fields = append(fields, `"field`+strconv.Itoa(i)+`":1`)
	}
	largeA := base
	largeA.Body = []byte(`{` + strings.Join(fields, ",") + `}`)
	reversed := append([]string(nil), fields...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	largeB := base
	largeB.Body = []byte(`{` + strings.Join(reversed, ",") + `}`)
	_, largeHashA := requestControlDedupHashes(largeA)
	_, largeHashB := requestControlDedupHashes(largeB)
	require.Equal(t, largeHashA, largeHashB)
}

func TestRequestControlDedupClientProfileIgnoresVersionAndIdentityValues(t *testing.T) {
	first := RequestControlCheckInput{
		Protocol: RequestControlProtocolResponse,
		Endpoint: "/v1/responses",
		Headers: http.Header{
			"User-Agent":              {"Codex Desktop/0.148.0-alpha.15 (Windows 10.0.26200; x86_64)"},
			"Originator":              {"Codex Desktop"},
			"Content-Type":            {"application/json"},
			"Accept":                  {"text/event-stream"},
			"Session-Id":              {"01a02a99-981a-992f-267a960a36a1"},
			"Thread-Id":               {"01a02a99-981a-7181-992f-267a960a36a1"},
			"X-Client-Request-Id":     {"01a02a99-981a-7181-992f-267a960a36a1"},
			"X-Codex-Installation-Id": {"ed12c212-f894-4ba5-9f47-22a0999590bc"},
			"X-Codex-Window-Id":       {"01a02a99-981a-7181-992f-267a960a36a1:0"},
			"X-Codex-Turn-Metadata":   {`{"installation_id":"ed12c212-f894-4ba5-9f47-22a0999590bc","session_id":"01a02a99-981a-7181-992f-267a960a36a1","thread_id":"01a02a99-981a-7181-992f-267a960a36a1","request_kind":"turn"}`},
		},
	}
	second := first
	second.Headers = first.Headers.Clone()
	second.Headers.Set("User-Agent", "Codex Desktop/0.149.0-alpha.4.1 (Windows 11.0.26100; x86_64)")
	second.Headers.Set("originator", "codex_work_desktop")
	second.Headers.Set("session-id", "01a02f27-90c8-7550-8a61-a3d3fcf9a8a1")
	second.Headers.Set("thread-id", "01a02f27-90c8-7550-8a61-a3d3fcf9a8a1")
	second.Headers.Set("x-client-request-id", "01a02f27-90c8-7550-8a61-a3d3fcf9a8a1")
	second.Headers.Set("x-codex-installation-id", "fdbe860e-be97-4bb8-9e56-33a91c5e099e")
	second.Headers.Set("x-codex-window-id", "01a02f27-90c8-7550-8a61-a3d3fcf9a8a1:3")
	second.Headers.Set("x-codex-turn-metadata", `{"installation_id":"fdbe860e-be97-4bb8-9e56-33a91c5e099e","session_id":"01a02f27-90c8-7550-8a61-a3d3fcf9a8a1","thread_id":"01a02f27-90c8-7550-8a61-a3d3fcf9a8a1","request_kind":"turn"}`)
	firstHeader, _ := requestControlDedupHashes(first)
	secondHeader, _ := requestControlDedupHashes(second)
	require.Equal(t, firstHeader, secondHeader)

	cli := first
	cli.Headers = first.Headers.Clone()
	cli.Headers.Set("User-Agent", "codex-tui/0.146.0 (Mac OS 26.5.0; arm64)")
	cli.Headers.Set("originator", "codex-tui")
	cliHeader, _ := requestControlDedupHashes(cli)
	require.NotEqual(t, firstHeader, cliHeader)
}

func TestRequestControlDedupBucketsUnknownClientHeaderValues(t *testing.T) {
	first := RequestControlCheckInput{
		Protocol: RequestControlProtocolResponse,
		Endpoint: "/v1/responses",
		Headers: http.Header{
			"User-Agent":        {"random-family-a/1"},
			"Originator":        {"random-origin-a"},
			"Content-Type":      {"application/random-a"},
			"X-App":             {"random-app-a"},
			"anthropic-version": {"random-version-a"},
		},
		Body: []byte(`{"model":"gpt-5","input":[]}`),
	}
	second := first
	second.Headers = http.Header{
		"User-Agent":        {"random-family-b/2"},
		"Originator":        {"random-origin-b"},
		"Content-Type":      {"application/random-b"},
		"X-App":             {"random-app-b"},
		"anthropic-version": {"random-version-b"},
	}
	firstHeader, _ := requestControlDedupHashes(first)
	secondHeader, _ := requestControlDedupHashes(second)
	require.Equal(t, firstHeader, secondHeader)
}

func TestRequestControlDedupBodyUsesFormatNotRequestContent(t *testing.T) {
	base := RequestControlCheckInput{Protocol: RequestControlProtocolResponse, Endpoint: "/v1/responses"}
	first := map[string]any{
		"protocol": "openai_responses", "parse": "object",
		"input": map[string]any{"kind": "array"}, "reasoning": map[string]any{"kind": "object"},
		"stream": true, "store": false, "parallel_tool_calls": true,
	}
	second := map[string]any{
		"protocol": "openai_responses", "parse": "object",
		"input": map[string]any{"kind": "array"}, "reasoning": map[string]any{"kind": "object"},
		"stream": false, "store": true, "parallel_tool_calls": false,
	}
	require.Equal(t, requestControlDedupBodyHash(base, first), requestControlDedupBodyHash(base, second))

	differentShape := map[string]any{
		"protocol": "openai_responses", "parse": "object",
		"input": map[string]any{"kind": "string"}, "reasoning": map[string]any{"kind": "object"},
	}
	require.NotEqual(t, requestControlDedupBodyHash(base, first), requestControlDedupBodyHash(base, differentShape))
}

func TestRequestControlUsesConfiguredBlockStatusAndMessage(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{
		Enabled:      true,
		AllGroups:    true,
		AllUsers:     true,
		BlockStatus:  http.StatusTeapot,
		BlockMessage: "custom request-control prompt",
	})
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolChat,
		Model:    "gpt-5",
		UserID:   1,
		Headers:  http.Header{},
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, http.StatusTeapot, decision.StatusCode)
	require.Equal(t, "custom request-control prompt", decision.Message)
}

func TestRequestControlConfigDefaultsNotificationAndBanSettings(t *testing.T) {
	cfg := defaultRequestControlConfig()
	require.False(t, cfg.RequestSnapshotEnabled)
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
	snapshotEnabled := true
	view, err := svc.UpdateConfig(context.Background(), UpdateRequestControlConfigInput{
		RequestSnapshotEnabled: &snapshotEnabled,
		EmailOnHit:             &emailOnHit,
		AutoBanEnabled:         &autoBan,
		BanThreshold:           &threshold,
		ViolationWindowHours:   &window,
	})
	require.NoError(t, err)
	require.True(t, view.RequestSnapshotEnabled)
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

func TestRequestControlEmailVariablesCarryConfiguredBlockMessage(t *testing.T) {
	userID := int64(7)
	message := "custom request-control prompt"
	vars := contentModerationEmailVariables(&ContentModerationLog{
		UserID:       &userID,
		BlockMessage: message,
	}, &ContentModerationConfig{BanThreshold: 4})
	require.Equal(t, message, vars["block_message"])
}

func TestRequestControlBuiltInEmailIncludesConfiguredBlockMessage(t *testing.T) {
	message := "<custom request-control prompt>"
	body := buildContentModerationViolationEmailBody("Sub2API", &ContentModerationLog{
		UserEmail:    "user@example.com",
		BlockMessage: message,
	}, &ContentModerationConfig{BanThreshold: 4})
	require.Contains(t, body, "&lt;custom request-control prompt&gt;")
	require.NotContains(t, body, message)
}

func TestRequestControlMetadataRedactsHeadersAndSummarizesBody(t *testing.T) {
	headers := http.Header{
		"Authorization":           {"Bearer secret-token"},
		"Content-Type":            {"application/json"},
		"X-Codex-Installation-Id": {"ed12c212-f894-4ba5-9f47-22a0999590bc"},
	}
	body := []byte(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"private prompt"}],"tools":[{"type":"function","function":{"name":"secret_tool","parameters":{"password":"do-not-store"}}}],"metadata":{"user_id":"session-1"}}`)
	requestHeaders, requestBody := buildRequestControlMetadata(RequestControlCheckInput{
		Protocol: RequestControlProtocolMessages,
		Headers:  headers,
		Body:     body,
	})
	require.Equal(t, "[redacted]", requestHeaders["authorization"])
	require.Equal(t, "application/json", requestHeaders["content-type"])
	bodyJSON := string(mustMarshalJSON(t, requestBody))
	require.NotContains(t, bodyJSON, "private prompt")
	require.NotContains(t, bodyJSON, "do-not-store")
	require.Equal(t, "gpt-5.6-sol", requestBody["model"])
	require.Equal(t, map[string]any{"kind": "object", "keys": []string{"user_id"}}, requestBody["metadata"])
	messagesSummary, ok := requestBody["messages"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []string{"user"}, messagesSummary["roles"])
	require.LessOrEqual(t, len(mustMarshalJSON(t, requestBody)), requestControlMetadataMaxJSONBytes)
}

func TestRequestControlHeaderMetadataSummarizesLongCodexTurnMetadata(t *testing.T) {
	raw := `{"installation_id":"ed12c212-f894-4ba5-9f47-22a0999590bc","session_id":"01a02a99-981a-7181-992f-267a960a36a1","thread_id":"01a02a99-981a-7181-992f-267a960a36a1","turn_id":"01a02a99-9887-73c0-af4a-af91958ec3ee","request_kind":"turn","tool_namespaces_info":{"tools":["` + strings.Repeat("x", 1024) + `"]}}`
	metadata := requestControlHeaderMetadata(http.Header{"X-Codex-Turn-Metadata": {raw}})
	require.Contains(t, metadata["x-codex-turn-metadata"], `"request_kind":"turn"`)
	require.NotContains(t, metadata["x-codex-turn-metadata"], strings.Repeat("x", 128))
	require.LessOrEqual(t, len(metadata["x-codex-turn-metadata"]), requestControlMetadataMaxHeaderRunes)
}

func TestRequestControlMetadataParsesLongTurnMetadataBeforeDisplayTruncation(t *testing.T) {
	turnMetadata := `{"installation_id":"ed12c212-f894-4ba5-9f47-22a0999590bc","session_id":"01a02a99-981a-7181-992f-267a960a36a1","thread_id":"01a02a99-981a-7181-992f-267a960a36a1","turn_id":"01a02a99-9887-73c0-af4a-af91958ec3ee","request_kind":"turn","tool_namespaces_info":{"tools":["` + strings.Repeat("x", 160) + `"]}}`
	body, err := json.Marshal(map[string]any{"client_metadata": map[string]any{"x-codex-turn-metadata": turnMetadata}})
	require.NoError(t, err)
	_, metadata := buildRequestControlMetadata(RequestControlCheckInput{Protocol: RequestControlProtocolResponse, Body: body})
	clientMetadata, ok := metadata["client_metadata"].(map[string]any)
	require.True(t, ok)
	identity, ok := clientMetadata["identity"].(map[string]any)
	require.True(t, ok)
	turnSummary, ok := identity["x-codex-turn-metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "turn", turnSummary["request_kind"])
}

func TestRequestControlMetadataCarriesResponseSessionAndCompactionEvidence(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[{"role":"user","content":[{"type":"input_text","text":"hidden"}]}],"stream":true,"store":false,"tool_choice":"none","max_output_tokens":819,"reasoning":{"effort":"low"}}`)
	_, metadata := buildRequestControlMetadata(RequestControlCheckInput{
		Protocol:  RequestControlProtocolResponse,
		Endpoint:  "/v1/responses",
		UserAgent: "pi (linux x64)",
		Headers:   http.Header{},
		Body:      body,
	})
	require.Equal(t, false, metadata["client_session_present"])
	require.Equal(t, "none", metadata["client_session_source"])
	require.Equal(t, "local_compaction_candidate", metadata["response_request_kind"])
	require.Equal(t, "strong_heuristic", metadata["response_request_kind_confidence"])
	evidence, ok := metadata["response_request_kind_evidence"].([]string)
	require.True(t, ok)
	require.Contains(t, evidence, "tool_choice:none")
}

func TestBuildRequestControlLogCarriesMetadataAndDiagnosticSnapshot(t *testing.T) {
	log := buildRequestControlLog(RequestControlCheckInput{
		Protocol: RequestControlProtocolChat,
		Headers:  http.Header{"Content-Type": {"application/json"}},
		MetadataHeaders: http.Header{
			"Authorization":  {"Bearer secret"},
			"X-Client-Trace": {"trace-1"},
		},
		Body: []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"hidden"}]}`),
	}, &RequestControlDecision{Action: RequestControlActionBlock, Reason: "test"}, true)
	require.Equal(t, "[redacted]", log.RequestHeaders["authorization"])
	require.Equal(t, "trace-1", log.RequestHeaders["x-client-trace"])
	require.NotContains(t, string(mustMarshalJSON(t, log.RequestBodyMetadata)), "hidden")
	require.Contains(t, log.RequestSnapshot.Body, "hidden")
	require.Equal(t, []string{"[redacted]"}, log.RequestSnapshot.Headers["authorization"])
	other := buildRequestControlLog(RequestControlCheckInput{
		Protocol:        RequestControlProtocolChat,
		MetadataHeaders: http.Header{"Content-Type": {"application/json"}},
		Body:            []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"a different length"}]}`),
	}, &RequestControlDecision{Action: RequestControlActionBlock, Reason: "test"}, true)
	require.Equal(t, log.RequestBodyHash, other.RequestBodyHash)

	differentOutcome := buildRequestControlLog(RequestControlCheckInput{
		Protocol: RequestControlProtocolChat,
		Body:     []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"hidden"}]}`),
	}, &RequestControlDecision{Action: RequestControlActionBlock, Reason: "different_policy_result"}, true)
	require.Equal(t, log.RequestBodyHash, differentOutcome.RequestBodyHash)
}

func TestBuildRequestControlLogForcesDiagnosticSnapshotForBlockedRequest(t *testing.T) {
	log := buildRequestControlLog(RequestControlCheckInput{
		Protocol: RequestControlProtocolChat,
		Body:     []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"private prompt"}]}`),
	}, &RequestControlDecision{Action: RequestControlActionBlock, Reason: "test", Blocked: true}, false)
	require.True(t, log.RequestSnapshot.Available)
	require.Contains(t, log.RequestSnapshot.Body, "private prompt")
	require.NotContains(t, log.Details, "request_snapshot")
}

func TestBuildRequestControlLogSkipsObservedSnapshotWhenDisabled(t *testing.T) {
	log := buildRequestControlLog(RequestControlCheckInput{
		Protocol: RequestControlProtocolChat,
		Body:     []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"private prompt"}]}`),
	}, &RequestControlDecision{Action: RequestControlActionObserve, Reason: "test", Observed: true}, false)
	require.False(t, log.RequestSnapshot.Available)
	require.Empty(t, log.RequestSnapshot.Body)
	require.Equal(t, "disabled_for_observed_request", log.Details["request_snapshot"])
}

func TestRequestControlDedupSeparatesLocalCompactionCandidateFromStandardResponses(t *testing.T) {
	base := RequestControlCheckInput{
		Protocol:  RequestControlProtocolResponse,
		Endpoint:  "/v1/responses",
		UserAgent: "pi (linux x64)",
		Headers:   http.Header{},
	}
	standard := base
	standard.Body = []byte(`{"model":"gpt-5.6-sol","input":[{"role":"user"}],"stream":true,"store":false,"tool_choice":"auto","max_output_tokens":819,"tools":[]}`)
	compaction := base
	compaction.Body = []byte(`{"model":"gpt-5.6-sol","input":[{"role":"user"}],"stream":true,"store":false,"tool_choice":"none","max_output_tokens":819}`)
	_, standardHash := requestControlDedupHashes(standard)
	_, compactionHash := requestControlDedupHashes(compaction)
	require.NotEqual(t, standardHash, compactionHash)
}

func TestRequestControlSessionSourceIsStableAcrossJSONFieldOrder(t *testing.T) {
	first := RequestControlCheckInput{
		Protocol: RequestControlProtocolResponse,
		Body:     []byte(`{"model":"gpt-5","session_id":"session-1","prompt_cache_key":"cache-1"}`),
	}
	second := first
	second.Body = []byte(`{"prompt_cache_key":"cache-1","session_id":"session-1","model":"gpt-5"}`)
	firstInspection := inspectRequestControlResponseSessionDetails(first)
	secondInspection := inspectRequestControlResponseSessionDetails(second)
	require.Equal(t, "body:prompt_cache_key", firstInspection.SessionSource)
	require.Equal(t, firstInspection.SessionSource, secondInspection.SessionSource)
}

func TestRequestControlLogDetailJSONIncludesMetadataWithoutTransientFields(t *testing.T) {
	detail := RequestControlLogDetail{
		RequestControlLog:   RequestControlLog{ID: 7, RequestHeaders: map[string]string{"internal": "ignored"}, RequestBodyMetadata: map[string]any{"internal": "ignored"}, RequestSnapshot: RequestControlRequestSnapshot{Body: "internal ignored"}},
		RequestHeaders:      map[string]string{"content-type": "application/json"},
		RequestBodyMetadata: map[string]any{"model": "gpt-5"},
		RequestSnapshot:     RequestControlRequestSnapshot{Available: true, Body: `{"model":"gpt-5"}`},
	}
	raw := string(mustMarshalJSON(t, detail))
	require.Contains(t, raw, "request_headers")
	require.Contains(t, raw, "request_body_metadata")
	require.Contains(t, raw, "request_snapshot")
	require.Contains(t, raw, `\"model\":\"gpt-5\"`)
	require.NotContains(t, raw, "internal")
}

func mustMarshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
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

func TestRequestControlQueueOmitsSnapshotAtByteBudgetButKeepsTask(t *testing.T) {
	svc := &RequestControlService{repo: requestControlRepoStub{}, queue: make(chan requestControlTask, 2)}
	log := &RequestControlLog{Protocol: RequestControlProtocolResponse, RequestSnapshot: RequestControlRequestSnapshot{Available: true, Body: strings.Repeat("x", 1024)}}
	snapshotBytes := requestControlSnapshotApproxBytes(log)
	svc.queueBytes.Store(requestControlSnapshotQueueBytes - snapshotBytes + 1)

	svc.enqueueLog(log)
	require.Equal(t, int64(0), svc.dropped.Load())
	require.Len(t, svc.queue, 1)
	require.False(t, log.RequestSnapshot.Available)
	require.Equal(t, "omitted_queue_memory_budget", log.Details["request_snapshot"])
	require.Equal(t, requestControlSnapshotQueueBytes-snapshotBytes+1, svc.queueBytes.Load())
	task := <-svc.queue
	require.Zero(t, task.bytes)

	svc.queueBytes.Store(0)
	log.RequestSnapshot = RequestControlRequestSnapshot{Available: true, Body: strings.Repeat("x", 1024)}
	svc.enqueueLog(log)
	require.Equal(t, 1, len(svc.queue))
	task = <-svc.queue
	require.Equal(t, snapshotBytes, task.bytes)
	require.Equal(t, snapshotBytes, svc.queueBytes.Load())
	status := svc.GetStatus()
	require.Equal(t, snapshotBytes, status.QueueBytes)
	require.Equal(t, int64(requestControlSnapshotQueueBytes), status.QueueMaxBytes)
}

func TestRequestControlSnapshotOmissionStillProcessesBlockedViolation(t *testing.T) {
	userID := int64(7)
	repo := &requestControlViolationRepoStub{}
	svc := &RequestControlService{repo: repo, queue: make(chan requestControlTask, 1)}
	log := &RequestControlLog{
		UserID: &userID, Blocked: true, Protocol: RequestControlProtocolResponse, EventAt: time.Now(),
		RequestSnapshot: RequestControlRequestSnapshot{Available: true, Body: strings.Repeat("x", 1024)},
	}
	snapshotBytes := requestControlSnapshotApproxBytes(log)
	svc.queueBytes.Store(requestControlSnapshotQueueBytes - snapshotBytes + 1)
	svc.enqueueLog(log)
	task := <-svc.queue
	require.Zero(t, task.bytes)
	require.NoError(t, svc.processQueuedLog(context.Background(), task.log))
	require.Len(t, repo.created, 1)
	require.True(t, repo.created[0].Counted)
	require.Equal(t, 1, repo.created[0].ViolationCount)
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
	require.Equal(t, "none", decision.Details["session_source"])
	require.Equal(t, "openai_responses_standard_or_unknown", decision.Details["request_kind"])
}

func TestRequestControlAllowsPiLocalCompactionWithoutClientSession(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:  RequestControlProtocolResponse,
		Endpoint:  "/v1/responses",
		Model:     "gpt-5.6-sol",
		UserID:    1,
		UserAgent: "pi (linux x64)",
		Headers:   http.Header{},
		Body:      []byte(`{"model":"gpt-5.6-sol","input":[{"role":"user","content":[{"type":"input_text","text":"summarize"}]}],"stream":true,"store":false,"tool_choice":"none","max_output_tokens":819,"reasoning":{"effort":"low"}}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.True(t, decision.Observed)
	require.False(t, decision.Blocked)
	require.Equal(t, "non_codex_user_agent", decision.Reason)
	require.Equal(t, "local_compaction_candidate", decision.Details["request_kind"])
	require.Equal(t, "strong_heuristic", decision.Details["request_kind_confidence"])
	require.Contains(t, decision.Details["request_kind_evidence"], "tool_choice:none")
	require.Equal(t, "synthetic", decision.Details["client_session"])
	require.Equal(t, "gateway:compaction_derived", decision.Details["session_source"])
}

func TestRequestControlAllowsExplicitCompactEndpoint(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolResponse, Endpoint: "/v1/responses/compact", Model: "gpt-5.6-sol", UserID: 1,
		UserAgent: "codex-tui/0.1.0", Headers: http.Header{}, Body: []byte(`{"model":"gpt-5.6-sol","input":[],"parallel_tool_calls":false}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.True(t, decision.Observed)
	require.False(t, decision.Blocked)
	require.Equal(t, requestControlReasonCompactionAllowed, decision.Reason)
	require.Equal(t, "openai_responses_compact_endpoint", decision.Details["request_kind"])
	require.Equal(t, "explicit", decision.Details["request_kind_confidence"])
}

func TestRequestControlAllowsLargeOpenAIJSCompactionShapeWithoutToolChoice(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true, BlockOpenAIResponses: true})
	history := strings.Repeat("conversation history ", 2200)
	body, err := json.Marshal(map[string]any{
		"model":             "gpt-5.6-sol",
		"input":             []any{map[string]any{"role": "developer", "content": "summarize"}, map[string]any{"role": "user", "content": history}},
		"stream":            true,
		"store":             false,
		"max_output_tokens": 13107,
		"reasoning":         map[string]any{"effort": "medium", "summary": "auto"},
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(body), requestControlLocalCompactionMinBodyBytes)

	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolResponse, Endpoint: "/v1/responses", Model: "gpt-5.6-sol", UserID: 1,
		UserAgent: "OpenAI/JS 6.40.0", Headers: http.Header{}, Body: body,
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.True(t, decision.Observed)
	require.False(t, decision.Blocked)
	require.Equal(t, "non_codex_user_agent", decision.Reason)
	require.Equal(t, "local_compaction_candidate", decision.Details["request_kind"])
	require.Contains(t, decision.Details["request_kind_evidence"], "body_bytes:large")
	require.Equal(t, "gateway:compaction_derived", decision.Details["session_source"])
}

func TestRequestControlHeuristicCompactionDoesNotBypassCodexSignaturePolicy(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true, BlockOpenAIResponses: true})
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolResponse, Endpoint: "/v1/responses", Model: "gpt-5.6-sol", UserID: 1,
		UserAgent: "codex-tui/0.146.0", Headers: http.Header{},
		Body: []byte(`{"model":"gpt-5.6-sol","input":[],"stream":true,"store":false,"tool_choice":"none","max_output_tokens":1024}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.False(t, decision.Allowed)
	require.Equal(t, "codex_request_signature_mismatch", decision.Reason)
	require.Equal(t, "local_compaction_candidate", decision.Details["request_kind"])
	require.Equal(t, "gateway:compaction_derived", decision.Details["session_source"])
}

func TestRequestControlStillBlocksSmallAnonymousResponsesWithoutCompactionSignal(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolResponse, Endpoint: "/v1/responses", Model: "gpt-5.6-sol", UserID: 1,
		UserAgent: "OpenAI/JS 6.40.0", Headers: http.Header{},
		Body: []byte(`{"model":"gpt-5.6-sol","input":[{"role":"user","content":"hello"}],"stream":true,"store":false,"max_output_tokens":1024}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, "anonymous_response_request", decision.Reason)
}

func TestRequestControlAnonymousBlockForcesSnapshotWhenCaptureSwitchIsOff(t *testing.T) {
	cfg := RequestControlConfig{
		Enabled:                true,
		RequestSnapshotEnabled: false,
		AllGroups:              true,
		AllUsers:               true,
	}
	cfg.normalize()
	svc := &RequestControlService{
		settingRepo: &requestControlSettingStub{},
		repo:        requestControlRepoStub{},
		queue:       make(chan requestControlTask, 1),
	}
	svc.runtime.Store(newRequestControlRuntimeConfig(true, &cfg))
	body := []byte(`{"model":"gpt-5.6-terra","input":[{"role":"user","content":"diagnose me"}],"stream":true}`)

	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolResponse,
		Endpoint: "/v1/responses",
		Model:    "gpt-5.6-terra",
		UserID:   1,
		Headers:  http.Header{},
		Body:     body,
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked)

	task := <-svc.queue
	require.NotNil(t, task.log)
	require.True(t, task.log.RequestSnapshot.Available)
	require.Equal(t, string(body), task.log.RequestSnapshot.Body)
	require.NotContains(t, task.log.Details, "request_snapshot")
}

func TestRequestControlAllowsCompactionTriggerWithoutClientSession(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true, BlockOpenAIResponses: true})
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolResponse, Endpoint: "/v1/responses", Model: "gpt-5.6-sol", UserID: 1,
		UserAgent: "any-agent/1.0", Headers: http.Header{},
		Body: []byte(`{"model":"gpt-5.6-sol","input":[{"type":"compaction_trigger"}]}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.True(t, decision.Observed)
	require.False(t, decision.Blocked)
	require.Equal(t, requestControlReasonCompactionAllowed, decision.Reason)
	require.Equal(t, "openai_responses_compaction_trigger", decision.Details["request_kind"])
}

func TestRequestControlAllowsExplicitCompactionDespiteUAMismatchAndProtocolBlock(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{
		Enabled: true, AllGroups: true, AllUsers: true, BlockOpenAIResponses: true,
	})
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolResponse, Endpoint: "/v1/responses/compact", Model: "gpt-5.6-sol", UserID: 1,
		UserAgent: "unknown-agent/1.0", Headers: http.Header{"Session_Id": {"session-1"}},
		Body: []byte(`{"model":"gpt-5.6-sol","input":[]}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.True(t, decision.Observed)
	require.False(t, decision.Blocked)
	require.Equal(t, requestControlReasonCompactionAllowed, decision.Reason)
	require.Equal(t, RequestControlActionObserve, decision.ExpectedAction)
	require.False(t, decision.ExpectedBlocked)
}

func TestRequestControlSessionDiagnosticIncludesSource(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolResponse, Endpoint: "/v1/responses", Model: "gpt-5", UserID: 1,
		UserAgent: "curl/8", Headers: http.Header{"Session_Id": {"session-1"}}, Body: []byte(`{"model":"gpt-5"}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.Equal(t, "present", decision.Details["client_session"])
	require.Equal(t, "header:session_id", decision.Details["session_source"])
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

func TestRequestControlExplicitUserExclusionOverridesAllUsers(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{
		Enabled:   true,
		AllGroups: true,
		AllUsers:  true,
		UserRules: []RequestControlUserRule{{UserID: 374, Participate: false}},
	})
	excluded, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolChat,
		Model:    "gpt-5.6-sol",
		UserID:   374,
	})
	require.NoError(t, err)
	require.True(t, excluded.Allowed)
	require.False(t, excluded.Blocked)

	included, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolChat,
		Model:    "gpt-5.6-sol",
		UserID:   375,
	})
	require.NoError(t, err)
	require.True(t, included.Blocked)
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

func TestRequestControlAcceptsCodexDesktopBodyCanonicalShape(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexDesktopRequestShape(t)
	headersBefore := headers.Clone()
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
	require.Equal(t, headersBefore, headers)
}

func TestRequestControlDesktopValidationDoesNotMutateCompatibilityHeaders(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexDesktopRequestShape(t)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	clientMetadata, ok := payload["client_metadata"].(map[string]any)
	require.True(t, ok)
	turnMetadata, ok := clientMetadata["x-codex-turn-metadata"].(string)
	require.True(t, ok)
	headers.Set("x-codex-turn-metadata", turnMetadata)
	headersBefore := headers.Clone()
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolResponse, Endpoint: "/v1/responses", Model: "gpt-5.6-sol", UserID: 1,
		UserAgent: headers.Get("User-Agent"), Originator: headers.Get("originator"), Headers: headers, Body: body,
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.Equal(t, headersBefore, headers)
}

func TestRequestControlAcceptsCodexDesktopDistinctSessionAndThreadIDs(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexDesktopRequestShape(t)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	clientMetadata, ok := payload["client_metadata"].(map[string]any)
	require.True(t, ok)
	clientMetadata["thread_id"] = "01a02a99-981a-7181-992f-267a960a36b2"
	turnMetadata, ok := clientMetadata["x-codex-turn-metadata"].(string)
	require.True(t, ok)
	var turn map[string]any
	require.NoError(t, json.Unmarshal([]byte(turnMetadata), &turn))
	turn["thread_id"] = clientMetadata["thread_id"]
	turnMetadataRaw, err := json.Marshal(turn)
	require.NoError(t, err)
	clientMetadata["x-codex-turn-metadata"] = string(turnMetadataRaw)
	body, err = json.Marshal(payload)
	require.NoError(t, err)
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolResponse, Endpoint: "/v1/responses", Model: "gpt-5.6-sol", UserID: 1,
		UserAgent: headers.Get("User-Agent"), Originator: headers.Get("originator"), Headers: headers, Body: body,
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
}

func TestRequestControlBlocksCodexDesktopInvalidTurnMetadata(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexDesktopRequestShape(t)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	clientMetadata, ok := payload["client_metadata"].(map[string]any)
	require.True(t, ok)
	clientMetadata["x-codex-turn-metadata"] = "not-json"
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol: RequestControlProtocolResponse, Endpoint: "/v1/responses", Model: "gpt-5.6-sol", UserID: 1,
		UserAgent: headers.Get("User-Agent"), Originator: headers.Get("originator"), Headers: headers, Body: body,
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, "missing_or_invalid", decision.Details["body_turn_metadata"])
}

func TestParseRequestControlTurnMetadataValueRequiresJSONString(t *testing.T) {
	value, err := parseRequestControlTurnMetadataValue([]byte(`{"installation_id":"ed12c212-e894-4ba5-9f47-22a0999590bc"}`))
	require.NoError(t, err)
	require.True(t, value.Present)
	require.False(t, value.Valid)
}

func TestRequestControlBlocksCodexDesktopBodyIdentityMismatch(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexDesktopRequestShape(t)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	clientMetadata, ok := payload["client_metadata"].(map[string]any)
	require.True(t, ok)
	clientMetadata["x-codex-window-id"] = "01a02a99-981a-7181-992f-267a960a36b2:1"
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
	require.True(t, decision.Blocked)
	require.False(t, decision.BodyMatched)
	require.Equal(t, "missing_or_mismatched", decision.Details["window_identity"])
}

func TestRequestControlKeepsCodexCLIHeaderProfileStrict(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	headers.Del("Accept")
	headers.Del("session-id")
	headers.Del("thread-id")
	headers.Del("x-client-request-id")
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

func TestRequestControlBlocksCodexInstallationIDMismatch(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	headers.Set("x-codex-installation-id", "8f5d7a5d-8b26-4f1a-9e3c-2c8e76c930a1")
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
}

func TestRequestControlAllowsOfficialCodexNormalRequestWithoutInstallationHeader(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	headers.Del("x-codex-installation-id")
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
	require.True(t, decision.BodyMatched)
}

func TestRequestControlBlocksCodexDuplicateInstallationHeaders(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	headers.Add("x-codex-installation-id", headers.Get("x-codex-installation-id"))
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
}

func TestRequestControlBlocksCodexBodyInstallationIDMismatch(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	clientMetadata, ok := payload["client_metadata"].(map[string]any)
	require.True(t, ok)
	clientMetadata["x-codex-installation-id"] = "8f5d7a5d-8b26-4f1a-9e3c-2c8e76c930a1"
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
	require.True(t, decision.Blocked)
	require.False(t, decision.BodyMatched)
}

func TestRequestControlBlocksCodexInstallationIDAliasDuplicate(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	clientMetadata, ok := payload["client_metadata"].(map[string]any)
	require.True(t, ok)
	clientMetadata["installation_id"] = clientMetadata["x-codex-installation-id"]
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
	require.True(t, decision.Blocked)
	require.False(t, decision.BodyMatched)
}

func TestRequestControlTLSMismatchIsObservedWithoutBlockingValidCodexRequest(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:       RequestControlProtocolResponse,
		Endpoint:       "/v1/responses",
		Model:          "gpt-5.6-sol",
		UserID:         1,
		UserAgent:      headers.Get("User-Agent"),
		Originator:     headers.Get("originator"),
		Headers:        headers,
		Body:           body,
		TLSFingerprint: "proxy:x-aether-tls-ja3-hash=not-the-codex-hash",
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.False(t, decision.Blocked)
	require.True(t, decision.Observed)
	require.NotNil(t, decision.TLSMatched)
	require.False(t, *decision.TLSMatched)
	require.Equal(t, "codex_tls_fingerprint_mismatch", decision.Reason)
}

func TestRequestControlTLSMatchAllowsAndDoesNotObserveOfficialCodexRequest(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, body := capturedCodexRequestShape(t)
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:       RequestControlProtocolResponse,
		Endpoint:       "/v1/responses",
		Model:          "gpt-5.6-sol",
		UserID:         1,
		UserAgent:      headers.Get("User-Agent"),
		Originator:     headers.Get("originator"),
		Headers:        headers,
		Body:           body,
		TLSFingerprint: "proxy:x-aether-tls-ja3-hash=23211f2b48104c7030b93680a2efcfd0",
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.False(t, decision.Blocked)
	require.False(t, decision.Observed)
	require.NotNil(t, decision.TLSMatched)
	require.True(t, *decision.TLSMatched)
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

func TestRequestControlAllowsCodexCompactWithoutInstallationID(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers, _ := capturedCodexRequestShape(t)
	headers.Del("Accept")
	headers.Del("x-client-request-id")
	headers.Del("x-codex-installation-id")
	body := []byte(`{"model":"gpt-5.6-sol","input":[],"parallel_tool_calls":false}`)
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
	require.True(t, decision.Observed)
	require.False(t, decision.Blocked)
	require.Equal(t, requestControlReasonCompactionAllowed, decision.Reason)
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

func TestRequestControlClaudeTLSMismatchIsObservedWithoutBlockingValidClaudeCode(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers := http.Header{}
	headers.Set("User-Agent", "claude-cli/2.1.234 (external, sdk-cli)")
	headers.Set("X-App", "cli")
	headers.Set("anthropic-beta", "claude-code-20250219,interleaved-thinking-2025-05-14")
	headers.Set("anthropic-version", "2023-06-01")
	body := []byte(`{"model":"claude-sonnet-4-6","metadata":{"user_id":"session-1"},"messages":[{"role":"user","content":"hello"}]}`)
	knownValid := true
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:        RequestControlProtocolMessages,
		Endpoint:        "/v1/messages",
		Model:           "claude-sonnet-4-6",
		UserID:          1,
		UserAgent:       headers.Get("User-Agent"),
		Headers:         headers,
		Body:            body,
		ClaudeCodeValid: &knownValid,
		TLSFingerprint:  "proxy:x-aether-tls-ja3-hash=not-the-claude-hash",
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.False(t, decision.Blocked)
	require.True(t, decision.Observed)
	require.NotNil(t, decision.TLSMatched)
	require.False(t, *decision.TLSMatched)
	require.Equal(t, "claude_code_tls_fingerprint_mismatch", decision.Reason)
}

func TestRequestControlClaudeTLSMatchDoesNotObserveValidClaudeCode(t *testing.T) {
	svc := requestControlTestService(t, RequestControlConfig{Enabled: true, AllGroups: true, AllUsers: true})
	headers := http.Header{}
	headers.Set("User-Agent", "claude-cli/2.1.234 (external, sdk-cli)")
	headers.Set("X-App", "cli")
	headers.Set("anthropic-beta", "claude-code-20250219,interleaved-thinking-2025-05-14")
	headers.Set("anthropic-version", "2023-06-01")
	body := []byte(`{"model":"claude-sonnet-4-6","metadata":{"user_id":"session-1"},"messages":[{"role":"user","content":"hello"}]}`)
	knownValid := true
	decision, err := svc.Check(context.Background(), RequestControlCheckInput{
		Protocol:        RequestControlProtocolMessages,
		Endpoint:        "/v1/messages",
		Model:           "claude-sonnet-4-6",
		UserID:          1,
		UserAgent:       headers.Get("User-Agent"),
		Headers:         headers,
		Body:            body,
		ClaudeCodeValid: &knownValid,
		TLSFingerprint:  "proxy:x-aether-tls-ja4=t13d1714h1_5b57614c22b0_7baf387fc6ff",
	})
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.False(t, decision.Blocked)
	require.False(t, decision.Observed)
	require.NotNil(t, decision.TLSMatched)
	require.True(t, *decision.TLSMatched)
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
	}, false)
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
		windowID       = "01a02a99-981a-7181-992f-267a960a36a1:0"
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
	headers.Set("x-codex-installation-id", installationID)
	headers.Set("x-codex-turn-metadata", string(turnMetadata))
	headers.Set("x-codex-window-id", windowID)
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
			"x-codex-installation-id": installationID,
			"x-codex-window-id":       windowID,
			"session_id":              sessionID,
			"thread_id":               sessionID,
			"x-codex-turn-metadata":   string(turnMetadata),
		},
	})
	require.NoError(t, err)
	return headers, body
}

func capturedCodexDesktopRequestShape(t *testing.T) (http.Header, []byte) {
	t.Helper()
	const (
		sessionID      = "01a02a99-981a-7181-992f-267a960a36a1"
		turnID         = "01a02a99-9887-73c0-af4a-af91958ec3ee"
		installationID = "ed12c212-f894-4ba5-9f47-22a0999590bc"
		windowID       = "01a02a99-981a-7181-992f-267a960a36a1:0"
	)
	headers := http.Header{}
	headers.Set("User-Agent", "Codex Desktop/0.148.0-alpha.15 (Windows 10.0.26200; x86_64) unknown (Codex Desktop; 26.814.41407)")
	headers.Set("originator", "Codex Desktop")
	headers.Set("Content-Type", "application/json")
	headers.Set("x-codex-window-id", windowID)
	// Desktop uses body client_metadata as the canonical identity. This
	// compatibility header is deliberately an unrecognized shape.
	// Desktop uses body client_metadata as the canonical identity and may omit
	// the CLI compatibility headers entirely.
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
		"client_metadata": map[string]any{
			"x-codex-installation-id": installationID,
			"x-codex-window-id":       windowID,
			"session_id":              sessionID,
			"thread_id":               sessionID,
			"turn_id":                 turnID,
			"x-codex-turn-metadata":   `{"installation_id":"ed12c212-f894-4ba5-9f47-22a0999590bc","session_id":"01a02a99-981a-7181-992f-267a960a36a1","thread_id":"01a02a99-981a-7181-992f-267a960a36a1","turn_id":"01a02a99-9887-73c0-af4a-af91958ec3ee","request_kind":"turn"}`,
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
	headers.Set("x-codex-installation-id", "ed12c212-f894-4ba5-9f47-22a0999590bc")
	headers.Set("x-codex-turn-metadata", turnMetadata)
	body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"` + strings.Repeat("x", 1024*1024) + `"}]}],"tool_choice":"auto","parallel_tool_calls":false,"reasoning":{"effort":"max"},"store":false,"stream":true,"include":["reasoning.encrypted_content"],"prompt_cache_key":"` + sessionID + `","client_metadata":{"session_id":"` + sessionID + `","thread_id":"` + sessionID + `","x-codex-turn-metadata":` + strconv.Quote(turnMetadata) + `}}`)
	body = bytes.Replace(body, []byte("\"client_metadata\":{\"session_id\""), []byte("\"client_metadata\":{\"x-codex-installation-id\":\"ed12c212-f894-4ba5-9f47-22a0999590bc\",\"session_id\""), 1)
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
