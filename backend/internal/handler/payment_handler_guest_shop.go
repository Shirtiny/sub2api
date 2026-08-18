package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	clientip "github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	defaultGuestShopReadLimit       = 30
	defaultGuestShopCreateLimit     = 5
	defaultGuestShopPublicWindow    = time.Minute
	guestShopMaxRequestBodyBytes    = 16 << 10
	guestShopRequestTooLargeReason  = "GUEST_SHOP_REQUEST_TOO_LARGE"
	guestShopRequestTooLargeMessage = "checkout request is too large"
)

type guestShopCreateRequest struct {
	Items    []service.GuestShopItemInput `json:"items"`
	Shipping string                       `json:"shipping"`
	Customer service.GuestShopCustomer    `json:"customer"`
}

type guestShopStatusRequest struct {
	PaymentIntentID string `json:"payment_intent_id"`
}

func (h *PaymentHandler) allowGuestShopPublic(c *gin.Context, limiter *cafeCouponLookupLimiter) bool {
	if h == nil || limiter == nil {
		return true
	}
	if limiter.allow(clientip.GetClientIP(c), time.Now()) {
		return true
	}
	response.ErrorFrom(c, infraerrors.TooManyRequests("GUEST_SHOP_RATE_LIMITED", "too many checkout attempts, please try again later"))
	return false
}

// GetGuestShopConfig returns the public Stripe checkout config for the apparel shop.
// GET /api/v1/payment/public/shop/config
func (h *PaymentHandler) GetGuestShopConfig(c *gin.Context) {
	if !h.allowGuestShopPublic(c, h.guestShopReadLimiter) {
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
	if !h.allowGuestShopPublic(c, h.guestShopCreateLimiter) {
		return
	}
	var req guestShopCreateRequest
	if err := bindStrictGuestShopJSON(c, &req); err != nil {
		writeGuestShopJSONError(c, err)
		return
	}
	resp, err := h.paymentService.CreateGuestShopPayment(c.Request.Context(), service.CreateGuestShopPaymentRequest{
		Items:    req.Items,
		Shipping: req.Shipping,
		Customer: req.Customer,
		ClientIP: clientip.GetClientIP(c),
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
	if !h.allowGuestShopPublic(c, h.guestShopReadLimiter) {
		return
	}
	var req guestShopStatusRequest
	if err := bindStrictGuestShopJSON(c, &req); err != nil {
		writeGuestShopJSONError(c, err)
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
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, guestShopMaxRequestBodyBytes)
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

func writeGuestShopJSONError(c *gin.Context, err error) {
	maxBytesErr := new(http.MaxBytesError)
	if errors.As(err, &maxBytesErr) {
		response.ErrorWithDetails(
			c,
			http.StatusRequestEntityTooLarge,
			guestShopRequestTooLargeMessage,
			guestShopRequestTooLargeReason,
			nil,
		)
		return
	}
	response.BadRequest(c, "Invalid request: "+err.Error())
}

var errRequestMustBeSingleObject = jsonSingleObjectError{}

type jsonSingleObjectError struct{}

func (jsonSingleObjectError) Error() string {
	return "request body must contain a single JSON object"
}
