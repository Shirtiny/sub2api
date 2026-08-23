package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

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
		{name: "unrelated policy", status: 400, body: `{"error":{"code":"content_policy","message":"blocked"}}`, hit: false},
		{name: "message on success", status: 200, body: `{"message":"possible cybersecurity risk"}`, hit: false},
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

func TestRecordCyberPolicyEventUsesRiskControlBanCounter(t *testing.T) {
	repo := &contentModerationTestRepo{}
	settings := &contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled: "true",
	}}
	svc := NewContentModerationService(settings, repo, nil, nil, nil, nil, nil)

	svc.RecordCyberPolicyEvent(context.Background(), CyberPolicyRecordInput{
		RequestID:       "req-cyber-1",
		UserID:          42,
		UserEmail:       "user@example.com",
		Endpoint:        "/v1/responses",
		Model:           "gpt-5",
		UpstreamMessage: "cyber policy blocked",
		UpstreamBody:    `{"error":{"code":"cyber_policy"}}`,
		UpstreamStatus:  400,
	})
	svc.RecordCyberPolicyEvent(context.Background(), CyberPolicyRecordInput{
		RequestID: "req-cyber-2", UserID: 42, UserEmail: "user@example.com", Endpoint: "/v1/responses", Model: "gpt-5",
		UpstreamMessage: "blocked again", UpstreamStatus: 400,
	})

	logs := repo.snapshotLogs()
	require.Len(t, logs, 2)
	require.Equal(t, ContentModerationActionCyberPolicy, logs[0].Action)
	require.Equal(t, ContentModerationActionCyberPolicy, logs[0].HighestCategory)
	require.True(t, logs[0].Flagged)
	require.Equal(t, 1.0, logs[0].HighestScore)
	require.Equal(t, 1, logs[0].ViolationCount)
	require.Equal(t, 2, logs[1].ViolationCount)
	require.Contains(t, logs[0].Error, "cyber_policy")
}

func TestRecordCyberPolicyEventAutoBansAtThreshold(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.BanThreshold = 1
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

	logs := repo.snapshotLogs()
	require.Len(t, logs, 1)
	require.True(t, logs[0].AutoBanned)
	require.Equal(t, StatusDisabled, userRepo.user.Status)
	require.Equal(t, []int64{userID}, invalidator.userIDs)
}

func TestCyberPolicyConfigRoundTrip(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.CyberPolicyExcludeFromBanCount = true
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	var decoded ContentModerationConfig
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.True(t, decoded.CyberPolicyExcludeFromBanCount)
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
