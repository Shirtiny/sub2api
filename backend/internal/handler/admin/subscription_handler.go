package admin

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// toResponsePagination converts pagination.PaginationResult to response.PaginationResult
func toResponsePagination(p *pagination.PaginationResult) *response.PaginationResult {
	if p == nil {
		return nil
	}
	return &response.PaginationResult{
		Total:    p.Total,
		Page:     p.Page,
		PageSize: p.PageSize,
		Pages:    p.Pages,
	}
}

// SubscriptionHandler handles admin subscription management
type SubscriptionHandler struct {
	subscriptionService *service.SubscriptionService
}

// NewSubscriptionHandler creates a new admin subscription handler
func NewSubscriptionHandler(subscriptionService *service.SubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{
		subscriptionService: subscriptionService,
	}
}

// AssignSubscriptionRequest represents assign subscription request
type AssignSubscriptionRequest struct {
	UserID       int64  `json:"user_id" binding:"required"`
	GroupID      int64  `json:"group_id" binding:"omitempty"`
	PlanID       *int64 `json:"plan_id" binding:"omitempty"`
	Multiplier   *int   `json:"multiplier" binding:"omitempty,min=1,max=100"`
	ValidityDays int    `json:"validity_days" binding:"omitempty,max=36500"` // max 100 years
	Notes        string `json:"notes"`
}

// BulkAssignSubscriptionRequest represents bulk assign subscription request
type BulkAssignSubscriptionRequest struct {
	UserIDs      []int64 `json:"user_ids" binding:"required,min=1"`
	GroupID      int64   `json:"group_id" binding:"required"`
	ValidityDays int     `json:"validity_days" binding:"omitempty,max=36500"` // max 100 years
	Notes        string  `json:"notes"`
}

// UpdateSubscriptionMultiplierRequest updates the multiplier of a plan-backed
// subscription without changing its validity period or usage.
type UpdateSubscriptionMultiplierRequest struct {
	PlanID     int64 `json:"plan_id" binding:"required"`
	Multiplier int   `json:"multiplier" binding:"required,min=1,max=100"`
}

// AdjustSubscriptionRequest represents adjust subscription request (extend or shorten)
type AdjustSubscriptionRequest struct {
	Days int `json:"days" binding:"required,min=-36500,max=36500"` // negative to shorten, positive to extend
}

// List handles listing all subscriptions with pagination and filters
// GET /api/v1/admin/subscriptions
func (h *SubscriptionHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)

	// Parse optional filters
	var userID, groupID *int64
	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if id, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
			userID = &id
		}
	}
	if groupIDStr := c.Query("group_id"); groupIDStr != "" {
		if id, err := strconv.ParseInt(groupIDStr, 10, 64); err == nil {
			groupID = &id
		}
	}
	status := c.Query("status")
	platform := c.Query("platform")

	// Parse sorting parameters
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")

	subscriptions, pagination, err := h.subscriptionService.List(c.Request.Context(), page, pageSize, userID, groupID, status, platform, sortBy, sortOrder)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.AdminUserSubscription, 0, len(subscriptions))
	for i := range subscriptions {
		out = append(out, *dto.UserSubscriptionFromServiceAdmin(&subscriptions[i]))
	}
	response.PaginatedWithResult(c, out, toResponsePagination(pagination))
}

// GetByID handles getting a subscription by ID
// GET /api/v1/admin/subscriptions/:id
func (h *SubscriptionHandler) GetByID(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	subscription, err := h.subscriptionService.GetByID(c.Request.Context(), subscriptionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.UserSubscriptionFromServiceAdmin(subscription))
}

// GetProgress handles getting subscription usage progress
// GET /api/v1/admin/subscriptions/:id/progress
func (h *SubscriptionHandler) GetProgress(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	progress, err := h.subscriptionService.GetSubscriptionProgress(c.Request.Context(), subscriptionID)
	if err != nil {
		response.NotFound(c, "Subscription not found")
		return
	}

	response.Success(c, progress)
}

// Assign handles assigning a subscription to a user
// POST /api/v1/admin/subscriptions/assign
func (h *SubscriptionHandler) Assign(c *gin.Context) {
	var req AssignSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Get admin user ID from context
	adminID := getAdminIDFromContext(c)

	var subscription *service.UserSubscription
	var err error
	if req.PlanID != nil {
		subscription, err = h.subscriptionService.AssignPlanSubscription(c.Request.Context(), &service.AssignPlanSubscriptionInput{
			UserID:       req.UserID,
			GroupID:      req.GroupID,
			PlanID:       *req.PlanID,
			Multiplier:   req.Multiplier,
			ValidityDays: req.ValidityDays,
			AssignedBy:   adminID,
			Notes:        req.Notes,
		})
	} else {
		if req.GroupID <= 0 {
			response.BadRequest(c, "group_id is required")
			return
		}
		if req.Multiplier != nil {
			response.BadRequest(c, "plan_id is required when multiplier is provided")
			return
		}
		subscription, err = h.subscriptionService.AssignSubscription(c.Request.Context(), &service.AssignSubscriptionInput{
			UserID:       req.UserID,
			GroupID:      req.GroupID,
			ValidityDays: req.ValidityDays,
			AssignedBy:   adminID,
			Notes:        req.Notes,
		})
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.UserSubscriptionFromServiceAdmin(subscription))
}

// BulkAssign handles bulk assigning subscriptions to multiple users
// POST /api/v1/admin/subscriptions/bulk-assign
func (h *SubscriptionHandler) BulkAssign(c *gin.Context) {
	var req BulkAssignSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Get admin user ID from context
	adminID := getAdminIDFromContext(c)

	result, err := h.subscriptionService.BulkAssignSubscription(c.Request.Context(), &service.BulkAssignSubscriptionInput{
		UserIDs:      req.UserIDs,
		GroupID:      req.GroupID,
		ValidityDays: req.ValidityDays,
		AssignedBy:   adminID,
		Notes:        req.Notes,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.BulkAssignResultFromService(result))
}

// UpdateMultiplier changes a subscription's selected plan multiplier.
// PUT /api/v1/admin/subscriptions/:id/multiplier
func (h *SubscriptionHandler) UpdateMultiplier(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}
	var req UpdateSubscriptionMultiplierRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	payload := struct {
		SubscriptionID int64                               `json:"subscription_id"`
		Body           UpdateSubscriptionMultiplierRequest `json:"body"`
	}{SubscriptionID: subscriptionID, Body: req}
	executeAdminIdempotentJSON(c, "admin.subscriptions.update_multiplier", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		subscription, execErr := h.subscriptionService.UpdateSubscriptionMultiplier(ctx, &service.UpdateSubscriptionMultiplierInput{
			SubscriptionID: subscriptionID,
			PlanID:         req.PlanID,
			Multiplier:     req.Multiplier,
		})
		if execErr != nil {
			return nil, execErr
		}
		return dto.UserSubscriptionFromServiceAdmin(subscription), nil
	})
}

// Extend handles adjusting a subscription (extend or shorten)
// POST /api/v1/admin/subscriptions/:id/extend
func (h *SubscriptionHandler) Extend(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	var req AdjustSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	idempotencyPayload := struct {
		SubscriptionID int64                     `json:"subscription_id"`
		Body           AdjustSubscriptionRequest `json:"body"`
	}{
		SubscriptionID: subscriptionID,
		Body:           req,
	}
	executeAdminIdempotentJSON(c, "admin.subscriptions.extend", idempotencyPayload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		subscription, execErr := h.subscriptionService.ExtendSubscription(ctx, subscriptionID, req.Days)
		if execErr != nil {
			return nil, execErr
		}
		return dto.UserSubscriptionFromServiceAdmin(subscription), nil
	})
}

// ResetSubscriptionQuotaRequest represents the reset quota request
type ResetSubscriptionQuotaRequest struct {
	Daily   bool `json:"daily"`
	Weekly  bool `json:"weekly"`
	Monthly bool `json:"monthly"`
}

// SetSubscriptionResetCountRequest sets the number of remaining user-facing
// daily/weekly quota resets. A pointer allows an explicit zero while still
// rejecting an omitted count.
type SetSubscriptionResetCountRequest struct {
	Count *int `json:"count"`
	// ResetCount is accepted as an explicit alias for API clients that mirror
	// the persisted field name.
	ResetCount      *int    `json:"reset_count,omitempty"`
	SubscriptionIDs []int64 `json:"subscription_ids,omitempty"`
}

func (r SetSubscriptionResetCountRequest) value() (*int, bool) {
	if r.Count != nil {
		return r.Count, true
	}
	if r.ResetCount != nil {
		return r.ResetCount, true
	}
	return nil, false
}

// SetResetCount sets one subscription's remaining user reset allowance.
// PUT|POST /api/v1/admin/subscriptions/:id/reset-count
func (h *SubscriptionHandler) SetResetCount(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || subscriptionID <= 0 {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}
	var req SetSubscriptionResetCountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "count is required")
		return
	}
	count, ok := req.value()
	if !ok {
		response.BadRequest(c, "count is required")
		return
	}
	payload := struct {
		SubscriptionID int64 `json:"subscription_id"`
		Count          int   `json:"count"`
	}{SubscriptionID: subscriptionID, Count: *count}
	executeAdminIdempotentJSON(c, "admin.subscriptions.set_reset_count", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		sub, setErr := h.subscriptionService.SetSubscriptionResetCount(ctx, subscriptionID, *count)
		if setErr != nil {
			return nil, setErr
		}
		return dto.UserSubscriptionFromServiceAdmin(sub), nil
	})
}

// BulkSetResetCount sets one allowance value for a bounded set of
// subscriptions. When subscription_ids is omitted, all active subscriptions
// are considered.
// POST /api/v1/admin/subscriptions/bulk-reset-count
func (h *SubscriptionHandler) BulkSetResetCount(c *gin.Context) {
	var req SetSubscriptionResetCountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "count is required")
		return
	}
	count, ok := req.value()
	if !ok {
		response.BadRequest(c, "count is required")
		return
	}
	if len(req.SubscriptionIDs) > 1000 {
		response.BadRequest(c, "subscription_ids cannot contain more than 1000 items")
		return
	}
	for _, id := range req.SubscriptionIDs {
		if id <= 0 {
			response.BadRequest(c, "subscription_ids contains an invalid ID")
			return
		}
	}
	payload := struct {
		Count           int     `json:"count"`
		SubscriptionIDs []int64 `json:"subscription_ids,omitempty"`
	}{Count: *count, SubscriptionIDs: req.SubscriptionIDs}
	executeAdminIdempotentJSON(c, "admin.subscriptions.bulk_set_reset_count", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.subscriptionService.BulkSetSubscriptionResetCount(ctx, req.SubscriptionIDs, *count)
	})
}

// ResetQuota resets daily, weekly, and/or monthly usage for a subscription.
// POST /api/v1/admin/subscriptions/:id/reset-quota
func (h *SubscriptionHandler) ResetQuota(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}
	var req ResetSubscriptionQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if !req.Daily && !req.Weekly && !req.Monthly {
		response.BadRequest(c, "At least one of 'daily', 'weekly', or 'monthly' must be true")
		return
	}
	sub, err := h.subscriptionService.AdminResetQuota(c.Request.Context(), subscriptionID, req.Daily, req.Weekly, req.Monthly)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.UserSubscriptionFromServiceAdmin(sub))
}

// BulkResetQuota resets daily, weekly, and/or monthly usage for all active subscriptions.
// POST /api/v1/admin/subscriptions/bulk-reset-quota
func (h *SubscriptionHandler) BulkResetQuota(c *gin.Context) {
	var req ResetSubscriptionQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if !req.Daily && !req.Weekly && !req.Monthly {
		response.BadRequest(c, "At least one of 'daily', 'weekly', or 'monthly' must be true")
		return
	}
	count, err := h.subscriptionService.AdminBulkResetQuota(c.Request.Context(), req.Daily, req.Weekly, req.Monthly)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"count": count})
}

// ShiftSubscriptionWindowFilters mirrors the list filters so the bulk shift can be scoped
// to exactly what the operator sees on screen.
type ShiftSubscriptionWindowFilters struct {
	Status   string `json:"status"`
	UserID   *int64 `json:"user_id"`
	GroupID  *int64 `json:"group_id"`
	Platform string `json:"platform"`
}

// ShiftSubscriptionWindowRequest represents the bulk window shift request.
type ShiftSubscriptionWindowRequest struct {
	Daily       bool                            `json:"daily"`
	Weekly      bool                            `json:"weekly"`
	Monthly     bool                            `json:"monthly"`
	OffsetHours int                             `json:"offset_hours" binding:"required,min=-720,max=720"`
	DryRun      bool                            `json:"dry_run"`
	Filters     *ShiftSubscriptionWindowFilters `json:"filters"`
}

// BulkShiftWindow shifts the reset window start of every matching subscription.
// POST /api/v1/admin/subscriptions/bulk-shift-window
func (h *SubscriptionHandler) BulkShiftWindow(c *gin.Context) {
	var req ShiftSubscriptionWindowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if !req.Daily && !req.Weekly && !req.Monthly {
		response.BadRequest(c, "At least one of 'daily', 'weekly', or 'monthly' must be true")
		return
	}

	input := &service.ShiftSubscriptionWindowInput{
		Daily:       req.Daily,
		Weekly:      req.Weekly,
		Monthly:     req.Monthly,
		OffsetHours: req.OffsetHours,
		DryRun:      req.DryRun,
	}
	if req.Filters != nil {
		input.Status = req.Filters.Status
		input.UserID = req.Filters.UserID
		input.GroupID = req.Filters.GroupID
		input.Platform = req.Filters.Platform
	}

	// dry-run 不写库，走幂等包装只会白占一个键，直接执行即可。
	if req.DryRun {
		result, err := h.subscriptionService.ShiftSubscriptionWindows(c.Request.Context(), input)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		response.Success(c, result)
		return
	}

	executeAdminIdempotentJSON(c, "admin.subscriptions.shift_window", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.subscriptionService.ShiftSubscriptionWindows(ctx, input)
	})
}

// GetStats returns aggregated quota/usage statistics across all active subscriptions.
// GET /api/v1/admin/subscriptions/stats
func (h *SubscriptionHandler) GetStats(c *gin.Context) {
	horizonDays := 7
	if raw := c.Query("horizon_days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 90 {
			response.BadRequest(c, "Invalid horizon_days")
			return
		}
		horizonDays = parsed
	}

	rankingLimit := 20
	if raw := c.Query("ranking_limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			response.BadRequest(c, "Invalid ranking_limit")
			return
		}
		rankingLimit = parsed
	}

	stats, err := h.subscriptionService.GetSubscriptionStats(c.Request.Context(), horizonDays, rankingLimit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

// GetUsageSeries returns the per-day / per-week / whole-cycle usage rates of one subscription.
// GET /api/v1/admin/subscriptions/:id/usage-series
func (h *SubscriptionHandler) GetUsageSeries(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	series, err := h.subscriptionService.GetSubscriptionUsageSeries(c.Request.Context(), subscriptionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, series)
}

// Revoke handles revoking a subscription
// DELETE /api/v1/admin/subscriptions/:id
func (h *SubscriptionHandler) Revoke(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	err = h.subscriptionService.RevokeSubscription(c.Request.Context(), subscriptionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Subscription revoked successfully"})
}

// ListByGroup handles listing subscriptions for a specific group
// GET /api/v1/admin/groups/:id/subscriptions
func (h *SubscriptionHandler) ListByGroup(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}

	page, pageSize := response.ParsePagination(c)

	subscriptions, pagination, err := h.subscriptionService.ListGroupSubscriptions(c.Request.Context(), groupID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.AdminUserSubscription, 0, len(subscriptions))
	for i := range subscriptions {
		out = append(out, *dto.UserSubscriptionFromServiceAdmin(&subscriptions[i]))
	}
	response.PaginatedWithResult(c, out, toResponsePagination(pagination))
}

// ListByUser handles listing subscriptions for a specific user
// GET /api/v1/admin/users/:id/subscriptions
func (h *SubscriptionHandler) ListByUser(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	subscriptions, err := h.subscriptionService.ListUserSubscriptions(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.AdminUserSubscription, 0, len(subscriptions))
	for i := range subscriptions {
		out = append(out, *dto.UserSubscriptionFromServiceAdmin(&subscriptions[i]))
	}
	response.Success(c, out)
}

// Helper function to get admin ID from context
func getAdminIDFromContext(c *gin.Context) int64 {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		return 0
	}
	return subject.UserID
}
