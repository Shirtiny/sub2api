package handler

import (
	"encoding/json"
	"io"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	defaultGuestShopPublicLimit  = 20
	defaultGuestShopPublicWindow = time.Minute
)

type guestShopCreateRequest struct {
	Items    []service.GuestShopItemInput `json:"items"`
	Shipping string                       `json:"shipping"`
	Customer service.GuestShopCustomer    `json:"customer"`
}

type guestShopStatusRequest struct {
	PaymentIntentID string `json:"payment_intent_id"`
}

func (h *PaymentHandler) allowGuestShopPublic(c *gin.Context) bool {
	if h == nil || h.guestShopLimiter == nil {
		return true
	}
	if h.guestShopLimiter.allow(c.ClientIP(), time.Now()) {
		return true
	}
	response.ErrorFrom(c, infraerrors.TooManyRequests("GUEST_SHOP_RATE_LIMITED", "too many checkout attempts, please try again later"))
	return false
}

// GetGuestShopConfig returns the public Stripe checkout config for the apparel shop.
// GET /api/v1/payment/public/shop/config
func (h *PaymentHandler) GetGuestShopConfig(c *gin.Context) {
	if !h.allowGuestShopPublic(c) {
		return
	}
	cfg, err := h.paymentService.GetGuestShopConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// CreateGuestShopPayment creates a Stripe PaymentIntent for a guest cart.
// POST /api/v1/payment/public/shop/payments
func (h *PaymentHandler) CreateGuestShopPayment(c *gin.Context) {
	if !h.allowGuestShopPublic(c) {
		return
	}
	var req guestShopCreateRequest
	if err := bindStrictGuestShopJSON(c, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	resp, err := h.paymentService.CreateGuestShopPayment(c.Request.Context(), service.CreateGuestShopPaymentRequest{
		Items:    req.Items,
		Shipping: req.Shipping,
		Customer: req.Customer,
		ClientIP: c.ClientIP(),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, resp)
}

// GetGuestShopPaymentStatus queries Stripe for a guest PaymentIntent.
// POST /api/v1/payment/public/shop/payments/status
func (h *PaymentHandler) GetGuestShopPaymentStatus(c *gin.Context) {
	if !h.allowGuestShopPublic(c) {
		return
	}
	var req guestShopStatusRequest
	if err := bindStrictGuestShopJSON(c, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	status, err := h.paymentService.GetGuestShopPaymentStatus(c.Request.Context(), req.PaymentIntentID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func bindStrictGuestShopJSON(c *gin.Context, dest any) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errRequestMustBeSingleObject
		}
		return err
	}
	return nil
}

var errRequestMustBeSingleObject = jsonSingleObjectError{}

type jsonSingleObjectError struct{}

func (jsonSingleObjectError) Error() string {
	return "request body must contain a single JSON object"
}
