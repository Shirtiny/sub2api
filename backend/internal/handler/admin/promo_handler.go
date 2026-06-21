package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PromoHandler handles admin promo code management
type PromoHandler struct {
	promoService   *service.PromoService
	paymentService *service.PaymentService
}

// NewPromoHandler creates a new admin promo handler
func NewPromoHandler(promoService *service.PromoService, paymentService *service.PaymentService) *PromoHandler {
	return &PromoHandler{
		promoService:   promoService,
		paymentService: paymentService,
	}
}

// CreatePromoCodeRequest represents create promo code request
type CreatePromoCodeRequest struct {
	Code        string  `json:"code"`                                  // 可选，为空则自动生成
	BonusAmount float64 `json:"bonus_amount" binding:"required,min=0"` // 赠送余额
	MaxUses     int     `json:"max_uses" binding:"min=0"`              // 最大使用次数，0=无限
	ExpiresAt   *int64  `json:"expires_at"`                            // 过期时间戳（秒）
	Notes       string  `json:"notes"`                                 // 备注
}

// UpdatePromoCodeRequest represents update promo code request
type UpdatePromoCodeRequest struct {
	Code        *string  `json:"code"`
	BonusAmount *float64 `json:"bonus_amount" binding:"omitempty,min=0"`
	MaxUses     *int     `json:"max_uses" binding:"omitempty,min=0"`
	Status      *string  `json:"status" binding:"omitempty,oneof=active disabled"`
	ExpiresAt   *int64   `json:"expires_at"`
	Notes       *string  `json:"notes"`
}

// ListCafeCoupons handles listing café coupons under promo code management.
// GET /api/v1/admin/promo-codes/cafe-coupons
func (h *PromoHandler) ListCafeCoupons(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filters := service.CafeCouponAdminListFilters{
		Search:     trimPromoSearch(c.Query("search")),
		Status:     c.Query("status"),
		CouponType: c.Query("type"),
	}
	if raw := strings.TrimSpace(c.Query("membership_level")); raw != "" {
		level, err := strconv.Atoi(raw)
		if err != nil || level < 0 || level > 3 {
			response.BadRequest(c, "Invalid membership level")
			return
		}
		filters.MembershipLevel = &level
	}

	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}
	coupons, paginationResult, err := h.paymentService.AdminListCafeCoupons(c.Request.Context(), params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.AdminCafeCoupon, 0, len(coupons))
	for i := range coupons {
		out = append(out, *dto.AdminCafeCouponFromService(&coupons[i]))
	}
	response.Paginated(c, out, paginationResult.Total, page, pageSize)
}

// GetCafeCoupon handles getting a café coupon by ID.
// GET /api/v1/admin/promo-codes/cafe-coupons/:id
func (h *PromoHandler) GetCafeCoupon(c *gin.Context) {
	couponID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid cafe coupon ID")
		return
	}
	coupon, err := h.paymentService.AdminGetCafeCoupon(c.Request.Context(), couponID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AdminCafeCouponFromService(coupon))
}

// VoidCafeCoupon handles voiding an unused issued café coupon.
// POST /api/v1/admin/promo-codes/cafe-coupons/:id/void
func (h *PromoHandler) VoidCafeCoupon(c *gin.Context) {
	couponID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid cafe coupon ID")
		return
	}
	coupon, err := h.paymentService.AdminVoidCafeCoupon(c.Request.Context(), couponID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AdminCafeCouponFromService(coupon))
}

type UpdateCafeCouponStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// UpdateCafeCouponStatus handles changing a café coupon status.
// PATCH /api/v1/admin/promo-codes/cafe-coupons/:id/status
func (h *PromoHandler) UpdateCafeCouponStatus(c *gin.Context) {
	couponID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid cafe coupon ID")
		return
	}
	var req UpdateCafeCouponStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	coupon, err := h.paymentService.AdminUpdateCafeCouponStatus(c.Request.Context(), couponID, req.Status)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AdminCafeCouponFromService(coupon))
}

// ResetCafeCouponClaimPeriod resets the owner's café coupon claim cooldown.
// POST /api/v1/admin/promo-codes/cafe-coupons/:id/reset-claim-period
func (h *PromoHandler) ResetCafeCouponClaimPeriod(c *gin.Context) {
	couponID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid cafe coupon ID")
		return
	}
	coupon, err := h.paymentService.AdminResetCafeCouponClaimPeriod(c.Request.Context(), couponID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AdminCafeCouponFromService(coupon))
}
func trimPromoSearch(search string) string {
	search = strings.TrimSpace(search)
	if len(search) > 100 {
		return search[:100]
	}
	return search
}

// List handles listing all promo codes with pagination
// GET /api/v1/admin/promo-codes
func (h *PromoHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	status := c.Query("status")
	search := trimPromoSearch(c.Query("search"))

	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}

	codes, paginationResult, err := h.promoService.List(c.Request.Context(), params, status, search)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.PromoCode, 0, len(codes))
	for i := range codes {
		out = append(out, *dto.PromoCodeFromService(&codes[i]))
	}
	response.Paginated(c, out, paginationResult.Total, page, pageSize)
}

// GetByID handles getting a promo code by ID
// GET /api/v1/admin/promo-codes/:id
func (h *PromoHandler) GetByID(c *gin.Context) {
	codeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid promo code ID")
		return
	}

	code, err := h.promoService.GetByID(c.Request.Context(), codeID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.PromoCodeFromService(code))
}

// Create handles creating a new promo code
// POST /api/v1/admin/promo-codes
func (h *PromoHandler) Create(c *gin.Context) {
	var req CreatePromoCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	input := &service.CreatePromoCodeInput{
		Code:        req.Code,
		BonusAmount: req.BonusAmount,
		MaxUses:     req.MaxUses,
		Notes:       req.Notes,
	}

	if req.ExpiresAt != nil {
		t := time.Unix(*req.ExpiresAt, 0)
		input.ExpiresAt = &t
	}

	code, err := h.promoService.Create(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.PromoCodeFromService(code))
}

// Update handles updating a promo code
// PUT /api/v1/admin/promo-codes/:id
func (h *PromoHandler) Update(c *gin.Context) {
	codeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid promo code ID")
		return
	}

	var req UpdatePromoCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	input := &service.UpdatePromoCodeInput{
		Code:        req.Code,
		BonusAmount: req.BonusAmount,
		MaxUses:     req.MaxUses,
		Status:      req.Status,
		Notes:       req.Notes,
	}

	if req.ExpiresAt != nil {
		if *req.ExpiresAt == 0 {
			// 0 表示清除过期时间
			input.ExpiresAt = nil
		} else {
			t := time.Unix(*req.ExpiresAt, 0)
			input.ExpiresAt = &t
		}
	}

	code, err := h.promoService.Update(c.Request.Context(), codeID, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.PromoCodeFromService(code))
}

// Delete handles deleting a promo code
// DELETE /api/v1/admin/promo-codes/:id
func (h *PromoHandler) Delete(c *gin.Context) {
	codeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid promo code ID")
		return
	}

	err = h.promoService.Delete(c.Request.Context(), codeID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Promo code deleted successfully"})
}

// GetUsages handles getting usage records for a promo code
// GET /api/v1/admin/promo-codes/:id/usages
func (h *PromoHandler) GetUsages(c *gin.Context) {
	codeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid promo code ID")
		return
	}

	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:     page,
		PageSize: pageSize,
	}

	usages, paginationResult, err := h.promoService.ListUsages(c.Request.Context(), codeID, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.PromoCodeUsage, 0, len(usages))
	for i := range usages {
		out = append(out, *dto.PromoCodeUsageFromService(&usages[i]))
	}
	response.Paginated(c, out, paginationResult.Total, page, pageSize)
}
