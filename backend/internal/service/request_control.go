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

	requestControlDefaultBlockStatus          = http.StatusForbidden
	requestControlDefaultBlockMessage         = "内容违规，多次尝试将被封禁"
	requestControlDefaultBanThreshold         = 4
	requestControlDefaultViolationWindowHours = 720
	// Request-control logs are retained for 30 days, so the counting window
	// cannot exceed the data available to the state query.
	requestControlMaxViolationWindowHours = 720
	requestControlMaxBanThreshold         = 1000
	requestControlHitSpacing              = 5 * time.Minute
	requestControlQueueSize               = 8192
	requestControlSnapshotQueueBytes      = 64 * 1024 * 1024
	requestControlWorkerCount             = 2
	requestControlRefreshInterval         = 30 * time.Second
	requestControlRefreshTimeout          = 5 * time.Second
	requestControlWorkerTimeout           = 30 * time.Second
	requestControlRetentionDays           = 30
	requestControlSnapshotRetentionDays   = 3
	requestControlMaxUserRules            = 2000
	requestControlMaxUAMarkers            = 200
	requestControlMaxUAMarkerRunes        = 200
	requestControlMaxDetails              = 32
	// Captured by Aether from the official Codex CLI transport profile. TLS is
	// diagnostic-only here: a mismatch is observed and logged, never used as a
	// blocking predicate.
	requestControlCodexDefaultJA3Hash = "23211f2b48104c7030b93680a2efcfd0"
	requestControlCodexDefaultJA3     = "771,4866-4867-4865-49196-49200-159-52393-52392-52394-49195-49199-158-49188-49192-107-49187-49191-103-49162-49172-57-49161-49171-51-157-156-61-60-53-47,65281-11-10-35-22-23-13-43-45-51,4588-29-23-30-24-25-256-257,0"
	// Captured from the official Claude Code Node.js transport profile.
	requestControlClaudeDefaultJA3Hash = "44f88fca027f27bab4bb08d4af15f23e"
	requestControlClaudeDefaultJA4     = "t13d1714h1_5b57614c22b0_7baf387fc6ff"
)

var codexRequestUserAgentPattern = regexp.MustCompile(`(?i)^([a-z0-9][a-z0-9_. -]*)/(\d+\.\d+\.\d+)(?:[-+][0-9a-z.-]+)?(?:\s|$)`)
var codexDesktopUserAgentPattern = regexp.MustCompile(`(?i)^codex desktop/\d+\.\d+\.\d+(?:[-+][0-9a-z.-]+)?(?:\s|$)`)

type RequestControlUserRule struct {
	UserID             int64    `json:"user_id"`
	Participate        bool     `json:"participate"`
	UserAgentWhitelist []string `json:"user_agent_whitelist"`
}

type RequestControlConfig struct {
	Enabled                    bool                         `json:"enabled"`
	RequestSnapshotEnabled     bool                         `json:"request_snapshot_enabled"`
	BlockOpenAIChat            bool                         `json:"block_openai_chat"`
	BlockClaudeMessages        bool                         `json:"block_claude_messages"`
	BlockOpenAIResponses       bool                         `json:"block_openai_responses"`
	AllGroups                  bool                         `json:"all_groups"`
	GroupIDs                   []int64                      `json:"group_ids"`
	ModelFilter                ContentModerationModelFilter `json:"model_filter"`
	AllUsers                   bool                         `json:"all_users"`
	UserRules                  []RequestControlUserRule     `json:"user_rules"`
	GlobalUserAgentWhitelist   []string                     `json:"global_user_agent_whitelist"`
	BlockStatus                int                          `json:"block_status"`
	BlockMessage               string                       `json:"block_message"`
	EmailOnHit                 bool                         `json:"email_on_hit"`
	AutoBanEnabled             bool                         `json:"auto_ban_enabled"`
	BanThreshold               int                          `json:"ban_threshold"`
	ViolationWindowHours       int                          `json:"violation_window_hours"`
	protocolSwitchesConfigured bool
}

// UnmarshalJSON keeps existing settings fail-closed when the per-protocol
// enforcement switches were introduced after the original config was saved.
func (cfg *RequestControlConfig) UnmarshalJSON(data []byte) error {
	type requestControlConfigAlias RequestControlConfig
	// Decode into the existing value instead of replacing it with a zero value.
	// loadConfig starts from defaults, so omitted fields in older persisted JSON
	// must retain those defaults while the newly introduced switches are filled
	// below.
	if err := json.Unmarshal(data, (*requestControlConfigAlias)(cfg)); err != nil {
		return err
	}
	cfg.protocolSwitchesConfigured = true
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if raw, ok := fields["block_openai_chat"]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		cfg.BlockOpenAIChat = true
	}
	if raw, ok := fields["block_claude_messages"]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		cfg.BlockClaudeMessages = true
	}
	if raw, ok := fields["block_openai_responses"]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		cfg.BlockOpenAIResponses = true
	}
	return nil
}

type RequestControlConfigView struct {
	Enabled                  bool                         `json:"enabled"`
	RequestSnapshotEnabled   bool                         `json:"request_snapshot_enabled"`
	BlockOpenAIChat          bool                         `json:"block_openai_chat"`
	BlockClaudeMessages      bool                         `json:"block_claude_messages"`
	BlockOpenAIResponses     bool                         `json:"block_openai_responses"`
	AllGroups                bool                         `json:"all_groups"`
	GroupIDs                 []int64                      `json:"group_ids"`
	ModelFilter              ContentModerationModelFilter `json:"model_filter"`
	AllUsers                 bool                         `json:"all_users"`
	UserRules                []RequestControlUserRule     `json:"user_rules"`
	GlobalUserAgentWhitelist []string                     `json:"global_user_agent_whitelist"`
	BlockStatus              int                          `json:"block_status"`
	BlockMessage             string                       `json:"block_message"`
	EmailOnHit               bool                         `json:"email_on_hit"`
	AutoBanEnabled           bool                         `json:"auto_ban_enabled"`
	BanThreshold             int                          `json:"ban_threshold"`
	ViolationWindowHours     int                          `json:"violation_window_hours"`
}

type UpdateRequestControlConfigInput struct {
	Enabled                  *bool                         `json:"enabled"`
	RequestSnapshotEnabled   *bool                         `json:"request_snapshot_enabled"`
	BlockOpenAIChat          *bool                         `json:"block_openai_chat"`
	BlockClaudeMessages      *bool                         `json:"block_claude_messages"`
	BlockOpenAIResponses     *bool                         `json:"block_openai_responses"`
	AllGroups                *bool                         `json:"all_groups"`
	GroupIDs                 *[]int64                      `json:"group_ids"`
	ModelFilter              *ContentModerationModelFilter `json:"model_filter"`
	AllUsers                 *bool                         `json:"all_users"`
	UserRules                *[]RequestControlUserRule     `json:"user_rules"`
	GlobalUserAgentWhitelist *[]string                     `json:"global_user_agent_whitelist"`
	BlockStatus              *int                          `json:"block_status"`
	BlockMessage             *string                       `json:"block_message"`
	EmailOnHit               *bool                         `json:"email_on_hit"`
	AutoBanEnabled           *bool                         `json:"auto_ban_enabled"`
	BanThreshold             *int                          `json:"ban_threshold"`
	ViolationWindowHours     *int                          `json:"violation_window_hours"`
}

type RequestControlCheckInput struct {
	RequestID         string
	RequestMethod     string
	RequestHost       string
	RequestPath       string
	RequestQuery      string
	ClientIP          string
	RemoteAddr        string
	ContentLength     int64
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
	MetadataHeaders   http.Header
	UserAgent         string
	Originator        string
	TLSFingerprint    string
	WebSocket         bool
	ClaudeCodeValid   *bool
}

type RequestControlDecision struct {
	Allowed            bool              `json:"allowed"`
	Blocked            bool              `json:"blocked"`
	Observed           bool              `json:"observed"`
	Action             string            `json:"action"`
	Reason             string            `json:"reason"`
	Message            string            `json:"message"`
	StatusCode         int               `json:"status_code"`
	ClientKind         string            `json:"client_kind"`
	HeaderMatched      bool              `json:"header_matched"`
	BodyMatched        bool              `json:"body_matched"`
	TLSMatched         *bool             `json:"tls_matched,omitempty"`
	Details            map[string]string `json:"details,omitempty"`
	ExpectedAction     string            `json:"expected_action,omitempty"`
	ExpectedReason     string            `json:"expected_reason,omitempty"`
	ExpectedBlocked    bool              `json:"expected_blocked,omitempty"`
	ExpectedStatusCode int               `json:"expected_status_code,omitempty"`
}

type RequestControlLog struct {
	ID                  int64                         `json:"id"`
	RequestID           string                        `json:"request_id"`
	UserID              *int64                        `json:"user_id"`
	UserEmail           string                        `json:"user_email"`
	APIKeyID            *int64                        `json:"api_key_id"`
	APIKeyName          string                        `json:"api_key_name"`
	GroupID             *int64                        `json:"group_id"`
	GroupName           string                        `json:"group_name"`
	Endpoint            string                        `json:"endpoint"`
	Provider            string                        `json:"provider"`
	Protocol            string                        `json:"protocol"`
	Model               string                        `json:"model"`
	Action              string                        `json:"action"`
	Reason              string                        `json:"reason"`
	Allowed             bool                          `json:"allowed"`
	Blocked             bool                          `json:"blocked"`
	Observed            bool                          `json:"observed"`
	ClientKind          string                        `json:"client_kind"`
	UserAgent           string                        `json:"user_agent"`
	Originator          string                        `json:"originator"`
	TLSFingerprint      string                        `json:"tls_fingerprint"`
	TLSMatch            *bool                         `json:"tls_match"`
	HeaderMatch         *bool                         `json:"header_match"`
	BodyMatch           *bool                         `json:"body_match"`
	Details             map[string]string             `json:"details"`
	ExpectedAction      string                        `json:"expected_action"`
	ExpectedReason      string                        `json:"expected_reason"`
	ExpectedBlocked     bool                          `json:"expected_blocked"`
	ExpectedStatusCode  int                           `json:"expected_status_code"`
	RequestHeadersHash  string                        `json:"-"`
	RequestBodyHash     string                        `json:"-"`
	EventAt             time.Time                     `json:"-"`
	ViolationCount      int                           `json:"violation_count"`
	Counted             bool                          `json:"counted_violation"`
	EmailSent           bool                          `json:"email_sent"`
	HitEmailSent        bool                          `json:"hit_email_sent"`
	BanEmailSent        bool                          `json:"ban_email_sent"`
	AutoBanned          bool                          `json:"auto_banned"`
	CreatedAt           time.Time                     `json:"created_at"`
	EventCount          int64                         `json:"event_count"`
	FirstSeenAt         time.Time                     `json:"first_seen_at"`
	LastSeenAt          time.Time                     `json:"last_seen_at"`
	RequestHeaders      map[string]string             `json:"-"`
	RequestBodyMetadata map[string]any                `json:"-"`
	RequestSnapshot     RequestControlRequestSnapshot `json:"-"`
}

type RequestControlLogDetail struct {
	RequestControlLog
	RequestHeaders      map[string]string             `json:"request_headers"`
	RequestBodyMetadata map[string]any                `json:"request_body_metadata"`
	RequestSnapshot     RequestControlRequestSnapshot `json:"request_snapshot"`
}

var ErrRequestControlLogNotFound = infraerrors.NotFound("REQUEST_CONTROL_LOG_NOT_FOUND", "request control log not found")

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
	Enabled                bool  `json:"enabled"`
	RiskControlEnabled     bool  `json:"risk_control_enabled"`
	RequestSnapshotEnabled bool  `json:"request_snapshot_enabled"`
	QueueSize              int   `json:"queue_size"`
	QueueLength            int   `json:"queue_length"`
	QueueBytes             int64 `json:"queue_bytes"`
	QueueMaxBytes          int64 `json:"queue_max_bytes"`
	Enqueued               int64 `json:"enqueued"`
	Processed              int64 `json:"processed"`
	Dropped                int64 `json:"dropped"`
	Errors                 int64 `json:"errors"`
}

type RequestControlRepository interface {
	CreateLog(context.Context, *RequestControlLog) error
	GetViolationState(context.Context, int64, time.Time) (int, *time.Time, error)
	UpdateLogSideEffects(context.Context, *RequestControlLog) error
	ListLogs(context.Context, RequestControlLogFilter) ([]RequestControlLog, *pagination.PaginationResult, error)
	GetLog(context.Context, int64) (*RequestControlLogDetail, error)
	CleanupLogs(context.Context, time.Time) (int64, error)
}

// RequestControlViolationStateRepository keeps rolling hit timestamps
// independent of audit-row cardinality. Implementations may be
// detected at runtime so lightweight test repositories and older adapters
// retain the legacy fallback behavior.
type RequestControlViolationStateRepository interface {
	RecordViolation(context.Context, int64, time.Time, time.Duration, time.Duration) (int, bool, error)
}

type RequestControlSnapshotRepository interface {
	CleanupSnapshots(context.Context, time.Time) (int64, error)
}

type requestControlRuntimeConfig struct {
	globalEnabled bool
	config        *RequestControlConfig
	userRules     map[int64]RequestControlUserRule
	groupIDs      map[int64]struct{}
	models        map[string]struct{}
}

type requestControlTask struct {
	log   *RequestControlLog
	bytes int64
}

type RequestControlService struct {
	settingRepo          SettingRepository
	repo                 RequestControlRepository
	groupRepo            GroupRepository
	userRepo             UserRepository
	authCacheInvalidator APIKeyAuthCacheInvalidator
	emailService         *EmailService
	runtime              atomic.Pointer[requestControlRuntimeConfig]
	refreshMu            sync.Mutex
	queue                chan requestControlTask
	queueBytes           atomic.Int64
	validator            *ClaudeCodeValidator
	violationMu          sync.Mutex
	enqueued             atomic.Int64
	processed            atomic.Int64
	dropped              atomic.Int64
	errors               atomic.Int64
}

func NewRequestControlService(settingRepo SettingRepository, repo RequestControlRepository, groupRepo GroupRepository) *RequestControlService {
	return newRequestControlService(settingRepo, repo, groupRepo, nil, nil, nil)
}

func newRequestControlService(settingRepo SettingRepository, repo RequestControlRepository, groupRepo GroupRepository, userRepo UserRepository, authCacheInvalidator APIKeyAuthCacheInvalidator, emailService *EmailService) *RequestControlService {
	svc := &RequestControlService{
		settingRepo:          settingRepo,
		repo:                 repo,
		groupRepo:            groupRepo,
		userRepo:             userRepo,
		authCacheInvalidator: authCacheInvalidator,
		emailService:         emailService,
		queue:                make(chan requestControlTask, requestControlQueueSize),
		validator:            NewClaudeCodeValidator(),
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
	if input.RequestSnapshotEnabled != nil {
		cfg.RequestSnapshotEnabled = *input.RequestSnapshotEnabled
	}
	if input.BlockOpenAIChat != nil {
		cfg.BlockOpenAIChat = *input.BlockOpenAIChat
	}
	if input.BlockClaudeMessages != nil {
		cfg.BlockClaudeMessages = *input.BlockClaudeMessages
	}
	if input.BlockOpenAIResponses != nil {
		cfg.BlockOpenAIResponses = *input.BlockOpenAIResponses
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
	if input.EmailOnHit != nil {
		cfg.EmailOnHit = *input.EmailOnHit
	}
	if input.AutoBanEnabled != nil {
		cfg.AutoBanEnabled = *input.AutoBanEnabled
	}
	if input.BanThreshold != nil {
		cfg.BanThreshold = *input.BanThreshold
	}
	if input.ViolationWindowHours != nil {
		cfg.ViolationWindowHours = *input.ViolationWindowHours
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
	var responseBody *requestControlResponsesBody
	var responseDiagnostics map[string]string
	if input.Protocol == RequestControlProtocolResponse {
		inspection := inspectRequestControlResponseSessionDetails(input)
		responseDiagnostics = requestControlResponseDiagnosticDetails(input, inspection.SessionPresent, inspection.SessionSource, inspection.Body, inspection.BodyParsed, inspection.BodyErr)
		requestKind := responseDiagnostics["request_kind"]
		if requestControlResponseRequestKindIsExplicitCompaction(requestKind) {
			decision := &RequestControlDecision{
				Allowed:    true,
				Observed:   true,
				Action:     RequestControlActionObserve,
				Reason:     requestControlReasonCompactionAllowed,
				ClientKind: "compaction",
				Details:    responseDiagnostics,
			}
			return s.finalizeDecision(input, cfg, decision), nil
		}
		// A local-summary wire shape is not proof of compaction. Treat it as a
		// synthetic session signal (the gateway injects the same routing session
		// downstream), then continue through the normal UA/signature policy.
		if !inspection.SessionPresent && requestControlResponseRequestKindIsHeuristicCompaction(requestKind) {
			inspection.SessionPresent = true
			responseDiagnostics["client_session"] = "synthetic"
			responseDiagnostics["session_source"] = "gateway:compaction_derived"
		}
		if !inspection.SessionPresent {
			decision := &RequestControlDecision{
				Blocked:    true,
				Observed:   true,
				Action:     RequestControlActionBlock,
				Reason:     "anonymous_response_request",
				Message:    cfg.BlockMessage,
				StatusCode: cfg.BlockStatus,
				ClientKind: "anonymous_response",
				Details:    responseDiagnostics,
			}
			if inspection.BodyErr != nil {
				decision.Details["request_body"] = "invalid_or_unreadable"
			}
			return s.finalizeDecision(input, cfg, decision), nil
		}
		if inspection.BodyParsed {
			responseBody = &inspection.Body
		}
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
			var headerMatched, bodyMatched bool
			var details map[string]string
			if responseBody != nil {
				headerMatched, bodyMatched, details = validateCodexResponsesRequestParsed(input, *responseBody, nil)
			} else {
				headerMatched, bodyMatched, details = validateCodexResponsesRequest(input)
			}
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
	mergeRequestControlDetails(decision, responseDiagnostics)
	return s.finalizeDecision(input, cfg, decision), nil
}

const requestControlReasonObserveOnly = "blocking_disabled_observe_only"

const requestControlReasonCompactionAllowed = "compaction_request_allowed"

func (s *RequestControlService) finalizeDecision(input RequestControlCheckInput, cfg *RequestControlConfig, decision *RequestControlDecision) *RequestControlDecision {
	if decision == nil {
		return nil
	}
	if decision.ExpectedAction == "" {
		decision.ExpectedAction = decision.Action
		decision.ExpectedReason = decision.Reason
		decision.ExpectedBlocked = decision.Blocked
		if decision.Blocked {
			decision.ExpectedStatusCode = decision.StatusCode
		}
	}
	if decision.Blocked && decision.Reason != "anonymous_response_request" && !cfg.blocksProtocol(input.Protocol) {
		decision.Allowed = true
		decision.Blocked = false
		decision.Observed = true
		decision.Action = RequestControlActionObserve
		decision.Reason = requestControlReasonObserveOnly
	}
	attachRequestControlTLSObservation(input, decision)
	if decision.Blocked || decision.Observed {
		s.enqueueLog(buildRequestControlLog(input, decision, cfg.RequestSnapshotEnabled))
	}
	return decision
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

func (s *RequestControlService) GetLog(ctx context.Context, id int64) (*RequestControlLogDetail, error) {
	if s == nil || s.repo == nil || id <= 0 {
		return nil, ErrRequestControlLogNotFound
	}
	return s.repo.GetLog(ctx, id)
}

func (s *RequestControlService) GetStatus() RequestControlRuntimeStatus {
	if s == nil {
		return RequestControlRuntimeStatus{}
	}
	status := RequestControlRuntimeStatus{
		QueueSize:     cap(s.queue),
		QueueLength:   len(s.queue),
		QueueBytes:    s.queueBytes.Load(),
		QueueMaxBytes: requestControlSnapshotQueueBytes,
		Enqueued:      s.enqueued.Load(),
		Processed:     s.processed.Load(),
		Dropped:       s.dropped.Load(),
		Errors:        s.errors.Load(),
	}
	if current := s.runtime.Load(); current != nil && current.config != nil {
		status.Enabled = current.config.Enabled
		status.RiskControlEnabled = current.globalEnabled
		status.RequestSnapshotEnabled = current.config.RequestSnapshotEnabled
	}
	return status
}

func (s *RequestControlService) enqueueLog(log *RequestControlLog) {
	if s == nil || log == nil || s.repo == nil {
		return
	}
	snapshotBytes := requestControlSnapshotApproxBytes(log)
	if snapshotBytes > 0 && !s.reserveRequestControlSnapshotBytes(snapshotBytes) {
		omitRequestControlSnapshot(log, "omitted_queue_memory_budget")
		slog.Warn("request_control.snapshot_omitted_queue_bytes", "user_id", log.UserID, "protocol", log.Protocol, "bytes", snapshotBytes, "queue_bytes", s.queueBytes.Load())
		snapshotBytes = 0
	}
	select {
	case s.queue <- requestControlTask{log: log, bytes: snapshotBytes}:
		s.enqueued.Add(1)
	default:
		s.queueBytes.Add(-snapshotBytes)
		s.dropped.Add(1)
		slog.Warn("request_control.log_queue_full", "user_id", log.UserID, "protocol", log.Protocol, "reason", log.Reason)
	}
}

func (s *RequestControlService) worker() {
	for task := range s.queue {
		if task.log == nil {
			s.queueBytes.Add(-task.bytes)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), requestControlWorkerTimeout)
		if err := s.processQueuedLog(ctx, task.log); err != nil {
			s.errors.Add(1)
			slog.Warn("request_control.log_persist_failed", "error", err)
		} else {
			s.processed.Add(1)
		}
		cancel()
		s.queueBytes.Add(-task.bytes)
	}
}

func (s *RequestControlService) reserveRequestControlSnapshotBytes(bytes int64) bool {
	if s == nil || bytes <= 0 || bytes > requestControlSnapshotQueueBytes {
		return false
	}
	for {
		current := s.queueBytes.Load()
		if current > requestControlSnapshotQueueBytes-bytes {
			return false
		}
		if s.queueBytes.CompareAndSwap(current, current+bytes) {
			return true
		}
	}
}

func requestControlSnapshotApproxBytes(log *RequestControlLog) int64 {
	if log == nil || !log.RequestSnapshot.Available {
		return 0
	}
	// The fixed allowance covers bounded metadata/maps; the two potentially
	// large values (body and multi-value headers) are counted explicitly.
	size := int64(64 * 1024)
	size += int64(len(log.RequestSnapshot.Body) + len(log.UserAgent) + len(log.RequestID))
	for key, values := range log.RequestSnapshot.Headers {
		size += int64(len(key))
		for _, value := range values {
			size += int64(len(value))
		}
	}
	return size
}

func omitRequestControlSnapshot(log *RequestControlLog, reason string) {
	if log == nil {
		return
	}
	log.RequestSnapshot = RequestControlRequestSnapshot{}
	if log.Details == nil {
		log.Details = make(map[string]string)
	}
	log.Details["request_snapshot"] = truncateRequestControlValue(reason, 128)
}

func (s *RequestControlService) processQueuedLog(ctx context.Context, log *RequestControlLog) error {
	if s == nil || s.repo == nil || log == nil {
		return nil
	}
	if log.Blocked && log.UserID != nil && *log.UserID > 0 {
		cfg := s.currentConfig()
		_, hasStateRepo := s.repo.(RequestControlViolationStateRepository)
		var err error
		if hasStateRepo {
			// The repository's state row lock serializes the rolling counter per
			// user, so different users can persist concurrently. The legacy
			// fallback still needs the process mutex around its read/insert pair.
			err = s.repo.CreateLog(ctx, log)
			if err == nil && log.ID > 0 {
				s.prepareViolationCount(ctx, cfg, log)
			}
		} else {
			s.violationMu.Lock()
			s.prepareViolationCount(ctx, cfg, log)
			err = s.repo.CreateLog(ctx, log)
			s.violationMu.Unlock()
		}
		if err != nil {
			return err
		}
		if log.ID <= 0 {
			return nil
		}
		if log.Counted {
			s.applyRequestControlSideEffects(ctx, cfg, log)
		}
		if err := s.repo.UpdateLogSideEffects(ctx, log); err != nil {
			slog.Warn("request_control.update_side_effects_failed", "user_id", *log.UserID, "error", err)
		}
		return nil
	}
	return s.repo.CreateLog(ctx, log)
}

func (s *RequestControlService) currentConfig() *RequestControlConfig {
	if s != nil {
		if current := s.runtime.Load(); current != nil && current.config != nil {
			return current.config
		}
	}
	cfg := defaultRequestControlConfig()
	cfg.normalize()
	return cfg
}

func (s *RequestControlService) prepareViolationCount(ctx context.Context, cfg *RequestControlConfig, log *RequestControlLog) {
	if s == nil || s.repo == nil || cfg == nil || log == nil || log.UserID == nil || *log.UserID <= 0 {
		return
	}
	eventAt := log.EventAt
	if eventAt.IsZero() {
		eventAt = log.CreatedAt
	}
	if eventAt.IsZero() {
		eventAt = time.Now()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = eventAt
	}
	if stateRepo, ok := s.repo.(RequestControlViolationStateRepository); ok {
		count, counted, err := stateRepo.RecordViolation(ctx, *log.UserID, eventAt, time.Duration(cfg.ViolationWindowHours)*time.Hour, requestControlHitSpacing)
		if err == nil {
			log.Counted = counted
			log.ViolationCount = count
			return
		}
		slog.Warn("request_control.violation_state_failed", "user_id", *log.UserID, "error", err)
		return
	}
	log.Counted = false
	log.ViolationCount = 0
	since := eventAt.Add(-time.Duration(cfg.ViolationWindowHours) * time.Hour)
	count, last, err := s.repo.GetViolationState(ctx, *log.UserID, since)
	if err != nil {
		slog.Warn("request_control.violation_state_failed", "user_id", *log.UserID, "error", err)
		return
	}
	log.ViolationCount = count
	if last == nil || eventAt.Sub(*last) > requestControlHitSpacing {
		log.Counted = true
		log.ViolationCount = count + 1
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
		if snapshotRepo, ok := s.repo.(RequestControlSnapshotRepository); ok {
			if cleared, err := snapshotRepo.CleanupSnapshots(ctx, time.Now().Add(-requestControlSnapshotRetentionDays*24*time.Hour)); err != nil {
				slog.Warn("request_control.snapshot_cleanup_failed", "error", err)
			} else if cleared > 0 {
				slog.Info("request_control.snapshot_cleanup_completed", "cleared", cleared)
			}
		}
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
	if cfg.BanThreshold < 1 || cfg.BanThreshold > requestControlMaxBanThreshold {
		return infraerrors.BadRequest("INVALID_REQUEST_CONTROL_BAN_THRESHOLD", "封禁触发次数必须在 1-1000 之间")
	}
	if cfg.ViolationWindowHours < 1 || cfg.ViolationWindowHours > requestControlMaxViolationWindowHours {
		return infraerrors.BadRequest("INVALID_REQUEST_CONTROL_VIOLATION_WINDOW", "累计窗口必须在 1-720 小时之间")
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
	return &RequestControlConfig{
		BlockOpenAIChat:            true,
		BlockClaudeMessages:        true,
		BlockOpenAIResponses:       true,
		AllGroups:                  true,
		ModelFilter:                ContentModerationModelFilter{Type: ContentModerationModelFilterAll},
		AllUsers:                   true,
		UserRules:                  []RequestControlUserRule{},
		GlobalUserAgentWhitelist:   []string{},
		BlockStatus:                requestControlDefaultBlockStatus,
		BlockMessage:               requestControlDefaultBlockMessage,
		EmailOnHit:                 true,
		AutoBanEnabled:             true,
		BanThreshold:               requestControlDefaultBanThreshold,
		ViolationWindowHours:       requestControlDefaultViolationWindowHours,
		protocolSwitchesConfigured: true,
	}
}

func (cfg *RequestControlConfig) normalize() {
	if cfg == nil {
		return
	}
	if !cfg.protocolSwitchesConfigured && !cfg.BlockOpenAIChat && !cfg.BlockClaudeMessages && !cfg.BlockOpenAIResponses {
		cfg.BlockOpenAIChat = true
		cfg.BlockClaudeMessages = true
		cfg.BlockOpenAIResponses = true
	}
	cfg.protocolSwitchesConfigured = true
	cfg.GroupIDs = normalizeInt64IDs(cfg.GroupIDs)
	cfg.ModelFilter = normalizeContentModerationModelFilter(cfg.ModelFilter)
	if cfg.BlockStatus <= 0 {
		cfg.BlockStatus = requestControlDefaultBlockStatus
	}
	if strings.TrimSpace(cfg.BlockMessage) == "" {
		cfg.BlockMessage = requestControlDefaultBlockMessage
	}
	cfg.BlockMessage = strings.TrimSpace(cfg.BlockMessage)
	if cfg.BanThreshold <= 0 {
		cfg.BanThreshold = requestControlDefaultBanThreshold
	}
	if cfg.BanThreshold > requestControlMaxBanThreshold {
		cfg.BanThreshold = requestControlMaxBanThreshold
	}
	if cfg.ViolationWindowHours <= 0 {
		cfg.ViolationWindowHours = requestControlDefaultViolationWindowHours
	}
	if cfg.ViolationWindowHours > requestControlMaxViolationWindowHours {
		cfg.ViolationWindowHours = requestControlMaxViolationWindowHours
	}
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
	return &RequestControlConfigView{Enabled: cfg.Enabled, RequestSnapshotEnabled: cfg.RequestSnapshotEnabled, BlockOpenAIChat: cfg.BlockOpenAIChat, BlockClaudeMessages: cfg.BlockClaudeMessages, BlockOpenAIResponses: cfg.BlockOpenAIResponses, AllGroups: cfg.AllGroups, GroupIDs: append([]int64(nil), cfg.GroupIDs...), ModelFilter: cfg.ModelFilter, AllUsers: cfg.AllUsers, UserRules: append([]RequestControlUserRule(nil), cfg.UserRules...), GlobalUserAgentWhitelist: append([]string(nil), cfg.GlobalUserAgentWhitelist...), BlockStatus: cfg.BlockStatus, BlockMessage: cfg.BlockMessage, EmailOnHit: cfg.EmailOnHit, AutoBanEnabled: cfg.AutoBanEnabled, BanThreshold: cfg.BanThreshold, ViolationWindowHours: cfg.ViolationWindowHours}
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

func (cfg *RequestControlConfig) blocksProtocol(protocol string) bool {
	if cfg == nil {
		return true
	}
	switch protocol {
	case RequestControlProtocolChat:
		return cfg.BlockOpenAIChat
	case RequestControlProtocolMessages:
		return cfg.BlockClaudeMessages
	case RequestControlProtocolResponse:
		return cfg.BlockOpenAIResponses
	default:
		return true
	}
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
	Present        bool
	Valid          bool
	HasSession     bool
	InstallationID requestControlJSONString
	WindowID       requestControlJSONString
	SessionID      requestControlJSONString
	ThreadID       requestControlJSONString
	TurnMetadata   requestControlJSONString
}

func parseRequestControlTurnMetadataValue(raw []byte) (requestControlJSONString, error) {
	value := requestControlJSONString{Present: true}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return value, nil
	}
	if trimmed[0] == '"' {
		parsed, err := parseRequestControlJSONString(trimmed, 0)
		if err != nil {
			return value, nil
		}
		parsed.Valid = false
		if _, ok := parseRequestControlTurnMetadata(parsed.Value); ok {
			parsed.Valid = true
		}
		return parsed, nil
	}
	return value, nil
}

func parseRequestControlClientMetadata(raw []byte) (requestControlClientMetadata, error) {
	metadata := requestControlClientMetadata{Present: true}
	err := openaiwsv2.VisitTopLevelObjectFields(raw, func(key, rawValue []byte) error {
		var err error
		switch string(key) {
		case "x-codex-installation-id", "installation_id":
			if metadata.InstallationID.Present {
				return errors.New("duplicate installation id metadata")
			}
			metadata.InstallationID, err = parseRequestControlJSONString(rawValue, openaiwsv2.ClientEnvelopeMaxRouteIDBytes)
		case "x-codex-window-id", "window_id":
			if metadata.WindowID.Present {
				return errors.New("duplicate window id metadata")
			}
			metadata.WindowID, err = parseRequestControlJSONString(rawValue, openaiwsv2.ClientEnvelopeMaxRouteIDBytes)
		case "session_id":
			metadata.SessionID, err = parseRequestControlJSONString(rawValue, openaiwsv2.ClientEnvelopeMaxRouteIDBytes)
		case "thread_id":
			metadata.ThreadID, err = parseRequestControlJSONString(rawValue, openaiwsv2.ClientEnvelopeMaxRouteIDBytes)
		case "sessionId", "conversation_id", "conversationId":
			value, parseErr := parseRequestControlJSONString(rawValue, openaiwsv2.ClientEnvelopeMaxRouteIDBytes)
			err = parseErr
			if err == nil && value.Valid && strings.TrimSpace(value.Value) != "" {
				metadata.HasSession = true
			}
		case "x-codex-turn-metadata":
			metadata.TurnMetadata, err = parseRequestControlTurnMetadataValue(rawValue)
			if err == nil && metadata.TurnMetadata.Valid {
				metadata.HasSession, err = requestControlJSONObjectHasSession([]byte(metadata.TurnMetadata.Value))
			}
		}
		if err == nil && (metadata.SessionID.Valid && strings.TrimSpace(metadata.SessionID.Value) != "" || metadata.ThreadID.Valid && strings.TrimSpace(metadata.ThreadID.Value) != "") {
			metadata.HasSession = true
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
	SessionPresent    bool
	SessionSource     string
	MaxOutputTokens   bool
	ToolsPresent      bool
}

type requestControlResponseSessionInspection struct {
	Body           requestControlResponsesBody
	BodyParsed     bool
	SessionPresent bool
	SessionSource  string
	BodyErr        error
}

var requestControlBodySessionSourcePriority = map[string]int{
	"body:prompt_cache_key":  1,
	"body:session_id":        2,
	"body:thread_id":         3,
	"body:sessionId":         4,
	"body:conversation_id":   5,
	"body:conversationId":    6,
	"body:metadata":          7,
	"body:conversationState": 8,
	"body:client_metadata":   9,
}

func requestControlPreferBodySessionSource(current, candidate string) string {
	if current == "" {
		return candidate
	}
	currentPriority, currentOK := requestControlBodySessionSourcePriority[current]
	candidatePriority, candidateOK := requestControlBodySessionSourcePriority[candidate]
	if candidateOK && (!currentOK || candidatePriority < currentPriority) {
		return candidate
	}
	return current
}

func requestControlJSONObjectHasSession(raw []byte) (bool, error) {
	found := false
	err := openaiwsv2.VisitTopLevelObjectFields(raw, func(key, rawValue []byte) error {
		switch string(key) {
		case "session_id", "thread_id", "sessionId", "conversation_id", "conversationId":
			value, err := parseRequestControlJSONString(rawValue, openaiwsv2.ClientEnvelopeMaxRouteIDBytes)
			if err != nil {
				return err
			}
			if value.Valid && strings.TrimSpace(value.Value) != "" {
				found = true
			}
		}
		return nil
	})
	return found, err
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
		case "max_output_tokens":
			body.MaxOutputTokens = true
		case "tools":
			body.ToolsPresent = true
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
			if err == nil && body.PromptCacheKey.Valid && strings.TrimSpace(body.PromptCacheKey.Value) != "" {
				body.SessionPresent = true
				body.SessionSource = requestControlPreferBodySessionSource(body.SessionSource, "body:prompt_cache_key")
			}
		case "session_id", "thread_id", "sessionId", "conversation_id", "conversationId":
			value, parseErr := parseRequestControlJSONString(rawValue, openaiwsv2.ClientEnvelopeMaxRouteIDBytes)
			err = parseErr
			if err == nil && value.Valid && strings.TrimSpace(value.Value) != "" {
				body.SessionPresent = true
				body.SessionSource = requestControlPreferBodySessionSource(body.SessionSource, "body:"+string(key))
			}
		case "metadata", "conversationState":
			var nestedSession bool
			nestedSession, err = requestControlJSONObjectHasSession(rawValue)
			if err == nil && nestedSession {
				body.SessionPresent = true
				body.SessionSource = requestControlPreferBodySessionSource(body.SessionSource, "body:"+string(key))
			}
		case "type":
			body.Type, err = parseRequestControlJSONString(rawValue, openaiwsv2.ClientEnvelopeMaxEventTypeBytes)
		case "client_metadata":
			body.ClientMetadata, err = parseRequestControlClientMetadata(rawValue)
			if err == nil && body.ClientMetadata.HasSession {
				body.SessionPresent = true
				body.SessionSource = requestControlPreferBodySessionSource(body.SessionSource, "body:client_metadata")
			}
		}
		return err
	})
	return body, err
}

var requestControlResponseSessionHeaders = []string{
	"x-aether-session-id",
	"session_id",
	"conversation_id",
	"session-id",
	"thread-id",
	"x-claude-code-session-id",
	"x-opencode-session-id",
}

func requestControlResponseSessionHeaderSource(headers http.Header) string {
	for _, name := range requestControlResponseSessionHeaders {
		if requestControlHeaderHasValue(headers, name) {
			return "header:" + strings.ToLower(name)
		}
	}
	return ""
}

func requestControlHeaderHasValue(headers http.Header, name string) bool {
	for key, candidates := range headers {
		if strings.EqualFold(key, name) {
			for _, value := range candidates {
				if strings.TrimSpace(value) != "" {
					return true
				}
			}
		}
	}
	return false
}

// inspectRequestControlResponseSession mirrors Aether's anonymous-avoidance
// rule: a request is non-anonymous when it carries a non-empty client session
// signal in supported headers or the request body. The parsed body is returned
// when it had to be scanned so Codex validation can reuse the same pass.
func inspectRequestControlResponseSessionDetails(input RequestControlCheckInput) requestControlResponseSessionInspection {
	if source := requestControlResponseSessionHeaderSource(input.Headers); source != "" {
		return requestControlResponseSessionInspection{SessionPresent: true, SessionSource: source}
	}
	for key, values := range input.Headers {
		if !strings.EqualFold(key, "x-codex-turn-metadata") {
			continue
		}
		for _, raw := range values {
			found, err := requestControlJSONObjectHasSession([]byte(raw))
			if err == nil && found {
				return requestControlResponseSessionInspection{SessionPresent: true, SessionSource: "header:x-codex-turn-metadata"}
			}
		}
	}
	body, err := parseRequestControlResponsesBody(input.Body)
	return requestControlResponseSessionInspection{
		Body:           body,
		BodyParsed:     true,
		SessionPresent: err == nil && body.SessionPresent,
		SessionSource:  body.SessionSource,
		BodyErr:        err,
	}
}

const requestControlLocalCompactionMinBodyBytes = 32 * 1024

// requestControlResponseRequestKind records explicit compact signals and a
// bounded, client-agnostic heuristic for local agent summarization. Explicit
// compact signals and strong local-summary shapes are exempt from request
// control blocking; the bounded request snapshot is persisted separately.
func requestControlResponseRequestKind(input RequestControlCheckInput, parsed requestControlResponsesBody, bodyParsed bool, bodyErr error) (string, string, []string) {
	endpoint := strings.ToLower(strings.TrimRight(strings.TrimSpace(input.Endpoint), "/"))
	if strings.HasSuffix(endpoint, "/responses/compact") {
		return "openai_responses_compact_endpoint", "explicit", []string{"endpoint:/responses/compact"}
	}
	if HasCompactionTriggerInInput(input.Body) {
		return "openai_responses_compaction_trigger", "explicit", []string{"input.type:compaction_trigger"}
	}
	for _, raw := range requestControlHeaderValues(input.Headers, "x-codex-turn-metadata") {
		if metadata, ok := parseRequestControlTurnMetadata(strings.TrimSpace(raw)); ok && metadata.RequestKind == "compaction" {
			return "codex_compaction_request", "explicit", []string{"x-codex-turn-metadata.request_kind:compaction"}
		}
	}
	if !bodyParsed {
		parsed, bodyErr = parseRequestControlResponsesBody(input.Body)
	}
	if bodyErr != nil {
		return "openai_responses_standard_or_unknown", "unknown", []string{"body:invalid_or_unreadable"}
	}
	if parsed.ClientMetadata.TurnMetadata.Valid {
		if metadata, ok := parseRequestControlTurnMetadata(parsed.ClientMetadata.TurnMetadata.Value); ok && metadata.RequestKind == "compaction" {
			return "codex_compaction_request", "explicit", []string{"client_metadata.request_kind:compaction"}
		}
	}
	localSummaryBase := parsed.Input.Present && parsed.Input.Kind == '[' &&
		parsed.Store.Present && parsed.Store.Valid && !parsed.Store.Value &&
		parsed.Stream.Present && parsed.Stream.Valid && parsed.Stream.Value &&
		parsed.MaxOutputTokens && !parsed.ToolsPresent && !parsed.PromptCacheKey.Present
	toolChoiceNone := parsed.ToolChoice.Present && parsed.ToolChoice.Valid && strings.EqualFold(strings.TrimSpace(parsed.ToolChoice.Value), "none")
	largeRequestWithoutToolChoice := !parsed.ToolChoice.Present && len(input.Body) >= requestControlLocalCompactionMinBodyBytes
	if localSummaryBase && (toolChoiceNone || largeRequestWithoutToolChoice) {
		evidence := []string{
			"input:array",
			"store:false",
			"stream:true",
			"max_output_tokens:present",
			"tools:missing",
			"prompt_cache_key:missing",
		}
		if toolChoiceNone {
			evidence = append(evidence, "tool_choice:none")
		} else {
			evidence = append(evidence, "tool_choice:missing", "body_bytes:large")
		}
		return "local_compaction_candidate", "strong_heuristic", evidence
	}
	return "openai_responses_standard_or_unknown", "default", nil
}

func requestControlResponseRequestKindIsCompaction(kind string) bool {
	return requestControlResponseRequestKindIsExplicitCompaction(kind) || requestControlResponseRequestKindIsHeuristicCompaction(kind)
}

func requestControlResponseRequestKindIsExplicitCompaction(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "openai_responses_compact_endpoint", "openai_responses_compaction_trigger", "codex_compaction_request":
		return true
	default:
		return false
	}
}

func requestControlResponseRequestKindIsHeuristicCompaction(kind string) bool {
	return strings.TrimSpace(kind) == "local_compaction_candidate"
}

// IsOpenAIResponsesCompactionRequest is shared by request control and the
// gateway's downstream session propagation. It intentionally does not depend
// on User-Agent: OpenAI/JS is an SDK identifier used by multiple agents.
func IsOpenAIResponsesCompactionRequest(input RequestControlCheckInput) bool {
	if input.Protocol != "" && input.Protocol != RequestControlProtocolResponse {
		return false
	}
	inspection := inspectRequestControlResponseSessionDetails(input)
	kind, _, _ := requestControlResponseRequestKind(input, inspection.Body, inspection.BodyParsed, inspection.BodyErr)
	return requestControlResponseRequestKindIsCompaction(kind)
}

func requestControlResponseDiagnosticDetails(input RequestControlCheckInput, sessionPresent bool, sessionSource string, parsed requestControlResponsesBody, bodyParsed bool, bodyErr error) map[string]string {
	details := map[string]string{
		// Keep client_session for API/UI compatibility. session_source is the
		// actionable diagnostic that was previously absent from anonymous logs.
		"client_session": "missing",
		"session_source": "none",
	}
	if sessionPresent {
		details["client_session"] = "present"
		if strings.TrimSpace(sessionSource) != "" {
			details["session_source"] = sessionSource
		}
	}
	kind, confidence, evidence := requestControlResponseRequestKind(input, parsed, bodyParsed, bodyErr)
	details["request_kind"] = kind
	details["request_kind_confidence"] = confidence
	if len(evidence) > 0 {
		details["request_kind_evidence"] = strings.Join(evidence, ",")
	}
	return details
}

func mergeRequestControlDetails(target *RequestControlDecision, extra map[string]string) {
	if target == nil {
		return
	}
	if target.Details == nil {
		target.Details = make(map[string]string)
	}
	for key, value := range extra {
		target.Details[key] = value
	}
}

// validateCodexResponsesRequest mirrors the stable request contract emitted by
// Codex core/client.rs and login/auth/default_client.rs. Compact and WebSocket
// requests intentionally have different transport fields.
func validateCodexResponsesRequest(input RequestControlCheckInput) (bool, bool, map[string]string) {
	body, err := parseRequestControlResponsesBody(input.Body)
	return validateCodexResponsesRequestParsed(input, body, err)
}

func validateCodexResponsesRequestParsed(input RequestControlCheckInput, body requestControlResponsesBody, bodyErr error) (bool, bool, map[string]string) {
	if isCodexDesktopRequest(input) {
		return validateCodexDesktopResponsesRequestParsed(input, body, bodyErr)
	}
	return validateCodexResponsesRequestParsedStrict(input, body, bodyErr)
}

func validateCodexResponsesRequestParsedStrict(input RequestControlCheckInput, body requestControlResponsesBody, bodyErr error) (bool, bool, map[string]string) {
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
	installationHeader, installationHeaderSingle := requestControlSingleHeader(input.Headers, "x-codex-installation-id")
	installationHeaderPresent := len(requestControlHeaderValues(input.Headers, "x-codex-installation-id")) > 0
	if installationHeaderPresent && (!installationHeaderSingle || !requestControlUUID(installationHeader)) {
		headerOK = false
		details["installation_id"] = "missing_invalid_or_duplicate"
	}
	if compact {
		// The compact endpoint emits x-codex-installation-id as a header instead
		// of carrying client_metadata in its body. Memory requests intentionally
		// omit identity fields from turn metadata, but still carry the UUID in
		// this header.
		if !installationHeaderSingle || !requestControlUUID(installationHeader) {
			headerOK = false
			details["installation_id"] = "missing_or_invalid"
		} else if turnMetadata.RequestKind != "memory" && installationHeader != turnMetadata.InstallationID {
			headerOK = false
			details["installation_id"] = "turn_metadata_mismatch"
		}
	} else if installationHeaderPresent && installationHeaderSingle && turnMetadata.RequestKind != "memory" && installationHeader != turnMetadata.InstallationID {
		headerOK = false
		details["installation_id"] = "turn_metadata_mismatch"
	}

	bodyOK := bodyErr == nil
	if bodyErr != nil {
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
	if !body.ClientMetadata.InstallationID.Present || !body.ClientMetadata.InstallationID.Valid ||
		!requestControlUUID(body.ClientMetadata.InstallationID.Value) {
		bodyOK = false
		details["body_installation_id"] = "missing_or_invalid"
	} else if turnMetadata.RequestKind != "memory" && body.ClientMetadata.InstallationID.Value != turnMetadata.InstallationID {
		bodyOK = false
		details["body_installation_id"] = "turn_metadata_mismatch"
	} else if installationHeaderPresent && installationHeaderSingle && body.ClientMetadata.InstallationID.Value != installationHeader {
		bodyOK = false
		details["body_installation_id"] = "header_mismatch"
	}
	return headerOK, bodyOK, details
}

func isCodexDesktopRequest(input RequestControlCheckInput) bool {
	userAgent, uaSingle := requestControlSingleHeader(input.Headers, "User-Agent")
	originator, originatorSingle := requestControlSingleHeader(input.Headers, "originator")
	return uaSingle && originatorSingle && codexDesktopUserAgentPattern.MatchString(userAgent) &&
		(strings.EqualFold(originator, "codex desktop") || strings.EqualFold(originator, "codex_work_desktop"))
}

// validateCodexDesktopResponsesRequestParsed accepts the Desktop wire profile,
// whose canonical identity is client_metadata rather than the CLI compatibility
// headers. Body identity and the core Responses contract remain mandatory.
func validateCodexDesktopResponsesRequestParsed(input RequestControlCheckInput, body requestControlResponsesBody, bodyErr error) (bool, bool, map[string]string) {
	details := map[string]string{"codex_profile": "desktop_body_metadata"}
	if bodyErr != nil {
		details["body_json"] = "invalid"
		return false, false, details
	}
	metadata := body.ClientMetadata
	if !metadata.Present || !metadata.Valid {
		details["client_metadata"] = "missing_or_invalid"
		return false, false, details
	}
	if !metadata.InstallationID.Valid || !requestControlUUID(metadata.InstallationID.Value) {
		details["body_installation_id"] = "missing_or_invalid"
		return false, false, details
	}
	if !metadata.SessionID.Valid || !requestControlUUID(metadata.SessionID.Value) || !metadata.ThreadID.Valid || !requestControlUUID(metadata.ThreadID.Value) {
		details["body_session_identity"] = "missing_or_mismatched"
		return false, false, details
	}
	windowID, windowSingle := requestControlSingleHeader(input.Headers, "x-codex-window-id")
	if !metadata.WindowID.Valid || !windowSingle || windowID != metadata.WindowID.Value {
		details["window_identity"] = "missing_or_mismatched"
		return false, false, details
	}
	if !metadata.TurnMetadata.Present {
		details["body_turn_metadata"] = "missing"
		return false, false, details
	}

	if !metadata.TurnMetadata.Valid || strings.TrimSpace(metadata.TurnMetadata.Value) == "" {
		details["body_turn_metadata"] = "missing_or_invalid"
		return false, false, details
	}
	// Derived compatibility headers are validation inputs only. Do not mutate
	// the original request header map, which is also used for the redacted audit
	// metadata and deduplication fingerprint.
	if input.Headers == nil {
		input.Headers = make(http.Header)
	} else {
		input.Headers = input.Headers.Clone()
	}
	canonicalTurnMetadata := metadata.TurnMetadata.Value
	turnHeaderValues := requestControlHeaderValues(input.Headers, "x-codex-turn-metadata")
	switch len(turnHeaderValues) {
	case 0:
		input.Headers.Set("x-codex-turn-metadata", canonicalTurnMetadata)
	case 1:
		current := strings.TrimSpace(turnHeaderValues[0])
		if parsed, ok := parseRequestControlTurnMetadata(current); ok {
			bodyMetadata, bodyOK := parseRequestControlTurnMetadata(canonicalTurnMetadata)
			if !bodyOK || !requestControlTurnMetadataMatches(parsed, bodyMetadata) {
				details["turn_metadata"] = "identity_mismatch"
				return false, false, details
			}
		} else {
			details["turn_metadata"] = "missing_or_invalid"
			return false, false, details
		}
	default:
		details["turn_metadata"] = "duplicate"
		return false, false, details
	}
	requestControlSetHeaderIfMissing(input.Headers, "Accept", "text/event-stream")
	requestControlSetHeaderIfMissing(input.Headers, "session-id", metadata.SessionID.Value)
	requestControlSetHeaderIfMissing(input.Headers, "thread-id", metadata.ThreadID.Value)
	requestControlSetHeaderIfMissing(input.Headers, "x-client-request-id", metadata.ThreadID.Value)
	requestControlSetHeaderIfMissing(input.Headers, "x-codex-installation-id", metadata.InstallationID.Value)

	headerOK, bodyOK, strictDetails := validateCodexResponsesRequestParsedStrict(input, body, nil)
	for key, value := range details {
		strictDetails[key] = value
	}
	return headerOK, bodyOK, strictDetails
}

func requestControlSetHeaderIfMissing(headers http.Header, name, value string) {
	if len(requestControlHeaderValues(headers, name)) == 0 {
		headers.Set(name, value)
	}
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
	values := requestControlHeaderValues(headers, name)
	if len(values) != 1 {
		return "", false
	}
	value := strings.TrimSpace(values[0])
	return value, value != ""
}

func requestControlHeaderValues(headers http.Header, name string) []string {
	if headers == nil {
		return nil
	}
	canonical := http.CanonicalHeaderKey(name)
	values := make([]string, 0, 1)
	for key, candidates := range headers {
		if key == canonical {
			values = append(values, candidates...)
			continue
		}
		if strings.EqualFold(key, name) {
			values = append(values, candidates...)
		}
	}
	return values
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

func attachRequestControlTLSObservation(input RequestControlCheckInput, decision *RequestControlDecision) {
	if decision == nil {
		return
	}
	var matched *bool
	switch input.Protocol {
	case RequestControlProtocolResponse:
		matched = requestControlTLSMatch(input.TLSFingerprint)
	case RequestControlProtocolMessages:
		matched = requestControlClaudeTLSMatch(input.TLSFingerprint)
	default:
		return
	}
	decision.TLSMatched = matched
	if matched == nil || *matched {
		return
	}
	if decision.Details == nil {
		decision.Details = make(map[string]string)
	}
	decision.Details["tls_fingerprint"] = "client_default_tls_mismatch"
	// TLS remains an observation signal only. A request that passed the
	// header/body contract is allowed, but is persisted for backend review.
	if (decision.ClientKind == "codex" || decision.ClientKind == "claude_code") && decision.Allowed && !decision.Blocked {
		decision.Observed = true
		if decision.ClientKind == "claude_code" {
			decision.Reason = "claude_code_tls_fingerprint_mismatch"
		} else {
			decision.Reason = "codex_tls_fingerprint_mismatch"
		}
	}
}

func requestControlTLSMatch(raw string) *bool {
	return requestControlTLSMatchExpected(raw, requestControlCodexDefaultJA3Hash, requestControlCodexDefaultJA3, "")
}

func requestControlClaudeTLSMatch(raw string) *bool {
	return requestControlTLSMatchExpected(raw, requestControlClaudeDefaultJA3Hash, "", requestControlClaudeDefaultJA4)
}

func requestControlTLSMatchExpected(raw, expectedJA3Hash, expectedJA3, expectedJA4 string) *bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	lower := strings.ToLower(raw)
	for _, key := range []string{
		"proxy:x-aether-tls-ja3-hash=",
		"proxy:cf-ja3-hash=",
		"ja3_hash=",
	} {
		if !strings.HasPrefix(lower, key) {
			continue
		}
		if expectedJA3Hash == "" {
			return nil
		}
		value := strings.TrimSpace(raw[len(key):])
		matched := strings.EqualFold(value, expectedJA3Hash)
		return &matched
	}
	for _, key := range []string{
		"proxy:x-aether-tls-ja3=",
		"ja3=",
	} {
		if !strings.HasPrefix(lower, key) {
			continue
		}
		if expectedJA3 == "" {
			return nil
		}
		value := strings.TrimSpace(raw[len(key):])
		matched := value == expectedJA3
		return &matched
	}
	for _, key := range []string{
		"proxy:x-aether-tls-ja4=",
		"ja4=",
	} {
		if !strings.HasPrefix(lower, key) {
			continue
		}
		if expectedJA4 == "" {
			return nil
		}
		value := strings.TrimSpace(raw[len(key):])
		matched := strings.EqualFold(value, expectedJA4)
		return &matched
	}
	return nil
}

func buildRequestControlLog(input RequestControlCheckInput, decision *RequestControlDecision, requestSnapshotEnabled bool) *RequestControlLog {
	eventAt := time.Now()
	log := &RequestControlLog{
		RequestID:          truncateRequestControlValue(input.RequestID, 128),
		UserEmail:          truncateRequestControlValue(input.UserEmail, 255),
		APIKeyName:         truncateRequestControlValue(input.APIKeyName, 100),
		GroupName:          truncateRequestControlValue(input.GroupName, 255),
		Endpoint:           truncateRequestControlValue(input.Endpoint, 128),
		Provider:           truncateRequestControlValue(input.Provider, 64),
		Protocol:           truncateRequestControlValue(input.Protocol, 64),
		Model:              truncateRequestControlValue(input.Model, 255),
		Action:             truncateRequestControlValue(decision.Action, 32),
		Reason:             truncateRequestControlValue(decision.Reason, 128),
		Allowed:            decision.Allowed,
		Blocked:            decision.Blocked,
		Observed:           decision.Observed,
		ClientKind:         truncateRequestControlValue(decision.ClientKind, 64),
		UserAgent:          truncateRequestControlValue(input.UserAgent, 512),
		Originator:         truncateRequestControlValue(input.Originator, 128),
		TLSFingerprint:     truncateRequestControlValue(input.TLSFingerprint, 128),
		Details:            limitRequestControlDetails(decision.Details),
		ExpectedAction:     truncateRequestControlValue(decision.ExpectedAction, 32),
		ExpectedReason:     truncateRequestControlValue(decision.ExpectedReason, 128),
		ExpectedBlocked:    decision.ExpectedBlocked,
		ExpectedStatusCode: decision.ExpectedStatusCode,
		CreatedAt:          eventAt,
		EventAt:            eventAt,
		EventCount:         1,
		FirstSeenAt:        eventAt,
		LastSeenAt:         eventAt,
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
	log.RequestHeaders, log.RequestBodyMetadata = buildRequestControlMetadata(input)
	if requestSnapshotEnabled {
		log.RequestSnapshot = buildRequestControlRequestSnapshot(input)
		log.RequestSnapshot.CapturedAt = eventAt
	} else {
		log.Details["request_snapshot"] = "disabled_by_config"
	}
	log.RequestHeadersHash = requestControlDedupHeaderHash(input)
	log.RequestBodyHash = requestControlDedupBodyHash(input, log.RequestBodyMetadata)
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
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(out) >= requestControlMaxDetails {
			break
		}
		value := in[key]
		out[key] = truncateRequestControlValue(value, 200)
	}
	return out
}
