package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingHandler_UpdateSettings_FrontendURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const currentURL = "https://current.example.com"
	for _, tt := range []struct {
		name   string
		body   string
		want   string
		status int
	}{
		{"omitted preserves current domain", `{"site_name":"Updated name"}`, currentURL, http.StatusOK},
		{"null preserves current domain", `{"frontend_url":null}`, currentURL, http.StatusOK},
		{"explicit edit is trimmed", `{"frontend_url":" https://new.example.com "}`, "https://new.example.com", http.StatusOK},
		{"explicit empty string clears override", `{"frontend_url":""}`, "", http.StatusOK},
		{"invalid edit does not write settings", `{"frontend_url":"javascript:alert(1)"}`, currentURL, http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &settingHandlerRepoStub{values: map[string]string{
				service.SettingKeyFrontendURL: currentURL,
			}}
			svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
			handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewBufferString(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")

			handler.UpdateSettings(c)

			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			require.Equal(t, tt.want, repo.values[service.SettingKeyFrontendURL])
			if tt.status != http.StatusOK {
				require.Nil(t, repo.lastUpdates)
				return
			}
			var resp response.Response
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			data, ok := resp.Data.(map[string]any)
			require.True(t, ok)
			require.Equal(t, tt.want, data["frontend_url"])
		})
	}
}
