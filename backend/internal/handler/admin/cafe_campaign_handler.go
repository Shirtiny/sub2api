package admin

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *PromoHandler) ListCafeCampaigns(c *gin.Context) {
	page, size := response.ParsePagination(c)
	rows, total, err := h.paymentService.ListCafeCampaigns(c.Request.Context(), page, size)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, rows, int64(total), page, size)
}
func (h *PromoHandler) CreateCafeCampaign(c *gin.Context) {
	var req service.CreateCafeCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	row, err := h.paymentService.CreateCafeCampaign(c.Request.Context(), getAdminIDFromContext(c), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, row)
}
func (h *PromoHandler) SetCafeCampaignEnabled(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req struct {
		Enabled           *bool     `json:"enabled"`
		ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	row, err := h.paymentService.SetCafeCampaignEnabled(c.Request.Context(), id, getAdminIDFromContext(c), *req.Enabled, req.ExpectedUpdatedAt)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, row)
}
func (h *PromoHandler) ListCafeCampaignUses(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	page, size := response.ParsePagination(c)
	rows, total, err := h.paymentService.ListCafeCampaignUses(c.Request.Context(), id, page, size)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, rows, int64(total), page, size)
}
