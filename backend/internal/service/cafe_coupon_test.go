//go:build unit

package service

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/cafecoupon"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestCafeCouponAdminListDetailAndVoid(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("admin-cafe@example.com").SetPasswordHash("hash").SetUsername("admin-cafe").Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	issued, err := client.CafeCoupon.Create().
		SetCode("CAFE-ADMIN-ISSUED").SetUserID(user.ID).SetMembershipLevel(2).SetCouponType(CafeCouponTypeCash).SetValue(15).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(now.Add(-time.Hour)).SetPeriodEnd(now.Add(time.Hour)).SetStatus(CafeCouponStatusIssued).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.CafeCoupon.Create().
		SetCode("CAFE-ADMIN-APPLIED").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeDiscount).SetValue(20).
		SetPeriod(CafeCouponPeriodWeek).SetPeriodStart(now.Add(-time.Hour)).SetPeriodEnd(now.Add(time.Hour)).SetStatus(CafeCouponStatusApplied).SetOrderID(123).SetAppliedAt(now).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{entClient: client}
	level := 2

	items, result, err := svc.AdminListCafeCoupons(ctx, pagination.PaginationParams{Page: 1, PageSize: 10, SortBy: "created_at", SortOrder: "desc"}, CafeCouponAdminListFilters{Search: fmt.Sprint(user.ID), Status: CafeCouponStatusIssued, MembershipLevel: &level})
	require.NoError(t, err)
	require.Equal(t, int64(1), result.Total)
	require.Len(t, items, 1)
	require.Equal(t, issued.Code, items[0].Code)
	require.NotNil(t, items[0].User)
	require.Equal(t, user.Email, items[0].User.Email)

	detail, err := svc.AdminGetCafeCoupon(ctx, issued.ID)
	require.NoError(t, err)
	require.Equal(t, issued.Code, detail.Code)
	require.Equal(t, user.ID, detail.UserID)
	_, periodEnd := cafeCouponRollingPeriodWindow(issued.PeriodStart, issued.Period)
	require.Equal(t, periodEnd, detail.PeriodEnd)
	require.Equal(t, cafeCouponExpiresAt(issued.PeriodStart, issued.Period), detail.ExpiresAt)

	voided, err := svc.AdminVoidCafeCoupon(ctx, issued.ID)
	require.NoError(t, err)
	require.Equal(t, CafeCouponStatusVoid, voided.Status)
	stored, err := client.CafeCoupon.Get(ctx, issued.ID)
	require.NoError(t, err)
	require.Equal(t, CafeCouponStatusVoid, stored.Status)
}

func TestCafeCouponAdminListRejectsInvalidFilters(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	svc := &PaymentService{entClient: client}
	level := 4

	_, _, err := svc.AdminListCafeCoupons(ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, CafeCouponAdminListFilters{Status: "bad"})
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_STATUS_INVALID", infraerrors.Reason(err))

	_, _, err = svc.AdminListCafeCoupons(ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, CafeCouponAdminListFilters{CouponType: "bonus"})
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_TYPE_INVALID", infraerrors.Reason(err))

	_, _, err = svc.AdminListCafeCoupons(ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, CafeCouponAdminListFilters{MembershipLevel: &level})
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_MEMBERSHIP_LEVEL_INVALID", infraerrors.Reason(err))
}

func TestCafeCouponAdminVoidRejectsAppliedCoupon(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("admin-cafe-applied@example.com").SetPasswordHash("hash").SetUsername("admin-cafe-applied").Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-ADMIN-VOID-APPLIED").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(now.Add(-time.Hour)).SetPeriodEnd(now.Add(time.Hour)).SetStatus(CafeCouponStatusApplied).SetOrderID(456).SetAppliedAt(now).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{entClient: client}

	_, err = svc.AdminVoidCafeCoupon(ctx, coupon.ID)
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_VOID_NOT_ALLOWED", infraerrors.Reason(err))
	stored, err := client.CafeCoupon.Get(ctx, coupon.ID)
	require.NoError(t, err)
	require.Equal(t, CafeCouponStatusApplied, stored.Status)
}

func TestCafeCouponClaimEligibilityAndIdempotency(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("cafe@example.com").SetPasswordHash("hash").SetUsername("cafe").Save(ctx)
	require.NoError(t, err)
	repo := cafeCouponSettingsRepo(map[string]string{
		SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":12.5,"period":"month"}}}`,
	})
	userRepo := &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}}
	svc := &PaymentService{entClient: client, userRepo: userRepo, configService: &PaymentConfigService{settingRepo: repo}}

	first, err := svc.ClaimCafeCoupon(ctx, user.ID)
	require.NoError(t, err)
	require.False(t, first.AlreadyClaimed)
	require.Equal(t, CafeCouponTypeCash, first.CouponType)
	require.Equal(t, 12.5, first.Value)

	second, err := svc.ClaimCafeCoupon(ctx, user.ID)
	require.NoError(t, err)
	require.True(t, second.AlreadyClaimed)
	require.Equal(t, first.Code, second.Code)

	count, err := client.CafeCoupon.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestCafeCouponConfigNormalizesTransferabilityAndMonthEndValidity(t *testing.T) {
	cfg := normalizeCafeCouponConfig(CafeCouponConfig{Levels: map[int]CafeCouponLevelConfig{
		1: {Enabled: true, Type: CafeCouponTypeDiscount, Value: 120, Period: CafeCouponPeriodWeek, Transferable: true, Validity: "legacy"},
	}})

	level := cfg.Levels[1]
	require.True(t, level.Enabled)
	require.True(t, level.Transferable)
	require.Equal(t, CafeCouponTypeDiscount, level.Type)
	require.Equal(t, 100.0, level.Value)
	require.Equal(t, CafeCouponPeriodWeek, level.Period)
	require.Equal(t, cafeCouponValidityMonthEnd, level.Validity)
	require.True(t, level.ValidUntilMonthEnd)
}

func TestCafeCouponClaimRejectsCurrentPeriodUsedCoupon(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("claimed-used@example.com").SetPasswordHash("hash").SetUsername("claimed-used").Save(ctx)
	require.NoError(t, err)
	start, end := cafeCouponRollingPeriodWindow(time.Now(), CafeCouponPeriodMonth)
	_, err = client.CafeCoupon.Create().
		SetCode("CAFE-CLAIMED-USED").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(start).SetPeriodEnd(end).SetStatus(CafeCouponStatusApplied).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{
		entClient:     client,
		userRepo:      &mockUserRepo{getByIDUser: &User{ID: user.ID, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		configService: &PaymentConfigService{settingRepo: cafeCouponSettingsRepo(map[string]string{SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":10,"period":"month"}}}`})},
	}

	_, err = svc.ClaimCafeCoupon(ctx, user.ID)
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_ALREADY_CLAIMED", infraerrors.Reason(err))
}

func TestCafeCouponStatusBlocksAppliedCurrentPeriodCoupon(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("status-used@example.com").SetPasswordHash("hash").SetUsername("status-used").Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	claimedAt := now.Add(-time.Hour)
	start, end := cafeCouponRollingPeriodWindow(now, CafeCouponPeriodMonth)
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-STATUS-USED").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(start).SetPeriodEnd(end).SetStatus(CafeCouponStatusApplied).SetCreatedAt(claimedAt).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{
		entClient:     client,
		userRepo:      &mockUserRepo{getByIDUser: &User{ID: user.ID, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		configService: &PaymentConfigService{settingRepo: cafeCouponSettingsRepo(map[string]string{SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":10,"period":"month"}}}`})},
	}

	status, err := svc.CafeCouponStatus(ctx, user.ID)
	require.NoError(t, err)
	require.True(t, status.Eligible)
	require.False(t, status.CanClaim)
	require.True(t, status.AlreadyClaimed)
	require.NotNil(t, status.Coupon)
	require.Equal(t, coupon.Code, status.Coupon.Code)
	require.False(t, status.Coupon.CanClaim)
	require.True(t, status.Coupon.AlreadyClaimed)
	require.True(t, status.NextClaimAt.After(now))
	require.True(t, status.NextClaimAt.Equal(cafeCouponClaimCooldownAt(coupon.CreatedAt, CafeCouponPeriodMonth)))
	require.Equal(t, cafeCouponRemainingDays(now, status.NextClaimAt), status.RemainingDays)
	_ = end
}

func TestCafeCouponStatusBlocksLatestAppliedCouponCooldown(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("status-applied-latest@example.com").SetPasswordHash("hash").SetUsername("status-applied-latest").Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	_, err = client.CafeCoupon.Create().
		SetCode("CAFE-OLD-ISSUED").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(now.AddDate(0, -1, 0)).SetPeriodEnd(cafeCouponClaimCooldownAt(now.AddDate(0, -1, 0), CafeCouponPeriodMonth)).SetStatus(CafeCouponStatusIssued).SetCreatedAt(now.AddDate(0, -1, -1)).
		Save(ctx)
	require.NoError(t, err)
	latest, err := client.CafeCoupon.Create().
		SetCode("CAFE-LATEST-APPLIED").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(cafeCouponClaimCooldownAt(now.AddDate(0, -1, 0), CafeCouponPeriodMonth)).SetPeriodEnd(cafeCouponClaimCooldownAt(now, CafeCouponPeriodMonth)).SetStatus(CafeCouponStatusApplied).SetCreatedAt(now.Add(-time.Hour)).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{
		entClient:     client,
		userRepo:      &mockUserRepo{getByIDUser: &User{ID: user.ID, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		configService: &PaymentConfigService{settingRepo: cafeCouponSettingsRepo(map[string]string{SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":10,"period":"month"}}}`})},
	}

	status, err := svc.CafeCouponStatus(ctx, user.ID)
	require.NoError(t, err)
	require.False(t, status.CanClaim)
	require.True(t, status.AlreadyClaimed)
	require.NotNil(t, status.Coupon)
	require.Equal(t, latest.Code, status.Coupon.Code)
	require.True(t, status.NextClaimAt.Equal(cafeCouponClaimCooldownAt(latest.CreatedAt, CafeCouponPeriodMonth)))
}

func TestCafeCouponClaimBlocksLatestCouponAfterPeriodChange(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("period-change@example.com").SetPasswordHash("hash").SetUsername("period-change").Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	_, err = client.CafeCoupon.Create().
		SetCode("CAFE-PERIOD-CHANGE").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(cafeCouponClaimCooldownAt(now.AddDate(0, -1, 0), CafeCouponPeriodMonth)).SetPeriodEnd(cafeCouponClaimCooldownAt(now, CafeCouponPeriodMonth)).SetStatus(CafeCouponStatusApplied).SetCreatedAt(now.Add(-time.Hour)).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{
		entClient:     client,
		userRepo:      &mockUserRepo{getByIDUser: &User{ID: user.ID, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		configService: &PaymentConfigService{settingRepo: cafeCouponSettingsRepo(map[string]string{SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":10,"period":"day"}}}`})},
	}

	_, err = svc.ClaimCafeCoupon(ctx, user.ID)
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_ALREADY_CLAIMED", infraerrors.Reason(err))
	count, err := client.CafeCoupon.Query().Where(cafecoupon.UserIDEQ(user.ID)).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestCafeCouponClaimBlocksLatestVoidCouponCooldown(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("void-cooldown@example.com").SetPasswordHash("hash").SetUsername("void-cooldown").Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	_, err = client.CafeCoupon.Create().
		SetCode("CAFE-VOID-COOLDOWN").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(cafeCouponClaimCooldownAt(now.AddDate(0, -1, 0), CafeCouponPeriodMonth)).SetPeriodEnd(cafeCouponClaimCooldownAt(now, CafeCouponPeriodMonth)).SetStatus(CafeCouponStatusVoid).SetCreatedAt(now.Add(-time.Hour)).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{
		entClient:     client,
		userRepo:      &mockUserRepo{getByIDUser: &User{ID: user.ID, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		configService: &PaymentConfigService{settingRepo: cafeCouponSettingsRepo(map[string]string{SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":10,"period":"month"}}}`})},
	}

	_, err = svc.ClaimCafeCoupon(ctx, user.ID)
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_ALREADY_CLAIMED", infraerrors.Reason(err))
}

func TestCafeCouponClaimReturnsLatestConfigMismatchedCouponCooldown(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("config-mismatch@example.com").SetPasswordHash("hash").SetUsername("config-mismatch").Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-CONFIG-MISMATCH").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(9).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(cafeCouponClaimCooldownAt(now.AddDate(0, -1, 0), CafeCouponPeriodMonth)).SetPeriodEnd(cafeCouponClaimCooldownAt(now, CafeCouponPeriodMonth)).SetStatus(CafeCouponStatusIssued).SetCreatedAt(now.Add(-time.Hour)).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{
		entClient:     client,
		userRepo:      &mockUserRepo{getByIDUser: &User{ID: user.ID, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		configService: &PaymentConfigService{settingRepo: cafeCouponSettingsRepo(map[string]string{SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":10,"period":"month"}}}`})},
	}

	claimed, err := svc.ClaimCafeCoupon(ctx, user.ID)
	require.NoError(t, err)
	require.True(t, claimed.AlreadyClaimed)
	require.Equal(t, coupon.Code, claimed.Code)
	require.Equal(t, 9.0, claimed.Value)
}

func TestCafeCouponStatusUsesLatestClaimIDTie(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("latest-id@example.com").SetPasswordHash("hash").SetUsername("latest-id").Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	createdAt := now.Add(-time.Hour)
	_, err = client.CafeCoupon.Create().
		SetCode("CAFE-SAME-TIME-OLD").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(cafeCouponClaimCooldownAt(now.AddDate(0, -1, 0), CafeCouponPeriodMonth)).SetPeriodEnd(cafeCouponClaimCooldownAt(now, CafeCouponPeriodMonth)).SetStatus(CafeCouponStatusIssued).SetCreatedAt(createdAt).
		Save(ctx)
	require.NoError(t, err)
	latest, err := client.CafeCoupon.Create().
		SetCode("CAFE-SAME-TIME-LATEST").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(cafeCouponClaimCooldownAt(now.AddDate(0, -2, 0), CafeCouponPeriodMonth)).SetPeriodEnd(cafeCouponClaimCooldownAt(now, CafeCouponPeriodMonth)).SetStatus(CafeCouponStatusVoid).SetCreatedAt(createdAt).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{
		entClient:     client,
		userRepo:      &mockUserRepo{getByIDUser: &User{ID: user.ID, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		configService: &PaymentConfigService{settingRepo: cafeCouponSettingsRepo(map[string]string{SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":10,"period":"month"}}}`})},
	}

	status, err := svc.CafeCouponStatus(ctx, user.ID)
	require.NoError(t, err)
	require.False(t, status.CanClaim)
	require.True(t, status.AlreadyClaimed)
	require.NotNil(t, status.Coupon)
	require.Equal(t, latest.Code, status.Coupon.Code)
}

func TestCafeCouponMonthlyCooldownUsesRollingClaimDate(t *testing.T) {
	claimAt := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	require.Equal(t, time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC), cafeCouponClaimCooldownAt(claimAt, CafeCouponPeriodMonth))
}

func TestCafeCouponExpiresAtUsesThirtyPercentFloor(t *testing.T) {
	claimAt := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := cafeCouponClaimCooldownAt(claimAt, CafeCouponPeriodMonth)
	expected := claimAt.Add(periodEnd.Sub(claimAt) * 30 / 100)
	require.Equal(t, expected, cafeCouponExpiresAt(claimAt, CafeCouponPeriodMonth))
	require.True(t, cafeCouponExpiresAt(claimAt, CafeCouponPeriodMonth).Before(periodEnd))
}

func TestCafeCouponClaimAllowsNewClaimAfterCooldown(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("after-cooldown@example.com").SetPasswordHash("hash").SetUsername("after-cooldown").Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	_, err = client.CafeCoupon.Create().
		SetCode("CAFE-AFTER-OLD").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodDay).SetPeriodStart(now.AddDate(0, 0, -2)).SetPeriodEnd(now.AddDate(0, 0, 1)).SetStatus(CafeCouponStatusApplied).SetCreatedAt(now.Add(-25 * time.Hour)).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{
		entClient:     client,
		userRepo:      &mockUserRepo{getByIDUser: &User{ID: user.ID, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		configService: &PaymentConfigService{settingRepo: cafeCouponSettingsRepo(map[string]string{SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":10,"period":"day"}}}`})},
	}

	claimed, err := svc.ClaimCafeCoupon(ctx, user.ID)
	require.NoError(t, err)
	require.False(t, claimed.AlreadyClaimed)
	require.NotEqual(t, "CAFE-AFTER-OLD", claimed.Code)
	count, err := client.CafeCoupon.Query().Where(cafecoupon.UserIDEQ(user.ID)).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

func TestCafeCouponClaimReturnsExistingValidIssuedCouponDuringCooldown(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("valid-issued-idempotent@example.com").SetPasswordHash("hash").SetUsername("valid-issued-idempotent").Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-VALID-ISSUED").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(cafeCouponClaimCooldownAt(now.AddDate(0, -1, 0), CafeCouponPeriodMonth)).SetPeriodEnd(cafeCouponClaimCooldownAt(now, CafeCouponPeriodMonth)).SetStatus(CafeCouponStatusIssued).SetCreatedAt(now.Add(-time.Hour)).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{
		entClient:     client,
		userRepo:      &mockUserRepo{getByIDUser: &User{ID: user.ID, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		configService: &PaymentConfigService{settingRepo: cafeCouponSettingsRepo(map[string]string{SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":10,"period":"month"}}}`})},
	}

	claimed, err := svc.ClaimCafeCoupon(ctx, user.ID)
	require.NoError(t, err)
	require.True(t, claimed.AlreadyClaimed)
	require.Equal(t, coupon.Code, claimed.Code)
}

func TestCafeCouponPreviewAllowsConfigChangedIssuedCoupon(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("<CAFE:EMAIL:K3ZQ7YBN6H3M2J6JZP7A>").SetPasswordHash("hash").SetUsername("snapshot-owner").Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-SNAPSHOT").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeDiscount).SetValue(30).
		SetPeriod(CafeCouponPeriodWeek).SetPeriodStart(now.AddDate(0, 0, -7)).SetPeriodEnd(now.AddDate(0, 0, 7)).SetStatus(CafeCouponStatusIssued).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{
		entClient: client,
		userRepo:  &mockUserRepo{getByIDUser: &User{ID: user.ID, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		configService: &PaymentConfigService{settingRepo: cafeCouponSettingsRepo(map[string]string{
			SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":10,"period":"month","transferable":false}}}`,
		})},
	}

	preview, err := svc.PreviewCafeCoupon(ctx, user.ID, coupon.Code, 100)
	require.NoError(t, err)
	require.Equal(t, CafeCouponTypeDiscount, preview.CouponType)
	require.Equal(t, 30.0, preview.DiscountAmount)
	require.Equal(t, 70.0, preview.PayableAmount)
	require.Equal(t, cafeCouponExpiresAt(coupon.CreatedAt, coupon.Period), preview.ExpiresAt)
}

func TestCafeCouponConcurrentClaimCreatesOneCoupon(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("cafe@example.com").SetPasswordHash("hash").SetUsername("cafe").Save(ctx)
	require.NoError(t, err)
	repo := cafeCouponSettingsRepo(map[string]string{
		SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":8,"period":"month"}}}`,
	})
	svc := &PaymentService{
		entClient:     client,
		userRepo:      &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		configService: &PaymentConfigService{settingRepo: repo},
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	results := make(chan *CafeCouponClaimResult, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := svc.ClaimCafeCoupon(ctx, user.ID)
			if err != nil {
				errCh <- err
				return
			}
			results <- claimed
		}()
	}
	wg.Wait()
	close(errCh)
	close(results)
	for err := range errCh {
		require.NoError(t, err)
	}
	seen := map[string]struct{}{}
	freshClaims := 0
	alreadyClaimed := 0
	for result := range results {
		require.NotNil(t, result)
		seen[result.Code] = struct{}{}
		if result.AlreadyClaimed {
			alreadyClaimed++
		} else {
			freshClaims++
		}
	}
	require.Len(t, seen, 1)
	require.Equal(t, 1, freshClaims)
	require.Equal(t, 7, alreadyClaimed)
	count, err := client.CafeCoupon.Query().Where(cafecoupon.UserIDEQ(user.ID)).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestCafeCouponPreviewRejectsInvalidInputsAndOwnership(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	owner, err := client.User.Create().SetEmail("owner@example.com").SetPasswordHash("hash").SetUsername("owner").Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{
		entClient:     client,
		userRepo:      &mockUserRepo{getByIDUser: &User{ID: 1, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		configService: &PaymentConfigService{settingRepo: cafeCouponSettingsRepo(map[string]string{SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":5,"period":"month","transferable":false}}}`})},
	}

	_, err = svc.PreviewCafeCoupon(ctx, 1, "bad code!", 100)
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_INVALID", infraerrors.Reason(err))

	start, end := cafeCouponRollingPeriodWindow(time.Now(), CafeCouponPeriodMonth)
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-OWNER").SetUserID(owner.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(5).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(start).SetPeriodEnd(end).SetStatus(CafeCouponStatusIssued).
		Save(ctx)
	require.NoError(t, err)

	_, err = svc.PreviewCafeCoupon(ctx, owner.ID+1, coupon.Code, 100)
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_FORBIDDEN", infraerrors.Reason(err))
}

func TestCafeCouponPreviewAllowsTransferableCoupon(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	owner, err := client.User.Create().SetEmail("transfer-owner@example.com").SetPasswordHash("hash").SetUsername("transfer-owner").Save(ctx)
	require.NoError(t, err)
	start, end := cafeCouponRollingPeriodWindow(time.Now(), CafeCouponPeriodMonth)
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-TRANSFER").SetUserID(owner.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(5).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(start).SetPeriodEnd(end).SetStatus(CafeCouponStatusIssued).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{
		entClient:     client,
		userRepo:      &mockUserRepo{getByIDUser: &User{ID: owner.ID + 1, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		configService: &PaymentConfigService{settingRepo: cafeCouponSettingsRepo(map[string]string{SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":5,"period":"month","transferable":true}}}`})},
	}

	preview, err := svc.PreviewCafeCoupon(ctx, owner.ID+1, coupon.Code, 100)
	require.NoError(t, err)
	require.True(t, preview.Transferable)
	require.Equal(t, 5.0, preview.DiscountAmount)
}

func TestCafeCouponCreateOrderUsesIssuedCouponSnapshotAfterConfigChange(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("<CAFE:EMAIL:X3UQUMODHUFZDIRSDS3A>").SetPasswordHash("hash").SetUsername("snapshot-order").Save(ctx)
	require.NoError(t, err)
	start, end := cafeCouponRollingPeriodWindow(time.Now(), CafeCouponPeriodWeek)
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-ORDER-SNAPSHOT").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeDiscount).SetValue(30).
		SetPeriod(CafeCouponPeriodWeek).SetPeriodStart(start).SetPeriodEnd(end).SetStatus(CafeCouponStatusIssued).
		Save(ctx)
	require.NoError(t, err)
	userRepo := &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}}
	svc := &PaymentService{
		entClient: client,
		configService: &PaymentConfigService{entClient: client, settingRepo: cafeCouponSettingsRepo(map[string]string{
			SettingPaymentEnabled:      "true",
			SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":10,"period":"month"}}}`,
		})},
		userRepo:      userRepo,
		redeemService: NewRedeemService(&paymentOrderLifecycleRedeemRepo{codesByCode: map[string]*RedeemCode{}}, userRepo, nil, nil, nil, client, nil, nil),
	}
	t.Setenv(paymentDevAutoSuccessEnv, paymentDevAutoSuccessToken)
	t.Setenv(paymentDevEnvironmentEnv, "development")

	resp, err := svc.CreateOrder(ctx, CreateOrderRequest{UserID: user.ID, Amount: 100, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeBalance, CafeCouponCode: coupon.Code, ClientIP: "<CAFE:IPV4:JCK7AX2VOF2VUU7FGILQ>", SrcHost: "app.example.com"})
	require.NoError(t, err)
	require.Equal(t, 70.0, resp.PayAmount)
	order, err := client.PaymentOrder.Get(ctx, resp.OrderID)
	require.NoError(t, err)
	require.Equal(t, 30.0, order.CafeCouponDiscount)
	updatedCoupon, err := client.CafeCoupon.Get(ctx, coupon.ID)
	require.NoError(t, err)
	require.Equal(t, CafeCouponStatusApplied, updatedCoupon.Status)
}

func TestCafeCouponCreateOrderAppliesServerSideDiscountAndMarksCoupon(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("buyer@example.com").SetPasswordHash("hash").SetUsername("buyer").Save(ctx)
	require.NoError(t, err)

	start, end := cafeCouponRollingPeriodWindow(time.Now(), CafeCouponPeriodMonth)
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-ORDER").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(start).SetPeriodEnd(end).SetStatus(CafeCouponStatusIssued).
		Save(ctx)
	require.NoError(t, err)

	userRepo := &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}}
	userRepo.updateBalanceFn = func(ctx context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		return nil
	}
	redeemRepo := &paymentOrderLifecycleRedeemRepo{codesByCode: map[string]*RedeemCode{}}
	svc := &PaymentService{
		entClient: client,
		configService: &PaymentConfigService{entClient: client, settingRepo: cafeCouponSettingsRepo(map[string]string{
			SettingPaymentEnabled:      "true",
			SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":10,"period":"month"}}}`,
		})},
		userRepo:      userRepo,
		redeemService: NewRedeemService(redeemRepo, userRepo, nil, nil, nil, client, nil, nil),
	}
	t.Setenv(paymentDevAutoSuccessEnv, paymentDevAutoSuccessToken)
	t.Setenv(paymentDevEnvironmentEnv, "development")

	resp, err := svc.CreateOrder(ctx, CreateOrderRequest{UserID: user.ID, Amount: 100, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeBalance, CafeCouponCode: coupon.Code, ClientIP: "127.0.0.1", SrcHost: "app.example.com"})
	require.NoError(t, err)
	require.Equal(t, 90.0, resp.PayAmount)

	order, err := client.PaymentOrder.Get(ctx, resp.OrderID)
	require.NoError(t, err)
	require.Equal(t, 90.0, order.PayAmount)
	require.Equal(t, 10.0, order.CafeCouponDiscount)
	require.Equal(t, coupon.Code, psStringValue(order.CafeCouponCode))

	updatedCoupon, err := client.CafeCoupon.Get(ctx, coupon.ID)
	require.NoError(t, err)
	require.Equal(t, CafeCouponStatusApplied, updatedCoupon.Status)
	require.NotNil(t, updatedCoupon.OrderID)
	require.Equal(t, resp.OrderID, *updatedCoupon.OrderID)

	auditCount, err := client.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("CAFE_COUPON_APPLIED")).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, auditCount)
}

func TestCafeCouponCreateOrderRejectsFinalPayAmountBelowMinimum(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("buyer-min-pay@example.com").SetPasswordHash("hash").SetUsername("buyer-min-pay").Save(ctx)
	require.NoError(t, err)

	start, end := cafeCouponRollingPeriodWindow(time.Now(), CafeCouponPeriodMonth)
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-MIN-PAY").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(start).SetPeriodEnd(end).SetStatus(CafeCouponStatusIssued).
		Save(ctx)
	require.NoError(t, err)

	userRepo := &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}}
	svc := &PaymentService{
		entClient: client,
		configService: &PaymentConfigService{entClient: client, settingRepo: cafeCouponSettingsRepo(map[string]string{
			SettingPaymentEnabled:      "true",
			SettingRechargeFeeRate:     "1",
			SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":10,"period":"month"}}}`,
		})},
		userRepo:      userRepo,
		redeemService: NewRedeemService(&paymentOrderLifecycleRedeemRepo{codesByCode: map[string]*RedeemCode{}}, userRepo, nil, nil, nil, client, nil, nil),
	}
	t.Setenv(paymentDevAutoSuccessEnv, paymentDevAutoSuccessToken)
	t.Setenv(paymentDevEnvironmentEnv, "development")

	_, err = svc.CreateOrder(ctx, CreateOrderRequest{UserID: user.ID, Amount: 10, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeBalance, CafeCouponCode: coupon.Code, ClientIP: "127.0.0.1", SrcHost: "app.example.com"})
	require.Error(t, err)
	require.Equal(t, "PAYMENT_AMOUNT_BELOW_MINIMUM", infraerrors.Reason(err))

	count, err := client.PaymentOrder.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, count)
	storedCoupon, err := client.CafeCoupon.Get(ctx, coupon.ID)
	require.NoError(t, err)
	require.Equal(t, CafeCouponStatusIssued, storedCoupon.Status)
}

func TestCafeCouponCreateOrderSelectsProviderWithCouponAdjustedAmount(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("provider-coupon@example.com").SetPasswordHash("hash").SetUsername("provider-coupon").Save(ctx)
	require.NoError(t, err)
	start, end := cafeCouponRollingPeriodWindow(time.Now(), CafeCouponPeriodMonth)
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-PROVIDER-LIMIT").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(20).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(start).SetPeriodEnd(end).SetStatus(CafeCouponStatusIssued).
		Save(ctx)
	require.NoError(t, err)
	lb := &cafeCouponAmountCaptureLoadBalancer{}
	svc := &PaymentService{
		entClient: client,
		configService: &PaymentConfigService{entClient: client, settingRepo: cafeCouponSettingsRepo(map[string]string{
			SettingPaymentEnabled:      "true",
			SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":20,"period":"month"}}}`,
		})},
		userRepo:     &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		loadBalancer: lb,
	}

	_, err = svc.CreateOrder(ctx, CreateOrderRequest{UserID: user.ID, Amount: 110, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeBalance, CafeCouponCode: coupon.Code, ClientIP: "127.0.0.1", SrcHost: "app.example.com"})
	require.Error(t, err)
	require.Equal(t, 90.0, lb.lastAmount)
}

func TestCafeCouponCreateOrderRecalculatesCurrencyFallbackFromOriginalAmount(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("currency-coupon@example.com").SetPasswordHash("hash").SetUsername("currency-coupon").Save(ctx)
	require.NoError(t, err)
	start, end := cafeCouponRollingPeriodWindow(time.Now(), CafeCouponPeriodMonth)
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-CURRENCY").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(20).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(start).SetPeriodEnd(end).SetStatus(CafeCouponStatusIssued).
		Save(ctx)
	require.NoError(t, err)
	lb := &cafeCouponAmountCaptureLoadBalancer{selection: &payment.InstanceSelection{ProviderKey: payment.TypeStripe, Config: map[string]string{"currency": "USD"}, SupportedTypes: payment.TypeAlipay}}
	svc := &PaymentService{
		entClient: client,
		configService: &PaymentConfigService{entClient: client, settingRepo: cafeCouponSettingsRepo(map[string]string{
			SettingPaymentEnabled:      "true",
			SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":20,"period":"month"}}}`,
		})},
		userRepo:     &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		loadBalancer: lb,
	}

	_, err = svc.CreateOrder(ctx, CreateOrderRequest{UserID: user.ID, Amount: 110, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeBalance, CafeCouponCode: coupon.Code, ClientIP: "127.0.0.1", SrcHost: "app.example.com"})
	require.Error(t, err)
	require.Equal(t, 90.0, lb.lastAmount)

	order, err := client.PaymentOrder.Query().Only(ctx)
	require.NoError(t, err)
	require.Equal(t, 90.0, order.PayAmount)
	require.Equal(t, 20.0, order.CafeCouponDiscount)
}

func TestCafeCouponCreateOrderRejectsUsedCoupon(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("used@example.com").SetPasswordHash("hash").SetUsername("used").Save(ctx)
	require.NoError(t, err)
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-USED").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(time.Now().Add(-time.Hour)).SetPeriodEnd(time.Now().Add(time.Hour)).SetStatus(CafeCouponStatusApplied).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient:     client,
		userRepo:      &mockUserRepo{getByIDUser: &User{ID: user.ID, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		configService: &PaymentConfigService{settingRepo: cafeCouponSettingsRepo(map[string]string{SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":10,"period":"month"}}}`})},
	}
	_, err = svc.PreviewCafeCoupon(ctx, user.ID, coupon.Code, 100)
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_USED", infraerrors.Reason(err))
}

func TestCafeCouponReleaseDoesNotReuseExpiredCoupon(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("expired-release@example.com").SetPasswordHash("hash").SetUsername("expired-release").Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).SetAmount(100).SetPayAmount(90).SetFeeRate(0).SetRechargeCode("PAY-EXPIRED").SetOutTradeNo("PAY-EXPIRED").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeBalance).SetStatus(OrderStatusPending).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").
		SetCafeCouponCode("CAFE-EXPIRED-RELEASE").SetCafeCouponDiscount(10).
		Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	_, err = client.CafeCoupon.Create().
		SetCode("CAFE-EXPIRED-RELEASE").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(now.AddDate(0, -2, 0)).SetPeriodEnd(now.Add(-time.Hour)).SetStatus(CafeCouponStatusApplied).SetOrderID(order.ID).SetAppliedAt(now.Add(-2 * time.Hour)).SetCreatedAt(now.AddDate(0, -2, 0)).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{entClient: client}

	svc.releaseCafeCouponForOrder(ctx, order.ID, "test")

	coupon, err := client.CafeCoupon.Query().Only(ctx)
	require.NoError(t, err)
	require.Equal(t, CafeCouponStatusApplied, coupon.Status)
	require.NotNil(t, coupon.OrderID)
}

func TestCafeCouponAdminUpdateStatusAndResetClaimPeriod(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("admin-cafe-status@example.com").SetPasswordHash("hash").SetUsername("admin-cafe-status").Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-ADMIN-STATUS").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(now).SetPeriodEnd(now.AddDate(0, 1, 0)).SetStatus(CafeCouponStatusIssued).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{entClient: client, userRepo: &mockUserRepo{getByIDUser: &User{ID: user.ID, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}}, configService: &PaymentConfigService{settingRepo: cafeCouponSettingsRepo(map[string]string{SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":10,"period":"month"}}}`})}}

	_, err = svc.AdminUpdateCafeCouponStatus(ctx, coupon.ID, CafeCouponStatusApplied)
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_STATUS_INVALID", infraerrors.Reason(err))

	voided, err := svc.AdminUpdateCafeCouponStatus(ctx, coupon.ID, CafeCouponStatusVoid)
	require.NoError(t, err)
	require.Equal(t, CafeCouponStatusVoid, voided.Status)

	restored, err := svc.AdminUpdateCafeCouponStatus(ctx, coupon.ID, CafeCouponStatusIssued)
	require.NoError(t, err)
	require.Equal(t, CafeCouponStatusIssued, restored.Status)
	require.Nil(t, restored.AppliedAt)
	require.Nil(t, restored.OrderID)

	_, err = svc.AdminUpdateCafeCouponStatus(ctx, coupon.ID, "bad")
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_STATUS_INVALID", infraerrors.Reason(err))

	reset, err := svc.AdminResetCafeCouponClaimPeriod(ctx, coupon.ID)
	require.NoError(t, err)
	require.Equal(t, CafeCouponStatusVoid, reset.Status)
	status, err := svc.CafeCouponStatus(ctx, user.ID)
	require.NoError(t, err)
	require.True(t, status.CanClaim)
}

func TestCafeCouponAdminRestoresAppliedCouponForPendingOrder(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("admin-cafe-pending@example.com").SetPasswordHash("hash").SetUsername("admin-cafe-pending").Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).SetAmount(100).SetPayAmount(90).SetFeeRate(0).SetRechargeCode("PAY-ADMIN-CAFE-PENDING").SetOutTradeNo("PAY-ADMIN-CAFE-PENDING").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeBalance).SetStatus(OrderStatusPending).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").
		Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-ADMIN-PENDING").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(now).SetPeriodEnd(now.AddDate(0, 1, 0)).SetStatus(CafeCouponStatusApplied).SetOrderID(order.ID).SetAppliedAt(now).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{entClient: client}

	restored, err := svc.AdminUpdateCafeCouponStatus(ctx, coupon.ID, CafeCouponStatusIssued)
	require.NoError(t, err)
	require.Equal(t, CafeCouponStatusIssued, restored.Status)
	require.Nil(t, restored.OrderID)
	require.Nil(t, restored.AppliedAt)
	stored, err := client.CafeCoupon.Get(ctx, coupon.ID)
	require.NoError(t, err)
	require.Equal(t, CafeCouponStatusIssued, stored.Status)
	require.Nil(t, stored.OrderID)
	require.Nil(t, stored.AppliedAt)
}

func TestCafeCouponAdminRejectsRestoreForCompletedOrder(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("admin-cafe-paid@example.com").SetPasswordHash("hash").SetUsername("admin-cafe-paid").Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).SetAmount(100).SetPayAmount(90).SetFeeRate(0).SetRechargeCode("PAY-ADMIN-CAFE-PAID").SetOutTradeNo("PAY-ADMIN-CAFE-PAID").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("TRADE-ADMIN-CAFE").SetOrderType(payment.OrderTypeBalance).SetStatus(OrderStatusCompleted).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").
		Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-ADMIN-PAID").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(now).SetPeriodEnd(now.AddDate(0, 1, 0)).SetStatus(CafeCouponStatusApplied).SetOrderID(order.ID).SetAppliedAt(now).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{entClient: client}

	_, err = svc.AdminUpdateCafeCouponStatus(ctx, coupon.ID, CafeCouponStatusIssued)
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_STATUS_NOT_ALLOWED", infraerrors.Reason(err))
	stored, err := client.CafeCoupon.Get(ctx, coupon.ID)
	require.NoError(t, err)
	require.Equal(t, CafeCouponStatusApplied, stored.Status)
	require.NotNil(t, stored.OrderID)
	require.Equal(t, order.ID, *stored.OrderID)
	require.NotNil(t, stored.AppliedAt)
}

func newCafeCouponTestClient(t *testing.T) *dbent.Client {
	t.Helper()
	dbName := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name())
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

type cafeCouponAmountCaptureLoadBalancer struct {
	lastAmount float64
	selection  *payment.InstanceSelection
}

func (l *cafeCouponAmountCaptureLoadBalancer) GetInstanceConfig(context.Context, int64) (map[string]string, error) {
	return map[string]string{}, nil
}

func (l *cafeCouponAmountCaptureLoadBalancer) SelectInstance(_ context.Context, _ string, paymentType payment.PaymentType, _ payment.Strategy, orderAmount float64) (*payment.InstanceSelection, error) {
	l.lastAmount = orderAmount
	if l.selection != nil {
		return l.selection, nil
	}
	return &payment.InstanceSelection{ProviderKey: payment.TypeAlipay, SupportedTypes: paymentType}, nil
}

type cafeCouponSettingsRepo map[string]string

func (r cafeCouponSettingsRepo) Get(context.Context, string) (*Setting, error) { return nil, nil }
func (r cafeCouponSettingsRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}
func (r cafeCouponSettingsRepo) Set(_ context.Context, key, value string) error {
	r[key] = value
	return nil
}
func (r cafeCouponSettingsRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = r[key]
	}
	return out, nil
}
func (r cafeCouponSettingsRepo) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		r[key] = value
	}
	return nil
}
func (r cafeCouponSettingsRepo) GetAll(context.Context) (map[string]string, error) { return r, nil }
func (r cafeCouponSettingsRepo) Delete(context.Context, string) error              { return nil }
