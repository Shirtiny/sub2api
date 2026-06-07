package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/cafecoupon"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const (
	CafeCouponTypeCash      = "cash"
	CafeCouponTypeDiscount  = "discount"
	CafeCouponPeriodDay     = "day"
	CafeCouponPeriodWeek    = "week"
	CafeCouponPeriodMonth   = "month"
	CafeCouponStatusIssued  = "issued"
	CafeCouponStatusApplied = "applied"
	CafeCouponStatusVoid    = "void"
)

const (
	cafeCouponCodePrefix = "CAFE-"
	cafeCouponMaxValue   = 1000000.0
)

type CafeCouponLevelConfig struct {
	Enabled bool    `json:"enabled"`
	Type    string  `json:"type"`
	Value   float64 `json:"value"`
	Period  string  `json:"period"`
}

type CafeCouponConfig struct {
	Levels map[int]CafeCouponLevelConfig `json:"levels"`
}

type CafeCouponClaimResult struct {
	Code            string    `json:"code"`
	CouponType      string    `json:"type"`
	Value           float64   `json:"value"`
	Period          string    `json:"period"`
	MembershipLevel int       `json:"membership_level"`
	PeriodStart     time.Time `json:"period_start"`
	PeriodEnd       time.Time `json:"period_end"`
	ClaimedAt       time.Time `json:"claimed_at"`
	AlreadyClaimed  bool      `json:"already_claimed"`
}

type CafeCouponPreview struct {
	Code            string    `json:"code"`
	CouponType      string    `json:"type"`
	Value           float64   `json:"value"`
	Period          string    `json:"period"`
	MembershipLevel int       `json:"membership_level"`
	ExpiresAt       time.Time `json:"expires_at"`
	ClaimedAt       time.Time `json:"claimed_at"`
	OriginalAmount  float64   `json:"original_amount"`
	DiscountAmount  float64   `json:"discount_amount"`
	PayableAmount   float64   `json:"payable_amount"`
}

func defaultCafeCouponConfig() CafeCouponConfig {
	levels := make(map[int]CafeCouponLevelConfig, 4)
	for level := 0; level <= 3; level++ {
		levels[level] = CafeCouponLevelConfig{Type: CafeCouponTypeCash, Period: CafeCouponPeriodMonth}
	}
	return CafeCouponConfig{Levels: levels}
}

func normalizeCafeCouponPeriod(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case CafeCouponPeriodDay:
		return CafeCouponPeriodDay
	case CafeCouponPeriodWeek:
		return CafeCouponPeriodWeek
	default:
		return CafeCouponPeriodMonth
	}
}

func normalizeCafeCouponType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case CafeCouponTypeDiscount:
		return CafeCouponTypeDiscount
	default:
		return CafeCouponTypeCash
	}
}

func normalizeCafeCouponLevelConfig(cfg CafeCouponLevelConfig) CafeCouponLevelConfig {
	cfg.Type = normalizeCafeCouponType(cfg.Type)
	cfg.Period = normalizeCafeCouponPeriod(cfg.Period)
	if math.IsNaN(cfg.Value) || math.IsInf(cfg.Value, 0) || cfg.Value < 0 {
		cfg.Value = 0
	}
	if cfg.Type == CafeCouponTypeDiscount && cfg.Value > 100 {
		cfg.Value = 100
	}
	if cfg.Type == CafeCouponTypeCash && cfg.Value > cafeCouponMaxValue {
		cfg.Value = cafeCouponMaxValue
	}
	if cfg.Value <= 0 {
		cfg.Enabled = false
	}
	return cfg
}

func normalizeCafeCouponConfig(cfg CafeCouponConfig) CafeCouponConfig {
	out := defaultCafeCouponConfig()
	if cfg.Levels == nil {
		return out
	}
	for level := 0; level <= 3; level++ {
		if item, ok := cfg.Levels[level]; ok {
			out.Levels[level] = normalizeCafeCouponLevelConfig(item)
		}
	}
	return out
}

func (s *SettingService) GetCafeCouponConfig(ctx context.Context) (CafeCouponConfig, error) {
	if s == nil || s.settingRepo == nil {
		return defaultCafeCouponConfig(), nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyCafeCouponConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return defaultCafeCouponConfig(), nil
		}
		return defaultCafeCouponConfig(), fmt.Errorf("get cafe coupon config: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return defaultCafeCouponConfig(), nil
	}
	var cfg CafeCouponConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return defaultCafeCouponConfig(), nil
	}
	return normalizeCafeCouponConfig(cfg), nil
}

func (s *SettingService) SetCafeCouponConfig(ctx context.Context, cfg CafeCouponConfig) error {
	cfg = normalizeCafeCouponConfig(cfg)
	b, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal cafe coupon config: %w", err)
	}
	return s.settingRepo.Set(ctx, SettingKeyCafeCouponConfig, string(b))
}

func (s *SettingService) cafeCouponLevelConfig(ctx context.Context, level int) (CafeCouponLevelConfig, error) {
	if level < 0 {
		level = 0
	}
	if level > 3 {
		level = 3
	}
	cfg, err := s.GetCafeCouponConfig(ctx)
	if err != nil {
		return CafeCouponLevelConfig{}, err
	}
	return normalizeCafeCouponLevelConfig(cfg.Levels[level]), nil
}

func cafeCouponPeriodWindow(now time.Time, period string) (time.Time, time.Time) {
	now = now.UTC()
	y, m, d := now.Date()
	switch normalizeCafeCouponPeriod(period) {
	case CafeCouponPeriodDay:
		start := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 0, 1)
	case CafeCouponPeriodWeek:
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(weekday - 1))
		return start, start.AddDate(0, 0, 7)
	default:
		start := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0)
	}
}

func normalizeCafeCouponCode(raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if code == "" || len(code) > 48 {
		return "", infraerrors.BadRequest("CAFE_COUPON_INVALID", "invalid cafe coupon code")
	}
	for _, r := range code {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return "", infraerrors.BadRequest("CAFE_COUPON_INVALID", "invalid cafe coupon code")
		}
	}
	return code, nil
}

func generateCafeCouponCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return cafeCouponCodePrefix + strings.ToUpper(hex.EncodeToString(b)), nil
}

func cafeCouponDiscountAmount(originalAmount float64, couponType string, value float64) (float64, error) {
	if originalAmount <= 0 || math.IsNaN(originalAmount) || math.IsInf(originalAmount, 0) {
		return 0, infraerrors.BadRequest("INVALID_AMOUNT", "amount must be a positive number")
	}
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, infraerrors.BadRequest("CAFE_COUPON_INVALID", "invalid cafe coupon value")
	}
	original := decimal.NewFromFloat(originalAmount)
	var discount decimal.Decimal
	switch normalizeCafeCouponType(couponType) {
	case CafeCouponTypeDiscount:
		if value > 100 {
			value = 100
		}
		discount = original.Mul(decimal.NewFromFloat(value)).Div(decimal.NewFromInt(100))
	default:
		discount = decimal.NewFromFloat(value)
	}
	if discount.GreaterThanOrEqual(original) {
		discount = original.Sub(decimal.NewFromFloat(0.01))
	}
	if discount.IsNegative() {
		discount = decimal.Zero
	}
	return discount.Round(2).InexactFloat64(), nil
}

func (s *PaymentService) settingServiceForCafeCoupons() *SettingService {
	if s == nil || s.configService == nil || s.configService.settingRepo == nil {
		return nil
	}
	return &SettingService{settingRepo: s.configService.settingRepo}
}

func (s *PaymentService) ClaimCafeCoupon(ctx context.Context, userID int64) (*CafeCouponClaimResult, error) {
	if s == nil || s.entClient == nil || s.userRepo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "payment service unavailable")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user.Status != payment.EntityStatusActive {
		return nil, infraerrors.Forbidden("USER_INACTIVE", "user account is disabled")
	}
	level := CalculateMembershipLevel(user.TotalRecharged)
	settingSvc := s.settingServiceForCafeCoupons()
	if settingSvc == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "settings service unavailable")
	}
	levelCfg, err := settingSvc.cafeCouponLevelConfig(ctx, level)
	if err != nil {
		return nil, err
	}
	if !levelCfg.Enabled || levelCfg.Value <= 0 {
		return nil, infraerrors.Forbidden("CAFE_COUPON_NOT_ELIGIBLE", "current membership level is not eligible for cafe coupons")
	}
	periodStart, periodEnd := cafeCouponPeriodWindow(time.Now(), levelCfg.Period)
	existing, err := s.entClient.CafeCoupon.Query().
		Where(cafecoupon.UserIDEQ(userID), cafecoupon.PeriodEQ(levelCfg.Period), cafecoupon.PeriodStartEQ(periodStart)).
		Only(ctx)
	if err == nil && existing != nil {
		return cafeCouponClaimResult(existing, true), nil
	}
	if err != nil && !dbent.IsNotFound(err) {
		return nil, fmt.Errorf("query cafe coupon: %w", err)
	}

	var created *dbent.CafeCoupon
	for attempt := 0; attempt < 5; attempt++ {
		code, err := generateCafeCouponCode()
		if err != nil {
			return nil, fmt.Errorf("generate cafe coupon code: %w", err)
		}
		created, err = s.entClient.CafeCoupon.Create().
			SetCode(code).
			SetUserID(userID).
			SetMembershipLevel(level).
			SetCouponType(levelCfg.Type).
			SetValue(levelCfg.Value).
			SetPeriod(levelCfg.Period).
			SetPeriodStart(periodStart).
			SetPeriodEnd(periodEnd).
			SetStatus(CafeCouponStatusIssued).
			Save(ctx)
		if err == nil {
			s.writeCafeCouponAudit(ctx, "CAFE_COUPON_CLAIMED", fmt.Sprintf("user:%d", userID), map[string]any{
				"coupon_id": created.ID,
				"code":      created.Code,
				"level":     level,
				"period":    levelCfg.Period,
			})
			return cafeCouponClaimResult(created, false), nil
		}
		if !dbent.IsConstraintError(err) {
			return nil, fmt.Errorf("create cafe coupon: %w", err)
		}
	}
	existing, err = s.entClient.CafeCoupon.Query().
		Where(cafecoupon.UserIDEQ(userID), cafecoupon.PeriodEQ(levelCfg.Period), cafecoupon.PeriodStartEQ(periodStart)).
		Only(ctx)
	if err == nil && existing != nil {
		return cafeCouponClaimResult(existing, true), nil
	}
	return nil, fmt.Errorf("create unique cafe coupon: exhausted retries")
}

func cafeCouponClaimResult(c *dbent.CafeCoupon, already bool) *CafeCouponClaimResult {
	if c == nil {
		return nil
	}
	return &CafeCouponClaimResult{
		Code:            c.Code,
		CouponType:      c.CouponType,
		Value:           c.Value,
		Period:          c.Period,
		MembershipLevel: c.MembershipLevel,
		PeriodStart:     c.PeriodStart,
		PeriodEnd:       c.PeriodEnd,
		ClaimedAt:       c.CreatedAt,
		AlreadyClaimed:  already,
	}
}

func (s *PaymentService) PreviewCafeCoupon(ctx context.Context, userID int64, code string, originalAmount float64) (*CafeCouponPreview, error) {
	coupon, err := s.validateCafeCoupon(ctx, userID, code)
	if err != nil {
		return nil, err
	}
	discount, err := cafeCouponDiscountAmount(originalAmount, coupon.CouponType, coupon.Value)
	if err != nil {
		return nil, err
	}
	payable := decimal.NewFromFloat(originalAmount).Sub(decimal.NewFromFloat(discount)).Round(2).InexactFloat64()
	if payable <= 0 {
		return nil, infraerrors.BadRequest("CAFE_COUPON_INVALID", "coupon discount exceeds order amount")
	}
	return &CafeCouponPreview{
		Code:            coupon.Code,
		CouponType:      coupon.CouponType,
		Value:           coupon.Value,
		Period:          coupon.Period,
		MembershipLevel: coupon.MembershipLevel,
		ExpiresAt:       coupon.PeriodEnd,
		ClaimedAt:       coupon.CreatedAt,
		OriginalAmount:  decimal.NewFromFloat(originalAmount).Round(2).InexactFloat64(),
		DiscountAmount:  discount,
		PayableAmount:   payable,
	}, nil
}

func (s *PaymentService) validateCafeCoupon(ctx context.Context, userID int64, rawCode string) (*dbent.CafeCoupon, error) {
	code, err := normalizeCafeCouponCode(rawCode)
	if err != nil {
		return nil, err
	}
	coupon, err := s.entClient.CafeCoupon.Query().Where(cafecoupon.CodeEQ(code)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("CAFE_COUPON_NOT_FOUND", "cafe coupon not found")
		}
		return nil, fmt.Errorf("get cafe coupon: %w", err)
	}
	if err := s.validateCafeCouponEntity(ctx, userID, coupon); err != nil {
		return nil, err
	}
	return coupon, nil
}

func (s *PaymentService) validateCafeCouponEntity(ctx context.Context, userID int64, coupon *dbent.CafeCoupon) error {
	if coupon == nil {
		return infraerrors.NotFound("CAFE_COUPON_NOT_FOUND", "cafe coupon not found")
	}
	if coupon.UserID != userID {
		return infraerrors.Forbidden("CAFE_COUPON_FORBIDDEN", "cafe coupon does not belong to current user")
	}
	if coupon.Status != CafeCouponStatusIssued {
		return infraerrors.Conflict("CAFE_COUPON_USED", "cafe coupon has already been applied")
	}
	if !coupon.PeriodEnd.After(time.Now().UTC()) {
		return infraerrors.Conflict("CAFE_COUPON_EXPIRED", "cafe coupon has expired")
	}
	if s.userRepo != nil {
		user, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return fmt.Errorf("get user: %w", err)
		}
		if user.Status != payment.EntityStatusActive {
			return infraerrors.Forbidden("USER_INACTIVE", "user account is disabled")
		}
		if CalculateMembershipLevel(user.TotalRecharged) < coupon.MembershipLevel {
			return infraerrors.Forbidden("CAFE_COUPON_NOT_ELIGIBLE", "membership level no longer satisfies this coupon")
		}
	}
	return nil
}

func (s *PaymentService) applyCafeCouponToOrderTx(ctx context.Context, tx *dbent.Tx, orderID, userID int64, rawCode string, originalAmount, expectedDiscount float64) error {
	if strings.TrimSpace(rawCode) == "" {
		return nil
	}
	code, err := normalizeCafeCouponCode(rawCode)
	if err != nil {
		return err
	}
	coupon, err := tx.Client().CafeCoupon.Query().
		Where(cafecoupon.CodeEQ(code)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return infraerrors.NotFound("CAFE_COUPON_NOT_FOUND", "cafe coupon not found")
		}
		return fmt.Errorf("lock cafe coupon: %w", err)
	}
	if err := s.validateCafeCouponEntity(ctx, userID, coupon); err != nil {
		return err
	}
	discount, err := cafeCouponDiscountAmount(originalAmount, coupon.CouponType, coupon.Value)
	if err != nil {
		return err
	}
	if math.Abs(discount-expectedDiscount) > 0.01 {
		return infraerrors.Conflict("CAFE_COUPON_CHANGED", "cafe coupon discount changed, please retry")
	}
	now := time.Now()
	updated, err := tx.Client().CafeCoupon.Update().
		Where(cafecoupon.IDEQ(coupon.ID), cafecoupon.StatusEQ(CafeCouponStatusIssued)).
		SetStatus(CafeCouponStatusApplied).
		SetOrderID(orderID).
		SetAppliedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("apply cafe coupon: %w", err)
	}
	if updated == 0 {
		return infraerrors.Conflict("CAFE_COUPON_USED", "cafe coupon has already been applied")
	}
	if err := s.writeAuditLogStrict(dbent.NewTxContext(ctx, tx), orderID, "CAFE_COUPON_APPLIED", fmt.Sprintf("user:%d", userID), map[string]any{
		"couponCode":     coupon.Code,
		"couponType":     coupon.CouponType,
		"couponValue":    coupon.Value,
		"discountAmount": discount,
	}); err != nil {
		return err
	}
	return nil
}

func (s *PaymentService) releaseCafeCouponForOrder(ctx context.Context, orderID int64, reason string) {
	if s == nil || s.entClient == nil || orderID <= 0 {
		return
	}
	updated, err := s.entClient.CafeCoupon.Update().
		Where(cafecoupon.OrderIDEQ(orderID), cafecoupon.StatusEQ(CafeCouponStatusApplied)).
		SetStatus(CafeCouponStatusIssued).
		ClearOrderID().
		ClearAppliedAt().
		Save(ctx)
	if err != nil || updated == 0 {
		return
	}
	s.writeAuditLog(ctx, orderID, "CAFE_COUPON_RELEASED", "system", map[string]any{"reason": reason})
}

func (s *PaymentService) writeCafeCouponAudit(ctx context.Context, action, op string, detail map[string]any) {
	if s == nil || s.entClient == nil {
		return
	}
	dj, _ := json.Marshal(detail)
	_, _ = s.entClient.PaymentAuditLog.Create().
		SetOrderID("cafe_coupon").
		SetAction(action).
		SetOperator(op).
		SetDetail(string(dj)).
		Save(ctx)
}

func cafeCouponOrderOriginalAmount(req CreateOrderRequest, plan *dbent.SubscriptionPlan, cfg *PaymentConfig) float64 {
	if plan != nil {
		return plan.Price
	}
	if req.OrderType == payment.OrderTypeBalance {
		return req.Amount
	}
	return req.Amount
}

func cafeCouponCreditedAmount(req CreateOrderRequest, plan *dbent.SubscriptionPlan, cfg *PaymentConfig) float64 {
	if plan != nil {
		return plan.Price
	}
	if req.OrderType == payment.OrderTypeBalance {
		return calculateCreditedBalance(req.Amount, cfg.BalanceRechargeMultiplier)
	}
	return req.Amount
}

func (s *PaymentService) prepareCafeCouponForOrder(ctx context.Context, req CreateOrderRequest, plan *dbent.SubscriptionPlan, cfg *PaymentConfig, limitAmount float64, currency string, feeRate float64) (float64, float64, float64, error) {
	if strings.TrimSpace(req.CafeCouponCode) == "" {
		_, payAmount, err := calculateCreateOrderPayAmount(limitAmount, feeRate, currency)
		return 0, limitAmount, payAmount, err
	}
	preview, err := s.PreviewCafeCoupon(ctx, req.UserID, req.CafeCouponCode, cafeCouponOrderOriginalAmount(req, plan, cfg))
	if err != nil {
		return 0, 0, 0, err
	}
	adjustedLimit := decimal.NewFromFloat(limitAmount).Sub(decimal.NewFromFloat(preview.DiscountAmount)).Round(2).InexactFloat64()
	if adjustedLimit <= 0 {
		return 0, 0, 0, infraerrors.BadRequest("CAFE_COUPON_INVALID", "coupon discount exceeds order amount")
	}
	_, adjustedPayAmount, err := calculateCreateOrderPayAmount(adjustedLimit, feeRate, currency)
	if err != nil {
		return 0, 0, 0, err
	}
	if adjustedPayAmount <= 0 {
		return 0, 0, 0, infraerrors.BadRequest("CAFE_COUPON_INVALID", "coupon discount exceeds order amount")
	}
	return preview.DiscountAmount, adjustedLimit, adjustedPayAmount, nil
}

func (s *PaymentService) PreviewCafeCouponForOrder(ctx context.Context, req CreateOrderRequest) (*CafeCouponPreview, error) {
	if req.OrderType == "" {
		req.OrderType = payment.OrderTypeBalance
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get payment config: %w", err)
	}
	plan, err := s.validateOrderInput(ctx, req, cfg)
	if err != nil {
		return nil, err
	}
	return s.PreviewCafeCoupon(ctx, req.UserID, req.CafeCouponCode, cafeCouponOrderOriginalAmount(req, plan, cfg))
}

func (s *PaymentService) CafeCouponForOrder(ctx context.Context, orderID int64) (*dbent.CafeCoupon, error) {
	coupon, err := s.entClient.CafeCoupon.Query().Where(cafecoupon.OrderIDEQ(orderID)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("CAFE_COUPON_NOT_FOUND", "cafe coupon not found")
		}
		return nil, err
	}
	return coupon, nil
}

func cafeCouponOrderSnapshot(order *dbent.PaymentOrder) map[string]any {
	if order == nil || strings.TrimSpace(psStringValue(order.CafeCouponCode)) == "" {
		return nil
	}
	return map[string]any{
		"code":     psStringValue(order.CafeCouponCode),
		"discount": order.CafeCouponDiscount,
	}
}

func cafeCouponAuditOrderID(orderID int64) string {
	if orderID <= 0 {
		return "cafe_coupon"
	}
	return strconv.FormatInt(orderID, 10)
}

func (s *PaymentService) hasAppliedCafeCoupon(ctx context.Context, orderID int64) bool {
	if s == nil || s.entClient == nil || orderID <= 0 {
		return false
	}
	exists, _ := s.entClient.CafeCoupon.Query().Where(cafecoupon.OrderIDEQ(orderID), cafecoupon.StatusEQ(CafeCouponStatusApplied)).Exist(ctx)
	return exists
}

func (s *PaymentService) orderCouponDiscount(ctx context.Context, orderID int64) float64 {
	if s == nil || s.entClient == nil || orderID <= 0 {
		return 0
	}
	order, err := s.entClient.PaymentOrder.Query().Where(paymentorder.IDEQ(orderID)).Only(ctx)
	if err != nil || order == nil {
		return 0
	}
	return order.CafeCouponDiscount
}
