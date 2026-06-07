//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type settingHandlerPublicRepoStub struct {
	values map[string]string
}

func (s *settingHandlerPublicRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *settingHandlerPublicRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingHandlerPublicRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingHandlerPublicRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *settingHandlerPublicRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingHandlerPublicRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingHandlerPublicRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSettingHandler_GetPublicSettings_ExposesAffiliateMembershipSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyAffiliateEnabled:                   "true",
			service.SettingKeyAffiliateRebateRateLevel0:          "0",
			service.SettingKeyAffiliateRebateRateLevel1:          "5",
			service.SettingKeyAffiliateRebateRateLevel2:          "12",
			service.SettingKeyAffiliateRebateRateLevel3:          "20",
			service.SettingKeyAffiliateInviteLimitLevel2:         "3",
			service.SettingKeyAffiliateRebatePerInviteeCapLevel0: "300",
		},
	}
	h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			AffiliateEnabled             bool    `json:"affiliate_enabled"`
			AffiliateRebateRateLevel0    float64 `json:"affiliate_rebate_rate_level0"`
			AffiliateRebateRateLevel1    float64 `json:"affiliate_rebate_rate_level1"`
			AffiliateRebateRateLevel2    float64 `json:"affiliate_rebate_rate_level2"`
			AffiliateRebateRateLevel3    float64 `json:"affiliate_rebate_rate_level3"`
			AffiliateInviteLimitLevel2   int     `json:"affiliate_invite_limit_level2"`
			AffiliateRebatePerInviteeCap float64 `json:"affiliate_rebate_per_invitee_cap"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.AffiliateEnabled)
	require.Equal(t, 0.0, resp.Data.AffiliateRebateRateLevel0)
	require.Equal(t, 5.0, resp.Data.AffiliateRebateRateLevel1)
	require.Equal(t, 12.0, resp.Data.AffiliateRebateRateLevel2)
	require.Equal(t, 20.0, resp.Data.AffiliateRebateRateLevel3)
	require.Equal(t, 3, resp.Data.AffiliateInviteLimitLevel2)
	require.Equal(t, 300.0, resp.Data.AffiliateRebatePerInviteeCap)
}

func TestSettingHandler_GetPublicSettings_ExposesForceEmailOnThirdPartySignup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyForceEmailOnThirdPartySignup: "true",
		},
	}
	h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			ForceEmailOnThirdPartySignup bool `json:"force_email_on_third_party_signup"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.ForceEmailOnThirdPartySignup)
}

func TestSettingHandler_GetPublicSettings_ExposesWeChatOAuthModeCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSettingHandler(service.NewSettingService(&settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyWeChatConnectEnabled:             "true",
			service.SettingKeyWeChatConnectAppID:               "wx-mp-app",
			service.SettingKeyWeChatConnectAppSecret:           "wx-mp-secret",
			service.SettingKeyWeChatConnectMode:                "mp",
			service.SettingKeyWeChatConnectScopes:              "snsapi_base",
			service.SettingKeyWeChatConnectOpenEnabled:         "true",
			service.SettingKeyWeChatConnectMPEnabled:           "true",
			service.SettingKeyWeChatConnectRedirectURL:         "https://api.example.com/api/v1/auth/oauth/wechat/callback",
			service.SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
		},
	}, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			WeChatOAuthEnabled     bool `json:"wechat_oauth_enabled"`
			WeChatOAuthOpenEnabled bool `json:"wechat_oauth_open_enabled"`
			WeChatOAuthMPEnabled   bool `json:"wechat_oauth_mp_enabled"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.WeChatOAuthEnabled)
	require.True(t, resp.Data.WeChatOAuthOpenEnabled)
	require.True(t, resp.Data.WeChatOAuthMPEnabled)
}
