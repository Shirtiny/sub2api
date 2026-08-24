package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDetectCyberPolicyResponse(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		hit    bool
	}{
		{name: "canonical 400", status: 400, body: `{"error":{"code":"cyber_policy","message":"blocked"}}`, hit: true},
		{name: "responses failed", status: 200, body: `{"response":{"error":{"code":"cyber_policy"}}}`, hit: true},
		{name: "400 stable message", status: 400, body: `{"error":{"message":"This content was flagged for possible cybersecurity risk"}}`, hit: true},
		{name: "200 structured error stable message", status: 200, body: `{"error":{"code":"invalid_prompt","message":"This content was flagged for possible cybersecurity risk"}}`, hit: true},
		{name: "200 failed event stable message", status: 200, body: `{"type":"response.failed","response":{"error":{"code":"invalid_prompt","message":"Trusted Access for Cyber: https://chatgpt.com/cyber"}}}`, hit: true},
		{name: "unrelated policy", status: 400, body: `{"error":{"code":"content_policy","message":"blocked"}}`, hit: false},
		{name: "message on success", status: 200, body: `{"message":"possible cybersecurity risk"}`, hit: false},
		{name: "marker in output delta", status: 200, body: `{"type":"response.output_text.delta","delta":"possible cybersecurity risk"}`, hit: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hit, code, _ := DetectCyberPolicyResponse(tc.status, []byte(tc.body))
			require.Equal(t, tc.hit, hit)
			if tc.hit {
				require.Equal(t, ContentModerationActionCyberPolicy, code)
			}
		})
	}
}

func TestRecordCyberPolicyEventUsesIndependentCyberCounter(t *testing.T) {
	repo := &contentModerationTestRepo{}
	userID := int64(42)
	require.NoError(t, repo.CreateLog(context.Background(), &ContentModerationLog{
		UserID:             &userID,
		Action:             ContentModerationActionBlock,
		Flagged:            true,
		SideEffectsApplied: true,
		CreatedAt:          time.Now(),
	}))
	settings := &contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled: "true",
	}}
	svc := NewContentModerationService(settings, repo, nil, nil, nil, nil, nil)

	svc.RecordCyberPolicyEvent(context.Background(), CyberPolicyRecordInput{
		RequestID:       "req-cyber-1",
		UserID:          userID,
		UserEmail:       "user@example.com",
		Endpoint:        "/v1/responses",
		Model:           "gpt-5",
		UpstreamMessage: "cyber policy blocked",
		UpstreamBody:    `{"error":{"code":"cyber_policy"}}`,
		UpstreamStatus:  400,
	})
	svc.RecordCyberPolicyEvent(context.Background(), CyberPolicyRecordInput{
		RequestID: "req-cyber-2", UserID: userID, UserEmail: "user@example.com", Endpoint: "/v1/responses", Model: "gpt-5",
		UpstreamMessage: "blocked again", UpstreamStatus: 400,
	})

	logs := repo.snapshotLogs()
	require.Len(t, logs, 3)
	require.Equal(t, ContentModerationActionCyberPolicy, logs[1].Action)
	require.Equal(t, ContentModerationActionCyberPolicy, logs[1].HighestCategory)
	require.True(t, logs[1].Flagged)
	require.Equal(t, 1.0, logs[1].HighestScore)
	require.Equal(t, 1, logs[1].ViolationCount)
	require.Equal(t, 2, logs[2].ViolationCount)
	require.Contains(t, logs[1].Error, "cyber_policy")
	contentCount, err := repo.CountFlaggedByUserSince(context.Background(), userID, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Equal(t, 1, contentCount)
}

func TestRecordCyberPolicyEventAutoBansAtThreshold(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.BanThreshold = 100
	cfg.CyberPolicyBanThreshold = 2
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationTestRepo{}
	userID := int64(7)
	userRepo := &contentModerationTestUserRepo{user: &User{ID: userID, Role: RoleUser, Status: StatusActive}}
	invalidator := &contentModerationTestAuthCacheInvalidator{}
	settings := &contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled:      "true",
		SettingKeyContentModerationConfig: string(raw),
	}}
	svc := NewContentModerationService(settings, repo, nil, nil, userRepo, invalidator, nil)

	svc.RecordCyberPolicyEvent(context.Background(), CyberPolicyRecordInput{
		UserID: userID, UserEmail: "user@example.com", Endpoint: "/v1/responses", Model: "gpt-5",
		UpstreamMessage: "blocked", UpstreamStatus: 400,
	})
	require.Equal(t, StatusActive, userRepo.user.Status)
	svc.RecordCyberPolicyEvent(context.Background(), CyberPolicyRecordInput{
		UserID: userID, UserEmail: "user@example.com", Endpoint: "/v1/responses", Model: "gpt-5",
		UpstreamMessage: "blocked again", UpstreamStatus: 400,
	})

	logs := repo.snapshotLogs()
	require.Len(t, logs, 2)
	require.False(t, logs[0].AutoBanned)
	require.True(t, logs[1].AutoBanned)
	require.Equal(t, 2, logs[1].ViolationCount)
	require.Equal(t, StatusDisabled, userRepo.user.Status)
	require.Equal(t, []int64{userID}, invalidator.userIDs)
}

func TestCyberPolicyConfigRoundTrip(t *testing.T) {
	cfg := defaultContentModerationConfig()
	require.True(t, cfg.CyberPolicyEnabled)
	require.True(t, cfg.CyberPolicyEmailEnabled)
	require.True(t, cfg.CyberPolicyAutoBanEnabled)
	cfg.CyberPolicyEmailMessage = "Custom Cyber notice"
	cfg.CyberPolicyBanThreshold = 6
	cfg.CyberPolicyWindowHours = 12
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	var decoded ContentModerationConfig
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, "Custom Cyber notice", decoded.CyberPolicyEmailMessage)
	require.Equal(t, 6, decoded.CyberPolicyBanThreshold)
	require.Equal(t, 12, decoded.CyberPolicyWindowHours)
}

func TestCyberPolicyConfigDefaultsForExistingSettings(t *testing.T) {
	svc := &ContentModerationService{settingRepo: &contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyContentModerationConfig: `{"mode":"pre_block","auto_ban_enabled":true,"ban_threshold":7,"violation_window_hours":48,"cyber_policy_exclude_from_ban_count":true}`,
	}}}

	cfg, err := svc.loadConfig(context.Background())
	require.NoError(t, err)
	require.True(t, cfg.CyberPolicyEnabled)
	require.True(t, cfg.CyberPolicyEmailEnabled)
	require.False(t, cfg.CyberPolicyAutoBanEnabled)
	require.Equal(t, 7, cfg.CyberPolicyBanThreshold)
	require.Equal(t, 48, cfg.CyberPolicyWindowHours)
}

func TestCyberPolicyConfigInheritsSharedValuesOnceForLegacySettings(t *testing.T) {
	svc := &ContentModerationService{settingRepo: &contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyContentModerationConfig: `{"mode":"pre_block","auto_ban_enabled":true,"ban_threshold":5,"violation_window_hours":24}`,
	}}}

	cfg, err := svc.loadConfig(context.Background())
	require.NoError(t, err)
	require.True(t, cfg.CyberPolicyAutoBanEnabled)
	require.Equal(t, 5, cfg.CyberPolicyBanThreshold)
	require.Equal(t, 24, cfg.CyberPolicyWindowHours)
}

func TestUpdateConfigPersistsIndependentCyberPolicy(t *testing.T) {
	settings := &contentModerationTestSettingRepo{values: map[string]string{}}
	svc := NewContentModerationService(settings, nil, nil, nil, nil, nil, nil)
	message := "Custom Cyber sentence."
	cyberAutoBan := true
	cyberThreshold := 5
	cyberWindow := 24

	view, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{
		CyberPolicyEmailMessage:   &message,
		CyberPolicyAutoBanEnabled: &cyberAutoBan,
		CyberPolicyBanThreshold:   &cyberThreshold,
		CyberPolicyWindowHours:    &cyberWindow,
	})
	require.NoError(t, err)
	require.Equal(t, message, view.CyberPolicyEmailMessage)
	require.Equal(t, 5, view.CyberPolicyBanThreshold)
	require.Equal(t, 24, view.CyberPolicyWindowHours)

	contentThreshold := 99
	contentWindow := 12
	view, err = svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{
		BanThreshold:         &contentThreshold,
		ViolationWindowHours: &contentWindow,
	})
	require.NoError(t, err)
	require.Equal(t, 99, view.BanThreshold)
	require.Equal(t, 12, view.ViolationWindowHours)
	require.Equal(t, 5, view.CyberPolicyBanThreshold)
	require.Equal(t, 24, view.CyberPolicyWindowHours)
}

func TestBuildCyberPolicyNoticeEmailBodyUsesCustomMessage(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.CyberPolicyEmailMessage = `Custom <Cyber> notice`
	body := buildCyberPolicyNoticeEmailBody("Cafe", cfg, &ContentModerationLog{
		UserEmail: "user@example.com",
		CreatedAt: time.Now(),
	})
	require.Contains(t, body, "Custom &lt;Cyber&gt; notice")
	require.NotContains(t, body, defaultCyberPolicyEmailMessageZH)
}

func TestRecordCyberPolicyEventHonorsDisabledConfig(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.CyberPolicyEnabled = false
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(&contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled:      "true",
		SettingKeyContentModerationConfig: string(raw),
	}}, repo, nil, nil, nil, nil, nil)

	svc.RecordCyberPolicyEvent(context.Background(), CyberPolicyRecordInput{UserID: 1, UpstreamMessage: "blocked"})

	require.Empty(t, repo.snapshotLogs())
}

func TestCyberPolicyDoesNotCoolDownUpstreamAccount(t *testing.T) {
	svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{}}
	account := &Account{ID: 99, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	shouldDisable := svc.handleOpenAIAccountUpstreamError(
		context.Background(), account, http.StatusBadRequest, http.Header{},
		[]byte(`{"error":{"code":"cyber_policy","message":"blocked"}}`), "gpt-5",
	)
	require.False(t, shouldDisable)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestCyberPolicyDoesNotCoolDownGrokAccount(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 100, Platform: PlatformGrok, Type: AccountTypeOAuth}
	svc.handleGrokAccountUpstreamError(
		context.Background(), account, http.StatusBadRequest, http.Header{},
		[]byte(`{"error":{"code":"cyber_policy","message":"blocked"}}`),
	)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestMarkOpenAIWSCyberPolicyCarriesUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"cyber_policy","message":"blocked"}}}`)
	require.True(t, markOpenAIWSCyberPolicy(c, payload, OpenAIUsage{InputTokens: 12, OutputTokens: 3}))
	mark := GetOpsCyberPolicy(c)
	require.NotNil(t, mark)
	require.Equal(t, 12, mark.UpstreamInTok)
	require.Equal(t, 3, mark.UpstreamOutTok)
}
