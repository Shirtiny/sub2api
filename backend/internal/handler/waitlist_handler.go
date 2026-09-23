package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type waitlistChallenge interface {
	VerifyToken(context.Context, string, string) error
}

type WaitlistHandler struct {
	waitlist  *service.WaitlistService
	challenge waitlistChallenge
}

func NewWaitlistHandler(waitlist *service.WaitlistService, challenge *service.TurnstileService) *WaitlistHandler {
	return &WaitlistHandler{waitlist: waitlist, challenge: challenge}
}

// Join is public and rate-limited. Never disclose whether an email already exists.
func (h *WaitlistHandler) Join(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req struct {
		Email          string `json:"email"`
		TurnstileToken string `json:"turnstile_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	email, err := service.NormalizeWaitlistEmail(req.Email)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.challenge.VerifyToken(c.Request.Context(), req.TurnstileToken, c.ClientIP()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.waitlist.Join(c.Request.Context(), email); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"accepted": true})
}

// List is registered only inside the administrator-authenticated route group.
func (h *WaitlistHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.waitlist.List(c.Request.Context(), pagination.PaginationParams{Page: page, PageSize: pageSize})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Paginated(c, items, total, page, pageSize)
}

// Approve is mounted exclusively behind administrator authentication.
func (h *WaitlistHandler) Approve(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid waiting list ID")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Authentication required")
		return
	}
	if err := h.waitlist.Approve(c.Request.Context(), id, subject.UserID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"approved": true})
}
