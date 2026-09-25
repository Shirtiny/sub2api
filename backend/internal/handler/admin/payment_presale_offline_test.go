package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPresaleOfflineHandlerRequiresServerAdminIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &PaymentHandler{paymentService: &service.PaymentService{}}
	r := gin.New()
	r.POST("/:id/presale-offline", h.ProcessPresaleOffline)
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"/1/presale-offline", `{"mode":"cancel","admin_id":999,"confirmed":true,"reason":"request","expected_updated_at":"2026-09-25T00:00:00Z"}`, http.StatusForbidden},
		{"/1/presale-offline", `{"mode":"cancel","expected_updated_at":"not-a-time"}`, http.StatusBadRequest},
		{"/1/presale-offline", `{`, http.StatusBadRequest},
		{"/abc/presale-offline", `{}`, http.StatusBadRequest},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code, w.Body.String())
	}
}
