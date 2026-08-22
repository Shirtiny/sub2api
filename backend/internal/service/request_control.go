package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	"github.com/google/uuid"
)

const (
	RequestControlActionBlock      = "block"
	RequestControlActionObserve    = "observe"
	RequestControlActionAllow      = "allow"
	RequestControlActionUABypass   = "ua_whitelist"
	RequestControlProtocolMessages = ContentModerationProtocolAnthropicMessages
	RequestControlProtocolChat     = ContentModerationProtocolOpenAIChat
	RequestControlProtocolResponse = ContentModerationProtocolOpenAIResponses

	requestControlDefaultBlockStatus  = http.StatusForbidden
	requestControlDefaultBlockMessage = "请求管控未通过客户端校验，请使用受支持的官方客户端"
	requestControlQueueSize           = 8192
	requestControlWorkerCount         = 2
	requestControlRefreshInterval     = 30 * time.Second
	requestControlRefreshTimeout      = 5 * time.Second
	requestControlRetentionDays       = 30
	requestControlMaxUserRules        = 2000
	requestControlMaxUAMarkers        = 200
	requestControlMaxUAMarkerRunes    = 200
	requestControlMaxDetails          = 32
)

var codexRequestUserAgentPattern = regexp.MustCompile(`(?i)^([a-z0-9][a-z0-9_. -]*)/(\d+\.\d+\.\d+)(?:[-+][0-9a-z.-]+)?(?:\s|$)`)

type RequestControlUserRule struct {
	UserID             int64    `json:"user_id"`
	Participate        bool     `json:"participate"`
	UserAgentWhitelist []string `json:"user_agent_whitelist"`
}

type RequestControlConfig struct {
	Enabled                  bool                         `json:"enabled"`
	AllGroups                bool                         `json:"all_groups"`
	GroupIDs                 []int64                      `json:"group_ids"`
	ModelFilter              ContentModerationModelFilter `json:"model_filter"`
	AllUsers                 bool                         `json:"all_users"`
	UserRules                []RequestControlUserRule     `json:"user_rules"`
	GlobalUserAgentWhitelist []string                     `json:"global_user_agent_whitelist"`
	BlockStatus              int                          `json:"block_status"`
	BlockMessage             string                       `json:"block_message"`
}

type RequestControlConfigView struct {
	Enabled                  bool                         `json:"enabled"`
	AllGroups                bool                         `json:"all_groups"`
	GroupIDs                 []int64                      `json:"group_ids"`
	ModelFilter              ContentModerationModelFilter `json:"model_filter"`
	AllUsers                 bool                         `json:"all_users"`
	UserRules                []RequestControlUserRule     `json:"user_rules"`
	GlobalUserAgentWhitelist []string                     `json:"global_user_agent_whitelist"`
	BlockStatus              int                          `json:"block_status"`
	BlockMessage             string                       `json:"block_message"`
}

type UpdateRequestControlConfigInput struct {
	Enabled                  *bool                         `json:"enabled"`
	AllGroups                *bool                         `json:"all_groups"`
	GroupIDs                 *[]int64                      `json:"group_ids"`
	ModelFilter              *ContentModerationModelFilter `json:"model_filter"`
	AllUsers                 *bool                         `json:"all_users"`
	UserRules                *[]RequestControlUserRule     `json:"user_rules"`
	GlobalUserAgentWhitelist *[]string                     `json:"global_user_agent_whitelist"`
	BlockStatus              *int                          `json:"block_status"`
	BlockMessage             *string                       `json:"block_message"`
}

type RequestControlCheckInput struct {
	RequestID         string
	UserID            int64
	UserEmail         string
	APIKeyID          int64
	APIKeyName        string
	GroupID           *int64
	EffectiveGroupIDs []int64
	GroupName         string
	Endpoint          string
	Provider          string
	Model             string
	Protocol          string
	Body              []byte
	Headers           http.Header
	UserAgent         string
	Originator        string
	TLSFingerprint    string
	WebSocket         bool
	ClaudeCodeValid   *bool
}

type RequestControlDecision struct {
	Allowed       bool              `json:"allowed"`
	Blocked       bool              `json:"blocked"`
	Observed      bool              `json:"observed"`
	Action        string            `json:"action"`
	Reason        string            `json:"reason"`
	Message       string            `json:"message"`
	StatusCode    int               `json:"status_code"`
	ClientKind    string            `json:"client_kind"`
	HeaderMatched bool              `json:"header_matched"`
	BodyMatched   bool              `json:"body_matched"`
	TLSMatched    *bool             `json:"tls_matched,omitempty"`
	Details       map[string]string `json:"details,omitempty"`
}

type RequestControlLog struct {
	ID             int64             `json:"id"`
	RequestID      string            `json:"request_id"`
	UserID         *int64            `json:"user_id"`
	UserEmail      string            `json:"user_email"`
	APIKeyID       *int64            `json:"api_key_id"`
	APIKeyName     string            `json:"api_key_name"`
	GroupID        *int64            `json:"group_id"`
	GroupName      string            `json:"group_name"`
	Endpoint       string            `json:"endpoint"`
	Provider       string            `json:"provider"`
	Protocol       string            `json:"protocol"`
	Model          string            `json:"model"`
	Action         string            `json:"action"`
	Reason         string            `json:"reason"`
	Allowed        bool              `json:"allowed"`
	Blocked        bool              `json:"blocked"`
	Observed       bool              `json:"observed"`
	ClientKind     string            `json:"client_kind"`
	UserAgent      string            `json:"user_agent"`
	Originator     string            `json:"originator"`
	TLSFingerprint string            `json:"tls_fingerprint"`
	TLSMatch       *bool             `json:"tls_match"`
	HeaderMatch    *bool             `json:"header_match"`
	BodyMatch      *bool             `json:"body_match"`
	Details        map[string]string `json:"details"`
	CreatedAt      time.Time         `json:"created_at"`
}

type RequestControlLogFilter struct {
	Pagination pagination.PaginationParams
	Action     string
	Protocol   string
	GroupID    *int64
	UserID     *int64
	Search     string
	From       *time.Time
	To         *time.Time
}

type RequestControlRuntimeStatus struct {
	Enabled            bool  `json:"enabled"`
	RiskControlEnabled bool  `json:"risk_control_enabled"`
	QueueSize          int   `json:"queue_size"`
	QueueLength        int   `json:"queue_length"`
	Enqueued           int64 `json:"enqueued"`
	Processed          int64 `json:"processed"`
	Dropped            int64 `json:"dropped"`
	Errors             int64 `json:"errors"`
}

type RequestControlRepository interface {
	CreateLog(context.Context, *RequestControlLog) error
	ListLogs(context.Context, RequestControlLogFilter) ([]RequestControlLog, *pagination.PaginationResult, error)
	CleanupLogs(context.Context, time.Time) (int64, error)
}

type requestControlRuntimeConfig struct {
	globalEnabled bool
	config        *RequestControlConfig
	userRules     map[int64]RequestControlUserRule
	groupIDs      map[int64]struct{}
	models        map[string]struct{}
}

type requestControlTask struct {
	log *RequestControlLog
}

type RequestControlService struct {
	settingRepo SettingRepository
	repo        RequestControlRepository
	groupRepo   GroupRepository
	runtime     atomic.Pointer[requestControlRuntimeConfig]
	refreshMu   sync.Mutex
	queue       chan requestControlTask
	validator   *ClaudeCodeValidator
	enqueued    atomic.Int64
	processed   atomic.Int64
	dropped     atomic.Int64
	errors      atomic.Int64
}

func NewRequestControlService(settingRepo SettingRepository, repo RequestControlRepository, groupRepo GroupRepository) *RequestControlService {
	svc := &RequestControlService{
		settingRepo: settingRepo,
		repo:        repo,
		groupRepo:   groupRepo,
		queue:       make(chan requestControlTask, requestControlQueueSize),
		validator:   NewClaudeCodeValidator(),
	}
	if settingRepo == nil {
		return svc
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestControlRefreshTimeout)
	if _, err := svc.refresh(ctx); err != nil {
		slog.Warn("request_control.runtime_warmup_failed", "error", err)
		fallback := defaultRequestControlConfig()
		fallback.normalize()
		svc.runtime.Store(newRequestControlRuntimeConfig(false, fallback))
	}
	cancel()
	for i := 0; i < requestControlWorkerCount; i++ {
		go svc.worker()
	}
	go svc.refreshWorker()
	go svc.cleanupWorker()
	return svc
}

func (s *RequestControlService) GetConfig(ctx context.Context) (*RequestControlConfigView, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	return requestControlConfigView(cfg), nil
}

func (s *RequestControlService) UpdateRiskControlEnabled(enabled bool) {
	if s == nil {
		return
	}
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	current := s.runtime.Load()
	if current == nil {
		return
	}
	next := *current
	next.globalEnabled = enabled
	s.runtime.Store(&next)
}

func (s *RequestControlService) UpdateConfig(ctx context.Context, input UpdateRequestControlConfigInput) (*RequestControlConfigView, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if input.Enabled != nil {
		cfg.Enabled = *input.Enabled
	}
	if input.AllGroups != nil {
		cfg.AllGroups = *input.AllGroups
	}
	if input.GroupIDs != nil {
		cfg.GroupIDs = append([]int64(nil), (*input.GroupIDs)...)
	}
	if input.ModelFilter != nil {
		cfg.ModelFilter = *input.ModelFilter
	}
	if input.AllUsers != nil {
		cfg.AllUsers = *input.AllUsers
	}
	if input.UserRules != nil {
		cfg.UserRules = append([]RequestControlUserRule(nil), (*input.UserRules)...)
	}
	if input.GlobalUserAgentWhitelist != nil {
		cfg.GlobalUserAgentWhitelist = append([]string(nil), (*input.GlobalUserAgentWhitelist)...)
	}
	if input.BlockStatus != nil {
		cfg.BlockStatus = *input.BlockStatus
	}
	if input.BlockMessage != nil {
		cfg.BlockMessage = *input.BlockMessage
	}
	cfg.normalize()
	if err := s.validateConfig(ctx, cfg); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal request control config: %w", err)
	}
	if s.settingRepo == nil {
		return nil, errors.New("request control settings are unavailable")
	}
	if err := s.settingRepo.Set(ctx, SettingKeyRequestControlConfig, string(raw)); err != nil {
		return nil, fmt.Errorf("save request control config: %w", err)
	}
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	riskEnabled := false
	if current := s.runtime.Load(); current != nil {
		riskEnabled = current.globalEnabled
	} else {
		var err error
		riskEnabled, err = s.loadRiskEnabled(ctx)
		if err != nil {
			return nil, fmt.Errorf("refresh request control feature state: %w", err)
		}
	}
	s.runtime.Store(newRequestControlRuntimeConfig(riskEnabled, cfg))
	return requestControlConfigView(cfg), nil
}

func (s *RequestControlService) Check(ctx context.Context, input RequestControlCheckInput) (*RequestControlDecision, error) {
	allow := &RequestControlDecision{Allowed: true, Action: RequestControlActionAllow}
	if s == nil || s.settingRepo == nil {
		return allow, nil
	}
	runtime := s.runtime.Load()
	if runtime == nil || runtime.config == nil {
		var err error
		runtime, err = s.refresh(ctx)
		if err != nil {
			return allow, nil
		}
	}
	if !runtime.globalEnabled || !runtime.config.Enabled {
		return allow, nil
	}
	cfg := runtime.config
	if !runtime.includesGroup(input.GroupID, input.EffectiveGroupIDs) || !runtime.includesModel(input.Model) || !cfg.includesUser(input.UserID, runtime.userRules) {
		return allow, nil
	}
	var userMarkers []string
	if rule, ok := runtime.userRules[input.UserID]; ok {
		userMarkers = rule.UserAgentWhitelist
	}
	if matchesAnyUA(cfg.GlobalUserAgentWhitelist, input.UserAgent) || matchesAnyUA(userMarkers, input.UserAgent) {
		decision := &RequestControlDecision{Allowed: true, Action: RequestControlActionUABypass, Reason: "user_agent_whitelist", ClientKind: "whitelisted"}
		return decision, nil
	}
	var decision *RequestControlDecision
	switch input.Protocol {
	case RequestControlProtocolChat:
		decision = &RequestControlDecision{Blocked: true, Action: RequestControlActionBlock, Reason: "openai_chat_completions_blocked", Message: cfg.BlockMessage, StatusCode: cfg.BlockStatus, ClientKind: "openai_chat"}
	case RequestControlProtocolMessages:
		headerMatched := validateClaudeCodeRequestHeaders(input.Headers, s.validator)
		structureMatched := validateClaudeCodeRequestStructure(input.Body)
		if input.ClaudeCodeValid != nil {
			valid := *input.ClaudeCodeValid && headerMatched && structureMatched
			decision = requestControlClaudeCodeDecision(cfg, valid, headerMatched)
		} else {
			var body map[string]any
			if err := json.Unmarshal(input.Body, &body); err != nil {
				decision = &RequestControlDecision{Blocked: true, Action: RequestControlActionBlock, Reason: "claude_code_body_invalid_json", Message: cfg.BlockMessage, StatusCode: cfg.BlockStatus, ClientKind: "claude_messages", HeaderMatched: headerMatched}
				break
			}
			validator := s.validator
			if validator == nil {
				validator = NewClaudeCodeValidator()
			}
			valid := validator.Validate(requestForInput(input), body) && headerMatched && structureMatched
			decision = requestControlClaudeCodeDecision(cfg, valid, headerMatched)
		}
	case RequestControlProtocolResponse:
		if !openai.IsCodexOfficialClientRequest(input.UserAgent) {
			decision = &RequestControlDecision{Allowed: true, Observed: true, Action: RequestControlActionObserve, Reason: "non_codex_user_agent", ClientKind: "non_codex"}
		} else {
			headerMatched, bodyMatched, details := validateCodexResponsesRequest(input)
			valid := headerMatched && bodyMatched
			decision = &RequestControlDecision{Allowed: valid, Blocked: !valid, Observed: !valid, Action: RequestControlActionAllow, Reason: "codex_request_valid", Message: cfg.BlockMessage, StatusCode: cfg.BlockStatus, ClientKind: "codex", BodyMatched: bodyMatched, HeaderMatched: headerMatched, Details: details}
			if !valid {
				decision.Action = RequestControlActionBlock
				decision.Reason = "codex_request_signature_mismatch"
			}
		}
	default:
		return allow, nil
	}
	if decision.Blocked || decision.Observed {
		s.enqueueLog(buildRequestControlLog(input, decision))
	}
	return decision, nil
}

func validateClaudeCodeRequestHeaders(headers http.Header, validator *ClaudeCodeValidator) bool {
	if validator == nil {
		return false
	}
	userAgent, uaSingle := requestControlSingleHeader(headers, "User-Agent")
	xApp, xAppSingle := requestControlSingleHeader(headers, "X-App")
	version, versionSingle := requestControlSingleHeader(headers, "anthropic-version")
	beta, betaSingle := requestControlSingleHeader(headers, "anthropic-beta")
	return uaSingle && validator.ValidateUserAgent(userAgent) &&
		xAppSingle && (xApp == "cli" || xApp == "claude-code") &&
		versionSingle && version == "2023-06-01" &&
		betaSingle && requestControlHeaderTokenContains(beta, "claude-code-20250219")
}

func validateClaudeCodeRequestStructure(body []byte) bool {
	err := openaiwsv2.VisitTopLevelObjectFields(body, func(key, rawValue []byte) error {
		if string(key) != "metadata" {
			return nil
		}
		return openaiwsv2.ValidateTopLevelObject(rawValue)
	})
	return err == nil
}

func requestControlClaudeCodeDecision(cfg *RequestControlConfig, valid, headerMatched bool) *RequestControlDecision {
	decision := &RequestControlDecision{
		Allowed:       valid,
		Blocked:       !valid,
		Action:        RequestControlActionAllow,
		Reason:        "claude_code_valid",
		Message:       cfg.BlockMessage,
		StatusCode:    cfg.BlockStatus,
		ClientKind:    "claude_code",
		BodyMatched:   valid,
		HeaderMatched: headerMatched,
	}
	if !valid {
		decision.Action = RequestControlActionBlock
		decision.Reason = "claude_code_signature_mismatch"
		decision.Details = map[string]string{"signature": "claude_code_headers_or_body_mismatch"}
	}
	return decision
}

func (s *RequestControlService) ListLogs(ctx context.Context, filter RequestControlLogFilter) ([]RequestControlLog, *pagination.PaginationResult, error) {
	if s == nil || s.repo == nil {
		params := filter.Pagination
		if params.Page <= 0 {
			params.Page = 1
		}
		if params.PageSize <= 0 {
			params.PageSize = 20
		}
		return []RequestControlLog{}, &pagination.PaginationResult{Total: 0, Page: params.Page, PageSize: params.PageSize, Pages: 0}, nil
	}
	return s.repo.ListLogs(ctx, filter)
}

func (s *RequestControlService) GetStatus() RequestControlRuntimeStatus {
	if s == nil {
		return RequestControlRuntimeStatus{}
	}
	status := RequestControlRuntimeStatus{
		QueueSize:   cap(s.queue),
		QueueLength: len(s.queue),
		Enqueued:    s.enqueued.Load(),
		Processed:   s.processed.Load(),
		Dropped:     s.dropped.Load(),
		Errors:      s.errors.Load(),
	}
	if current := s.runtime.Load(); current != nil && current.config != nil {
		status.Enabled = current.config.Enabled
		status.RiskControlEnabled = current.globalEnabled
	}
	return status
}

func (s *RequestControlService) enqueueLog(log *RequestControlLog) {
	if s == nil || log == nil || s.repo == nil {
		return
	}
	select {
	case s.queue <- requestControlTask{log: log}:
		s.enqueued.Add(1)
	default:
		s.dropped.Add(1)
		slog.Warn("request_control.log_queue_full", "user_id", log.UserID, "protocol", log.Protocol, "reason", log.Reason)
	}
}

func (s *RequestControlService) worker() {
	for task := range s.queue {
		if task.log == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.repo.CreateLog(ctx, task.log); err != nil {
			s.errors.Add(1)
			slog.Warn("request_control.log_persist_failed", "error", err)
		} else {
			s.processed.Add(1)
		}
		cancel()
	}
}

func (s *RequestControlService) refreshWorker() {
	ticker := time.NewTicker(requestControlRefreshInterval)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), requestControlRefreshTimeout)
		if _, err := s.refresh(ctx); err != nil {
			slog.Warn("request_control.runtime_refresh_failed", "error", err)
		}
		cancel()
	}
}

func (s *RequestControlService) cleanupWorker() {
	if s == nil || s.repo == nil {
		return
	}
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if deleted, err := s.repo.CleanupLogs(ctx, time.Now().Add(-requestControlRetentionDays*24*time.Hour)); err != nil {
			slog.Warn("request_control.cleanup_failed", "error", err)
		} else if deleted > 0 {
			slog.Info("request_control.cleanup_completed", "deleted", deleted)
		}
		cancel()
	}
}

func (s *RequestControlService) refresh(ctx context.Context) (*requestControlRuntimeConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("request control settings are unavailable")
	}
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	riskEnabled, err := s.loadRiskEnabled(ctx)
	if err != nil {
		return nil, err
	}
	next := newRequestControlRuntimeConfig(riskEnabled, cfg)
	s.runtime.Store(next)
	return next, nil
}

func newRequestControlRuntimeConfig(riskEnabled bool, cfg *RequestControlConfig) *requestControlRuntimeConfig {
	rules := make(map[int64]RequestControlUserRule, len(cfg.UserRules))
	for _, rule := range cfg.UserRules {
		rules[rule.UserID] = rule
	}
	groups := make(map[int64]struct{}, len(cfg.GroupIDs))
	for _, groupID := range cfg.GroupIDs {
		groups[groupID] = struct{}{}
	}
	models := make(map[string]struct{}, len(cfg.ModelFilter.Models))
	for _, model := range cfg.ModelFilter.Models {
		models[strings.ToLower(strings.TrimSpace(model))] = struct{}{}
	}
	return &requestControlRuntimeConfig{
		globalEnabled: riskEnabled,
		config:        cfg,
		userRules:     rules,
		groupIDs:      groups,
		models:        models,
	}
}

func (s *RequestControlService) loadRiskEnabled(ctx context.Context) (bool, error) {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyRiskControlEnabled)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(raw) == "true", nil
}

func (s *RequestControlService) loadConfig(ctx context.Context) (*RequestControlConfig, error) {
	cfg := defaultRequestControlConfig()
	if s.settingRepo == nil {
		return cfg, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyRequestControlConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return cfg, nil
		}
		return nil, fmt.Errorf("get request control config: %w", err)
	}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), cfg); err != nil {
			return nil, infraerrors.BadRequest("INVALID_REQUEST_CONTROL_CONFIG", "请求管控配置不是有效 JSON")
		}
	}
	cfg.normalize()
	return cfg, nil
}

func (s *RequestControlService) validateConfig(ctx context.Context, cfg *RequestControlConfig) error {
	if cfg == nil {
		return infraerrors.BadRequest("INVALID_REQUEST_CONTROL_CONFIG", "请求管控配置不能为空")
	}
	if cfg.BlockStatus < 400 || cfg.BlockStatus > 599 {
		return infraerrors.BadRequest("INVALID_REQUEST_CONTROL_BLOCK_STATUS", "拦截 HTTP 状态码必须在 400-599 之间")
	}
	if cfg.ModelFilter.Type != ContentModerationModelFilterAll && len(cfg.ModelFilter.Models) == 0 {
		return infraerrors.BadRequest("INVALID_REQUEST_CONTROL_MODEL_FILTER", "指定或排除模型时至少需要配置 1 个模型")
	}
	if !cfg.AllGroups && s.groupRepo != nil {
		for _, id := range cfg.GroupIDs {
			if _, err := s.groupRepo.GetByIDLite(ctx, id); err != nil {
				return infraerrors.BadRequest("INVALID_REQUEST_CONTROL_GROUP", fmt.Sprintf("管控分组不存在: %d", id))
			}
		}
	}
	return nil
}

func defaultRequestControlConfig() *RequestControlConfig {
	return &RequestControlConfig{AllGroups: true, ModelFilter: ContentModerationModelFilter{Type: ContentModerationModelFilterAll}, AllUsers: true, UserRules: []RequestControlUserRule{}, GlobalUserAgentWhitelist: []string{}, BlockStatus: requestControlDefaultBlockStatus, BlockMessage: requestControlDefaultBlockMessage}
}

func (cfg *RequestControlConfig) normalize() {
	if cfg == nil {
		return
	}
	cfg.GroupIDs = normalizeInt64IDs(cfg.GroupIDs)
	cfg.ModelFilter = normalizeContentModerationModelFilter(cfg.ModelFilter)
	if cfg.BlockStatus <= 0 {
		cfg.BlockStatus = requestControlDefaultBlockStatus
	}
	if strings.TrimSpace(cfg.BlockMessage) == "" {
		cfg.BlockMessage = requestControlDefaultBlockMessage
	}
	cfg.BlockMessage = strings.TrimSpace(cfg.BlockMessage)
	if len(cfg.UserRules) > requestControlMaxUserRules {
		cfg.UserRules = cfg.UserRules[:requestControlMaxUserRules]
	}
	rules := make(map[int64]RequestControlUserRule, len(cfg.UserRules))
	for _, rule := range cfg.UserRules {
		if rule.UserID <= 0 {
			continue
		}
		rule.UserAgentWhitelist = normalizeUAMarkers(rule.UserAgentWhitelist)
		rules[rule.UserID] = rule
	}
	cfg.UserRules = make([]RequestControlUserRule, 0, len(rules))
	for _, rule := range rules {
		cfg.UserRules = append(cfg.UserRules, rule)
	}
	sort.Slice(cfg.UserRules, func(i, j int) bool { return cfg.UserRules[i].UserID < cfg.UserRules[j].UserID })
	cfg.GlobalUserAgentWhitelist = normalizeUAMarkers(cfg.GlobalUserAgentWhitelist)
}

func requestControlConfigView(cfg *RequestControlConfig) *RequestControlConfigView {
	if cfg == nil {
		return nil
	}
	return &RequestControlConfigView{Enabled: cfg.Enabled, AllGroups: cfg.AllGroups, GroupIDs: append([]int64(nil), cfg.GroupIDs...), ModelFilter: cfg.ModelFilter, AllUsers: cfg.AllUsers, UserRules: append([]RequestControlUserRule(nil), cfg.UserRules...), GlobalUserAgentWhitelist: append([]string(nil), cfg.GlobalUserAgentWhitelist...), BlockStatus: cfg.BlockStatus, BlockMessage: cfg.BlockMessage}
}

func (runtime *requestControlRuntimeConfig) includesGroup(groupID *int64, effective []int64) bool {
	if runtime == nil || runtime.config == nil {
		return false
	}
	if runtime.config.AllGroups {
		return true
	}
	if groupID != nil {
		if _, ok := runtime.groupIDs[*groupID]; ok {
			return true
		}
	}
	for _, candidate := range effective {
		if _, ok := runtime.groupIDs[candidate]; ok {
			return true
		}
	}
	return false
}

func (runtime *requestControlRuntimeConfig) includesModel(model string) bool {
	if runtime == nil || runtime.config == nil {
		return false
	}
	_, listed := runtime.models[strings.ToLower(strings.TrimSpace(model))]
	switch runtime.config.ModelFilter.Type {
	case ContentModerationModelFilterInclude:
		return listed
	case ContentModerationModelFilterExclude:
		return !listed
	default:
		return true
	}
}

func (cfg *RequestControlConfig) includesUser(userID int64, rules map[int64]RequestControlUserRule) bool {
	rule, ok := rules[userID]
	if ok {
		return rule.Participate
	}
	return cfg.AllUsers
}

func normalizeUAMarkers(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" || len([]rune(value)) > requestControlMaxUAMarkerRunes {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
		if len(out) >= requestControlMaxUAMarkers {
			break
		}
	}
	sort.Strings(out)
	return out
}

func matchesAnyUA(markers []string, ua string) bool {
	ua = strings.ToLower(strings.TrimSpace(ua))
	if ua == "" {
		return false
	}
	for _, marker := range markers {
		if strings.HasPrefix(ua, marker) {
			return true
		}
	}
	return false
}

func requestForInput(input RequestControlCheckInput) *http.Request {
	r := &http.Request{Header: input.Headers, URL: &url.URL{Path: input.Endpoint}}
	return r
}

type requestControlJSONKind struct {
	Present bool
	Kind    byte
}

func parseRequestControlJSONKind(raw []byte) (requestControlJSONKind, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return requestControlJSONKind{}, errors.New("empty JSON value")
	}
	return requestControlJSONKind{Present: true, Kind: trimmed[0]}, nil
}

type requestControlJSONString struct {
	Present bool
	Valid   bool
	Value   string
}

func parseRequestControlJSONString(raw []byte, maxDecodedBytes int) (requestControlJSONString, error) {
	value := requestControlJSONString{Present: true}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return value, errors.New("empty JSON string")
	}
	if trimmed[0] != '"' {
		return value, nil
	}
	if maxDecodedBytes > 0 && len(trimmed) > maxDecodedBytes*6+2 {
		return value, errors.New("JSON string is too large")
	}
	if err := json.Unmarshal(trimmed, &value.Value); err != nil {
		return value, err
	}
	if maxDecodedBytes > 0 && len(value.Value) > maxDecodedBytes {
		return value, errors.New("JSON string is too large")
	}
	value.Valid = true
	return value, nil
}

type requestControlJSONBool struct {
	Present bool
	Valid   bool
	Value   bool
}

func parseRequestControlJSONBool(raw []byte) requestControlJSONBool {
	trimmed := bytes.TrimSpace(raw)
	value := requestControlJSONBool{Present: true}
	switch {
	case bytes.Equal(trimmed, []byte("true")):
		value.Valid = true
		value.Value = true
	case bytes.Equal(trimmed, []byte("false")):
		value.Valid = true
	}
	return value
}

type requestControlStringArray struct {
	Present bool
	Valid   bool
	Values  []string
}

func parseRequestControlStringArray(raw []byte) (requestControlStringArray, error) {
	value := requestControlStringArray{Present: true}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return value, nil
	}
	if len(trimmed) > 4096 {
		return value, errors.New("JSON string array is too large")
	}
	if err := json.Unmarshal(trimmed, &value.Values); err != nil {
		return value, err
	}
	value.Valid = true
	return value, nil
}

func (value requestControlStringArray) Contains(expected string) bool {
	if !value.Valid {
		return false
	}
	for _, item := range value.Values {
		if item == expected {
			return true
		}
	}
	return false
}

type requestControlClientMetadata struct {
	Present      bool
	Valid        bool
	SessionID    requestControlJSONString
	ThreadID     requestControlJSONString
	TurnMetadata requestControlJSONString
}

func parseRequestControlClientMetadata(raw []byte) (requestControlClientMetadata, error) {
	metadata := requestControlClientMetadata{Present: true}
	err := openaiwsv2.VisitTopLevelObjectFields(raw, func(key, rawValue []byte) error {
		var err error
		switch string(key) {
		case "session_id":
			metadata.SessionID, err = parseRequestControlJSONString(rawValue, openaiwsv2.ClientEnvelopeMaxRouteIDBytes)
		case "thread_id":
			metadata.ThreadID, err = parseRequestControlJSONString(rawValue, openaiwsv2.ClientEnvelopeMaxRouteIDBytes)
		case "x-codex-turn-metadata":
			metadata.TurnMetadata, err = parseRequestControlJSONString(rawValue, 0)
		}
		return err
	})
	if err != nil {
		return metadata, err
	}
	metadata.Valid = true
	return metadata, nil
}

type requestControlResponsesBody struct {
	Model             requestControlJSONString
	Input             requestControlJSONKind
	ToolChoice        requestControlJSONString
	ParallelToolCalls requestControlJSONBool
	Reasoning         requestControlJSONKind
	Store             requestControlJSONBool
	Stream            requestControlJSONBool
	Include           requestControlStringArray
	PromptCacheKey    requestControlJSONString
	Type              requestControlJSONString
	ClientMetadata    requestControlClientMetadata
}

func parseRequestControlResponsesBody(raw []byte) (requestControlResponsesBody, error) {
	var body requestControlResponsesBody
	err := openaiwsv2.VisitTopLevelObjectFields(raw, func(key, rawValue []byte) error {
		var err error
		switch string(key) {
		case "model":
			body.Model, err = parseRequestControlJSONString(rawValue, openaiwsv2.ClientEnvelopeMaxIdentifierBytes)
		case "input":
			body.Input, err = parseRequestControlJSONKind(rawValue)
		case "tool_choice":
			body.ToolChoice, err = parseRequestControlJSONString(rawValue, openaiwsv2.ClientEnvelopeMaxOptionBytes)
		case "parallel_tool_calls":
			body.ParallelToolCalls = parseRequestControlJSONBool(rawValue)
		case "reasoning":
			body.Reasoning, err = parseRequestControlJSONKind(rawValue)
		case "store":
			body.Store = parseRequestControlJSONBool(rawValue)
		case "stream":
			body.Stream = parseRequestControlJSONBool(rawValue)
		case "include":
			body.Include, err = parseRequestControlStringArray(rawValue)
		case "prompt_cache_key":
			body.PromptCacheKey, err = parseRequestControlJSONString(rawValue, openaiwsv2.ClientEnvelopeMaxCacheKeyBytes)
		case "type":
			body.Type, err = parseRequestControlJSONString(rawValue, openaiwsv2.ClientEnvelopeMaxEventTypeBytes)
		case "client_metadata":
			body.ClientMetadata, err = parseRequestControlClientMetadata(rawValue)
		}
		return err
	})
	return body, err
}

// validateCodexResponsesRequest mirrors the stable request contract emitted by
// Codex core/client.rs and login/auth/default_client.rs. Compact and WebSocket
// requests intentionally have different transport fields.
func validateCodexResponsesRequest(input RequestControlCheckInput) (bool, bool, map[string]string) {
	details := make(map[string]string)
	userAgent, uaSingle := requestControlSingleHeader(input.Headers, "User-Agent")
	originator, originatorSingle := requestControlSingleHeader(input.Headers, "originator")
	uaMatch := codexRequestUserAgentPattern.FindStringSubmatch(userAgent)
	headerOK := uaSingle && originatorSingle && len(uaMatch) == 3 &&
		openai.IsCodexOfficialClientRequest(userAgent) &&
		openai.IsCodexOfficialClientOriginator(originator)
	if !headerOK {
		details["client_headers"] = "missing_or_invalid_codex_identity"
	}

	compact := strings.HasSuffix(strings.ToLower(strings.TrimRight(input.Endpoint, "/")), "/responses/compact")
	if !input.WebSocket {
		contentType, ok := requestControlSingleHeader(input.Headers, "Content-Type")
		mediaType, _, err := mime.ParseMediaType(contentType)
		if !ok || err != nil || !strings.EqualFold(mediaType, "application/json") {
			headerOK = false
			details["content_type"] = "expected_application_json"
		}
		if !compact {
			accept, ok := requestControlSingleHeader(input.Headers, "Accept")
			if !ok || !requestControlHeaderTokenContains(accept, "text/event-stream") {
				headerOK = false
				details["accept"] = "expected_text_event_stream"
			}
		}
	} else {
		beta, ok := requestControlSingleHeader(input.Headers, "OpenAI-Beta")
		if !ok || !requestControlHeaderTokenContains(beta, "responses_websockets=2026-02-06") {
			headerOK = false
			details["websocket_beta"] = "missing"
		}
	}

	sessionID, sessionSingle := requestControlSingleHeader(input.Headers, "session-id")
	threadID, threadSingle := requestControlSingleHeader(input.Headers, "thread-id")
	if !sessionSingle || !threadSingle || !requestControlUUID(sessionID) || !requestControlUUID(threadID) {
		headerOK = false
		details["session_identity"] = "missing_or_invalid"
	}
	clientRequestID, clientRequestSingle := requestControlSingleHeader(input.Headers, "x-client-request-id")
	if !compact || input.WebSocket {
		if !clientRequestSingle || clientRequestID != threadID {
			headerOK = false
			details["client_request_id"] = "missing_or_thread_mismatch"
		}
	} else if clientRequestSingle && clientRequestID != threadID {
		headerOK = false
		details["client_request_id"] = "thread_mismatch"
	}

	turnMetadataRaw, turnMetadataSingle := requestControlSingleHeader(input.Headers, "x-codex-turn-metadata")
	turnMetadata, turnMetadataOK := parseRequestControlTurnMetadata(turnMetadataRaw)
	turnIdentityMatches := turnMetadata.RequestKind == "memory" ||
		(turnMetadata.SessionID == sessionID && turnMetadata.ThreadID == threadID)
	if !turnMetadataSingle || !turnMetadataOK || !turnIdentityMatches {
		headerOK = false
		details["turn_metadata"] = "missing_invalid_or_identity_mismatch"
	}

	body, err := parseRequestControlResponsesBody(input.Body)
	bodyOK := err == nil
	if err != nil {
		details["body_json"] = "invalid"
		return headerOK, false, details
	}
	if !body.Model.Present || !body.Model.Valid || strings.TrimSpace(body.Model.Value) == "" {
		bodyOK = false
		details["model"] = "missing"
	}
	if !body.Input.Present || body.Input.Kind != '[' {
		bodyOK = false
		details["input"] = "expected_array"
	}
	if !body.ParallelToolCalls.Present || !body.ParallelToolCalls.Valid {
		bodyOK = false
		details["parallel_tool_calls"] = "expected_boolean"
	}

	if compact {
		if body.Reasoning.Present && body.Reasoning.Kind != '{' {
			bodyOK = false
			details["reasoning"] = "expected_object"
		}
		if body.PromptCacheKey.Present && (!body.PromptCacheKey.Valid || strings.TrimSpace(body.PromptCacheKey.Value) == "") {
			bodyOK = false
			details["prompt_cache_key"] = "expected_non_empty_string"
		}
		return headerOK, bodyOK, details
	}
	if !body.Reasoning.Present || body.Reasoning.Kind != '{' {
		bodyOK = false
		details["reasoning"] = "expected_object"
	}
	if !body.PromptCacheKey.Present || !body.PromptCacheKey.Valid || strings.TrimSpace(body.PromptCacheKey.Value) == "" {
		bodyOK = false
		details["prompt_cache_key"] = "missing"
	}
	if !body.Store.Present || !body.Store.Valid || body.Store.Value {
		bodyOK = false
		details["store"] = "expected_false"
	}
	if !body.Stream.Present || !body.Stream.Valid || !body.Stream.Value {
		bodyOK = false
		details["stream"] = "expected_true"
	}
	if !body.ToolChoice.Valid || strings.TrimSpace(body.ToolChoice.Value) != "auto" {
		bodyOK = false
		details["tool_choice"] = "expected_auto"
	}
	if !body.Include.Contains("reasoning.encrypted_content") {
		bodyOK = false
		details["include"] = "missing_reasoning_encrypted_content"
	}
	if input.WebSocket && (!body.Type.Valid || body.Type.Value != "response.create") {
		bodyOK = false
		details["websocket_type"] = "expected_response_create"
	}
	bodyTurnMetadata := strings.TrimSpace(body.ClientMetadata.TurnMetadata.Value)
	bodyMetadata, bodyMetadataOK := parseRequestControlTurnMetadata(bodyTurnMetadata)
	if !body.ClientMetadata.Present || !body.ClientMetadata.Valid || !body.ClientMetadata.TurnMetadata.Valid ||
		bodyTurnMetadata == "" || !bodyMetadataOK || !requestControlTurnMetadataMatches(turnMetadata, bodyMetadata) {
		bodyOK = false
		details["body_turn_metadata"] = "missing_invalid_or_header_mismatch"
	}
	if !body.ClientMetadata.SessionID.Valid || !body.ClientMetadata.ThreadID.Valid ||
		body.ClientMetadata.SessionID.Value != sessionID || body.ClientMetadata.ThreadID.Value != threadID {
		bodyOK = false
		details["body_session_identity"] = "header_mismatch"
	}
	return headerOK, bodyOK, details
}

func requestControlTurnMetadataMatches(header, body requestControlTurnMetadata) bool {
	return header.InstallationID == body.InstallationID &&
		header.SessionID == body.SessionID &&
		header.ThreadID == body.ThreadID &&
		header.TurnID == body.TurnID &&
		header.RequestKind == body.RequestKind
}

type requestControlTurnMetadata struct {
	InstallationID string `json:"installation_id"`
	SessionID      string `json:"session_id"`
	ThreadID       string `json:"thread_id"`
	TurnID         string `json:"turn_id"`
	RequestKind    string `json:"request_kind"`
}

func parseRequestControlTurnMetadata(raw string) (requestControlTurnMetadata, bool) {
	var metadata requestControlTurnMetadata
	if openaiwsv2.ValidateTopLevelObject([]byte(raw)) != nil {
		return metadata, false
	}
	if json.Unmarshal([]byte(raw), &metadata) != nil {
		return metadata, false
	}
	metadata.RequestKind = strings.TrimSpace(metadata.RequestKind)
	valid := false
	switch metadata.RequestKind {
	case "memory":
		valid = metadata.InstallationID == "" && metadata.SessionID == "" &&
			metadata.ThreadID == "" && metadata.TurnID == ""
	case "turn", "prewarm", "compaction":
		valid = requestControlUUID(metadata.InstallationID) && requestControlUUID(metadata.SessionID) &&
			requestControlUUID(metadata.ThreadID) &&
			(strings.TrimSpace(metadata.TurnID) == "" || requestControlUUID(metadata.TurnID)) &&
			metadata.RequestKind != ""
	}
	return metadata, valid
}

func requestControlSingleHeader(headers http.Header, name string) (string, bool) {
	values := headers.Values(name)
	if len(values) != 1 {
		return "", false
	}
	value := strings.TrimSpace(values[0])
	return value, value != ""
}

func requestControlHeaderTokenContains(value, expected string) bool {
	for _, token := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(token), expected) {
			return true
		}
	}
	return false
}

func requestControlUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}

func buildRequestControlLog(input RequestControlCheckInput, decision *RequestControlDecision) *RequestControlLog {
	log := &RequestControlLog{
		RequestID:      truncateRequestControlValue(input.RequestID, 128),
		UserEmail:      truncateRequestControlValue(input.UserEmail, 255),
		APIKeyName:     truncateRequestControlValue(input.APIKeyName, 100),
		GroupName:      truncateRequestControlValue(input.GroupName, 255),
		Endpoint:       truncateRequestControlValue(input.Endpoint, 128),
		Provider:       truncateRequestControlValue(input.Provider, 64),
		Protocol:       truncateRequestControlValue(input.Protocol, 64),
		Model:          truncateRequestControlValue(input.Model, 255),
		Action:         truncateRequestControlValue(decision.Action, 32),
		Reason:         truncateRequestControlValue(decision.Reason, 128),
		Allowed:        decision.Allowed,
		Blocked:        decision.Blocked,
		Observed:       decision.Observed,
		ClientKind:     truncateRequestControlValue(decision.ClientKind, 64),
		UserAgent:      truncateRequestControlValue(input.UserAgent, 512),
		Originator:     truncateRequestControlValue(input.Originator, 128),
		TLSFingerprint: truncateRequestControlValue(input.TLSFingerprint, 128),
		Details:        limitRequestControlDetails(decision.Details),
		CreatedAt:      time.Now(),
	}
	if input.UserID > 0 {
		log.UserID = &input.UserID
	}
	if input.APIKeyID > 0 {
		log.APIKeyID = &input.APIKeyID
	}
	log.GroupID = cloneInt64Ptr(input.GroupID)
	log.HeaderMatch = requestControlBoolPtr(decision.HeaderMatched)
	log.BodyMatch = requestControlBoolPtr(decision.BodyMatched)
	log.TLSMatch = decision.TLSMatched
	return log
}

func requestControlBoolPtr(value bool) *bool { return &value }
func truncateRequestControlValue(value string, limit int) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ToValidUTF8(value, ""), "\x00", ""))
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
func limitRequestControlDetails(in map[string]string) map[string]string {
	out := make(map[string]string)
	count := 0
	for key, value := range in {
		if count >= requestControlMaxDetails {
			break
		}
		out[key] = truncateRequestControlValue(value, 200)
		count++
	}
	return out
}
