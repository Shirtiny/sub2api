package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/cafecampaign"
	"github.com/Wei-Shaw/sub2api/ent/cafecampaignuse"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const cafeCampaignPrefix = "CAFE-PUBLIC-"

var errCafeCampaignUsageLimit = infraerrors.Conflict("CAFE_CAMPAIGN_USAGE_LIMIT", "cannot apply this code: each account may use it only once").WithMetadata(map[string]string{"limit": "1"})

func isCafeCampaignCode(code string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(code)), cafeCampaignPrefix)
}

type CreateCafeCampaignRequest struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	DiscountPercent int    `json:"discount_percent"`
	StartDate       string `json:"start_date"`
	EndDate         string `json:"end_date"`
	Enabled         bool   `json:"enabled"`
}

func cafeCampaignDates(start, end string) (time.Time, time.Time, error) {
	from, e1 := time.ParseInLocation("2006-01-02", start, presaleLocation)
	last, e2 := time.ParseInLocation("2006-01-02", end, presaleLocation)
	if e1 != nil || e2 != nil || last.Before(from) || from.Year() < 2000 || last.Year() > 9998 {
		return time.Time{}, time.Time{}, infraerrors.BadRequest("CAFE_CAMPAIGN_INVALID", "use valid start and inclusive end dates in Asia/Shanghai")
	}
	return from, last.AddDate(0, 0, 1), nil
}

func (s *PaymentService) CreateCafeCampaign(ctx context.Context, adminID int64, req CreateCafeCampaignRequest) (*dbent.CafeCampaign, error) {
	if adminID <= 0 {
		return nil, infraerrors.Forbidden("FORBIDDEN", "administrator required")
	}
	req.Name = strings.TrimSpace(req.Name)
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if !strings.HasPrefix(code, cafeCampaignPrefix) {
		code = cafeCampaignPrefix + code
	}
	normalized, err := normalizeCafeCouponCode(code)
	if err != nil || len(normalized) <= len(cafeCampaignPrefix) || normalized[len(cafeCampaignPrefix)] == '-' || req.Name == "" || utf8.RuneCountInString(req.Name) > 100 || req.DiscountPercent < 1 || req.DiscountPercent > 99 {
		return nil, infraerrors.BadRequest("CAFE_CAMPAIGN_INVALID", "supply a code, name and discount between 1 and 99 percent")
	}
	starts, expires, err := cafeCampaignDates(req.StartDate, req.EndDate)
	if err != nil {
		return nil, err
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// Match PostgreSQL timestamp precision in returned optimistic-lock tokens.
	now := time.Now().Truncate(time.Microsecond)
	campaign, err := tx.CafeCampaign.Create().SetCode(normalized).SetName(req.Name).SetDiscountPercent(req.DiscountPercent).SetStartsAt(starts).SetExpiresAt(expires).SetEnabled(req.Enabled).SetCreatedBy(adminID).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if dbent.IsConstraintError(err) {
		return nil, infraerrors.Conflict("CAFE_CAMPAIGN_EXISTS", "this campaign code already exists")
	}
	if err != nil {
		return nil, err
	}
	if err = cafeCampaignAudit(ctx, tx, campaign, adminID, "CREATED"); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return campaign, nil
}

func cafeCampaignAudit(ctx context.Context, tx *dbent.Tx, c *dbent.CafeCampaign, adminID int64, action string) error {
	detail, err := json.Marshal(map[string]any{"campaign_id": c.ID, "code": c.Code, "discount_percent": c.DiscountPercent, "enabled": c.Enabled, "starts_at": c.StartsAt, "expires_at": c.ExpiresAt, "scope": "presale", "per_user_limit": 1})
	if err != nil {
		return err
	}
	// Audit order/action is globally unique; each admin change has its own identity.
	_, err = tx.PaymentAuditLog.Create().SetOrderID(fmt.Sprintf("cafe_campaign:%d:%d", c.ID, c.UpdatedAt.UnixNano())).SetAction("CAFE_CAMPAIGN_" + action).SetOperator(fmt.Sprintf("admin:%d", adminID)).SetDetail(string(detail)).Save(ctx)
	return err
}

func (s *PaymentService) SetCafeCampaignEnabled(ctx context.Context, id, adminID int64, enabled bool, expected time.Time) (*dbent.CafeCampaign, error) {
	if adminID <= 0 {
		return nil, infraerrors.Forbidden("FORBIDDEN", "administrator required")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	q := tx.CafeCampaign.Query().Where(cafecampaign.IDEQ(id))
	if supportsForUpdate(tx.Client()) {
		q = q.ForUpdate()
	}
	c, err := q.Only(ctx)
	if dbent.IsNotFound(err) {
		return nil, infraerrors.NotFound("NOT_FOUND", "campaign not found")
	}
	if err != nil {
		return nil, err
	}
	if expected.IsZero() || !c.UpdatedAt.Equal(expected) {
		return nil, infraerrors.Conflict("CAFE_CAMPAIGN_STALE", "campaign changed; refresh before editing")
	}
	if c.Enabled != enabled {
		c, err = tx.CafeCampaign.UpdateOneID(id).SetEnabled(enabled).SetUpdatedAt(time.Now().Truncate(time.Microsecond)).Save(ctx)
		if err != nil {
			return nil, err
		}
		if err = cafeCampaignAudit(ctx, tx, c, adminID, "STATUS"); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *PaymentService) ListCafeCampaigns(ctx context.Context, page, size int) ([]*dbent.CafeCampaign, int, error) {
	total, err := s.entClient.CafeCampaign.Query().Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.entClient.CafeCampaign.Query().Order(dbent.Desc(cafecampaign.FieldID)).Offset((page - 1) * size).Limit(size).All(ctx)
	return rows, total, err
}

type CafeCampaignUsage struct {
	UserID         int64      `json:"user_id"`
	UserEmail      string     `json:"user_email"`
	OrderID        int64      `json:"order_id"`
	OrderStatus    string     `json:"order_status"`
	UsedAt         *time.Time `json:"used_at"`
	PaidAt         *time.Time `json:"paid_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	DiscountAmount float64    `json:"discount_amount"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (s *PaymentService) ListCafeCampaignUses(ctx context.Context, id int64, page, size int) ([]CafeCampaignUsage, int, error) {
	exists, err := s.entClient.CafeCampaign.Query().Where(cafecampaign.IDEQ(id)).Exist(ctx)
	if err != nil {
		return nil, 0, err
	}
	if !exists {
		return nil, 0, infraerrors.NotFound("NOT_FOUND", "campaign not found")
	}
	q := s.entClient.CafeCampaignUse.Query().Where(cafecampaignuse.CampaignIDEQ(id))
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	uses, err := q.Order(dbent.Desc(cafecampaignuse.FieldID)).Offset((page - 1) * size).Limit(size).All(ctx)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]int64, 0, len(uses))
	for _, u := range uses {
		ids = append(ids, u.OrderID)
	}
	orders, err := s.entClient.PaymentOrder.Query().Where(paymentorder.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, 0, err
	}
	byID := make(map[int64]*dbent.PaymentOrder, len(orders))
	for _, o := range orders {
		byID[o.ID] = o
	}
	rows := make([]CafeCampaignUsage, 0, len(uses))
	for _, u := range uses {
		if o := byID[u.OrderID]; o != nil {
			rows = append(rows, CafeCampaignUsage{UserID: u.UserID, UserEmail: o.UserEmail, OrderID: o.ID, OrderStatus: o.Status, UsedAt: u.UsedAt, PaidAt: o.PaidAt, ExpiresAt: o.ExpiresAt, DiscountAmount: o.CafeCouponDiscount, CreatedAt: u.CreatedAt})
		}
	}
	return rows, total, nil
}

func validateCafeCampaignWindow(c *dbent.CafeCampaign, now time.Time) error {
	if !c.Enabled {
		return infraerrors.Conflict("CAFE_CAMPAIGN_DISABLED", "campaign is paused")
	}
	if now.Before(c.StartsAt) {
		return infraerrors.Conflict("CAFE_CAMPAIGN_NOT_STARTED", "campaign has not started")
	}
	if !now.Before(c.ExpiresAt) {
		return infraerrors.Conflict("CAFE_COUPON_EXPIRED", "cafe coupon has expired")
	}
	return nil
}
func loadCafeCampaign(ctx context.Context, client *dbent.Client, code string, lock bool) (*dbent.CafeCampaign, error) {
	normalized, err := normalizeCafeCouponCode(code)
	if err != nil {
		return nil, err
	}
	q := client.CafeCampaign.Query().Where(cafecampaign.CodeEQ(normalized))
	if lock && supportsForUpdate(client) {
		q = q.ForUpdate()
	}
	c, err := q.Only(ctx)
	if dbent.IsNotFound(err) {
		return nil, infraerrors.NotFound("CAFE_COUPON_NOT_FOUND", "cafe coupon not found")
	}
	return c, err
}
func cafeCampaignSlot(ctx context.Context, client *dbent.Client, id, userID int64, now time.Time) (*dbent.CafeCampaignUse, error) {
	u, err := client.CafeCampaignUse.Query().Where(cafecampaignuse.CampaignIDEQ(id), cafecampaignuse.UserIDEQ(userID)).Only(ctx)
	if dbent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if u.UsedAt != nil {
		return nil, errCafeCampaignUsageLimit
	}
	order, err := client.PaymentOrder.Get(ctx, u.OrderID)
	if err != nil {
		return nil, err
	}
	if order.PaidAt != nil {
		return nil, errCafeCampaignUsageLimit
	}
	if order.Status == OrderStatusCancelled || order.Status == OrderStatusExpired || order.Status == OrderStatusFailed || (order.Status == OrderStatusPending && !order.ExpiresAt.After(now)) {
		return u, nil
	}
	return nil, infraerrors.Conflict("CAFE_CAMPAIGN_RESERVED", "an unpaid order already reserves this code; cancel it first")
}
func (s *PaymentService) cafeCampaignInfo(ctx context.Context, userID int64, code string) (*CafeCouponInfo, error) {
	if userID <= 0 {
		return nil, infraerrors.Forbidden("FORBIDDEN", "login required")
	}
	c, err := loadCafeCampaign(ctx, s.entClient, code, false)
	if err != nil {
		return nil, err
	}
	if err = validateCafeCampaignWindow(c, time.Now()); err != nil {
		return nil, err
	}
	if _, err = cafeCampaignSlot(ctx, s.entClient, c.ID, userID, time.Now()); err != nil {
		return nil, err
	}
	return &CafeCouponInfo{Code: c.Code, CouponType: CafeCouponTypeDiscount, Value: float64(c.DiscountPercent), Period: CafeCouponPeriodMonth, ExpiresAt: c.ExpiresAt, ClaimedAt: c.CreatedAt, Validity: "fixed", PresaleOnly: true}, nil
}
func (s *PaymentService) previewCafeCouponForPurchase(ctx context.Context, req CreateOrderRequest, original float64) (*CafeCouponPreview, error) {
	if !isCafeCampaignCode(req.CafeCouponCode) {
		return s.PreviewCafeCoupon(ctx, req.UserID, req.CafeCouponCode, original)
	}
	if req.OrderType != payment.OrderTypeSubscription || req.PresaleMonth == "" {
		return nil, infraerrors.BadRequest("CAFE_CAMPAIGN_PRESALE_ONLY", "this coupon is only for presale subscriptions")
	}
	info, err := s.cafeCampaignInfo(ctx, req.UserID, req.CafeCouponCode)
	if err != nil {
		return nil, err
	}
	discount, err := cafeCouponDiscountAmount(original, CafeCouponTypeDiscount, info.Value)
	if err != nil {
		return nil, err
	}
	return &CafeCouponPreview{Code: info.Code, CouponType: CafeCouponTypeDiscount, Value: info.Value, Period: info.Period, ExpiresAt: info.ExpiresAt, ClaimedAt: info.ClaimedAt, Validity: "fixed", OriginalAmount: original, DiscountAmount: discount, PayableAmount: decimal.NewFromFloat(original).Sub(decimal.NewFromFloat(discount)).Round(2).InexactFloat64(), PresaleOnly: true}, nil
}

// Caller holds the payment user lock. Lock campaign against concurrent admin
// disable; a unique row is the cross-plan, cross-group per-account reservation.
func (s *PaymentService) reserveCafeCampaignTx(ctx context.Context, tx *dbent.Tx, o *dbent.PaymentOrder, code string, limitAmount float64) (float64, error) {
	if o.OrderType != payment.OrderTypeSubscription || o.PresaleStartsAt == nil || o.PresaleExpiresAt == nil {
		return 0, infraerrors.BadRequest("CAFE_CAMPAIGN_PRESALE_ONLY", "this coupon is only for presale subscriptions")
	}
	c, err := loadCafeCampaign(ctx, tx.Client(), code, true)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	if err = validateCafeCampaignWindow(c, now); err != nil {
		return 0, err
	}
	use, err := cafeCampaignSlot(ctx, tx.Client(), c.ID, o.UserID, now)
	if err != nil {
		return 0, err
	}
	discount, err := cafeCouponDiscountAmount(o.Amount, CafeCouponTypeDiscount, float64(c.DiscountPercent))
	if err != nil {
		return 0, err
	}
	if !decimal.NewFromFloat(o.Amount).Sub(decimal.NewFromFloat(discount)).Round(2).Equal(decimal.NewFromFloat(limitAmount).Round(2)) {
		return 0, infraerrors.Conflict("CAFE_COUPON_CHANGED", "coupon price changed; retry checkout")
	}
	if use == nil {
		_, err = tx.CafeCampaignUse.Create().SetCampaignID(c.ID).SetUserID(o.UserID).SetOrderID(o.ID).Save(ctx)
	} else {
		_, err = tx.CafeCampaignUse.UpdateOneID(use.ID).SetOrderID(o.ID).Save(ctx)
	}
	if err != nil {
		return 0, err
	}
	if o.ExpiresAt.After(c.ExpiresAt) {
		o.ExpiresAt = c.ExpiresAt
		if _, err = tx.PaymentOrder.UpdateOneID(o.ID).SetExpiresAt(c.ExpiresAt).Save(ctx); err != nil {
			return 0, err
		}
	}
	err = s.writeAuditLogStrict(dbent.NewTxContext(ctx, tx), o.ID, "CAFE_CAMPAIGN_RESERVED", fmt.Sprintf("user:%d", o.UserID), map[string]any{"campaign_id": c.ID, "code": c.Code, "discount_percent": c.DiscountPercent, "discount_amount": discount, "scope": "presale"})
	return discount, err
}

func (s *PaymentService) consumeCafeCampaignForPaidOrder(ctx context.Context, o *dbent.PaymentOrder) error {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockPaymentUserForUpdate(ctx, tx, o.UserID); err != nil {
		return err
	}
	q := tx.PaymentOrder.Query().Where(paymentorder.IDEQ(o.ID))
	if supportsForUpdate(tx.Client()) {
		q = q.ForUpdate()
	}
	current, err := q.Only(ctx)
	if err != nil {
		return err
	}
	if current.OrderType != payment.OrderTypeSubscription || current.PresaleStartsAt == nil || current.PresaleExpiresAt == nil || current.PaidAt == nil || !psSliceContains([]string{OrderStatusPaid, OrderStatusRecharging, OrderStatusFailed}, current.Status) {
		return infraerrors.Conflict("CAFE_COUPON_CHANGED", "invalid paid presale order")
	}
	c, err := loadCafeCampaign(ctx, tx.Client(), psStringValue(current.CafeCouponCode), false)
	if err != nil {
		return err
	}
	u, err := tx.CafeCampaignUse.Query().Where(cafecampaignuse.CampaignIDEQ(c.ID), cafecampaignuse.UserIDEQ(current.UserID)).Only(ctx)
	if err != nil {
		return err
	}
	if u.OrderID != current.ID {
		return infraerrors.Conflict("CAFE_COUPON_USED", "a newer order owns this coupon reservation")
	}
	// Immutable terms and original accepted order snapshot are authoritative after
	// payment. Do not invalidate an already-issued payment when an admin pauses it.
	expected, err := cafeCouponDiscountAmount(current.Amount, CafeCouponTypeDiscount, float64(c.DiscountPercent))
	if err != nil {
		return err
	}
	if current.CreatedAt.Before(c.StartsAt) || !current.CreatedAt.Before(c.ExpiresAt) || expected != current.CafeCouponDiscount {
		return infraerrors.Conflict("CAFE_COUPON_CHANGED", "invalid coupon snapshot")
	}
	if u.UsedAt == nil {
		// Do not depend on the timeout worker having changed PENDING to EXPIRED.
		// Keep the existing short callback grace, not an unlimited stale discount.
		if !current.PaidAt.Before(current.ExpiresAt.Add(paymentGraceMinutes * time.Minute)) {
			return infraerrors.Conflict("CAFE_CAMPAIGN_PAYMENT_EXPIRED", "discounted payment arrived after the payment deadline; contact support to reconcile")
		}
		if _, err = tx.CafeCampaignUse.UpdateOneID(u.ID).SetUsedAt(*current.PaidAt).Save(ctx); err != nil {
			return err
		}
		if err = s.writeAuditLogStrict(dbent.NewTxContext(ctx, tx), current.ID, "CAFE_CAMPAIGN_USED", "system", map[string]any{"campaign_id": c.ID, "code": c.Code, "user_id": current.UserID}); err != nil {
			return err
		}
	}
	return tx.Commit()
}
