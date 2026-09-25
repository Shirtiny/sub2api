package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivity"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivityparticipation"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivityplan"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

type PresaleBalanceBenefit struct {
	Currency       string    `json:"currency"`
	CreditedAmount float64   `json:"credited_amount,omitempty"`
	Version        string    `json:"version"`
	ActivityID     int64     `json:"activity_id"`
	Name           string    `json:"name"`
	Amount         float64   `json:"amount"`
	StartsAt       time.Time `json:"starts_at"`
	EndsAt         time.Time `json:"ends_at"`
	MaxUsesPerUser int       `json:"max_uses_per_user"`
}

func activePresaleBalanceActivities(ctx context.Context, client *dbent.Client, ids []int64, now time.Time) ([]*dbent.PromotionActivity, error) {
	q := client.PromotionActivity.Query().Where(promotionactivity.ActivityTypeEQ(PromotionActivityTypePresaleBalance), promotionactivity.EnabledEQ(true), promotionactivity.StartsAtLTE(now), promotionactivity.EndsAtGT(now), promotionactivity.HasPlanBonusesWith(promotionactivityplan.PlanIDIn(ids...))).WithPlanBonuses(func(q *dbent.PromotionActivityPlanQuery) { q.Where(promotionactivityplan.PlanIDIn(ids...)) }).Order(dbent.Asc(promotionactivity.FieldEndsAt), dbent.Asc(promotionactivity.FieldID))
	if dbent.TxFromContext(ctx) != nil && supportsForUpdate(client) {
		q = q.ForShare()
	}
	return q.All(ctx)
}
func presaleBalanceBenefit(a *dbent.PromotionActivity, p *dbent.PromotionActivityPlan) *PresaleBalanceBenefit {
	// Hash economic terms, not mutable display names. Normalize times to PostgreSQL
	// precision so quotes survive a database round trip without false conflicts.
	terms := fmt.Sprintf("%d|%d|%s|%s|%d|%d|%d", a.ID, p.PlanID, a.BonusCurrency,
		decimal.NewFromFloat(p.BonusBalance).String(), a.StartsAt.UnixMicro(), a.EndsAt.UnixMicro(), a.MaxUsesPerUser)
	version := fmt.Sprintf("%x", sha256.Sum256([]byte(terms)))
	credited := p.BonusBalance
	if a.BonusCurrency == "CNY" {
		credited = 0
	}
	return &PresaleBalanceBenefit{Currency: a.BonusCurrency, CreditedAmount: credited, Version: version, ActivityID: a.ID, Name: a.Name, Amount: p.BonusBalance, StartsAt: a.StartsAt, EndsAt: a.EndsAt, MaxUsesPerUser: a.MaxUsesPerUser}
}

// Called on both the quote and locked order path. A changed CNY conversion
// rate must require renewed confirmation, just like a changed face value.
func convertPresaleBalanceBenefit(b *PresaleBalanceBenefit, configuredRate float64) error {
	if b == nil || b.Currency != "CNY" {
		return nil
	}
	rate := decimal.NewFromFloat(normalizeBalanceRechargeMultiplier(configuredRate)).Round(8)
	if !rate.IsPositive() || rate.GreaterThanOrEqual(decimal.NewFromInt(1000000000000)) {
		return infraerrors.BadRequest("PRESALE_BALANCE_BONUS_RATE_INVALID", "invalid gift conversion rate")
	}
	credited := decimal.NewFromFloat(b.Amount).Mul(rate).Round(8).InexactFloat64()
	if !validPresaleCreditedBonusAmount(credited) {
		return infraerrors.BadRequest("PRESALE_BALANCE_BONUS_RATE_INVALID", "gift conversion is out of range")
	}
	b.CreditedAmount = credited
	b.Version = fmt.Sprintf("%x", sha256.Sum256([]byte(b.Version+"|"+rate.String())))
	return nil
}
func validPresaleCreditedBonusAmount(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0 && v <= 1000000
}

func (s *PaymentConfigService) PublicPresaleBalanceBenefits(ctx context.Context, ids []int64) (map[int64]*PresaleBalanceBenefit, error) {
	out := map[int64]*PresaleBalanceBenefit{}
	if len(ids) == 0 {
		return out, nil
	}
	activities, err := activePresaleBalanceActivities(ctx, s.entClient, ids, time.Now())
	if err != nil {
		return nil, err
	}
	for _, a := range activities {
		for _, p := range a.Edges.PlanBonuses {
			if _, ok := out[p.PlanID]; !ok {
				out[p.PlanID] = presaleBalanceBenefit(a, p)
			}
		}
	}
	return out, nil
}
func resolvePresaleBalanceBonus(ctx context.Context, client *dbent.Client, userID, planID, expectedID int64, now time.Time) (*PresaleBalanceBenefit, error) {
	activities, err := activePresaleBalanceActivities(ctx, client, []int64{planID}, now)
	if err != nil {
		return nil, err
	}
	for _, a := range activities {
		if expectedID > 0 && expectedID != a.ID {
			continue
		}
		count, err := client.PromotionActivityParticipation.Query().Where(promotionactivityparticipation.ActivityIDEQ(a.ID), promotionactivityparticipation.UserIDEQ(userID), promotionactivityparticipation.StatusIn(PromotionParticipationStatusReserved, PromotionParticipationStatusGranted)).Count(ctx)
		if err != nil {
			return nil, err
		}
		if count < a.MaxUsesPerUser && len(a.Edges.PlanBonuses) > 0 {
			return presaleBalanceBenefit(a, a.Edges.PlanBonuses[0]), nil
		}
	}
	if expectedID > 0 {
		return nil, infraerrors.Conflict("ACTIVITY_BENEFIT_CHANGED", "balance gift is no longer available; refresh checkout")
	}
	return nil, nil
}
func (s *PaymentService) reservePresaleBalanceBonusTx(ctx context.Context, client *dbent.Client, o *dbent.PaymentOrder, b *PresaleBalanceBenefit) error {
	if b == nil {
		return nil
	}
	_, err := client.PromotionActivityParticipation.Create().SetActivityID(b.ActivityID).SetUserID(o.UserID).SetOrderID(o.ID).SetPlanID(*o.PlanID).SetBonusDays(0).SetBonusBalance(o.PresaleBalanceBonusAmount).SetStatus(PromotionParticipationStatusReserved).SetReservedAt(time.Now()).Save(ctx)
	if err != nil {
		return err
	}
	return s.writeAuditLogStrict(ctx, o.ID, "PRESALE_BALANCE_BONUS_RESERVED", fmt.Sprintf("user:%d", o.UserID), map[string]any{"activity_id": b.ActivityID, "name": b.Name, "amount_usd": o.PresaleBalanceBonusAmount, "face_amount": b.Amount, "face_currency": b.Currency, "balance_per_payment_unit": o.PresaleBalanceBonusRate, "ends_at": b.EndsAt})
}

// Called under the same user/order transaction as the pending entitlement.
func (s *PaymentService) grantPresaleBalanceBonusTx(ctx context.Context, client *dbent.Client, o *dbent.PaymentOrder) error {
	if o.PresaleBalanceBonusActivityID == nil {
		return nil
	}
	p, err := client.PromotionActivityParticipation.Query().Where(promotionactivityparticipation.OrderIDEQ(o.ID)).Only(ctx)
	if err != nil {
		return err
	}
	if p.ActivityID != *o.PresaleBalanceBonusActivityID || p.UserID != o.UserID || p.BonusBalance != o.PresaleBalanceBonusAmount || !validPresaleCreditedBonusAmount(p.BonusBalance) {
		return fmt.Errorf("invalid presale gift snapshot")
	}
	if p.Status == PromotionParticipationStatusGranted {
		return nil
	}
	// Expiry is capped at the activity end when the order is created. A first
	// late receipt cannot earn the gift; timely callback retries remain valid.
	if o.PaidAt == nil || !o.PaidAt.Before(o.ExpiresAt) {
		return infraerrors.Conflict("PRESALE_BALANCE_BONUS_EXPIRED", "payment arrived after the gift deadline; contact support")
	}
	if p.Status == PromotionParticipationStatusReleased {
		a, err := client.PromotionActivity.Get(ctx, p.ActivityID)
		if err != nil {
			return err
		}
		used, err := client.PromotionActivityParticipation.Query().Where(promotionactivityparticipation.ActivityIDEQ(a.ID), promotionactivityparticipation.UserIDEQ(o.UserID), promotionactivityparticipation.StatusIn(PromotionParticipationStatusReserved, PromotionParticipationStatusGranted)).Count(ctx)
		if err != nil {
			return err
		}
		if used >= a.MaxUsesPerUser {
			return infraerrors.Conflict("PRESALE_BALANCE_BONUS_LIMIT", "gift participation has been replaced; contact support")
		}
	} else if p.Status != PromotionParticipationStatusReserved {
		return fmt.Errorf("invalid presale gift state")
	}
	if _, err = client.User.UpdateOneID(o.UserID).AddBalance(p.BonusBalance).Save(ctx); err != nil {
		return err
	}
	// Adjustment records are visible in recharge history, but do not increase
	// total_recharged, membership points, or affiliate commission.
	if err = recordPresaleBalanceAdjustment(ctx, client, o.ID, o.UserID, p.BonusBalance, false); err != nil {
		return err
	}
	if _, err = client.PromotionActivityParticipation.UpdateOneID(p.ID).SetStatus(PromotionParticipationStatusGranted).SetGrantedAt(time.Now()).ClearReleasedAt().ClearReleaseReason().Save(ctx); err != nil {
		return err
	}
	return s.writeAuditLogStrict(ctx, o.ID, "PRESALE_BALANCE_BONUS_GRANTED", "system", map[string]any{"activity_id": p.ActivityID, "amount_usd": p.BonusBalance})
}
func recordPresaleBalanceAdjustment(ctx context.Context, client *dbent.Client, orderID, userID int64, amount float64, reclaim bool) error {
	prefix, notes := "PBG-", fmt.Sprintf("预售活动赠送余额 · 订单 #%d", orderID)
	if reclaim {
		prefix = "PBR-"
		notes = fmt.Sprintf("预售活动赠送余额回收 · 订单 #%d", orderID)
	}
	_, err := client.RedeemCode.Create().SetCode(prefix + strconv.FormatInt(orderID, 10)).SetType(AdjustmentTypeAdminBalance).SetValue(amount).SetStatus(StatusUsed).SetUsedBy(userID).SetUsedAt(time.Now()).SetNotes(notes).Save(ctx)
	return err
}
func (s *PaymentService) invalidatePresaleBalanceCaches(ctx context.Context, userID int64) {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.redeemService != nil && s.redeemService.billingCacheService != nil {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.redeemService.billingCacheService.InvalidateUserBalance(cacheCtx, userID)
	}
}

// Amounts are USD for balance and original payment currency for the deduction.
func (s *PaymentService) applyPresaleBalanceRefund(ctx context.Context, o *dbent.PaymentOrder, q *PresaleRefundQuote) error {
	if o.PresaleBalanceBonusActivityID == nil {
		return nil
	}
	client := s.entClient
	if tx := dbent.TxFromContext(ctx); tx != nil {
		client = tx.Client()
	}
	p, err := client.PromotionActivityParticipation.Query().Where(promotionactivityparticipation.OrderIDEQ(o.ID)).Only(ctx)
	if err != nil {
		return err
	}
	if p.Status != PromotionParticipationStatusGranted {
		return nil
	}
	if p.BalanceReclaimedAt != nil {
		audit, err := client.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(o.ID, 10)), paymentauditlog.ActionEQ("PRESALE_BALANCE_BONUS_RECLAIMED")).Only(ctx)
		if err != nil {
			return err
		}
		var settled struct {
			Quote PresaleRefundQuote `json:"quote"`
		}
		if err = json.Unmarshal([]byte(audit.Detail), &settled); err != nil {
			return err
		}
		q.BalanceBonusAmount, q.BalanceBonusReclaim, q.BalanceBonusDeduction = settled.Quote.BalanceBonusAmount, settled.Quote.BalanceBonusReclaim, settled.Quote.BalanceBonusDeduction
	} else {
		u, err := client.User.Get(ctx, o.UserID)
		if err != nil {
			return err
		}
		if o.PresaleBalanceBonusRate <= 0 {
			return fmt.Errorf("missing gift conversion snapshot")
		}
		amount := decimal.NewFromFloat(p.BonusBalance)
		available := decimal.Min(amount, decimal.Max(decimal.Zero, decimal.NewFromFloat(u.Balance))).Truncate(8)
		deduction := amount.Sub(available).Div(decimal.NewFromFloat(o.PresaleBalanceBonusRate)).RoundUp(int32(payment.CurrencyMaxFractionDigits(q.Currency)))
		q.BalanceBonusAmount, q.BalanceBonusReclaim, q.BalanceBonusDeduction = p.BonusBalance, available.InexactFloat64(), deduction.InexactFloat64()
	}
	if q.BalanceBonusDeduction >= q.GatewayAmount && q.BalanceBonusDeduction > 0 {
		return infraerrors.Conflict("PRESALE_BALANCE_BONUS_REFUND_MANUAL", "remaining refund cannot cover the consumed gift; contact support or replenish your balance")
	}
	if q.GatewayAmount > 0 && q.BalanceBonusDeduction > 0 {
		gross := decimal.NewFromFloat(q.GatewayAmount)
		net := gross.Sub(decimal.NewFromFloat(q.BalanceBonusDeduction))
		q.RefundAmount = decimal.NewFromFloat(q.RefundAmount).Mul(net).Div(gross).Round(2).InexactFloat64()
		q.GatewayAmount = net.InexactFloat64()
	}
	return nil
}
func (s *PaymentService) reclaimPresaleBalanceBonusTx(ctx context.Context, tx *dbent.Tx, o *dbent.PaymentOrder, q *PresaleRefundQuote) error {
	if q.BalanceBonusAmount <= 0 {
		// A failed paid fulfillment never delivered its gift. Release only its
		// ungranted reservation; successful grants stay consumed after refunds.
		if o.PresaleBalanceBonusActivityID != nil {
			released, err := releaseSubscriptionBonusForOrderTx(ctx, tx.Client(), o.ID, "PRESALE_CANCELLED_UNFULFILLED")
			if err != nil {
				return err
			}
			if released {
				return s.writeAuditLogStrict(ctx, o.ID, "PRESALE_BALANCE_BONUS_RELEASED", "system", map[string]any{"reason": "unfulfilled"})
			}
		}
		return nil
	}
	p, err := tx.PromotionActivityParticipation.Query().Where(promotionactivityparticipation.OrderIDEQ(o.ID)).Only(ctx)
	if err != nil {
		return err
	}
	if p.BalanceReclaimedAt != nil {
		return nil
	}
	if q.BalanceBonusReclaim > 0 {
		if _, err = tx.User.UpdateOneID(o.UserID).AddBalance(-q.BalanceBonusReclaim).Save(ctx); err != nil {
			return err
		}
		if err = recordPresaleBalanceAdjustment(ctx, tx.Client(), o.ID, o.UserID, -q.BalanceBonusReclaim, true); err != nil {
			return err
		}
	}
	if _, err = tx.PromotionActivityParticipation.UpdateOneID(p.ID).SetBalanceReclaimedAt(time.Now()).Save(ctx); err != nil {
		return err
	}
	// Status remains granted: refunding a paid activity never restores its use.
	return s.writeAuditLogStrict(ctx, o.ID, "PRESALE_BALANCE_BONUS_RECLAIMED", "system", map[string]any{"quote": q})
}
