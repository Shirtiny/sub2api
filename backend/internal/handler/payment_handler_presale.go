package handler

import (
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

type PresaleOrderInfo struct {
	PresaleStartsAt    *time.Time `json:"presale_starts_at,omitempty"`
	PresaleExpiresAt   *time.Time `json:"presale_expires_at,omitempty"`
	PresaleActivatedAt *time.Time `json:"presale_activated_at,omitempty"`
	PresaleRenewal     bool       `json:"presale_renewal,omitempty"`
	PresalePlanName    string     `json:"presale_plan_name,omitempty"`
	PresaleResetCards  int        `json:"presale_reset_cards,omitempty"`
}

func presaleOrderInfo(o *dbent.PaymentOrder) PresaleOrderInfo {
	return PresaleOrderInfo{o.PresaleStartsAt, o.PresaleExpiresAt, o.PresaleActivatedAt, o.PresaleRenewal, o.PresalePlanName, o.PresaleResetCards}
}

type presaleCatalogPlan struct {
	checkoutPlan
	Badge string `json:"presale_badge"`
}

func (h *PaymentHandler) GetPresaleCatalog(c *gin.Context) {
	ctx := c.Request.Context()
	cfg, err := h.configService.GetPaymentConfig(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	plans, err := h.configService.ListPresalePlans(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	groups := h.configService.GetGroupInfoMap(ctx, plans)
	result := make([]presaleCatalogPlan, 0, len(plans))
	for _, p := range plans {
		gi, ok := groups[p.GroupID]
		if !ok {
			continue
		}
		result = append(result, presaleCatalogPlan{checkoutPlan: checkoutPlan{
			ID: p.ID, GroupID: p.GroupID, GroupName: gi.Name, GroupPlatform: gi.Platform, RateMultiplier: gi.RateMultiplier,
			DailyLimitUSD: gi.DailyLimitUSD, WeeklyLimitUSD: gi.WeeklyLimitUSD, MonthlyLimitUSD: gi.MonthlyLimitUSD, ModelScopes: gi.ModelScopes,
			Name: p.Name, Description: p.Description, Price: p.Price, OriginalPrice: p.OriginalPrice, ValidityDays: 1, ValidityUnit: "months", Concurrency: p.Concurrency,
			Features: parseFeatures(p.Features), CustomMultiplierEnabled: p.CustomMultiplierEnabled, CustomMultiplierMin: p.CustomMultiplierMin, CustomMultiplierMax: p.CustomMultiplierMax, PresaleEnabled: true,
		}, Badge: p.PresaleBadge})
	}
	response.Success(c, gin.H{"period": service.NextPresalePeriod(time.Now()), "plans": result, "enabled": cfg.Enabled && len(result) > 0, "server_time": time.Now()})
}
func (h *PaymentHandler) GetPresaleQuote(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid ID")
		return
	}
	quote, err := h.paymentService.GetPresaleQuote(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, quote)
}
func (h *PaymentHandler) GetMyPresales(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	orders, err := h.paymentService.GetMyPresales(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, sanitizePaymentOrdersForResponse(orders))
}
func (h *PaymentHandler) GetPresaleRefundQuote(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid ID")
		return
	}
	order, err := h.paymentService.GetOrder(c.Request.Context(), id, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	quote, err := h.paymentService.GetPresaleRefundQuote(c.Request.Context(), order, time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, quote)
}
