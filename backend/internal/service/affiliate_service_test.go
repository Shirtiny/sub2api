//go:build unit

package service

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveRebateRatePercent_PerUserOverride(t *testing.T) {
	t.Parallel()
	svc := &AffiliateService{}

	require.Zero(t, svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{}))
	require.InDelta(t, AffiliateRebateRateLevel1Default,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{TotalRecharged: 20.01}), 1e-9)
	require.InDelta(t, AffiliateRebateRateLevel2Default,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{TotalRecharged: 300.01}), 1e-9)
	require.InDelta(t, AffiliateRebateRateLevel3Default,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{TotalRecharged: 1000.01}), 1e-9)

	// exclusive rate set → overrides global
	rate := 50.0
	require.InDelta(t, 50.0,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &rate}), 1e-9)

	// exclusive rate 0 → returns 0 (no rebate, intentional)
	zero := 0.0
	require.InDelta(t, 0.0,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &zero}), 1e-9)

	// exclusive rate above max → clamped to Max
	tooHigh := 250.0
	require.InDelta(t, AffiliateRebateRateMax,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &tooHigh}), 1e-9)

	// exclusive rate below min → clamped to Min
	tooLow := -5.0
	require.InDelta(t, AffiliateRebateRateMin,
		svc.resolveRebateRatePercent(context.Background(), &AffiliateSummary{AffRebateRatePercent: &tooLow}), 1e-9)
}

// TestIsEnabled_NilSettingServiceReturnsDefault verifies that IsEnabled
// safely handles a nil settingService dependency by returning the default
// (off). This protects callers from nil-pointer crashes in misconfigured
// environments.
func TestIsEnabled_NilSettingServiceReturnsDefault(t *testing.T) {
	t.Parallel()
	svc := &AffiliateService{}
	require.False(t, svc.IsEnabled(context.Background()))
	require.Equal(t, AffiliateEnabledDefault, svc.IsEnabled(context.Background()))
}

// TestValidateExclusiveRate_BoundaryAndInvalid covers the validator used by
// admin-facing rate setters: nil is always valid (clear), in-range values
// are accepted, NaN/Inf and out-of-range values produce a typed BadRequest.
func TestValidateExclusiveRate_BoundaryAndInvalid(t *testing.T) {
	t.Parallel()
	require.NoError(t, validateExclusiveRate(nil))

	for _, v := range []float64{0, 0.01, 50, 99.99, 100} {
		v := v
		require.NoError(t, validateExclusiveRate(&v), "value %v should be valid", v)
	}

	for _, v := range []float64{-0.01, 100.01, -100, 200} {
		v := v
		require.Error(t, validateExclusiveRate(&v), "value %v should be rejected", v)
	}

	nan := math.NaN()
	require.Error(t, validateExclusiveRate(&nan))
	posInf := math.Inf(1)
	require.Error(t, validateExclusiveRate(&posInf))
	negInf := math.Inf(-1)
	require.Error(t, validateExclusiveRate(&negInf))
}

func TestMaskEmail(t *testing.T) {
	t.Parallel()
	require.Equal(t, "a***@g***.com", maskEmail("alice@gmail.com"))
	require.Equal(t, "x***@d***", maskEmail("x@domain"))
	require.Equal(t, "", maskEmail(""))
}

func TestIsValidAffiliateCodeFormat(t *testing.T) {
	t.Parallel()

	// 邀请码格式校验同时服务于：
	// 1) 系统自动生成的 12 位随机码（A-Z 去 I/O，2-9 去 0/1）
	// 2) 管理员设置的自定义专属码（如 "VIP2026"、"NEW_USER-1"）
	// 因此校验放宽到 [A-Z0-9_-]{4,32}（要求调用方先 ToUpper）。
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"valid canonical 12-char", "ABCDEFGHJKLM", true},
		{"valid all digits 2-9", "234567892345", true},
		{"valid mixed", "A2B3C4D5E6F7", true},
		{"valid admin custom short", "VIP1", true},
		{"valid admin custom with hyphen", "NEW-USER", true},
		{"valid admin custom with underscore", "VIP_2026", true},
		{"valid 32-char max", "ABCDEFGHIJKLMNOPQRSTUVWXYZ012345", true},
		// Previously-excluded chars (I/O/0/1) are now allowed since admins may use them.
		{"letter I now allowed", "IBCDEFGHJKLM", true},
		{"letter O now allowed", "OBCDEFGHJKLM", true},
		{"digit 0 now allowed", "0BCDEFGHJKLM", true},
		{"digit 1 now allowed", "1BCDEFGHJKLM", true},
		{"too short (3 chars)", "ABC", false},
		{"too long (33 chars)", "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456", false},
		{"lowercase rejected (caller must ToUpper first)", "abcdefghjklm", false},
		{"empty", "", false},
		{"utf8 non-ascii", "ÄÄÄÄÄÄ", false}, // bytes out of charset
		{"ascii punctuation .", "ABCDEFGHJK.M", false},
		{"whitespace", "ABCDEFGHJK M", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, isValidAffiliateCodeFormat(tc.in))
		})
	}
}

func TestCalculateMembershipLevel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		total float64
		want  int
	}{
		{"none", 0, 0},
		{"level one threshold is strict", 20, 0},
		{"level one", 20.01, 1},
		{"level two threshold is strict", 300, 1},
		{"level two", 300.01, 2},
		{"level three threshold is strict", 1000, 2},
		{"level three", 1000.01, 3},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, CalculateMembershipLevel(tc.total))
		})
	}
}

func TestCalculateSubscriptionRebateDaysByMembership(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		level int
		want  int
	}{
		{"no level", 0, 0},
		{"level one", 1, 1},
		{"level two", 2, 3},
		{"level three", 3, 7},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, calculateSubscriptionRebateDaysByMembership(tc.level))
		})
	}
}

func TestResolveSubscriptionInviteRebate_SkipsShortSubscription(t *testing.T) {
	t.Parallel()
	repo := &affiliateRepoThresholdStub{}
	svc := &AffiliateService{repo: repo}

	got, err := svc.ResolveSubscriptionInviteRebate(context.Background(), 1, AffiliateSubscriptionRebateMinDays-1)

	require.NoError(t, err)
	require.Zero(t, got.RebateDays)
	require.Equal(t, "subscription days below monthly rebate threshold", got.Reason)
	require.Zero(t, repo.ensureCalls)
}

func TestResolveSubscriptionInviteRebate_SkipsLevelZeroInviter(t *testing.T) {
	t.Parallel()
	inviterID := int64(2)
	repo := &affiliateRepoThresholdStub{summaries: map[int64]*AffiliateSummary{
		1: {UserID: 1, InviterID: &inviterID},
		2: {UserID: 2, TotalRecharged: MembershipLevel1Threshold},
	}}
	svc := &AffiliateService{repo: repo, settingService: &SettingService{settingRepo: affiliateSettingRepoStub{}}}

	got, err := svc.ResolveSubscriptionInviteRebate(context.Background(), 1, AffiliateSubscriptionRebateMinDays)

	require.NoError(t, err)
	require.Zero(t, got.RebateDays)
	require.Equal(t, "inviter membership level not eligible", got.Reason)
}

func TestAccrueInviteRebateForOrder_SkipsLevelZeroInviter(t *testing.T) {
	t.Parallel()
	inviterID := int64(2)
	repo := &affiliateRepoThresholdStub{summaries: map[int64]*AffiliateSummary{
		1: {UserID: 1, InviterID: &inviterID},
		2: {UserID: 2, TotalRecharged: MembershipLevel1Threshold},
	}}
	svc := &AffiliateService{repo: repo, settingService: &SettingService{settingRepo: affiliateSettingRepoStub{}}}

	got, err := svc.AccrueInviteRebateForOrder(context.Background(), 1, 100, nil)

	require.NoError(t, err)
	require.Zero(t, got)
	require.Zero(t, repo.accrueCalls)
}

func TestGetAffiliateDetail_HidesAffCodeForLevelZero(t *testing.T) {
	t.Parallel()
	repo := &affiliateRepoThresholdStub{summaries: map[int64]*AffiliateSummary{
		1: {UserID: 1, AffCode: "LEAKME", TotalRecharged: MembershipLevel1Threshold},
	}}
	svc := &AffiliateService{repo: repo}

	got, err := svc.GetAffiliateDetail(context.Background(), 1)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.False(t, got.CanInvite)
	require.Equal(t, 0, got.MembershipLevel)
	require.Empty(t, got.AffCode)
}

func TestGetAffiliateDetail_ExposesAffCodeForEligibleUser(t *testing.T) {
	t.Parallel()
	repo := &affiliateRepoThresholdStub{summaries: map[int64]*AffiliateSummary{
		1: {UserID: 1, AffCode: "VISIBLE", TotalRecharged: MembershipLevel1Threshold + 0.01},
	}}
	svc := &AffiliateService{repo: repo}

	got, err := svc.GetAffiliateDetail(context.Background(), 1)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.True(t, got.CanInvite)
	require.Equal(t, "VISIBLE", got.AffCode)
}

type affiliateRepoThresholdStub struct {
	ensureCalls int
	accrueCalls int
	summaries   map[int64]*AffiliateSummary
}

func (r *affiliateRepoThresholdStub) EnsureUserAffiliate(ctx context.Context, userID int64) (*AffiliateSummary, error) {
	r.ensureCalls++
	if r.summaries != nil && r.summaries[userID] != nil {
		return r.summaries[userID], nil
	}
	return &AffiliateSummary{UserID: userID}, nil
}

func (r *affiliateRepoThresholdStub) GetAffiliateByCode(ctx context.Context, code string) (*AffiliateSummary, error) {
	return nil, nil
}

func (r *affiliateRepoThresholdStub) BindInviter(ctx context.Context, userID, inviterID int64, inviteLimit int) (bool, error) {
	return false, nil
}

func (r *affiliateRepoThresholdStub) AccrueQuota(ctx context.Context, inviterID, inviteeUserID int64, amount float64, freezeHours int, sourceOrderID *int64) (bool, error) {
	r.accrueCalls++
	return true, nil
}

func (r *affiliateRepoThresholdStub) GetAccruedRebateFromInvitee(ctx context.Context, inviterID, inviteeUserID int64) (float64, error) {
	return 0, nil
}

func (r *affiliateRepoThresholdStub) ThawFrozenQuota(ctx context.Context, userID int64) (float64, error) {
	return 0, nil
}

func (r *affiliateRepoThresholdStub) TransferQuotaToBalance(ctx context.Context, userID int64) (float64, float64, error) {
	return 0, 0, nil
}

func (r *affiliateRepoThresholdStub) ListInvitees(ctx context.Context, inviterID int64, limit int) ([]AffiliateInvitee, error) {
	return nil, nil
}

func (r *affiliateRepoThresholdStub) UpdateUserAffCode(ctx context.Context, userID int64, newCode string) error {
	return nil
}

func (r *affiliateRepoThresholdStub) ResetUserAffCode(ctx context.Context, userID int64) (string, error) {
	return "", nil
}

func (r *affiliateRepoThresholdStub) SetUserRebateRate(ctx context.Context, userID int64, ratePercent *float64) error {
	return nil
}

func (r *affiliateRepoThresholdStub) SetUserInviteLimit(ctx context.Context, userID int64, limit *int) error {
	return nil
}

func (r *affiliateRepoThresholdStub) BatchSetUserRebateRate(ctx context.Context, userIDs []int64, ratePercent *float64) error {
	return nil
}

func (r *affiliateRepoThresholdStub) ListUsersWithCustomSettings(ctx context.Context, filter AffiliateAdminFilter) ([]AffiliateAdminEntry, int64, error) {
	return nil, 0, nil
}

func (r *affiliateRepoThresholdStub) ListAffiliateInviteRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateInviteRecord, int64, error) {
	return nil, 0, nil
}

func (r *affiliateRepoThresholdStub) ListAffiliateRebateRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateRebateRecord, int64, error) {
	return nil, 0, nil
}

func (r *affiliateRepoThresholdStub) ListAffiliateTransferRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateTransferRecord, int64, error) {
	return nil, 0, nil
}

func (r *affiliateRepoThresholdStub) GetAffiliateUserOverview(ctx context.Context, userID int64) (*AffiliateUserOverview, error) {
	return nil, nil
}

type affiliateSettingRepoStub struct{}

func (affiliateSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	return nil, ErrSettingNotFound
}
func (affiliateSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if key == SettingKeyAffiliateEnabled {
		return "true", nil
	}
	return "", ErrSettingNotFound
}
func (affiliateSettingRepoStub) Set(ctx context.Context, key, value string) error { return nil }
func (affiliateSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	return nil, nil
}
func (affiliateSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	return nil
}
func (affiliateSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	return nil, nil
}
func (affiliateSettingRepoStub) Delete(ctx context.Context, key string) error { return nil }
