package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type settingHandlerRepoStub struct {
	values      map[string]string
	lastUpdates map[string]string
}

func (s *settingHandlerRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *settingHandlerRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if s.values != nil {
		if value, ok := s.values[key]; ok {
			return value, nil
		}
	}
	return "", nil
}

func (s *settingHandlerRepoStub) Set(ctx context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func (s *settingHandlerRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *settingHandlerRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	s.lastUpdates = make(map[string]string, len(settings))
	for key, value := range settings {
		s.lastUpdates[key] = value
		if s.values == nil {
			s.values = map[string]string{}
		}
		s.values[key] = value
	}
	return nil
}

func (s *settingHandlerRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *settingHandlerRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

type failingAuthSourceSettingsRepoStub struct {
	values map[string]string
	err    error
}

func (s *failingAuthSourceSettingsRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *failingAuthSourceSettingsRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *failingAuthSourceSettingsRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *failingAuthSourceSettingsRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *failingAuthSourceSettingsRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	if _, ok := settings[service.SettingKeyAuthSourceDefaultEmailBalance]; ok {
		return s.err
	}
	for key, value := range settings {
		if s.values == nil {
			s.values = map[string]string{}
		}
		s.values[key] = value
	}
	return nil
}

func (s *failingAuthSourceSettingsRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *failingAuthSourceSettingsRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSettingHandler_GetSettings_InjectsAuthSourceDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyRegistrationEnabled:                 "true",
			service.SettingKeyPromoCodeEnabled:                    "true",
			service.SettingKeyAuthSourceDefaultEmailBalance:       "9.5",
			service.SettingKeyAuthSourceDefaultEmailConcurrency:   "8",
			service.SettingKeyAuthSourceDefaultEmailSubscriptions: `[{"group_id":31,"validity_days":15}]`,
			service.SettingKeyForceEmailOnThirdPartySignup:        "true",
		},
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)

	handler.GetSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, 9.5, data["auth_source_default_email_balance"])
	require.Equal(t, float64(8), data["auth_source_default_email_concurrency"])
	require.Equal(t, true, data["force_email_on_third_party_signup"])

	subscriptions, ok := data["auth_source_default_email_subscriptions"].([]any)
	require.True(t, ok)
	require.Len(t, subscriptions, 1)
}

func TestSettingHandler_UpdateSettings_PreservesOmittedAuthSourceDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyRegistrationEnabled:                    "false",
			service.SettingKeyPromoCodeEnabled:                       "true",
			service.SettingKeyAuthSourceDefaultEmailBalance:          "9.5",
			service.SettingKeyAuthSourceDefaultEmailConcurrency:      "8",
			service.SettingKeyAuthSourceDefaultEmailSubscriptions:    `[{"group_id":31,"validity_days":15}]`,
			service.SettingKeyAuthSourceDefaultEmailGrantOnSignup:    "true",
			service.SettingKeyAuthSourceDefaultEmailGrantOnFirstBind: "false",
			service.SettingKeyForceEmailOnThirdPartySignup:           "true",
		},
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"registration_enabled":              true,
		"promo_code_enabled":                true,
		"auth_source_default_email_balance": 12.75,
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "12.75000000", repo.values[service.SettingKeyAuthSourceDefaultEmailBalance])
	require.Equal(t, "8", repo.values[service.SettingKeyAuthSourceDefaultEmailConcurrency])
	require.Equal(t, `[{"group_id":31,"validity_days":15}]`, repo.values[service.SettingKeyAuthSourceDefaultEmailSubscriptions])
	require.Equal(t, "true", repo.values[service.SettingKeyForceEmailOnThirdPartySignup])

	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, 12.75, data["auth_source_default_email_balance"])
	require.Equal(t, float64(8), data["auth_source_default_email_concurrency"])
	require.Equal(t, true, data["force_email_on_third_party_signup"])
}

func TestSettingHandler_UpdateSettings_PersistsPaymentVisibleMethodsAndAdvancedScheduler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyPromoCodeEnabled: "true",
		},
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"promo_code_enabled":                    true,
		"payment_visible_method_alipay_source":  "easypay",
		"payment_visible_method_wxpay_source":   "wxpay",
		"payment_visible_method_alipay_enabled": true,
		"payment_visible_method_wxpay_enabled":  false,
		"openai_advanced_scheduler_enabled":     true,
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, service.VisibleMethodSourceEasyPayAlipay, repo.values[service.SettingPaymentVisibleMethodAlipaySource])
	require.Equal(t, service.VisibleMethodSourceOfficialWechat, repo.values[service.SettingPaymentVisibleMethodWxpaySource])
	require.Equal(t, "true", repo.values[service.SettingPaymentVisibleMethodAlipayEnabled])
	require.Equal(t, "false", repo.values[service.SettingPaymentVisibleMethodWxpayEnabled])
	require.Equal(t, "true", repo.values["openai_advanced_scheduler_enabled"])

	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, service.VisibleMethodSourceEasyPayAlipay, data["payment_visible_method_alipay_source"])
	require.Equal(t, service.VisibleMethodSourceOfficialWechat, data["payment_visible_method_wxpay_source"])
	require.Equal(t, true, data["payment_visible_method_alipay_enabled"])
	require.Equal(t, false, data["payment_visible_method_wxpay_enabled"])
	require.Equal(t, true, data["openai_advanced_scheduler_enabled"])
}

func TestSettingHandler_UpdateSettings_PreservesLegacyBlankPaymentVisibleMethodSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyPromoCodeEnabled:               "true",
			service.SettingPaymentVisibleMethodAlipayEnabled: "true",
			service.SettingPaymentVisibleMethodAlipaySource:  "",
			service.SettingPaymentVisibleMethodWxpayEnabled:  "false",
			service.SettingPaymentVisibleMethodWxpaySource:   "",
		},
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"promo_code_enabled": false,
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "", repo.values[service.SettingPaymentVisibleMethodAlipaySource])
	require.Equal(t, "true", repo.values[service.SettingPaymentVisibleMethodAlipayEnabled])
}

func TestSettingHandler_UpdateSettings_PersistsExplicitFalseOIDCCompatibilityFlags(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyPromoCodeEnabled:               "true",
			service.SettingKeyOIDCConnectEnabled:             "true",
			service.SettingKeyOIDCConnectProviderName:        "OIDC",
			service.SettingKeyOIDCConnectClientID:            "oidc-client",
			service.SettingKeyOIDCConnectClientSecret:        "oidc-secret",
			service.SettingKeyOIDCConnectIssuerURL:           "https://issuer.example.com",
			service.SettingKeyOIDCConnectAuthorizeURL:        "https://issuer.example.com/auth",
			service.SettingKeyOIDCConnectTokenURL:            "https://issuer.example.com/token",
			service.SettingKeyOIDCConnectUserInfoURL:         "https://issuer.example.com/userinfo",
			service.SettingKeyOIDCConnectJWKSURL:             "https://issuer.example.com/jwks",
			service.SettingKeyOIDCConnectScopes:              "openid email profile",
			service.SettingKeyOIDCConnectRedirectURL:         "https://example.com/api/v1/auth/oauth/oidc/callback",
			service.SettingKeyOIDCConnectFrontendRedirectURL: "/auth/oidc/callback",
			service.SettingKeyOIDCConnectTokenAuthMethod:     "client_secret_post",
			service.SettingKeyOIDCConnectUsePKCE:             "true",
			service.SettingKeyOIDCConnectValidateIDToken:     "true",
			service.SettingKeyOIDCConnectAllowedSigningAlgs:  "RS256",
			service.SettingKeyOIDCConnectClockSkewSeconds:    "120",
		},
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"promo_code_enabled":                true,
		"oidc_connect_enabled":              true,
		"oidc_connect_use_pkce":             false,
		"oidc_connect_validate_id_token":    false,
		"oidc_connect_allowed_signing_algs": "",
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyOIDCConnectUsePKCE])
	require.Equal(t, "false", repo.values[service.SettingKeyOIDCConnectValidateIDToken])

	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, data["oidc_connect_use_pkce"])
	require.Equal(t, false, data["oidc_connect_validate_id_token"])
}

func TestSettingHandler_UpdateSettings_DoesNotSolidifyImplicitOIDCSecurityDefaultsOnLegacyUpgrade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyPromoCodeEnabled:                "true",
			service.SettingKeyOIDCConnectEnabled:              "true",
			service.SettingKeyOIDCConnectProviderName:         "OIDC",
			service.SettingKeyOIDCConnectClientID:             "oidc-client",
			service.SettingKeyOIDCConnectClientSecret:         "oidc-secret",
			service.SettingKeyOIDCConnectIssuerURL:            "https://issuer.example.com",
			service.SettingKeyOIDCConnectAuthorizeURL:         "https://issuer.example.com/auth",
			service.SettingKeyOIDCConnectTokenURL:             "https://issuer.example.com/token",
			service.SettingKeyOIDCConnectUserInfoURL:          "https://issuer.example.com/userinfo",
			service.SettingKeyOIDCConnectJWKSURL:              "https://issuer.example.com/jwks",
			service.SettingKeyOIDCConnectScopes:               "openid email profile",
			service.SettingKeyOIDCConnectRedirectURL:          "https://example.com/api/v1/auth/oauth/oidc/callback",
			service.SettingKeyOIDCConnectFrontendRedirectURL:  "/auth/oidc/callback",
			service.SettingKeyOIDCConnectTokenAuthMethod:      "client_secret_post",
			service.SettingKeyOIDCConnectAllowedSigningAlgs:   "RS256",
			service.SettingKeyOIDCConnectClockSkewSeconds:     "120",
			service.SettingKeyOIDCConnectRequireEmailVerified: "false",
			service.SettingKeyOIDCConnectUserInfoEmailPath:    "",
			service.SettingKeyOIDCConnectUserInfoIDPath:       "",
			service.SettingKeyOIDCConnectUserInfoUsernamePath: "",
		},
	}
	svc := service.NewSettingService(repo, &config.Config{
		Default: config.DefaultConfig{UserConcurrency: 5},
		OIDC: config.OIDCConnectConfig{
			Enabled:             true,
			ProviderName:        "OIDC",
			ClientID:            "oidc-client",
			ClientSecret:        "oidc-secret",
			IssuerURL:           "https://issuer.example.com",
			AuthorizeURL:        "https://issuer.example.com/auth",
			TokenURL:            "https://issuer.example.com/token",
			UserInfoURL:         "https://issuer.example.com/userinfo",
			JWKSURL:             "https://issuer.example.com/jwks",
			Scopes:              "openid email profile",
			RedirectURL:         "https://example.com/api/v1/auth/oauth/oidc/callback",
			FrontendRedirectURL: "/auth/oidc/callback",
			TokenAuthMethod:     "client_secret_post",
			UsePKCE:             true,
			ValidateIDToken:     true,
			AllowedSigningAlgs:  "RS256",
			ClockSkewSeconds:    120,
		},
	})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"promo_code_enabled":   true,
		"oidc_connect_enabled": true,
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyOIDCConnectUsePKCE])
	require.Equal(t, "false", repo.values[service.SettingKeyOIDCConnectValidateIDToken])
}

func TestSettingHandler_UpdateSettings_RejectsInvalidPaymentVisibleMethodSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyPromoCodeEnabled: "true",
		},
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"promo_code_enabled":                   true,
		"payment_visible_method_alipay_source": "bogus",
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NotContains(t, repo.values, service.SettingPaymentVisibleMethodAlipaySource)
}

func TestSettingHandler_UpdateSettings_DoesNotPersistPartialSystemSettingsWhenAuthSourceDefaultsFail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &failingAuthSourceSettingsRepoStub{
		values: map[string]string{
			service.SettingKeyRegistrationEnabled:                 "false",
			service.SettingKeyPromoCodeEnabled:                    "true",
			service.SettingKeyAuthSourceDefaultEmailBalance:       "9.5",
			service.SettingKeyAuthSourceDefaultEmailConcurrency:   "8",
			service.SettingKeyAuthSourceDefaultEmailSubscriptions: `[{"group_id":31,"validity_days":15}]`,
		},
		err: errors.New("write auth source defaults failed"),
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"registration_enabled":              true,
		"promo_code_enabled":                true,
		"auth_source_default_email_balance": 12.75,
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyRegistrationEnabled])
	require.Equal(t, "9.5", repo.values[service.SettingKeyAuthSourceDefaultEmailBalance])
}

func TestSettingHandler_UpdateSettings_PersistsAffiliateInviteLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyRegistrationEnabled:      "true",
			service.SettingKeyPromoCodeEnabled:         "true",
			service.SettingKeyAffiliateInviteLimit:     "0",
			service.SettingKeyDefaultConcurrency:       "5",
			service.SettingKeyDefaultBalance:           "0",
			service.SettingKeyDefaultSubscriptions:     "[]",
			service.SettingKeyTableDefaultPageSize:     "20",
			service.SettingKeyTablePageSizeOptions:     "[10,20,50,100]",
			service.SettingPaymentEnabled:              "false",
			service.SettingEnabledPaymentTypes:         "[]",
			service.SettingKeyAccountQuotaNotifyEmails: "[]",
		},
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"promo_code_enabled":     true,
		"affiliate_invite_limit": 123,
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	putRec := httptest.NewRecorder()
	putCtx, _ := gin.CreateTestContext(putRec)
	putCtx.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	putCtx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(putCtx)

	require.Equal(t, http.StatusOK, putRec.Code)
	require.Equal(t, "123", repo.values[service.SettingKeyAffiliateInviteLimit])

	var putResp response.Response
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &putResp))
	putData, ok := putResp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(123), putData["affiliate_invite_limit"])

	getRec := httptest.NewRecorder()
	getCtx, _ := gin.CreateTestContext(getRec)
	getCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)

	handler.GetSettings(getCtx)

	require.Equal(t, http.StatusOK, getRec.Code)
	var getResp response.Response
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	getData, ok := getResp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(123), getData["affiliate_invite_limit"])
}

func TestSettingHandler_UpdateSettings_PersistsCafeCouponConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyRegistrationEnabled:                "true",
			service.SettingKeyPromoCodeEnabled:                   "true",
			service.SettingKeyCafeCouponConfig:                   `{"levels":{"1":{"enabled":false,"type":"cash","value":0,"period":"month"}}}`,
			service.SettingKeyAffiliateRebatePerInviteeCapLevel1: "100.00000000",
			service.SettingKeyAffiliateRebatePerInviteeCapLevel2: "300.00000000",
			service.SettingKeyAffiliateRebatePerInviteeCapLevel3: "1000.00000000",
			service.SettingKeyDefaultConcurrency:                 "5",
			service.SettingKeyDefaultBalance:                     "0",
			service.SettingKeyDefaultSubscriptions:               "[]",
			service.SettingKeyTableDefaultPageSize:               "20",
			service.SettingKeyTablePageSizeOptions:               "[10,20,50,100]",
			service.SettingPaymentEnabled:                        "false",
			service.SettingEnabledPaymentTypes:                   "[]",
			service.SettingKeyAccountQuotaNotifyEmails:           "[]",
		},
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"promo_code_enabled": true,
		"cafe_coupon_config": map[string]any{
			"levels": map[string]any{
				"1": map[string]any{
					"enabled":      true,
					"type":         "cash",
					"value":        12.5,
					"period":       "month",
					"transferable": true,
				},
			},
		},
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	putRec := httptest.NewRecorder()
	putCtx, _ := gin.CreateTestContext(putRec)
	putCtx.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	putCtx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(putCtx)

	require.Equal(t, http.StatusOK, putRec.Code)
	var saved service.CafeCouponConfig
	require.NoError(t, json.Unmarshal([]byte(repo.values[service.SettingKeyCafeCouponConfig]), &saved))
	require.True(t, saved.Levels[1].Enabled)
	require.Equal(t, 12.5, saved.Levels[1].Value)
	require.True(t, saved.Levels[1].Transferable)
	require.Equal(t, "100.00000000", repo.values[service.SettingKeyAffiliateRebatePerInviteeCapLevel1])
	require.Equal(t, "300.00000000", repo.values[service.SettingKeyAffiliateRebatePerInviteeCapLevel2])
	require.Equal(t, "1000.00000000", repo.values[service.SettingKeyAffiliateRebatePerInviteeCapLevel3])

	var putResp response.Response
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &putResp))
	putData, ok := putResp.Data.(map[string]any)
	require.True(t, ok)
	require.Contains(t, putData, "cafe_coupon_config")
	require.Equal(t, float64(100), putData["affiliate_rebate_per_invitee_cap_level1"])
	require.Equal(t, float64(300), putData["affiliate_rebate_per_invitee_cap_level2"])
	require.Equal(t, float64(1000), putData["affiliate_rebate_per_invitee_cap_level3"])
}

func TestSettingHandler_UpdateSettings_RestoresAffiliateCapLevels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyRegistrationEnabled:                "true",
			service.SettingKeyPromoCodeEnabled:                   "true",
			service.SettingKeyCafeCouponConfig:                   `{"levels":{"1":{"enabled":false,"type":"cash","value":0,"period":"month"}}}`,
			service.SettingKeyAffiliateRebatePerInviteeCap:       "0.00000000",
			service.SettingKeyAffiliateRebatePerInviteeCapLevel0: "0.00000000",
			service.SettingKeyAffiliateRebatePerInviteeCapLevel1: "0.00000000",
			service.SettingKeyAffiliateRebatePerInviteeCapLevel2: "0.00000000",
			service.SettingKeyAffiliateRebatePerInviteeCapLevel3: "0.00000000",
			service.SettingKeyDefaultConcurrency:                 "5",
			service.SettingKeyDefaultBalance:                     "0",
			service.SettingKeyDefaultSubscriptions:               "[]",
			service.SettingKeyTableDefaultPageSize:               "20",
			service.SettingKeyTablePageSizeOptions:               "[10,20,50,100]",
			service.SettingPaymentEnabled:                        "false",
			service.SettingEnabledPaymentTypes:                   "[]",
			service.SettingKeyAccountQuotaNotifyEmails:           "[]",
		},
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"promo_code_enabled":                      true,
		"affiliate_rebate_per_invitee_cap":        0,
		"affiliate_rebate_per_invitee_cap_level0": 0,
		"affiliate_rebate_per_invitee_cap_level1": 100,
		"affiliate_rebate_per_invitee_cap_level2": 300,
		"affiliate_rebate_per_invitee_cap_level3": 1000,
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	putRec := httptest.NewRecorder()
	putCtx, _ := gin.CreateTestContext(putRec)
	putCtx.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	putCtx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(putCtx)

	require.Equal(t, http.StatusOK, putRec.Code)
	require.Equal(t, "0.00000000", repo.values[service.SettingKeyAffiliateRebatePerInviteeCapLevel0])
	require.Equal(t, "100.00000000", repo.values[service.SettingKeyAffiliateRebatePerInviteeCapLevel1])
	require.Equal(t, "300.00000000", repo.values[service.SettingKeyAffiliateRebatePerInviteeCapLevel2])
	require.Equal(t, "1000.00000000", repo.values[service.SettingKeyAffiliateRebatePerInviteeCapLevel3])

	var putResp response.Response
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &putResp))
	putData, ok := putResp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(0), putData["affiliate_rebate_per_invitee_cap_level0"])
	require.Equal(t, float64(100), putData["affiliate_rebate_per_invitee_cap_level1"])
	require.Equal(t, float64(300), putData["affiliate_rebate_per_invitee_cap_level2"])
	require.Equal(t, float64(1000), putData["affiliate_rebate_per_invitee_cap_level3"])

	getRec := httptest.NewRecorder()
	getCtx, _ := gin.CreateTestContext(getRec)
	getCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)

	handler.GetSettings(getCtx)

	require.Equal(t, http.StatusOK, getRec.Code)
	var getResp response.Response
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	getData, ok := getResp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(0), getData["affiliate_rebate_per_invitee_cap_level0"])
	require.Equal(t, float64(100), getData["affiliate_rebate_per_invitee_cap_level1"])
	require.Equal(t, float64(300), getData["affiliate_rebate_per_invitee_cap_level2"])
	require.Equal(t, float64(1000), getData["affiliate_rebate_per_invitee_cap_level3"])
}

func TestSettingHandler_UpdateSettings_PersistsAffiliateCapLevelsFromFullSettingsPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyRegistrationEnabled:                "true",
			service.SettingKeyPromoCodeEnabled:                   "true",
			service.SettingKeyCafeCouponConfig:                   `{"levels":{"1":{"enabled":false,"type":"cash","value":0,"period":"month"}}}`,
			service.SettingKeyAffiliateRebatePerInviteeCap:       "0.00000000",
			service.SettingKeyAffiliateRebatePerInviteeCapLevel0: "0.00000000",
			service.SettingKeyAffiliateRebatePerInviteeCapLevel1: "0.00000000",
			service.SettingKeyAffiliateRebatePerInviteeCapLevel2: "0.00000000",
			service.SettingKeyAffiliateRebatePerInviteeCapLevel3: "0.00000000",
			service.SettingKeyDefaultConcurrency:                 "5",
			service.SettingKeyDefaultBalance:                     "0",
			service.SettingKeyDefaultSubscriptions:               "[]",
			service.SettingKeyTableDefaultPageSize:               "20",
			service.SettingKeyTablePageSizeOptions:               "[10,20,50,100]",
			service.SettingPaymentEnabled:                        "false",
			service.SettingEnabledPaymentTypes:                   "[]",
			service.SettingKeyAccountQuotaNotifyEmails:           "[]",
		},
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	getRec := httptest.NewRecorder()
	getCtx, _ := gin.CreateTestContext(getRec)
	getCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)

	handler.GetSettings(getCtx)

	require.Equal(t, http.StatusOK, getRec.Code)
	var getResp response.Response
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	body, ok := getResp.Data.(map[string]any)
	require.True(t, ok)
	body["affiliate_rebate_per_invitee_cap"] = 0
	body["affiliate_rebate_per_invitee_cap_level0"] = 0
	body["affiliate_rebate_per_invitee_cap_level1"] = 100
	body["affiliate_rebate_per_invitee_cap_level2"] = 300
	body["affiliate_rebate_per_invitee_cap_level3"] = 1000
	body["cafe_coupon_config"] = map[string]any{
		"levels": map[string]any{
			"1": map[string]any{
				"enabled":      true,
				"type":         "cash",
				"value":        12.5,
				"period":       "month",
				"transferable": true,
			},
		},
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	putRec := httptest.NewRecorder()
	putCtx, _ := gin.CreateTestContext(putRec)
	putCtx.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	putCtx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(putCtx)

	require.Equal(t, http.StatusOK, putRec.Code)
	require.Equal(t, "0.00000000", repo.values[service.SettingKeyAffiliateRebatePerInviteeCapLevel0])
	require.Equal(t, "100.00000000", repo.values[service.SettingKeyAffiliateRebatePerInviteeCapLevel1])
	require.Equal(t, "300.00000000", repo.values[service.SettingKeyAffiliateRebatePerInviteeCapLevel2])
	require.Equal(t, "1000.00000000", repo.values[service.SettingKeyAffiliateRebatePerInviteeCapLevel3])
	var saved service.CafeCouponConfig
	require.NoError(t, json.Unmarshal([]byte(repo.values[service.SettingKeyCafeCouponConfig]), &saved))
	require.True(t, saved.Levels[1].Enabled)
	require.Equal(t, 12.5, saved.Levels[1].Value)
	require.True(t, saved.Levels[1].Transferable)

	persisted, err := svc.GetAllSettings(putCtx.Request.Context())
	require.NoError(t, err)
	require.Equal(t, float64(0), persisted.AffiliateRebatePerInviteeCapLevel0)
	require.Equal(t, float64(100), persisted.AffiliateRebatePerInviteeCapLevel1)
	require.Equal(t, float64(300), persisted.AffiliateRebatePerInviteeCapLevel2)
	require.Equal(t, float64(1000), persisted.AffiliateRebatePerInviteeCapLevel3)
	require.True(t, persisted.CafeCouponConfig.Levels[1].Enabled)
	require.Equal(t, 12.5, persisted.CafeCouponConfig.Levels[1].Value)
	require.True(t, persisted.CafeCouponConfig.Levels[1].Transferable)

	var putResp response.Response
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &putResp))
	putData, ok := putResp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(0), putData["affiliate_rebate_per_invitee_cap_level0"])
	require.Equal(t, float64(100), putData["affiliate_rebate_per_invitee_cap_level1"])
	require.Equal(t, float64(300), putData["affiliate_rebate_per_invitee_cap_level2"])
	require.Equal(t, float64(1000), putData["affiliate_rebate_per_invitee_cap_level3"])

	followRec := httptest.NewRecorder()
	followCtx, _ := gin.CreateTestContext(followRec)
	followCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)

	handler.GetSettings(followCtx)

	require.Equal(t, http.StatusOK, followRec.Code)
	var followResp response.Response
	require.NoError(t, json.Unmarshal(followRec.Body.Bytes(), &followResp))
	followData, ok := followResp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(0), followData["affiliate_rebate_per_invitee_cap_level0"])
	require.Equal(t, float64(100), followData["affiliate_rebate_per_invitee_cap_level1"])
	require.Equal(t, float64(300), followData["affiliate_rebate_per_invitee_cap_level2"])
	require.Equal(t, float64(1000), followData["affiliate_rebate_per_invitee_cap_level3"])
}

func TestDiffSettings_IncludesAuthSourceDefaultsAndForceEmail(t *testing.T) {
	changed := diffSettings(
		&service.SystemSettings{},
		&service.SystemSettings{},
		&service.AuthSourceDefaultSettings{
			Email: service.ProviderDefaultGrantSettings{
				Balance:          0,
				Concurrency:      5,
				Subscriptions:    nil,
				GrantOnSignup:    true,
				GrantOnFirstBind: false,
			},
			ForceEmailOnThirdPartySignup: false,
		},
		&service.AuthSourceDefaultSettings{
			Email: service.ProviderDefaultGrantSettings{
				Balance:          12.5,
				Concurrency:      7,
				Subscriptions:    []service.DefaultSubscriptionSetting{{GroupID: 21, ValidityDays: 30}},
				GrantOnSignup:    false,
				GrantOnFirstBind: true,
			},
			ForceEmailOnThirdPartySignup: true,
		},
		UpdateSettingsRequest{},
	)

	require.Contains(t, changed, "auth_source_default_email_balance")
	require.Contains(t, changed, "auth_source_default_email_concurrency")
	require.Contains(t, changed, "auth_source_default_email_subscriptions")
	require.Contains(t, changed, "auth_source_default_email_grant_on_signup")
	require.Contains(t, changed, "auth_source_default_email_grant_on_first_bind")
	require.Contains(t, changed, "force_email_on_third_party_signup")
}
