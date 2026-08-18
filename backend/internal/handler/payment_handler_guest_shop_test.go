//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGuestShopReadLimiterBlocksAfterLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := newCafeCouponLookupLimiter(1, time.Minute)
	h := &PaymentHandler{guestShopReadLimiter: limiter}

	first := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(first)
	firstCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/payment/public/shop/config", nil)
	firstCtx.Request.RemoteAddr = "192.0.2.8:1234"
	require.True(t, h.allowGuestShopPublic(firstCtx, limiter))

	second := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(second)
	secondCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/payment/public/shop/config", nil)
	secondCtx.Request.RemoteAddr = "192.0.2.8:1234"
	h.GetGuestShopConfig(secondCtx)
	require.Equal(t, http.StatusTooManyRequests, second.Code)
	require.Contains(t, second.Body.String(), "GUEST_SHOP_RATE_LIMITED")
}

func TestGuestShopReadLimiterUsesForwardedClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := newCafeCouponLookupLimiter(1, time.Minute)
	h := &PaymentHandler{guestShopReadLimiter: limiter}

	for _, forwardedIP := range []string{"192.0.2.8", "192.0.2.9"} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/payment/public/shop/config", nil)
		ctx.Request.RemoteAddr = "127.0.0.1:1234"
		ctx.Request.Header.Set("X-Forwarded-For", forwardedIP)
		require.True(t, h.allowGuestShopPublic(ctx, limiter))
	}
}

func TestGuestShopCreateEndpointUsesIndependentLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &PaymentHandler{
		guestShopReadLimiter:   newCafeCouponLookupLimiter(1, time.Minute),
		guestShopCreateLimiter: newCafeCouponLookupLimiter(1, time.Minute),
	}
	const remoteAddr = "192.0.2.8:1234"
	require.True(t, h.guestShopReadLimiter.allow("192.0.2.8", time.Now()))

	first := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(first)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/payment/public/shop/payments", strings.NewReader("not-json"))
	firstCtx.Request.RemoteAddr = remoteAddr
	h.CreateGuestShopPayment(firstCtx)
	require.Equal(t, http.StatusBadRequest, first.Code, "exhausted read limiter must not block create")

	second := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(second)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/payment/public/shop/payments", strings.NewReader("not-json"))
	secondCtx.Request.RemoteAddr = remoteAddr
	h.CreateGuestShopPayment(secondCtx)
	require.Equal(t, http.StatusTooManyRequests, second.Code)
	require.Contains(t, second.Body.String(), "GUEST_SHOP_RATE_LIMITED")
}

func TestGuestShopCreateEndpointRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &PaymentHandler{guestShopCreateLimiter: newCafeCouponLookupLimiter(1, time.Minute)}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body := `{"items":[{"id":"blazer","qty":1}],"shipping":"air","customer":{"name":"Ada","email":"ada@example.com","address":"` +
		strings.Repeat("a", guestShopMaxRequestBodyBytes) +
		`","city":"Provo","state":"UT","zip":"84601","country":"United States"}}`
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/payment/public/shop/payments", strings.NewReader(body))
	ctx.Request.RemoteAddr = "192.0.2.8:1234"

	h.CreateGuestShopPayment(ctx)
	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	require.Contains(t, recorder.Body.String(), guestShopRequestTooLargeReason)
}

func TestBindStrictGuestShopJSONRejectsUnknownAndTrailingObjects(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []string{
		`{"payment_intent_id":"pi_test","unknown":true}`,
		`{"payment_intent_id":"pi_test"} {"payment_intent_id":"pi_other"}`,
	}
	for _, body := range tests {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/payment/public/shop/payments/status", strings.NewReader(body))
		var req guestShopStatusRequest
		require.Error(t, bindStrictGuestShopJSON(ctx, &req))
	}
}

func TestGuestShopRouteCookieBindsTokenToPaymentIntent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const intentID = "pi_cookie_guest_shop"
	const token = "eyJ2IjoxfQ.signature"
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, guestShopRouteCookiePath, nil)
	ctx.Request.Header.Set("X-Forwarded-Proto", "https")

	setGuestShopRouteCookie(ctx, " "+intentID+" ", " "+token+" ")
	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, guestShopRouteCookieName(intentID), cookies[0].Name)
	require.Equal(t, token, cookies[0].Value)
	require.Equal(t, guestShopRouteCookiePath, cookies[0].Path)
	require.True(t, cookies[0].HttpOnly)
	require.True(t, cookies[0].Secure)
	require.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
	require.Equal(t, int(service.GuestShopRouteTokenTTL.Seconds()), cookies[0].MaxAge)

	readRecorder := httptest.NewRecorder()
	readCtx, _ := gin.CreateTestContext(readRecorder)
	readCtx.Request = httptest.NewRequest(http.MethodPost, guestShopRouteCookiePath, nil)
	readCtx.Request.AddCookie(cookies[0])
	require.Equal(t, token, guestShopRouteTokenFromCookie(readCtx, " "+intentID+" "))
}
