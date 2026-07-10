package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gatewayModelsResponseForTest struct {
	Object string         `json:"object"`
	Data   []openai.Model `json:"data"`
}

func TestGatewayModels_ReturnsStaticOpenAICatalogWithoutAPIKeyContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)

	(&GatewayHandler{}).Models(c)

	assertStaticOpenAIModelsResponse(t, rec)
}

func TestGatewayModels_IgnoresAPIKeyGroupAndModelsListConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		apiKey *service.APIKey
	}{
		{
			name: "OpenAI mapping and custom list",
			apiKey: &service.APIKey{
				Group: &service.Group{
					Platform: service.PlatformOpenAI,
					ModelsListConfig: service.GroupModelsListConfig{
						Enabled: true,
						Models:  []string{"gpt-5.4"},
					},
				},
			},
		},
		{
			name: "Gemini group",
			apiKey: &service.APIKey{
				Group: &service.Group{Platform: service.PlatformGemini},
			},
		},
		{
			name: "Anthropic group",
			apiKey: &service.APIKey{
				Group: &service.Group{Platform: service.PlatformAnthropic},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			c.Set(string(middleware2.ContextKeyAPIKey), tt.apiKey)

			(&GatewayHandler{}).Models(c)

			assertStaticOpenAIModelsResponse(t, rec)
		})
	}
}

func assertStaticOpenAIModelsResponse(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "list", got.Object)
	require.Equal(t, openai.DefaultModels, got.Data)

	modelIDs := make(map[string]struct{}, len(got.Data))
	for _, model := range got.Data {
		modelIDs[model.ID] = struct{}{}
	}
	for _, modelID := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
		_, ok := modelIDs[modelID]
		require.Truef(t, ok, "static catalog must include %s", modelID)
	}
}
