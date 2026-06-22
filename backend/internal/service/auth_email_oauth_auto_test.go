//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newEmailOAuthAutoAuthService(
	userRepo UserRepository,
	settings map[string]string,
	quotaRepo UserPlatformQuotaRepository,
) *AuthService {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-secret",
			ExpireHour:               1,
			AccessTokenExpireMinutes: 60,
			RefreshTokenExpireDays:   7,
		},
		Default: config.DefaultConfig{
			UserBalance:     3.5,
			UserConcurrency: 2,
		},
	}

	settingService := NewSettingService(&settingRepoStub{values: settings}, cfg)

	return NewAuthService(
		nil, // entClient — nil, updateUserSignupSource early return
		userRepo,
		nil, // redeemRepo — invitationCode="" 时不触发
		&refreshTokenCacheStub{},
		cfg,
		settingService,
		nil, // emailService
		nil, // turnstileService
		nil, // emailQueueService
		nil, // promoService
		nil, // defaultSubAssigner — nil, assignSubscriptions early return
		nil, // affiliateService — nil, bindOAuthAffiliate early return
		quotaRepo,
	)
}

func TestEmailOAuthAuto_SnapshotsPlatformQuotaDefaults(t *testing.T) {
	userRepo := &userRepoStub{nextID: 88}
	quotaRepo := &userPlatformQuotaRepoStub{}

	svc := newEmailOAuthAutoAuthService(
		userRepo,
		map[string]string{
			SettingKeyRegistrationEnabled:   "true",
			SettingKeyDefaultPlatformQuotas: `{"gemini": {"monthly": 100.0}}`,
		},
		quotaRepo,
	)

	user, err := svc.createEmailOAuthUser(
		context.Background(),
		"newoauth@example.com",
		"newoauth",
		"github",
		"", // invitationCode
		"", // affiliateCode
	)
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, int64(88), user.ID)

	require.Len(t, quotaRepo.bulkInsertCalls, 1, "createEmailOAuthUser must snapshot platform quotas via BulkInsertInitial")

	records := quotaRepo.bulkInsertCalls[0]
	var geminiRecord *UserPlatformQuotaRecord
	for i := range records {
		if records[i].Platform == "gemini" {
			geminiRecord = &records[i]
			break
		}
	}
	require.NotNil(t, geminiRecord, "expected gemini platform record")
	require.NotNil(t, geminiRecord.MonthlyLimitUSD)
	require.InDelta(t, 100.0, *geminiRecord.MonthlyLimitUSD, 0.0001)
}

func TestEmailOAuthAuto_AffiliateAdmissionRequiresBinding(t *testing.T) {
	userRepo := &userRepoStub{nextID: 89}
	quotaRepo := &userPlatformQuotaRepoStub{}
	settings := map[string]string{
		SettingKeyRegistrationEnabled:   "true",
		SettingKeyInvitationCodeEnabled: "true",
		SettingKeyAffiliateEnabled:      "true",
		SettingKeyDefaultPlatformQuotas: `{"gemini": {"monthly": 100.0}}`,
	}
	affiliateRepo := &affiliateRepoThresholdStub{
		bindErr: ErrAffiliateInviteLimitReached,
		summaries: map[int64]*AffiliateSummary{
			88: {UserID: 88, AffCode: "NHRPKQKESRWT", TotalRecharged: MembershipLevel1Threshold + 1},
		},
	}
	settingService := NewSettingService(&settingRepoStub{values: settings}, &config.Config{})
	affiliateService := NewAffiliateService(affiliateRepo, settingService, nil, nil, nil)
	svc := newEmailOAuthAutoAuthService(userRepo, settings, quotaRepo)
	svc.affiliateService = affiliateService

	user, err := svc.createEmailOAuthUser(
		context.Background(),
		"newoauth-aff-limit@example.com",
		"newoauth",
		"github",
		"NHRPKQKESRWT",
		"", // affiliateCode
	)

	require.ErrorIs(t, err, ErrAffiliateInviteLimitReached)
	require.Nil(t, user)
	require.Equal(t, []int64{89}, userRepo.deletedIDs)
	require.Equal(t, 1, affiliateRepo.bindCalls)
	require.Empty(t, quotaRepo.bulkInsertCalls)
}
