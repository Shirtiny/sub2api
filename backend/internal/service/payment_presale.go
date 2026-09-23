package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const PresaleTimezone = "Asia/Shanghai"

// The business calendar is deliberately independent of host/browser timezone.
// Contemporary Shanghai has UTC+8 year-round; no system tzdata dependency.
var presaleLocation = time.FixedZone("CST", 8*60*60)

type PresalePeriod struct {
	Month            string    `json:"month"`
	Timezone         string    `json:"timezone"`
	StartsAt         time.Time `json:"starts_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	FullRefundBefore time.Time `json:"full_refund_before"`
}

func NextPresalePeriod(now time.Time) PresalePeriod {
	local := now.In(presaleLocation)
	start := time.Date(local.Year(), local.Month()+1, 1, 0, 0, 0, 0, presaleLocation)
	return PresalePeriod{Month: start.Format("2006-01"), Timezone: PresaleTimezone, StartsAt: start, ExpiresAt: start.AddDate(0, 1, 0), FullRefundBefore: start.Add(-72 * time.Hour)}
}

func validatePresalePlanFields(badge string, cards int) error {
	if utf8.RuneCountInString(strings.TrimSpace(badge)) > 40 || cards < 0 || cards > 1000 {
		return infraerrors.BadRequest("PRESALE_CONFIG_INVALID", "presale badge must be at most 40 characters and reset cards between 0 and 1000")
	}
	return nil
}

func validatePresaleOrderPlan(plan *dbent.SubscriptionPlan, req CreateOrderRequest, now time.Time) error {
	if req.PresaleMonth == "" {
		if plan.PresaleEnabled {
			return infraerrors.BadRequest("PRESALE_REQUIRED", "this plan is sold through the presale page")
		}
		return nil
	}
	if !plan.ForSale || !plan.PresaleEnabled || !plan.PresaleVisible {
		return infraerrors.NotFound("PRESALE_NOT_AVAILABLE", "this plan is not available for presale")
	}
	if req.PresaleMonth != NextPresalePeriod(now).Month {
		return infraerrors.Conflict("PRESALE_MONTH_CHANGED", "the presale month changed; refresh before paying")
	}
	if req.ExpectedSubscriptionBonusActivityID != 0 {
		return infraerrors.BadRequest("PRESALE_BONUS_UNSUPPORTED", "calendar-month presales do not include legacy bonus-day promotions")
	}
	return nil
}

// ListPresalePlans returns only intentionally published, usable plans. The
// public handler maps these entities to a whitelist of product display fields.
func (s *PaymentConfigService) ListPresalePlans(ctx context.Context) ([]*dbent.SubscriptionPlan, error) {
	ids, err := s.entClient.Group.Query().Where(group.StatusEQ(payment.EntityStatusActive), group.SubscriptionTypeEQ(SubscriptionTypeSubscription), group.IsCustomSubscriptionGroupEQ(false), group.DeletedAtIsNil()).IDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list presale groups: %w", err)
	}
	return s.entClient.SubscriptionPlan.Query().Where(subscriptionplan.ForSaleEQ(true), subscriptionplan.PresaleEnabledEQ(true), subscriptionplan.PresaleVisibleEQ(true), subscriptionplan.GroupIDIn(ids...)).Order(subscriptionplan.BySortOrder(), subscriptionplan.ByID()).All(ctx)
}

type PresaleQuote struct {
	PresalePeriod
	PlanID                int64      `json:"plan_id"`
	Renewal               bool       `json:"renewal"`
	CurrentSubscriptionID *int64     `json:"current_subscription_id,omitempty"`
	CurrentExpiresAt      *time.Time `json:"current_expires_at,omitempty"`
}

func (s *PaymentService) GetPresaleQuote(ctx context.Context, userID, planID int64) (*PresaleQuote, error) {
	plan, err := s.validateSubOrder(ctx, CreateOrderRequest{UserID: userID, PlanID: planID, PresaleMonth: NextPresalePeriod(time.Now()).Month})
	if err != nil {
		return nil, err
	}
	return s.presaleQuoteForPlan(ctx, userID, plan, time.Now(), 0)
}

// Called again under the payment user lock when an order is created. Pending
// payment orders also occupy the slot; a double click never buys two months.
func (s *PaymentService) presaleQuoteForPlan(ctx context.Context, userID int64, plan *dbent.SubscriptionPlan, now time.Time, excludeOrderID int64) (*PresaleQuote, error) {
	period := NextPresalePeriod(now)
	if err := s.checkPresaleSlot(ctx, userID, plan.GroupID, period.StartsAt, excludeOrderID); err != nil {
		return nil, err
	}
	quote := &PresaleQuote{PresalePeriod: period, PlanID: plan.ID}
	existing, err := s.entClient.UserSubscription.Query().Where(usersubscription.UserIDEQ(userID), usersubscription.GroupIDEQ(plan.GroupID), usersubscription.DeletedAtIsNil()).Only(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return nil, fmt.Errorf("load presale renewal: %w", err)
	}
	if existing != nil {
		if existing.ExpiresAt.After(period.StartsAt) {
			return nil, infraerrors.Conflict("PRESALE_COVERAGE_OVERLAP", "your subscription already covers part of the presale month; contact support before purchasing")
		}
		quote.Renewal = true
		quote.CurrentSubscriptionID = &existing.ID
		quote.CurrentExpiresAt = &existing.ExpiresAt
	}
	legacy, err := s.findActiveCustomSubscriptionGroup(ctx, userID, plan.ID)
	if err != nil && !dbent.IsNotFound(err) {
		return nil, err
	}
	if legacy != nil {
		return nil, infraerrors.Conflict("PRESALE_LEGACY_SUBSCRIPTION", "contact support to migrate your existing custom subscription before presale renewal")
	}
	return quote, nil
}

func (s *PaymentService) checkPresaleSlot(ctx context.Context, userID, groupID int64, start time.Time, excludeOrderID int64) error {
	exists, err := s.entClient.PaymentOrder.Query().Where(
		paymentorder.UserIDEQ(userID), paymentorder.SubscriptionSourceGroupIDEQ(groupID), paymentorder.PresaleStartsAtEQ(start), paymentorder.IDNEQ(excludeOrderID),
		paymentorder.Or(
			paymentorder.StatusIn(OrderStatusPaid, OrderStatusRecharging, OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefunding, OrderStatusRefundFailed),
			paymentorder.And(paymentorder.StatusEQ(OrderStatusPending), paymentorder.ExpiresAtGT(time.Now())),
			paymentorder.And(paymentorder.StatusEQ(OrderStatusFailed), paymentorder.PaidAtNotNil()),
		),
	).Exist(ctx)
	if err != nil {
		return fmt.Errorf("check presale slot: %w", err)
	}
	if exists {
		return infraerrors.Conflict("PRESALE_ALREADY_RESERVED", "you already have an order for this group and month; check your orders")
	}
	return nil
}

func (s *PaymentService) GetMyPresales(ctx context.Context, userID int64) ([]*dbent.PaymentOrder, error) {
	return s.entClient.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID), paymentorder.PresaleStartsAtNotNil(), paymentorder.PaidAtNotNil()).Order(dbent.Desc(paymentorder.FieldID)).Limit(100).All(ctx)
}
