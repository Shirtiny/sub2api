package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCommonRoutesPingIsPublicTimingProbe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterCommonRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	req.Header.Set("Origin", "https://console.example.com")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "*", recorder.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "*", recorder.Header().Get("Timing-Allow-Origin"))
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))

	var payload map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "ok", payload["status"])
}
