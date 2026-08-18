//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGuestShopPublicLimiterBlocksAfterLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := newCafeCouponLookupLimiter(1, time.Minute)
	h := &PaymentHandler{guestShopLimiter: limiter}

	first := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(first)
	firstCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/payment/public/shop/config", nil)
	firstCtx.Request.RemoteAddr = "192.0.2.8:1234"
	require.True(t, h.allowGuestShopPublic(firstCtx))

	second := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(second)
	secondCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/payment/public/shop/config", nil)
	secondCtx.Request.RemoteAddr = "192.0.2.8:1234"
	h.GetGuestShopConfig(secondCtx)
	require.Equal(t, http.StatusTooManyRequests, second.Code)
	require.Contains(t, second.Body.String(), "GUEST_SHOP_RATE_LIMITED")
}
