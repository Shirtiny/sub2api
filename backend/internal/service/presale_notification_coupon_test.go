//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestPresaleNoticeCouponConfiguration(t *testing.T) {
	ctx := context.Background()
	payment, _, _ := newPresaleFixture(t)
	s := payment.configService
	s.settingRepo = newNotificationEmailMemorySettingRepo()
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, presaleLocation)
	start, end, err := cafeCampaignDates("2026-09-25", "2026-09-30")
	require.NoError(t, err)
	campaign := payment.entClient.CafeCampaign.Create().SetCode("CAFE-PUBLIC-AUTUMN40").SetName("Autumn").SetDiscountPercent(40).SetEnabled(true).SetStartsAt(start).SetExpiresAt(end).SetCreatedBy(1).SaveX(ctx)
	code, coupon, err := s.GetPresaleNoticeCoupon(ctx, now)
	require.NoError(t, err)
	require.Empty(t, code)
	require.Nil(t, coupon)
	code, err = s.SavePresaleNoticeCoupon(ctx, " cafe-public-autumn40 ", now)
	require.NoError(t, err)
	require.Equal(t, campaign.Code, code)
	// Reload through a separate service: configuration is not browser memory.
	reloaded := NewPaymentConfigService(payment.entClient, s.settingRepo, nil)
	code, coupon, err = reloaded.GetPresaleNoticeCoupon(ctx, now)
	require.NoError(t, err)
	require.Equal(t, campaign.Code, code)
	require.Equal(t, 40, coupon.DiscountPercent)
	require.True(t, coupon.ExpiresAt.Equal(end))

	for _, invalid := range []string{"CAFE-PERSONAL-123", "CAFE-PUBLIC-NOT-FOUND", `<script>alert(1)</script>`, "CAFE-PUBLIC-" + strings.Repeat("A", 48)} {
		_, err = s.SavePresaleNoticeCoupon(ctx, invalid, now)
		require.Equal(t, "PRESALE_NOTICE_COUPON_INVALID", infraerrors.Reason(err))
		retained, _, err := s.GetPresaleNoticeCoupon(ctx, now)
		require.NoError(t, err)
		require.Equal(t, campaign.Code, retained, "invalid input must not overwrite the saved code")
	}

	// A scheduled offer in this month's remaining presale window may be announced.
	_, err = s.SavePresaleNoticeCoupon(ctx, campaign.Code, start.Add(-24*time.Hour))
	require.NoError(t, err)
	_, err = s.SavePresaleNoticeCoupon(ctx, campaign.Code, start.AddDate(0, -1, 0))
	require.Equal(t, "PRESALE_NOTICE_COUPON_INVALID", infraerrors.Reason(err), "offer starts outside that month's presale period")

	// Expiry/disable hides display without erasing the administrator's selection.
	code, coupon, err = s.GetPresaleNoticeCoupon(ctx, end.Add(-time.Second))
	require.NoError(t, err)
	require.Equal(t, campaign.Code, code)
	require.NotNil(t, coupon)
	payment.entClient.CafeCampaign.UpdateOneID(campaign.ID).SetEnabled(false).SaveX(ctx)
	code, coupon, err = s.GetPresaleNoticeCoupon(ctx, now)
	require.NoError(t, err)
	require.Equal(t, campaign.Code, code)
	require.Nil(t, coupon)
	_, err = s.SavePresaleNoticeCoupon(ctx, campaign.Code, now)
	require.Equal(t, "PRESALE_NOTICE_COUPON_INVALID", infraerrors.Reason(err))
	payment.entClient.CafeCampaign.UpdateOneID(campaign.ID).SetEnabled(true).SaveX(ctx)
	_, err = s.SavePresaleNoticeCoupon(ctx, campaign.Code, end)
	require.Equal(t, "PRESALE_NOTICE_COUPON_INVALID", infraerrors.Reason(err))
	code, coupon, err = s.GetPresaleNoticeCoupon(ctx, end)
	require.NoError(t, err)
	require.Empty(t, code, "a new month must not reuse the old month's code")
	require.Nil(t, coupon)

	_, err = s.SavePresaleNoticeCoupon(ctx, "  ", now)
	require.NoError(t, err)
	code, coupon, err = s.GetPresaleNoticeCoupon(ctx, now)
	require.NoError(t, err)
	require.Empty(t, code)
	require.Nil(t, coupon)
	require.Zero(t, payment.entClient.CafeCampaignUse.Query().CountX(ctx), "display configuration must never consume a coupon")
}

func TestPresaleNoticeExpiredCouponHiddenWithinSameMonth(t *testing.T) {
	ctx := context.Background()
	payment, _, _ := newPresaleFixture(t)
	s := payment.configService
	s.settingRepo = newNotificationEmailMemorySettingRepo()
	start, end, err := cafeCampaignDates("2026-09-25", "2026-09-28")
	require.NoError(t, err)
	campaign := payment.entClient.CafeCampaign.Create().SetCode("CAFE-PUBLIC-SHORT").SetName("Short offer").SetDiscountPercent(20).SetEnabled(true).SetStartsAt(start).SetExpiresAt(end).SetCreatedBy(1).SaveX(ctx)
	_, err = s.SavePresaleNoticeCoupon(ctx, campaign.Code, start)
	require.NoError(t, err)
	code, coupon, err := s.GetPresaleNoticeCoupon(ctx, end)
	require.NoError(t, err)
	require.Equal(t, campaign.Code, code)
	require.Nil(t, coupon)
}
