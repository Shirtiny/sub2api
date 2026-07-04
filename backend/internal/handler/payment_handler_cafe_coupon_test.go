//go:build unit

package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCafeCouponLookupLimiterBlocksAfterLimitAndResetsWindow(t *testing.T) {
	limiter := newCafeCouponLookupLimiter(2, time.Minute)
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

	require.True(t, limiter.allow("user-1:ip-1", now))
	require.True(t, limiter.allow("user-1:ip-1", now.Add(10*time.Second)))
	require.False(t, limiter.allow("user-1:ip-1", now.Add(20*time.Second)))
	require.True(t, limiter.allow("user-2:ip-1", now.Add(20*time.Second)))
	require.True(t, limiter.allow("user-1:ip-1", now.Add(time.Minute)))
}

func TestCafeCouponLookupLimiterCapsBucketGrowth(t *testing.T) {
	limiter := newCafeCouponLookupLimiter(2, time.Minute)
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	stale := now.Add(-cafeCouponLookupCleanupAge - time.Second)

	for i := 0; i < cafeCouponLookupMaxBuckets; i++ {
		limiter.buckets[fmt.Sprintf("stale-%d", i)] = cafeCouponLookupBucket{windowStart: stale, count: 1, lastSeen: stale}
	}
	require.True(t, limiter.allow("fresh-after-cleanup", now))
	require.Len(t, limiter.buckets, 1)

	for i := 0; i < cafeCouponLookupMaxBuckets-1; i++ {
		limiter.buckets[fmt.Sprintf("fresh-%d", i)] = cafeCouponLookupBucket{windowStart: now, count: 1, lastSeen: now}
	}
	require.False(t, limiter.allow("overflow", now))
	require.True(t, limiter.allow("fresh-after-cleanup", now.Add(10*time.Second)))
}

func TestCafeCouponInfoReturnsRateLimitedBeforeServiceLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := newCafeCouponLookupLimiter(1, time.Minute)
	now := time.Now()
	require.True(t, limiter.allow("1:192.0.2.1", now))

	h := &PaymentHandler{cafeCouponLookupLimiter: limiter}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 1})
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/payment/cafe-coupons/info", bytes.NewBufferString(`{"code":"CAFE-LOOKUP"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.CafeCouponInfo(ctx)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Contains(t, recorder.Body.String(), "CAFE_COUPON_LOOKUP_RATE_LIMITED")
}
