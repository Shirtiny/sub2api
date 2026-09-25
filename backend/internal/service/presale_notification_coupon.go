package service

import (
	"context"
	"errors"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/cafecampaign"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const presaleNoticeCouponSettingPrefix = "presale_notice_coupon."

type PresaleNoticeConfig struct {
	Month      string `json:"month"`
	CouponCode string `json:"coupon_code"`
}

// This is display data, not a coupon grant or proof of personal eligibility.
type PresaleNoticeCoupon struct {
	Code            string    `json:"code"`
	DiscountPercent int       `json:"discount_percent"`
	StartsAt        time.Time `json:"starts_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

func (s *PaymentConfigService) presaleNoticeCoupon(ctx context.Context, code string, now time.Time) (*PresaleNoticeCoupon, error) {
	normalized, err := normalizeCafeCouponCode(code)
	if err != nil || !isCafeCampaignCode(normalized) {
		return nil, nil
	}
	c, err := s.entClient.CafeCampaign.Query().Where(cafecampaign.CodeEQ(normalized)).Only(ctx)
	if dbent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Future-starting offers can be announced with their explicit dates, but
	// never advertise an offer that starts after this presale period closes.
	if !c.Enabled || !c.ExpiresAt.After(now) || !c.StartsAt.Before(NextPresalePeriod(now).StartsAt) {
		return nil, nil
	}
	return &PresaleNoticeCoupon{Code: c.Code, DiscountPercent: c.DiscountPercent, StartsAt: c.StartsAt, ExpiresAt: c.ExpiresAt}, nil
}

func (s *PaymentConfigService) GetPresaleNoticeCoupon(ctx context.Context, now time.Time) (string, *PresaleNoticeCoupon, error) {
	code, err := s.settingRepo.GetValue(ctx, presaleNoticeCouponSettingPrefix+NextPresalePeriod(now).Month)
	if errors.Is(err, ErrSettingNotFound) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return "", nil, nil
	}
	coupon, err := s.presaleNoticeCoupon(ctx, code, now)
	return code, coupon, err
}

func (s *PaymentConfigService) SavePresaleNoticeCoupon(ctx context.Context, code string, now time.Time) (string, error) {
	code = strings.TrimSpace(code)
	if code != "" {
		coupon, err := s.presaleNoticeCoupon(ctx, code, now)
		if err != nil {
			return "", err
		}
		if coupon == nil {
			return "", infraerrors.BadRequest("PRESALE_NOTICE_COUPON_INVALID", "请填写已启用、未过期且适用于本期预售的通用咖啡券完整券码")
		}
		code = coupon.Code
	}
	if err := s.settingRepo.Set(ctx, presaleNoticeCouponSettingPrefix+NextPresalePeriod(now).Month, code); err != nil {
		return "", err
	}
	return code, nil
}

func (s *PresaleNotificationService) UpdateConfig(ctx context.Context, req PresaleNoticeConfig, adminID int64) (*PresaleNoticeConfig, error) {
	if adminID <= 0 {
		return nil, infraerrors.Forbidden("FORBIDDEN", "administrator required")
	}
	now := s.now()
	month := NextPresalePeriod(now).Month
	if req.Month != month {
		return nil, infraerrors.Conflict("PRESALE_NOTICE_CHANGED", "预售月份已变化，请刷新预览后重新配置")
	}
	code, err := s.catalog.SavePresaleNoticeCoupon(ctx, req.CouponCode, now)
	if err != nil {
		return nil, err
	}
	return &PresaleNoticeConfig{Month: month, CouponCode: code}, nil
}
