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
	"sync"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/cafecoupon"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	entuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
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
	cafeCouponCodePrefix       = "CAFE-"
	cafeCouponMaxValue         = 1000000.0
	cafeCouponValidityMonthEnd = "month_end"
)

var cafeCouponUserClaimLocks sync.Map

type CafeCouponLevelConfig struct {
	Enabled            bool    `json:"enabled"`
	Type               string  `json:"type"`
	Value              float64 `json:"value"`
	Period             string  `json:"period"`
	Transferable       bool    `json:"transferable"`
	Validity           string  `json:"validity"`
	ValidUntilMonthEnd bool    `json:"valid_until_month_end"`
}

type CafeCouponConfig struct {
	Levels map[int]CafeCouponLevelConfig `json:"levels"`
}

type CafeCouponAdminListFilters struct {
	Search          string
	Status          string
	CouponType      string
	MembershipLevel *int
}

type CafeCouponAdminRecord struct {
	ID              int64
	Code            string
	UserID          int64
	MembershipLevel int
	CouponType      string
	Value           float64
	Period          string
	PeriodStart     time.Time
	PeriodEnd       time.Time
	ExpiresAt       time.Time
	Status          string
	OrderID         *int64
	AppliedAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	User            *User
}

type CafeCouponClaimResult struct {
	Code               string    `json:"code"`
	CouponType         string    `json:"type"`
	Value              float64   `json:"value"`
	Period             string    `json:"period"`
	MembershipLevel    int       `json:"membership_level"`
	PeriodStart        time.Time `json:"period_start"`
	PeriodEnd          time.Time `json:"period_end"`
	ExpiresAt          time.Time `json:"expires_at"`
	ClaimedAt          time.Time `json:"claimed_at"`
	AlreadyClaimed     bool      `json:"already_claimed"`
	CanClaim           bool      `json:"can_claim"`
	NextClaimAt        time.Time `json:"next_claim_at"`
	RemainingDays      int       `json:"remaining_days"`
	Status             string    `json:"status"`
	Transferable       bool      `json:"transferable"`
	Validity           string    `json:"validity"`
	ValidUntilMonthEnd bool      `json:"valid_until_month_end"`
}

type CafeCouponStatusResult struct {
	Eligible           bool                   `json:"eligible"`
	CanClaim           bool                   `json:"can_claim"`
	AlreadyClaimed     bool                   `json:"already_claimed"`
	NextClaimAt        time.Time              `json:"next_claim_at,omitempty"`
	RemainingDays      int                    `json:"remaining_days"`
	MembershipLevel    int                    `json:"membership_level"`
	CouponType         string                 `json:"type"`
	Value              float64                `json:"value"`
	Period             string                 `json:"period"`
	PeriodStart        time.Time              `json:"period_start,omitempty"`
	PeriodEnd          time.Time              `json:"period_end,omitempty"`
	ExpiresAt          time.Time              `json:"expires_at,omitempty"`
	Transferable       bool                   `json:"transferable"`
	Validity           string                 `json:"validity"`
	ValidUntilMonthEnd bool                   `json:"valid_until_month_end"`
	Coupon             *CafeCouponClaimResult `json:"coupon,omitempty"`
}

type CafeCouponPreview struct {
	Code               string    `json:"code"`
	CouponType         string    `json:"type"`
	Value              float64   `json:"value"`
	Period             string    `json:"period"`
	MembershipLevel    int       `json:"membership_level"`
	ExpiresAt          time.Time `json:"expires_at"`
	ClaimedAt          time.Time `json:"claimed_at"`
	OriginalAmount     float64   `json:"original_amount"`
	DiscountAmount     float64   `json:"discount_amount"`
	PayableAmount      float64   `json:"payable_amount"`
	Transferable       bool      `json:"transferable"`
	Validity           string    `json:"validity"`
	ValidUntilMonthEnd bool      `json:"valid_until_month_end"`
}

func defaultCafeCouponConfig() CafeCouponConfig {
	levels := make(map[int]CafeCouponLevelConfig, 4)
	for level := 0; level <= 3; level++ {
		levels[level] = CafeCouponLevelConfig{Type: CafeCouponTypeCash, Period: CafeCouponPeriodMonth, Validity: cafeCouponValidityMonthEnd, ValidUntilMonthEnd: true}
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

func normalizeCafeCouponValidity(raw string) string {
	return cafeCouponValidityMonthEnd
}

func normalizeCafeCouponLevelConfig(cfg CafeCouponLevelConfig) CafeCouponLevelConfig {
	cfg.Type = normalizeCafeCouponType(cfg.Type)
	cfg.Period = normalizeCafeCouponPeriod(cfg.Period)
	cfg.Validity = normalizeCafeCouponValidity(cfg.Validity)
	cfg.ValidUntilMonthEnd = true
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

func cafeCouponClaimCooldownAt(claimedAt time.Time, period string) time.Time {
	claimedAt = claimedAt.UTC()
	switch normalizeCafeCouponPeriod(period) {
	case CafeCouponPeriodDay:
		return claimedAt.Add(24 * time.Hour)
	case CafeCouponPeriodWeek:
		return claimedAt.AddDate(0, 0, 7)
	default:
		return claimedAt.AddDate(0, 1, 0)
	}
}

func cafeCouponExpiresAt(claimedAt time.Time, period string) time.Time {
	claimedAt = claimedAt.UTC()
	periodEnd := cafeCouponClaimCooldownAt(claimedAt, period)
	return claimedAt.Add(periodEnd.Sub(claimedAt) * 30 / 100)
}

func cafeCouponRollingPeriodWindow(claimedAt time.Time, period string) (time.Time, time.Time) {
	start := claimedAt.UTC()
	return start, cafeCouponClaimCooldownAt(start, period)
}

func cafeCouponRemainingDays(now, next time.Time) int {
	if !next.After(now) {
		return 0
	}
	hours := next.Sub(now.UTC()).Hours()
	return int(math.Ceil(hours / 24))
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

func (s *PaymentService) AdminListCafeCoupons(ctx context.Context, params pagination.PaginationParams, filters CafeCouponAdminListFilters) ([]CafeCouponAdminRecord, *pagination.PaginationResult, error) {
	if s == nil || s.entClient == nil {
		return nil, nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "payment service unavailable")
	}
	params.SortOrder = pagination.NormalizeSortOrder(params.SortOrder, pagination.SortOrderDesc)
	var err error
	filters, err = validateAdminCafeCouponListFilters(filters)
	if err != nil {
		return nil, nil, err
	}
	query := s.entClient.CafeCoupon.Query().WithUser()
	applyAdminCafeCouponFilters(query, filters)
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("count cafe coupons: %w", err)
	}
	items, err := query.Order(adminCafeCouponOrder(params)...).Offset(params.Offset()).Limit(params.Limit()).All(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list cafe coupons: %w", err)
	}
	out := make([]CafeCouponAdminRecord, 0, len(items))
	for _, item := range items {
		out = append(out, cafeCouponAdminRecordFromEnt(item))
	}
	return out, cafeCouponPaginationResult(total, params), nil
}

func (s *PaymentService) AdminGetCafeCoupon(ctx context.Context, id int64) (*CafeCouponAdminRecord, error) {
	if s == nil || s.entClient == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "payment service unavailable")
	}
	coupon, err := s.entClient.CafeCoupon.Query().Where(cafecoupon.IDEQ(id)).WithUser().Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("CAFE_COUPON_NOT_FOUND", "cafe coupon not found")
		}
		return nil, fmt.Errorf("get cafe coupon: %w", err)
	}
	record := cafeCouponAdminRecordFromEnt(coupon)
	return &record, nil
}

func (s *PaymentService) AdminVoidCafeCoupon(ctx context.Context, id int64) (*CafeCouponAdminRecord, error) {
	if s == nil || s.entClient == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "payment service unavailable")
	}
	coupon, err := s.entClient.CafeCoupon.Query().Where(cafecoupon.IDEQ(id)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("CAFE_COUPON_NOT_FOUND", "cafe coupon not found")
		}
		return nil, fmt.Errorf("get cafe coupon: %w", err)
	}
	if coupon.Status != CafeCouponStatusIssued {
		return nil, infraerrors.Conflict("CAFE_COUPON_VOID_NOT_ALLOWED", "only issued cafe coupons can be voided")
	}
	if coupon.OrderID != nil || coupon.AppliedAt != nil {
		return nil, infraerrors.Conflict("CAFE_COUPON_VOID_NOT_ALLOWED", "applied cafe coupons cannot be voided")
	}
	updated, err := s.entClient.CafeCoupon.UpdateOneID(id).Where(cafecoupon.StatusEQ(CafeCouponStatusIssued), cafecoupon.OrderIDIsNil()).SetStatus(CafeCouponStatusVoid).Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.Conflict("CAFE_COUPON_VOID_NOT_ALLOWED", "cafe coupon can no longer be voided")
		}
		return nil, fmt.Errorf("void cafe coupon: %w", err)
	}
	s.writeCafeCouponAudit(ctx, "CAFE_COUPON_VOIDED", "admin", map[string]any{"coupon_id": updated.ID, "code": updated.Code})
	return s.AdminGetCafeCoupon(ctx, id)
}

func (s *PaymentService) AdminUpdateCafeCouponStatus(ctx context.Context, id int64, status string) (*CafeCouponAdminRecord, error) {
	if s == nil || s.entClient == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "payment service unavailable")
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status != CafeCouponStatusIssued && status != CafeCouponStatusVoid {
		return nil, infraerrors.BadRequest("CAFE_COUPON_STATUS_INVALID", "invalid cafe coupon status")
	}
	coupon, err := s.entClient.CafeCoupon.Query().Where(cafecoupon.IDEQ(id)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("CAFE_COUPON_NOT_FOUND", "cafe coupon not found")
		}
		return nil, fmt.Errorf("get cafe coupon: %w", err)
	}
	if coupon.Status == CafeCouponStatusApplied && status != CafeCouponStatusIssued {
		return nil, infraerrors.Conflict("CAFE_COUPON_STATUS_NOT_ALLOWED", "applied cafe coupons can only be restored to issued")
	}
	if coupon.Status == CafeCouponStatusApplied && status == CafeCouponStatusIssued && coupon.OrderID != nil {
		finalPaid, err := s.cafeCouponOrderIsFinalPaid(ctx, *coupon.OrderID)
		if err != nil {
			return nil, err
		}
		if finalPaid {
			return nil, infraerrors.Conflict("CAFE_COUPON_STATUS_NOT_ALLOWED", "paid order cafe coupons cannot be restored to issued")
		}
	}
	update := s.entClient.CafeCoupon.UpdateOneID(id).SetStatus(status)
	if status == CafeCouponStatusIssued {
		update.ClearOrderID().ClearAppliedAt()
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("update cafe coupon status: %w", err)
	}
	s.writeCafeCouponAudit(ctx, "CAFE_COUPON_STATUS_UPDATED", "admin", map[string]any{"coupon_id": updated.ID, "code": updated.Code, "status": status})
	return s.AdminGetCafeCoupon(ctx, id)
}

func (s *PaymentService) cafeCouponOrderIsFinalPaid(ctx context.Context, orderID int64) (bool, error) {
	order, err := s.entClient.PaymentOrder.Query().Where(paymentorder.IDEQ(orderID)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return false, infraerrors.Conflict("CAFE_COUPON_STATUS_NOT_ALLOWED", "coupon order not found")
		}
		return false, fmt.Errorf("get cafe coupon order: %w", err)
	}
	switch order.Status {
	case OrderStatusPaid, OrderStatusRecharging, OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefunding, OrderStatusPartiallyRefunded, OrderStatusRefunded, OrderStatusRefundFailed:
		return true, nil
	default:
		return false, nil
	}
}

func (s *PaymentService) AdminResetCafeCouponClaimPeriod(ctx context.Context, id int64) (*CafeCouponAdminRecord, error) {
	if s == nil || s.entClient == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "payment service unavailable")
	}
	coupon, err := s.entClient.CafeCoupon.Query().Where(cafecoupon.IDEQ(id)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("CAFE_COUPON_NOT_FOUND", "cafe coupon not found")
		}
		return nil, fmt.Errorf("get cafe coupon: %w", err)
	}
	latest, err := s.latestCafeCouponClaim(ctx, coupon.UserID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return s.AdminGetCafeCoupon(ctx, id)
		}
		return nil, fmt.Errorf("query latest cafe coupon: %w", err)
	}
	now := time.Now().UTC()
	resetStart := adminCafeCouponResetClaimStart(now, latest.Period).Add(-time.Duration(latest.ID) * time.Second)
	resetEnd := now.Add(-time.Minute)
	status := latest.Status
	if latest.Status == CafeCouponStatusIssued && latest.OrderID == nil && latest.AppliedAt == nil {
		status = CafeCouponStatusVoid
	}
	var result entsql.Result
	if err := s.entClient.Driver().Exec(ctx, `UPDATE cafe_coupons SET created_at = $1, period_start = $2, period_end = CASE WHEN period_end <= $2 THEN $3 ELSE period_end END, status = $4, updated_at = $5 WHERE id = $6`, []any{resetStart, resetStart, resetEnd, status, now, latest.ID}, &result); err != nil {
		return nil, fmt.Errorf("reset cafe coupon claim period: %w", err)
	}
	s.writeCafeCouponAudit(ctx, "CAFE_COUPON_CLAIM_PERIOD_RESET", "admin", map[string]any{"coupon_id": latest.ID, "code": latest.Code, "user_id": latest.UserID})
	return s.AdminGetCafeCoupon(ctx, latest.ID)
}

func adminCafeCouponResetClaimStart(now time.Time, period string) time.Time {
	now = now.UTC()
	switch normalizeCafeCouponPeriod(period) {
	case CafeCouponPeriodDay:
		return now.AddDate(0, 0, -1).Add(-time.Minute)
	case CafeCouponPeriodWeek:
		return now.AddDate(0, 0, -7).Add(-time.Minute)
	default:
		return now.AddDate(0, -1, 0).Add(-time.Minute)
	}
}

func validateAdminCafeCouponListFilters(filters CafeCouponAdminListFilters) (CafeCouponAdminListFilters, error) {
	filters.Status = strings.ToLower(strings.TrimSpace(filters.Status))
	if filters.Status != "" && filters.Status != CafeCouponStatusIssued && filters.Status != CafeCouponStatusApplied && filters.Status != CafeCouponStatusVoid {
		return filters, infraerrors.BadRequest("CAFE_COUPON_STATUS_INVALID", "invalid cafe coupon status")
	}
	filters.CouponType = strings.ToLower(strings.TrimSpace(filters.CouponType))
	if filters.CouponType != "" && filters.CouponType != CafeCouponTypeCash && filters.CouponType != CafeCouponTypeDiscount {
		return filters, infraerrors.BadRequest("CAFE_COUPON_TYPE_INVALID", "invalid cafe coupon type")
	}
	if filters.MembershipLevel != nil && (*filters.MembershipLevel < 0 || *filters.MembershipLevel > 3) {
		return filters, infraerrors.BadRequest("CAFE_COUPON_MEMBERSHIP_LEVEL_INVALID", "invalid membership level")
	}
	return filters, nil
}

func applyAdminCafeCouponFilters(query *dbent.CafeCouponQuery, filters CafeCouponAdminListFilters) {
	if query == nil {
		return
	}
	if status := strings.TrimSpace(filters.Status); status != "" {
		query.Where(cafecoupon.StatusEQ(status))
	}
	if couponType := normalizeAdminCafeCouponType(filters.CouponType); couponType != "" {
		query.Where(cafecoupon.CouponTypeEQ(couponType))
	}
	if filters.MembershipLevel != nil {
		query.Where(cafecoupon.MembershipLevelEQ(*filters.MembershipLevel))
	}
	search := strings.TrimSpace(filters.Search)
	if search == "" {
		return
	}
	predicates := []predicate.CafeCoupon{cafecoupon.CodeContainsFold(search)}
	if userID, err := strconv.ParseInt(search, 10, 64); err == nil && userID > 0 {
		predicates = append(predicates, cafecoupon.UserIDEQ(userID))
	}
	query.Where(cafecoupon.Or(predicates...))
}

func normalizeAdminCafeCouponType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case CafeCouponTypeCash:
		return CafeCouponTypeCash
	case CafeCouponTypeDiscount:
		return CafeCouponTypeDiscount
	default:
		return ""
	}
}

func adminCafeCouponOrder(params pagination.PaginationParams) []cafecoupon.OrderOption {
	orderOpt := entsql.OrderDesc()
	if params.SortOrder == pagination.SortOrderAsc {
		orderOpt = entsql.OrderAsc()
	}
	switch strings.TrimSpace(params.SortBy) {
	case "code":
		return []cafecoupon.OrderOption{cafecoupon.ByCode(orderOpt), cafecoupon.ByID(entsql.OrderDesc())}
	case "user_id":
		return []cafecoupon.OrderOption{cafecoupon.ByUserID(orderOpt), cafecoupon.ByID(entsql.OrderDesc())}
	case "membership_level":
		return []cafecoupon.OrderOption{cafecoupon.ByMembershipLevel(orderOpt), cafecoupon.ByID(entsql.OrderDesc())}
	case "type", "coupon_type":
		return []cafecoupon.OrderOption{cafecoupon.ByCouponType(orderOpt), cafecoupon.ByID(entsql.OrderDesc())}
	case "status":
		return []cafecoupon.OrderOption{cafecoupon.ByStatus(orderOpt), cafecoupon.ByID(entsql.OrderDesc())}
	case "expires_at", "period_end":
		return []cafecoupon.OrderOption{cafecoupon.ByPeriodEnd(orderOpt), cafecoupon.ByID(entsql.OrderDesc())}
	case "applied_at":
		return []cafecoupon.OrderOption{cafecoupon.ByAppliedAt(orderOpt), cafecoupon.ByID(entsql.OrderDesc())}
	case "updated_at":
		return []cafecoupon.OrderOption{cafecoupon.ByUpdatedAt(orderOpt), cafecoupon.ByID(entsql.OrderDesc())}
	default:
		return []cafecoupon.OrderOption{cafecoupon.ByCreatedAt(orderOpt), cafecoupon.ByID(entsql.OrderDesc())}
	}
}

func cafeCouponAdminRecordFromEnt(c *dbent.CafeCoupon) CafeCouponAdminRecord {
	if c == nil {
		return CafeCouponAdminRecord{}
	}
	periodStart, periodEnd := cafeCouponRollingPeriodWindow(c.PeriodStart, c.Period)
	return CafeCouponAdminRecord{
		ID:              c.ID,
		Code:            c.Code,
		UserID:          c.UserID,
		MembershipLevel: c.MembershipLevel,
		CouponType:      c.CouponType,
		Value:           c.Value,
		Period:          c.Period,
		PeriodStart:     periodStart,
		PeriodEnd:       periodEnd,
		ExpiresAt:       cafeCouponExpiresAt(c.PeriodStart, c.Period),
		Status:          c.Status,
		OrderID:         c.OrderID,
		AppliedAt:       c.AppliedAt,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
		User:            cafeCouponAdminUserFromEnt(c.Edges.User),
	}
}

func cafeCouponAdminUserFromEnt(u *dbent.User) *User {
	if u == nil {
		return nil
	}
	return &User{ID: u.ID, Email: u.Email, Username: u.Username, Role: u.Role, Balance: u.Balance, Concurrency: u.Concurrency, Status: u.Status, TotalRecharged: u.TotalRecharged, RPMLimit: u.RpmLimit, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt, DeletedAt: u.DeletedAt}
}

func cafeCouponPaginationResult(total int, params pagination.PaginationParams) *pagination.PaginationResult {
	limit := params.Limit()
	pages := total / limit
	if total%limit > 0 {
		pages++
	}
	return &pagination.PaginationResult{Total: int64(total), Page: params.Page, PageSize: limit, Pages: pages}
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
	if s.entClient.Driver().Dialect() == dialect.SQLite {
		unlock, err := lockCafeCouponUserClaim(userID)
		if err != nil {
			return nil, err
		}
		defer unlock()
	}
	return s.claimCafeCouponTx(ctx, userID, level, levelCfg)
}

func lockCafeCouponUserClaim(userID int64) (func(), error) {
	value, _ := cafeCouponUserClaimLocks.LoadOrStore(userID, &sync.Mutex{})
	mu, ok := value.(*sync.Mutex)
	if !ok {
		cafeCouponUserClaimLocks.Delete(userID)
		return nil, fmt.Errorf("invalid cafe coupon claim lock for user %d", userID)
	}
	mu.Lock()
	return mu.Unlock, nil
}

func (s *PaymentService) claimCafeCouponTx(ctx context.Context, userID int64, level int, levelCfg CafeCouponLevelConfig) (*CafeCouponClaimResult, error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin cafe coupon claim transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if s.entClient.Driver().Dialect() != dialect.SQLite {
		_, err = tx.User.Query().Where(entuser.IDEQ(userID)).ForUpdate().OnlyID(ctx)
		if err != nil {
			if dbent.IsNotFound(err) {
				return nil, infraerrors.NotFound("USER_NOT_FOUND", "user not found")
			}
			return nil, fmt.Errorf("lock cafe coupon user: %w", err)
		}
	}
	now := time.Now().UTC()
	latest, err := latestCafeCouponClaimTx(ctx, tx, userID)
	if err != nil && !dbent.IsNotFound(err) {
		return nil, fmt.Errorf("query latest cafe coupon: %w", err)
	}
	if result, err := currentCafeCouponClaimResult(ctx, s, latest, level, levelCfg, now); result != nil || err != nil {
		return result, err
	}
	periodStart, periodEnd := cafeCouponRollingPeriodWindow(now, levelCfg.Period)
	var created *dbent.CafeCoupon
	for attempt := 0; attempt < 5; attempt++ {
		code, err := generateCafeCouponCode()
		if err != nil {
			return nil, fmt.Errorf("generate cafe coupon code: %w", err)
		}
		created, err = tx.CafeCoupon.Create().
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
			if err := tx.Commit(); err != nil {
				return nil, fmt.Errorf("commit cafe coupon claim: %w", err)
			}
			s.writeCafeCouponAudit(ctx, "CAFE_COUPON_CLAIMED", fmt.Sprintf("user:%d", userID), map[string]any{
				"coupon_id": created.ID,
				"code":      created.Code,
				"level":     level,
				"period":    levelCfg.Period,
				"expiresAt": periodEnd,
			})
			return cafeCouponClaimResult(created, false, levelCfg, now), nil
		}
		if !dbent.IsConstraintError(err) {
			return nil, fmt.Errorf("create cafe coupon: %w", err)
		}
	}
	latest, err = latestCafeCouponClaimTx(ctx, tx, userID)
	if err != nil && !dbent.IsNotFound(err) {
		return nil, fmt.Errorf("query latest cafe coupon: %w", err)
	}
	if result, err := currentCafeCouponClaimResult(ctx, s, latest, level, levelCfg, now); result != nil || err != nil {
		return result, err
	}
	return nil, fmt.Errorf("create unique cafe coupon: exhausted retries")
}

func latestCafeCouponClaimTx(ctx context.Context, tx *dbent.Tx, userID int64) (*dbent.CafeCoupon, error) {
	return tx.CafeCoupon.Query().
		Where(cafecoupon.UserIDEQ(userID)).
		Order(cafecoupon.ByCreatedAt(entsql.OrderDesc()), cafecoupon.ByID(entsql.OrderDesc())).
		First(ctx)
}

func currentCafeCouponClaimResult(ctx context.Context, s *PaymentService, latest *dbent.CafeCoupon, level int, levelCfg CafeCouponLevelConfig, now time.Time) (*CafeCouponClaimResult, error) {
	if latest == nil || !cafeCouponClaimCooldownAt(latest.CreatedAt, latest.Period).After(now) {
		return nil, nil
	}
	if latest.Status == CafeCouponStatusIssued && latest.MembershipLevel <= level {
		if latestCfg, err := cafeCouponConfigFromCoupon(ctx, s, latest); err == nil {
			return cafeCouponClaimResult(latest, true, latestCfg, now), nil
		}
		return cafeCouponClaimResult(latest, true, levelCfg, now), nil
	}
	return nil, cafeCouponAlreadyClaimedError(latest, levelCfg, now)
}

func (s *PaymentService) latestCafeCouponClaim(ctx context.Context, userID int64) (*dbent.CafeCoupon, error) {
	return s.entClient.CafeCoupon.Query().
		Where(cafecoupon.UserIDEQ(userID)).
		Order(cafecoupon.ByCreatedAt(entsql.OrderDesc()), cafecoupon.ByID(entsql.OrderDesc())).
		First(ctx)
}

func cafeCouponAlreadyClaimedError(coupon *dbent.CafeCoupon, cfg CafeCouponLevelConfig, now time.Time) error {
	result := cafeCouponClaimResult(coupon, true, cfg, now)
	metadata := map[string]string{}
	if result != nil {
		metadata["already_claimed"] = "true"
		metadata["can_claim"] = "false"
		metadata["next_claim_at"] = result.NextClaimAt.Format(time.RFC3339)
		metadata["remaining_days"] = strconv.Itoa(result.RemainingDays)
	}
	return infraerrors.Conflict("CAFE_COUPON_ALREADY_CLAIMED", "current cafe coupon claim cooldown has not elapsed").WithMetadata(metadata)
}

func cafeCouponClaimResult(c *dbent.CafeCoupon, already bool, cfg CafeCouponLevelConfig, now time.Time) *CafeCouponClaimResult {
	if c == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nextClaimAt := cafeCouponClaimCooldownAt(c.CreatedAt, c.Period)
	periodStart, periodEnd := cafeCouponRollingPeriodWindow(c.CreatedAt, c.Period)
	return &CafeCouponClaimResult{
		Code:               c.Code,
		CouponType:         c.CouponType,
		Value:              c.Value,
		Period:             c.Period,
		MembershipLevel:    c.MembershipLevel,
		PeriodStart:        periodStart,
		PeriodEnd:          periodEnd,
		ExpiresAt:          cafeCouponExpiresAt(c.CreatedAt, c.Period),
		ClaimedAt:          c.CreatedAt,
		AlreadyClaimed:     already,
		CanClaim:           !already,
		NextClaimAt:        nextClaimAt,
		RemainingDays:      cafeCouponRemainingDays(now, nextClaimAt),
		Status:             c.Status,
		Transferable:       cfg.Transferable,
		Validity:           cfg.Validity,
		ValidUntilMonthEnd: cfg.ValidUntilMonthEnd,
	}
}

func (s *PaymentService) CafeCouponStatus(ctx context.Context, userID int64) (*CafeCouponStatusResult, error) {
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
	now := time.Now().UTC()
	periodStart, periodEnd := cafeCouponRollingPeriodWindow(now, levelCfg.Period)
	status := &CafeCouponStatusResult{
		Eligible:           levelCfg.Enabled && levelCfg.Value > 0,
		CanClaim:           levelCfg.Enabled && levelCfg.Value > 0,
		MembershipLevel:    level,
		CouponType:         levelCfg.Type,
		Value:              levelCfg.Value,
		Period:             levelCfg.Period,
		PeriodStart:        periodStart,
		PeriodEnd:          periodEnd,
		ExpiresAt:          cafeCouponExpiresAt(now, levelCfg.Period),
		NextClaimAt:        periodEnd,
		RemainingDays:      cafeCouponRemainingDays(now, periodEnd),
		Transferable:       levelCfg.Transferable,
		Validity:           levelCfg.Validity,
		ValidUntilMonthEnd: levelCfg.ValidUntilMonthEnd,
	}
	if !status.Eligible {
		status.CanClaim = false
		return status, nil
	}
	latest, err := s.latestCafeCouponClaim(ctx, userID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return status, nil
		}
		return nil, fmt.Errorf("query cafe coupon: %w", err)
	}
	status.AlreadyClaimed = true
	status.Coupon = cafeCouponClaimResult(latest, true, levelCfg, now)
	if status.Coupon != nil {
		status.NextClaimAt = status.Coupon.NextClaimAt
		status.RemainingDays = status.Coupon.RemainingDays
		status.CanClaim = !status.NextClaimAt.After(now)
		if status.CanClaim {
			status.AlreadyClaimed = false
			status.Coupon = nil
		} else if latest.MembershipLevel > level {
			status.Coupon = nil
		} else if latestCfg, err := cafeCouponConfigFromCoupon(ctx, s, latest); err == nil {
			status.Coupon = cafeCouponClaimResult(latest, true, latestCfg, now)
		}
	}
	return status, nil
}

func (s *PaymentService) PreviewCafeCoupon(ctx context.Context, userID int64, code string, originalAmount float64) (*CafeCouponPreview, error) {
	coupon, levelCfg, err := s.validateCafeCoupon(ctx, userID, code)
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
		Code:               coupon.Code,
		CouponType:         coupon.CouponType,
		Value:              coupon.Value,
		Period:             coupon.Period,
		MembershipLevel:    coupon.MembershipLevel,
		ExpiresAt:          cafeCouponExpiresAt(coupon.CreatedAt, coupon.Period),
		ClaimedAt:          coupon.CreatedAt,
		OriginalAmount:     decimal.NewFromFloat(originalAmount).Round(2).InexactFloat64(),
		DiscountAmount:     discount,
		PayableAmount:      payable,
		Transferable:       levelCfg.Transferable,
		Validity:           levelCfg.Validity,
		ValidUntilMonthEnd: levelCfg.ValidUntilMonthEnd,
	}, nil
}

func (s *PaymentService) validateCafeCoupon(ctx context.Context, userID int64, rawCode string) (*dbent.CafeCoupon, CafeCouponLevelConfig, error) {
	code, err := normalizeCafeCouponCode(rawCode)
	if err != nil {
		return nil, CafeCouponLevelConfig{}, err
	}
	coupon, err := s.entClient.CafeCoupon.Query().Where(cafecoupon.CodeEQ(code)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, CafeCouponLevelConfig{}, infraerrors.NotFound("CAFE_COUPON_NOT_FOUND", "cafe coupon not found")
		}
		return nil, CafeCouponLevelConfig{}, fmt.Errorf("get cafe coupon: %w", err)
	}
	levelCfg, err := s.validateCafeCouponEntity(ctx, userID, coupon)
	if err != nil {
		return nil, CafeCouponLevelConfig{}, err
	}
	return coupon, levelCfg, nil
}

func (s *PaymentService) validateCafeCouponEntity(ctx context.Context, userID int64, coupon *dbent.CafeCoupon) (CafeCouponLevelConfig, error) {
	return s.validateCafeCouponEntityForStatus(ctx, userID, coupon, CafeCouponStatusIssued)
}

func cafeCouponConfigFromCoupon(ctx context.Context, s *PaymentService, coupon *dbent.CafeCoupon) (CafeCouponLevelConfig, error) {
	cfg := CafeCouponLevelConfig{Enabled: true, Type: coupon.CouponType, Value: coupon.Value, Period: normalizeCafeCouponPeriod(coupon.Period), Validity: cafeCouponValidityMonthEnd, ValidUntilMonthEnd: true}
	settingSvc := s.settingServiceForCafeCoupons()
	if settingSvc == nil {
		return cfg, nil
	}
	levelCfg, err := settingSvc.cafeCouponLevelConfig(ctx, coupon.MembershipLevel)
	if err != nil {
		return cfg, err
	}
	cfg.Transferable = levelCfg.Transferable
	return cfg, nil
}

func (s *PaymentService) validateCafeCouponEntityForStatus(ctx context.Context, userID int64, coupon *dbent.CafeCoupon, allowedStatus string) (CafeCouponLevelConfig, error) {
	if coupon == nil {
		return CafeCouponLevelConfig{}, infraerrors.NotFound("CAFE_COUPON_NOT_FOUND", "cafe coupon not found")
	}
	if coupon.Status != allowedStatus {
		return CafeCouponLevelConfig{}, infraerrors.Conflict("CAFE_COUPON_USED", "cafe coupon has already been applied")
	}
	if !cafeCouponExpiresAt(coupon.CreatedAt, coupon.Period).After(time.Now().UTC()) {
		return CafeCouponLevelConfig{}, infraerrors.Conflict("CAFE_COUPON_EXPIRED", "cafe coupon has expired")
	}
	if s.userRepo != nil {
		user, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return CafeCouponLevelConfig{}, fmt.Errorf("get user: %w", err)
		}
		if user.Status != payment.EntityStatusActive {
			return CafeCouponLevelConfig{}, infraerrors.Forbidden("USER_INACTIVE", "user account is disabled")
		}
		if CalculateMembershipLevel(user.TotalRecharged) < coupon.MembershipLevel {
			return CafeCouponLevelConfig{}, infraerrors.Forbidden("CAFE_COUPON_NOT_ELIGIBLE", "membership level no longer satisfies this coupon")
		}
	}
	levelCfg, err := cafeCouponConfigFromCoupon(ctx, s, coupon)
	if err != nil {
		return CafeCouponLevelConfig{}, err
	}
	if coupon.UserID != userID && !levelCfg.Transferable {
		return CafeCouponLevelConfig{}, infraerrors.Forbidden("CAFE_COUPON_FORBIDDEN", "cafe coupon does not belong to current user")
	}
	return levelCfg, nil
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
	if _, err := s.validateCafeCouponEntity(ctx, userID, coupon); err != nil {
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
	coupon, err := s.entClient.CafeCoupon.Query().
		Where(cafecoupon.OrderIDEQ(orderID), cafecoupon.StatusEQ(CafeCouponStatusApplied)).
		Only(ctx)
	if err != nil {
		if !dbent.IsNotFound(err) {
			s.writeAuditLog(ctx, orderID, "CAFE_COUPON_RELEASE_FAILED", "system", map[string]any{"reason": reason, "error": err.Error()})
		}
		return
	}
	if !cafeCouponExpiresAt(coupon.CreatedAt, coupon.Period).After(time.Now().UTC()) {
		return
	}
	updated, err := s.entClient.CafeCoupon.Update().
		Where(cafecoupon.IDEQ(coupon.ID), cafecoupon.StatusEQ(CafeCouponStatusApplied)).
		SetStatus(CafeCouponStatusIssued).
		ClearOrderID().
		ClearAppliedAt().
		Save(ctx)
	if err != nil {
		s.writeAuditLog(ctx, orderID, "CAFE_COUPON_RELEASE_FAILED", "system", map[string]any{"reason": reason, "coupon_id": coupon.ID, "error": err.Error()})
		return
	}
	if updated == 0 {
		s.writeAuditLog(ctx, orderID, "CAFE_COUPON_RELEASE_FAILED", "system", map[string]any{"reason": reason, "coupon_id": coupon.ID, "error": "not_updated"})
		return
	}
	s.writeAuditLog(ctx, orderID, "CAFE_COUPON_RELEASED", "system", map[string]any{"reason": reason})
}

func (s *PaymentService) ensureCafeCouponAppliedForPaidOrder(ctx context.Context, order *dbent.PaymentOrder) error {
	if s == nil || s.entClient == nil || order == nil || strings.TrimSpace(psStringValue(order.CafeCouponCode)) == "" {
		return nil
	}
	code, err := normalizeCafeCouponCode(psStringValue(order.CafeCouponCode))
	if err != nil {
		return err
	}
	coupon, err := s.entClient.CafeCoupon.Query().Where(cafecoupon.CodeEQ(code)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			s.writeAuditLog(ctx, order.ID, "CAFE_COUPON_PAYMENT_INVALID", "system", map[string]any{"reason": "coupon_not_found", "code": code})
			return infraerrors.Conflict("CAFE_COUPON_CHANGED", "cafe coupon is no longer available")
		}
		return fmt.Errorf("get cafe coupon: %w", err)
	}
	if coupon.Status == CafeCouponStatusApplied && coupon.OrderID != nil && *coupon.OrderID == order.ID {
		if _, err := s.validateCafeCouponEntityForStatus(ctx, order.UserID, coupon, CafeCouponStatusApplied); err != nil {
			s.writeAuditLog(ctx, order.ID, "CAFE_COUPON_PAYMENT_INVALID", "system", map[string]any{"reason": infraerrors.Reason(err), "code": code})
			return err
		}
		return nil
	}
	if _, err := s.validateCafeCouponEntity(ctx, order.UserID, coupon); err != nil {
		s.writeAuditLog(ctx, order.ID, "CAFE_COUPON_PAYMENT_INVALID", "system", map[string]any{"reason": infraerrors.Reason(err), "code": code, "coupon_status": coupon.Status})
		return err
	}
	updated, err := s.entClient.CafeCoupon.Update().
		Where(cafecoupon.IDEQ(coupon.ID), cafecoupon.StatusEQ(CafeCouponStatusIssued)).
		SetStatus(CafeCouponStatusApplied).
		SetOrderID(order.ID).
		SetAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("reapply cafe coupon: %w", err)
	}
	if updated == 0 {
		s.writeAuditLog(ctx, order.ID, "CAFE_COUPON_PAYMENT_INVALID", "system", map[string]any{"reason": "coupon_reused", "code": code})
		return infraerrors.Conflict("CAFE_COUPON_USED", "cafe coupon has already been applied")
	}
	s.writeAuditLog(ctx, order.ID, "CAFE_COUPON_REAPPLIED", "system", map[string]any{"code": code})
	return nil
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
		return subscriptionOrderAmount(plan, req.Multiplier)
	}
	if req.OrderType == payment.OrderTypeBalance {
		return req.Amount
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
	if plan != nil {
		multiplier, err := s.resolveSubscriptionOrderMultiplier(ctx, req.UserID, plan, req.Multiplier)
		if err != nil {
			return nil, err
		}
		req.Multiplier = multiplier
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
