//go:build unit

package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCafeCampaignAdminAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newPromoCafeCouponTestClient(t)
	s := service.NewPaymentService(client, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewPromoHandler(nil, s)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if c.GetHeader("Test-Admin") == "yes" {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 99})
		}
		c.Next()
	})
	r.POST("/campaigns", h.CreateCafeCampaign)
	r.GET("/campaigns", h.ListCafeCampaigns)
	r.PATCH("/campaigns/:id/status", h.SetCafeCampaignEnabled)
	r.GET("/campaigns/:id/usages", h.ListCafeCampaignUses)
	request := func(method, path string, body any, admin bool) *httptest.ResponseRecorder {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		if admin {
			req.Header.Set("Test-Admin", "yes")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	body := map[string]any{"code": "SEP40", "name": "test", "discount_percent": 40, "start_date": "2026-09-25", "end_date": "2026-09-30", "created_by": 999, "admin_id": 999}
	w := request(http.MethodPost, "/campaigns", body, false)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	w = request(http.MethodPost, "/campaigns", body, true)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var result struct {
		Data struct {
			ID        int64     `json:"id"`
			CreatedBy int64     `json:"created_by"`
			Enabled   bool      `json:"enabled"`
			ExpiresAt time.Time `json:"expires_at"`
		}
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Equal(t, int64(99), result.Data.CreatedBy)
	require.False(t, result.Data.Enabled)
	require.Equal(t, "2026-09-30T16:00:00Z", result.Data.ExpiresAt.UTC().Format(time.RFC3339))
	w = request(http.MethodPatch, "/campaigns/1/status", map[string]any{}, true)
	require.Equal(t, http.StatusBadRequest, w.Code)
	body["discount_percent"] = 100
	body["code"] = "BAD"
	w = request(http.MethodPost, "/campaigns", body, true)
	require.Equal(t, http.StatusBadRequest, w.Code)
	w = request(http.MethodGet, "/campaigns?page_size=20", nil, true)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "CAFE-PUBLIC-SEP40")
	w = request(http.MethodGet, "/campaigns/999/usages", nil, true)
	require.Equal(t, http.StatusNotFound, w.Code)
}
