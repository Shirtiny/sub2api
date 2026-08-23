package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ContentModerationHandler struct {
	service        *service.ContentModerationService
	requestControl *service.RequestControlService
}

func NewContentModerationHandler(svc *service.ContentModerationService, requestControl *service.RequestControlService) *ContentModerationHandler {
	return &ContentModerationHandler{service: svc, requestControl: requestControl}
}

type requestControlConfigRequest struct {
	Enabled                  *bool                                 `json:"enabled"`
	BlockOpenAIChat          *bool                                 `json:"block_openai_chat"`
	BlockClaudeMessages      *bool                                 `json:"block_claude_messages"`
	BlockOpenAIResponses     *bool                                 `json:"block_openai_responses"`
	AllGroups                *bool                                 `json:"all_groups"`
	GroupIDs                 *[]int64                              `json:"group_ids"`
	ModelFilter              *service.ContentModerationModelFilter `json:"model_filter"`
	AllUsers                 *bool                                 `json:"all_users"`
	UserRules                *[]service.RequestControlUserRule     `json:"user_rules"`
	GlobalUserAgentWhitelist *[]string                             `json:"global_user_agent_whitelist"`
	BlockStatus              *int                                  `json:"block_status"`
	BlockMessage             *string                               `json:"block_message"`
	EmailOnHit               *bool                                 `json:"email_on_hit"`
	AutoBanEnabled           *bool                                 `json:"auto_ban_enabled"`
	BanThreshold             *int                                  `json:"ban_threshold"`
	ViolationWindowHours     *int                                  `json:"violation_window_hours"`
}

type contentModerationConfigRequest struct {
	Enabled              *bool                                 `json:"enabled"`
	Mode                 *string                               `json:"mode"`
	BaseURL              *string                               `json:"base_url"`
	Model                *string                               `json:"model"`
	APIKey               *string                               `json:"api_key"`
	APIKeys              *[]string                             `json:"api_keys"`
	APIKeysMode          string                                `json:"api_keys_mode"`
	DeleteAPIKeyHashes   *[]string                             `json:"delete_api_key_hashes"`
	ClearAPIKey          bool                                  `json:"clear_api_key"`
	TimeoutMS            *int                                  `json:"timeout_ms"`
	SampleRate           *int                                  `json:"sample_rate"`
	AllGroups            *bool                                 `json:"all_groups"`
	GroupIDs             *[]int64                              `json:"group_ids"`
	RecordNonHits        *bool                                 `json:"record_non_hits"`
	Thresholds           *map[string]float64                   `json:"thresholds"`
	WorkerCount          *int                                  `json:"worker_count"`
	QueueSize            *int                                  `json:"queue_size"`
	BlockStatus          *int                                  `json:"block_status"`
	BlockMessage         *string                               `json:"block_message"`
	EmailOnHit           *bool                                 `json:"email_on_hit"`
	AutoBanEnabled       *bool                                 `json:"auto_ban_enabled"`
	BanThreshold         *int                                  `json:"ban_threshold"`
	ViolationWindowHours *int                                  `json:"violation_window_hours"`
	RetryCount           *int                                  `json:"retry_count"`
	HitRetentionDays     *int                                  `json:"hit_retention_days"`
	NonHitRetentionDays  *int                                  `json:"non_hit_retention_days"`
	PreHashCheckEnabled  *bool                                 `json:"pre_hash_check_enabled"`
	BlockedKeywords      *[]string                             `json:"blocked_keywords"`
	KeywordWhitelist     *[]string                             `json:"keyword_whitelist"`
	KeywordBlockingMode  *string                               `json:"keyword_blocking_mode"`
	BuiltInFilterEnabled *bool                                 `json:"built_in_filter_enabled"`
	ModelFilter          *service.ContentModerationModelFilter `json:"model_filter"`
}

type contentModerationAPIKeyTestRequest struct {
	APIKeys   []string `json:"api_keys"`
	BaseURL   string   `json:"base_url"`
	Model     string   `json:"model"`
	TimeoutMS int      `json:"timeout_ms"`
	Prompt    string   `json:"prompt"`
	Images    []string `json:"images"`
}

type contentModerationHashRequest struct {
	InputHash string `json:"input_hash"`
}

func (h *ContentModerationHandler) GetConfig(c *gin.Context) {
	cfg, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *ContentModerationHandler) UpdateConfig(c *gin.Context) {
	var req contentModerationConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.service.UpdateConfig(c.Request.Context(), service.UpdateContentModerationConfigInput{
		Enabled:              req.Enabled,
		Mode:                 req.Mode,
		BaseURL:              req.BaseURL,
		Model:                req.Model,
		APIKey:               req.APIKey,
		APIKeys:              req.APIKeys,
		APIKeysMode:          req.APIKeysMode,
		DeleteAPIKeyHashes:   req.DeleteAPIKeyHashes,
		ClearAPIKey:          req.ClearAPIKey,
		TimeoutMS:            req.TimeoutMS,
		SampleRate:           req.SampleRate,
		AllGroups:            req.AllGroups,
		GroupIDs:             req.GroupIDs,
		RecordNonHits:        req.RecordNonHits,
		Thresholds:           req.Thresholds,
		WorkerCount:          req.WorkerCount,
		QueueSize:            req.QueueSize,
		BlockStatus:          req.BlockStatus,
		BlockMessage:         req.BlockMessage,
		EmailOnHit:           req.EmailOnHit,
		AutoBanEnabled:       req.AutoBanEnabled,
		BanThreshold:         req.BanThreshold,
		ViolationWindowHours: req.ViolationWindowHours,
		RetryCount:           req.RetryCount,
		HitRetentionDays:     req.HitRetentionDays,
		NonHitRetentionDays:  req.NonHitRetentionDays,
		PreHashCheckEnabled:  req.PreHashCheckEnabled,
		BlockedKeywords:      req.BlockedKeywords,
		KeywordWhitelist:     req.KeywordWhitelist,
		KeywordBlockingMode:  req.KeywordBlockingMode,
		BuiltInFilterEnabled: req.BuiltInFilterEnabled,
		ModelFilter:          req.ModelFilter,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *ContentModerationHandler) TestAPIKeys(c *gin.Context) {
	var req contentModerationAPIKeyTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.service.TestAPIKeys(c.Request.Context(), service.TestContentModerationAPIKeysInput{
		APIKeys:   req.APIKeys,
		BaseURL:   req.BaseURL,
		Model:     req.Model,
		TimeoutMS: req.TimeoutMS,
		Prompt:    req.Prompt,
		Images:    req.Images,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ContentModerationHandler) GetStatus(c *gin.Context) {
	status, err := h.service.GetStatus(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func (h *ContentModerationHandler) ListLogs(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filter := service.ContentModerationLogFilter{
		Pagination: pagination.PaginationParams{
			Page:      page,
			PageSize:  pageSize,
			SortOrder: pagination.SortOrderDesc,
		},
		Result:   c.Query("result"),
		Endpoint: c.Query("endpoint"),
		Search:   c.Query("search"),
	}
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		groupID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || groupID <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &groupID
	}
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		t, _, err := parseContentModerationDate(raw)
		if err != nil {
			response.BadRequest(c, "Invalid from")
			return
		}
		filter.From = &t
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		t, dateOnly, err := parseContentModerationDate(raw)
		if err != nil {
			response.BadRequest(c, "Invalid to")
			return
		}
		if dateOnly {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		filter.To = &t
	}
	items, pageResult, err := h.service.ListLogs(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, pageResult.Total, pageResult.Page, pageResult.PageSize)
}

func (h *ContentModerationHandler) UnbanUser(c *gin.Context) {
	userID, err := strconv.ParseInt(strings.TrimSpace(c.Param("user_id")), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user_id")
		return
	}
	result, err := h.service.UnbanUser(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ContentModerationHandler) DeleteFlaggedHash(c *gin.Context) {
	var req contentModerationHashRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.service.DeleteFlaggedInputHash(c.Request.Context(), req.InputHash)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ContentModerationHandler) ClearFlaggedHashes(c *gin.Context) {
	result, err := h.service.ClearFlaggedInputHashes(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ContentModerationHandler) GetRequestControlConfig(c *gin.Context) {
	if h.requestControl == nil {
		response.Error(c, 503, "Request control is unavailable")
		return
	}
	cfg, err := h.requestControl.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *ContentModerationHandler) UpdateRequestControlConfig(c *gin.Context) {
	if h.requestControl == nil {
		response.Error(c, 503, "Request control is unavailable")
		return
	}
	var req requestControlConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.requestControl.UpdateConfig(c.Request.Context(), service.UpdateRequestControlConfigInput{
		Enabled: req.Enabled, BlockOpenAIChat: req.BlockOpenAIChat, BlockClaudeMessages: req.BlockClaudeMessages, BlockOpenAIResponses: req.BlockOpenAIResponses,
		AllGroups: req.AllGroups, GroupIDs: req.GroupIDs, ModelFilter: req.ModelFilter,
		AllUsers: req.AllUsers, UserRules: req.UserRules, GlobalUserAgentWhitelist: req.GlobalUserAgentWhitelist,
		BlockStatus: req.BlockStatus, BlockMessage: req.BlockMessage, EmailOnHit: req.EmailOnHit,
		AutoBanEnabled: req.AutoBanEnabled, BanThreshold: req.BanThreshold, ViolationWindowHours: req.ViolationWindowHours,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *ContentModerationHandler) GetRequestControlStatus(c *gin.Context) {
	if h.requestControl == nil {
		response.Error(c, 503, "Request control is unavailable")
		return
	}
	response.Success(c, h.requestControl.GetStatus())
}

func (h *ContentModerationHandler) ListRequestControlLogs(c *gin.Context) {
	if h.requestControl == nil {
		response.Error(c, 503, "Request control is unavailable")
		return
	}
	page, pageSize := response.ParsePagination(c)
	filter := service.RequestControlLogFilter{
		Pagination: pagination.PaginationParams{Page: page, PageSize: pageSize, SortOrder: pagination.SortOrderDesc},
		Action:     c.Query("action"), Protocol: c.Query("protocol"), Search: c.Query("search"),
	}
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filter.UserID = &id
	}
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		t, _, err := parseContentModerationDate(raw)
		if err != nil {
			response.BadRequest(c, "Invalid from")
			return
		}
		filter.From = &t
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		t, dateOnly, err := parseContentModerationDate(raw)
		if err != nil {
			response.BadRequest(c, "Invalid to")
			return
		}
		if dateOnly {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		filter.To = &t
	}
	items, pageResult, err := h.requestControl.ListLogs(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, pageResult.Total, pageResult.Page, pageResult.PageSize)
}

func (h *ContentModerationHandler) GetRequestControlLog(c *gin.Context) {
	if h.requestControl == nil {
		response.Error(c, 503, "Request control is unavailable")
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid request control log id")
		return
	}
	item, err := h.requestControl.GetLog(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func parseContentModerationDate(raw string) (time.Time, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, false, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	return t, err == nil, err
}
