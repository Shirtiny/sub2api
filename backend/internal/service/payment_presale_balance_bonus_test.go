//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivityparticipation"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func balanceGiftActivity(t *testing.T, s *PaymentService, p *dbent.SubscriptionPlan, amount float64) *PromotionActivityView {
	t.Helper()
	a, err := s.configService.CreatePromotionActivity(context.Background(), UpsertPromotionActivityRequest{
		Name: "Presale coffee", Type: PromotionActivityTypePresaleBalance, Enabled: true, StartsAt: time.Now().Add(-time.Hour).Truncate(time.Second), EndsAt: time.Now().Add(time.Hour).Truncate(time.Second), MaxUsesPerUser: 1,
		PlanBonuses: []PromotionActivityPlanInput{{PlanID: p.ID, BonusBalance: amount}},
	})
	require.NoError(t, err)
	return a
}
func balanceGiftOrder(t *testing.T, s *PaymentService, u *dbent.User, p *dbent.SubscriptionPlan, rate float64, multiplier int) *dbent.PaymentOrder {
	t.Helper()
	amount := p.Price * float64(multiplier)
	o, err := s.createOrderInTx(context.Background(), CreateOrderRequest{UserID: u.ID, PlanID: p.ID, Multiplier: multiplier, OrderType: payment.OrderTypeSubscription, PaymentType: payment.TypeAlipay, PresaleMonth: NextPresalePeriod(time.Now()).Month}, &User{ID: u.ID, Email: u.Email, Username: u.Username}, p, &PaymentConfig{BalanceRechargeMultiplier: rate}, amount, amount, 0, amount, nil)
	require.NoError(t, err)
	return o
}
func TestPresaleBalanceGiftValidation(t *testing.T) {
	for _, v := range []float64{0, -1, math.NaN(), math.Inf(1), .001, 1000001} {
		require.False(t, validPresaleBalanceBonusAmount(v))
	}
	for _, v := range []float64{.01, .29, 3.75, 1000000} {
		require.True(t, validPresaleBalanceBonusAmount(v))
	}
	s, _, p := newPresaleFixture(t)
	a := balanceGiftActivity(t, s, p, 5)
	req := UpsertPromotionActivityRequest{Name: "Second", Type: PromotionActivityTypePresaleBalance, Enabled: true, StartsAt: a.StartsAt, EndsAt: a.EndsAt, MaxUsesPerUser: 1, PlanBonuses: []PromotionActivityPlanInput{{PlanID: p.ID, BonusBalance: 10}}}
	_, err := s.configService.CreatePromotionActivity(context.Background(), req)
	require.Equal(t, "PROMOTION_ACTIVITY_OVERLAP", infraerrors.Reason(err))
	req.Enabled = false
	req.PlanBonuses[0].BonusDays = 1
	_, err = s.configService.CreatePromotionActivity(context.Background(), req)
	require.Equal(t, "PROMOTION_ACTIVITY_PLAN_INVALID", infraerrors.Reason(err))
	req.PlanBonuses[0].BonusDays = 0
	s.entClient.SubscriptionPlan.UpdateOneID(p.ID).SetPresaleEnabled(false).ExecX(context.Background())
	_, err = s.configService.CreatePromotionActivity(context.Background(), req)
	require.Equal(t, "PROMOTION_ACTIVITY_PRESALE_REQUIRED", infraerrors.Reason(err))
}
func TestPresaleBalanceGiftFixedGrantAndRefund(t *testing.T) {
	for _, multiplier := range []int{1, 3} {
		t.Run(fmt.Sprint(multiplier), func(t *testing.T) {
			ctx := context.Background()
			s, u, p := newPresaleFixture(t)
			p = s.entClient.SubscriptionPlan.UpdateOneID(p.ID).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).SaveX(ctx)
			a := balanceGiftActivity(t, s, p, 10)
			q, err := s.GetPresaleQuote(ctx, u.ID, p.ID)
			require.NoError(t, err)
			require.Equal(t, 10.0, q.BalanceBonus.Amount)
			o := balanceGiftOrder(t, s, u, p, .2, multiplier)
			require.Equal(t, 10.0, o.PresaleBalanceBonusAmount)
			require.Equal(t, .2, o.PresaleBalanceBonusRate)
			for range 2 {
				require.NoError(t, s.toPaid(ctx, o, "paid", o.PayAmount, payment.TypeAlipay))
			}
			require.Equal(t, 10.0, s.entClient.User.GetX(ctx, u.ID).Balance)
			require.Equal(t, o.PayAmount, s.entClient.User.GetX(ctx, u.ID).TotalRecharged)
			require.Equal(t, 1, s.entClient.RedeemCode.Query().Where(redeemcode.UsedByEQ(u.ID)).CountX(ctx))
			o = s.entClient.PaymentOrder.GetX(ctx, o.ID)
			// Spend $4, lock a $6 recovery plus CNY20 shortfall at the original .2 rate.
			s.entClient.User.UpdateOneID(u.ID).SetBalance(6).ExecX(ctx)
			quote, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
			require.NoError(t, err)
			base, err := PresaleRefundQuoteForOrder(o, time.Now())
			require.NoError(t, err)
			require.Equal(t, 6.0, quote.BalanceBonusReclaim)
			require.Equal(t, 20.0, quote.BalanceBonusDeduction)
			require.InDelta(t, base.GatewayAmount-20, quote.GatewayAmount, .0001)
			require.NoError(t, s.RequestRefund(ctx, o.ID, u.ID, "test", quote.GatewayAmount))
			require.NoError(t, s.RequestRefund(ctx, o.ID, u.ID, "test", quote.GatewayAmount))
			require.Zero(t, s.entClient.User.GetX(ctx, u.ID).Balance)
			require.Equal(t, 2, s.entClient.RedeemCode.Query().Where(redeemcode.UsedByEQ(u.ID)).CountX(ctx))
			s.entClient.User.UpdateOneID(u.ID).SetBalance(77).ExecX(ctx)
			frozen, err := s.GetPresaleRefundQuote(ctx, s.entClient.PaymentOrder.GetX(ctx, o.ID), time.Now().Add(30*24*time.Hour))
			require.NoError(t, err)
			require.Equal(t, quote.GatewayAmount, frozen.GatewayAmount)
			require.Equal(t, 6.0, frozen.BalanceBonusReclaim)
			part := s.entClient.PromotionActivityParticipation.Query().Where(promotionactivityparticipation.OrderIDEQ(o.ID)).OnlyX(ctx)
			require.Equal(t, PromotionParticipationStatusGranted, part.Status)
			require.NotNil(t, part.BalanceReclaimedAt)
			gift, err := resolvePresaleBalanceBonus(ctx, s.entClient, u.ID, p.ID, 0, time.Now())
			require.NoError(t, err)
			require.Nil(t, gift)
			_, err = resolvePresaleBalanceBonus(ctx, s.entClient, u.ID, p.ID, a.ID, time.Now())
			require.Equal(t, "ACTIVITY_BENEFIT_CHANGED", infraerrors.Reason(err))
		})
	}
}
func TestPresaleBalanceGiftStacksWithCoupon(t *testing.T) {
	s, u, p, coupon := newCafeCampaignFixture(t)
	ctx := context.Background()
	balanceGiftActivity(t, s, p, 12.5)
	o, err := cafeCampaignOrder(t, s, u, p, coupon)
	require.NoError(t, err)
	require.Equal(t, 60.0, o.PayAmount)
	require.Equal(t, 40.0, o.CafeCouponDiscount)
	require.Equal(t, 12.5, o.PresaleBalanceBonusAmount)
	require.NoError(t, s.toPaid(ctx, o, "coupon-paid", 60, payment.TypeAlipay))
	require.Equal(t, 12.5, s.entClient.User.GetX(ctx, u.ID).Balance)
}
func TestPresaleBalanceGiftDeadlineAndAuditRollback(t *testing.T) {
	for _, action := range []string{"PRESALE_BALANCE_BONUS_RESERVED", "PRESALE_BALANCE_BONUS_GRANTED"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			s, u, p := newPresaleFixture(t)
			balanceGiftActivity(t, s, p, 8)
			failed := true
			s.entClient.PaymentAuditLog.Use(func(next dbent.Mutator) dbent.Mutator {
				return dbent.MutateFunc(func(ctx context.Context, m dbent.Mutation) (dbent.Value, error) {
					if mutation, ok := m.(*dbent.PaymentAuditLogMutation); ok {
						if a, _ := mutation.Action(); a == action && failed {
							return nil, errors.New("audit offline")
						}
					}
					return next.Mutate(ctx, m)
				})
			})
			if action == "PRESALE_BALANCE_BONUS_RESERVED" {
				_, err := s.createOrderInTx(ctx, CreateOrderRequest{UserID: u.ID, PlanID: p.ID, Multiplier: 1, OrderType: payment.OrderTypeSubscription, PresaleMonth: NextPresalePeriod(time.Now()).Month}, &User{ID: u.ID}, p, &PaymentConfig{}, 100, 100, 0, 100, nil)
				require.Error(t, err)
				require.Zero(t, s.entClient.PaymentOrder.Query().CountX(ctx))
				require.Zero(t, s.entClient.PromotionActivityParticipation.Query().CountX(ctx))
				return
			}
			o := balanceGiftOrder(t, s, u, p, 1, 1)
			require.ErrorContains(t, s.toPaid(ctx, o, "timely", 100, payment.TypeAlipay), "audit offline")
			require.Zero(t, s.entClient.User.GetX(ctx, u.ID).Balance)
			require.Zero(t, s.entClient.RedeemCode.Query().CountX(ctx))
			o = s.entClient.PaymentOrder.GetX(ctx, o.ID)
			require.NotNil(t, o.PaidAt)
			// A retry after timeout honors the original, timely verified payment.
			s.entClient.PaymentOrder.UpdateOneID(o.ID).SetExpiresAt(o.PaidAt.Add(time.Millisecond)).ExecX(ctx)
			time.Sleep(2 * time.Millisecond)
			failed = false
			require.NoError(t, s.RetryFulfillment(ctx, o.ID))
			require.Equal(t, 8.0, s.entClient.User.GetX(ctx, u.ID).Balance)
		})
	}
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	balanceGiftActivity(t, s, p, 10)
	o := balanceGiftOrder(t, s, u, p, 1, 1)
	s.entClient.PaymentOrder.UpdateOneID(o.ID).SetExpiresAt(time.Now().Add(-time.Second)).ExecX(ctx)
	require.Equal(t, "PRESALE_BALANCE_BONUS_EXPIRED", infraerrors.Reason(s.toPaid(ctx, o, "late", 100, payment.TypeAlipay)))
	require.Zero(t, s.entClient.User.GetX(ctx, u.ID).Balance)
}
func TestPresaleBalanceGiftRefundChangedAndOfflineGuard(t *testing.T) {
	ctx := context.Background()
	s, u, p := newPresaleFixture(t)
	balanceGiftActivity(t, s, p, 10)
	o := balanceGiftOrder(t, s, u, p, .2, 1)
	require.NoError(t, s.toPaid(ctx, o, "paid", 100, payment.TypeAlipay))
	o = s.entClient.PaymentOrder.GetX(ctx, o.ID)
	quote, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
	require.NoError(t, err)
	require.Equal(t, 10.0, quote.BalanceBonusReclaim)
	s.entClient.User.UpdateOneID(u.ID).SetBalance(2).ExecX(ctx)
	require.Equal(t, "PRESALE_REFUND_AMOUNT_CHANGED", infraerrors.Reason(s.RequestRefund(ctx, o.ID, u.ID, "test", quote.GatewayAmount)))
	require.Equal(t, 2.0, s.entClient.User.GetX(ctx, u.ID).Balance)
	req := offlineRequest(o, "refund")
	_, err = s.ProcessPresaleOffline(ctx, o.ID, 99, req)
	require.Equal(t, "PRESALE_OFFLINE_BONUS_AMOUNT", infraerrors.Reason(err))
	// Cancel-only settles the gift; later admin refund must honor the frozen shortfall.
	_, err = s.ProcessPresaleOffline(ctx, o.ID, 99, offlineRequest(o, "cancel"))
	require.NoError(t, err)
	require.Zero(t, s.entClient.User.GetX(ctx, u.ID).Balance)
	o = s.entClient.PaymentOrder.GetX(ctx, o.ID)
	req = offlineRequest(o, "refund")
	_, err = s.ProcessPresaleOffline(ctx, o.ID, 99, req)
	require.Equal(t, "PRESALE_OFFLINE_BONUS_AMOUNT", infraerrors.Reason(err))
	req.Amount = 60
	for range 2 {
		_, err = s.ProcessPresaleOffline(ctx, o.ID, 99, req)
		require.NoError(t, err)
	}
	require.Equal(t, 1, s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("PRESALE_BALANCE_BONUS_RECLAIMED")).CountX(ctx))
	require.Equal(t, 2, s.entClient.RedeemCode.Query().CountX(ctx))
}
func TestPresaleBalanceGiftInsufficientRefundAndRollback(t *testing.T) {
	ctx := context.Background()
	s, u, p := newPresaleFixture(t)
	balanceGiftActivity(t, s, p, 200)
	o := balanceGiftOrder(t, s, u, p, 1, 1)
	require.NoError(t, s.toPaid(ctx, o, "paid", 100, payment.TypeAlipay))
	o = s.entClient.PaymentOrder.GetX(ctx, o.ID)
	s.entClient.User.UpdateOneID(u.ID).SetBalance(0).ExecX(ctx)
	_, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
	require.Equal(t, "PRESALE_BALANCE_BONUS_REFUND_MANUAL", infraerrors.Reason(err))
	s.entClient.User.UpdateOneID(u.ID).SetBalance(200).ExecX(ctx)
	quote, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
	require.NoError(t, err)
	s.entClient.PaymentAuditLog.Use(func(next dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(ctx context.Context, m dbent.Mutation) (dbent.Value, error) {
			if mutation, ok := m.(*dbent.PaymentAuditLogMutation); ok {
				if a, _ := mutation.Action(); a == "PRESALE_BALANCE_BONUS_RECLAIMED" {
					return nil, errors.New("reclaim audit offline")
				}
			}
			return next.Mutate(ctx, m)
		})
	})
	require.Error(t, s.RequestRefund(ctx, o.ID, u.ID, "test", quote.GatewayAmount))
	require.Equal(t, 200.0, s.entClient.User.GetX(ctx, u.ID).Balance)
	require.Equal(t, OrderStatusCompleted, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
	require.Equal(t, 1, s.entClient.RedeemCode.Query().CountX(ctx))
}
func TestPresaleBalanceGiftSharedLimitWindowAndImmutable(t *testing.T) {
	ctx := context.Background()
	s, u, p := newPresaleFixture(t)
	a := balanceGiftActivity(t, s, p, 5)
	for _, now := range []time.Time{a.StartsAt.Add(-time.Second), a.EndsAt} {
		gift, err := resolvePresaleBalanceBonus(ctx, s.entClient, u.ID, p.ID, 0, now)
		require.NoError(t, err)
		require.Nil(t, gift)
	}
	gift, err := resolvePresaleBalanceBonus(ctx, s.entClient, u.ID, p.ID, a.ID, a.StartsAt)
	require.NoError(t, err)
	require.NotNil(t, gift)
	o := balanceGiftOrder(t, s, u, p, 1, 1)
	tx, err := s.entClient.Tx(ctx)
	require.NoError(t, err)
	_, err = releaseSubscriptionBonusForOrderTx(dbent.NewTxContext(ctx, tx), tx.Client(), o.ID, "test")
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	gift, err = resolvePresaleBalanceBonus(ctx, s.entClient, u.ID, p.ID, 0, time.Now())
	require.NoError(t, err)
	require.NotNil(t, gift)
	req := UpsertPromotionActivityRequest{Name: a.Name, Type: a.Type, Enabled: a.Enabled, StartsAt: a.StartsAt, EndsAt: a.EndsAt, MaxUsesPerUser: 2, PlanBonuses: []PromotionActivityPlanInput{{PlanID: p.ID, BonusBalance: 5}}}
	_, err = s.configService.UpdatePromotionActivity(ctx, a.ID, req)
	require.Equal(t, "PROMOTION_ACTIVITY_IMMUTABLE", infraerrors.Reason(err))
}

func giftUpdateRequest(a *PromotionActivityView, planID int64, amount float64) UpsertPromotionActivityRequest {
	return UpsertPromotionActivityRequest{BonusCurrency: a.BonusCurrency, Name: a.Name, Type: a.Type, Enabled: a.Enabled, StartsAt: a.StartsAt, EndsAt: a.EndsAt, MaxUsesPerUser: a.MaxUsesPerUser, PlanBonuses: []PromotionActivityPlanInput{{PlanID: planID, BonusBalance: amount}}}
}

func TestPresaleBalanceGiftQuoteVersionRejectsChangedTerms(t *testing.T) {
	ctx := context.Background()
	s, u, p := newPresaleFixture(t)
	a := balanceGiftActivity(t, s, p, 10)
	quote, err := s.GetPresaleQuote(ctx, u.ID, p.ID)
	require.NoError(t, err)
	require.Len(t, quote.BalanceBonus.Version, 64)
	_, err = s.configService.UpdatePromotionActivity(ctx, a.ID, giftUpdateRequest(a, p.ID, 1))
	require.NoError(t, err)
	req := CreateOrderRequest{UserID: u.ID, PlanID: p.ID, Multiplier: 1, OrderType: payment.OrderTypeSubscription, PaymentType: payment.TypeAlipay, PresaleMonth: NextPresalePeriod(time.Now()).Month, ExpectedSubscriptionBonusActivityID: a.ID}
	// Missing version (old checkout) and stale version both fail before any writes.
	for _, version := range []string{"", quote.BalanceBonus.Version} {
		req.ExpectedPresaleBonusVersion = version
		_, err = s.createOrderInTx(ctx, req, &User{ID: u.ID}, p, &PaymentConfig{BalanceRechargeMultiplier: .2}, 100, 100, 0, 100, nil)
		require.Equal(t, "ACTIVITY_BENEFIT_CHANGED", infraerrors.Reason(err))
		require.Zero(t, s.entClient.PaymentOrder.Query().CountX(ctx))
		require.Zero(t, s.entClient.PromotionActivityParticipation.Query().CountX(ctx))
	}
	fresh, err := s.GetPresaleQuote(ctx, u.ID, p.ID)
	require.NoError(t, err)
	require.NotEqual(t, quote.BalanceBonus.Version, fresh.BalanceBonus.Version)
	// Cosmetic changes do not force a second confirmation.
	rename := giftUpdateRequest(a, p.ID, 1)
	rename.Name = "New display name"
	_, err = s.configService.UpdatePromotionActivity(ctx, a.ID, rename)
	require.NoError(t, err)
	req.ExpectedPresaleBonusVersion = fresh.BalanceBonus.Version
	order, err := s.createOrderInTx(ctx, req, &User{ID: u.ID}, p, &PaymentConfig{BalanceRechargeMultiplier: .2}, 100, 100, 0, 100, nil)
	require.NoError(t, err)
	require.Equal(t, 1.0, order.PresaleBalanceBonusAmount)
}

func TestPresaleBalanceGiftCanDisableAfterPlanPresaleTurnedOff(t *testing.T) {
	for _, hasHistory := range []bool{false, true} {
		t.Run(fmt.Sprint(hasHistory), func(t *testing.T) {
			ctx := context.Background()
			s, u, p := newPresaleFixture(t)
			a := balanceGiftActivity(t, s, p, 10)
			if hasHistory {
				balanceGiftOrder(t, s, u, p, 1, 1)
			}
			off := false
			_, err := s.configService.UpdatePlan(ctx, p.ID, UpdatePlanRequest{PresaleEnabled: &off})
			require.NoError(t, err)
			req := giftUpdateRequest(a, p.ID, 10)
			req.Name = "Renamed after plan paused"
			_, err = s.configService.UpdatePromotionActivity(ctx, a.ID, req)
			require.NoError(t, err)
			req.Enabled = false
			_, err = s.configService.UpdatePromotionActivity(ctx, a.ID, req)
			require.NoError(t, err)
			req.Enabled = true
			_, err = s.configService.UpdatePromotionActivity(ctx, a.ID, req)
			require.Equal(t, "PROMOTION_ACTIVITY_PRESALE_REQUIRED", infraerrors.Reason(err))
		})
	}
}

func TestPublicPresaleActivitiesOnlyAdvertisesSupportedVisibleRewards(t *testing.T) {
	ctx := context.Background()
	s, _, p := newPresaleFixture(t)
	now := time.Date(2026, 9, 25, 4, 0, 0, 0, time.UTC)
	req := UpsertPromotionActivityRequest{Name: "Current", Type: PromotionActivityTypePresaleBalance, Enabled: true, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), MaxUsesPerUser: 1, PlanBonuses: []PromotionActivityPlanInput{{PlanID: p.ID, BonusBalance: 3}}}
	current, err := s.configService.CreatePromotionActivity(ctx, req)
	require.NoError(t, err)
	req.Name = "Upcoming"
	req.StartsAt = now.Add(2 * time.Hour)
	req.EndsAt = now.Add(3 * time.Hour)
	upcoming, err := s.configService.CreatePromotionActivity(ctx, req)
	require.NoError(t, err)
	req.Name = "Disabled"
	req.Enabled = false
	_, err = s.configService.CreatePromotionActivity(ctx, req)
	require.NoError(t, err)
	req.Name = "Ended"
	req.Enabled = true
	req.StartsAt = now.Add(-3 * time.Hour)
	req.EndsAt = now.Add(-time.Hour)
	_, err = s.configService.CreatePromotionActivity(ctx, req)
	require.NoError(t, err)
	req.Name = "Next cycle"
	req.StartsAt = NextPresalePeriod(now).StartsAt
	req.EndsAt = req.StartsAt.Add(time.Hour)
	_, err = s.configService.CreatePromotionActivity(ctx, req)
	require.NoError(t, err)
	req.Name = "Legacy days"
	req.Type = PromotionActivityTypeSubscriptionBonusDays
	req.StartsAt = now.Add(-time.Hour)
	req.EndsAt = now.Add(time.Hour)
	req.PlanBonuses = []PromotionActivityPlanInput{{PlanID: p.ID, BonusDays: 3}}
	_, err = s.configService.CreatePromotionActivity(ctx, req)
	require.NoError(t, err)
	activities, err := s.configService.PublicPresaleActivities(ctx, []int64{p.ID}, now)
	require.NoError(t, err)
	require.Len(t, activities, 2)
	require.Equal(t, current.ID, activities[0].ID)
	require.Equal(t, upcoming.ID, activities[1].ID)
	require.Equal(t, 3.0, activities[0].PlanBonuses[0].BonusBalance)
	empty, err := s.configService.PublicPresaleActivities(ctx, []int64{p.ID + 999}, now)
	require.NoError(t, err)
	require.Empty(t, empty)
	empty, err = s.configService.PublicPresaleActivities(ctx, nil, now)
	require.NoError(t, err)
	require.Empty(t, empty)
}

func TestPresaleBalanceGiftCNYConversionAndRefundSnapshot(t *testing.T) {
	ctx := context.Background()
	s, u, p := newPresaleFixture(t)
	settings := &paymentConfigSettingRepoStub{values: map[string]string{SettingBalanceRechargeMult: "0.2"}}
	s.configService.settingRepo = settings
	p = s.entClient.SubscriptionPlan.UpdateOneID(p.ID).SetCustomMultiplierEnabled(true).SetCustomMultiplierMin(1).SetCustomMultiplierMax(5).SaveX(ctx)
	a := balanceGiftActivity(t, s, p, 20)
	update := giftUpdateRequest(a, p.ID, 20)
	update.BonusCurrency = "CNY"
	_, err := s.configService.UpdatePromotionActivity(ctx, a.ID, update)
	require.NoError(t, err)
	quote, err := s.GetPresaleQuote(ctx, u.ID, p.ID)
	require.NoError(t, err)
	require.Equal(t, "CNY", quote.BalanceBonus.Currency)
	require.Equal(t, 20.0, quote.BalanceBonus.Amount)
	require.Equal(t, 4.0, quote.BalanceBonus.CreditedAmount)
	req := CreateOrderRequest{UserID: u.ID, PlanID: p.ID, Multiplier: 3, OrderType: payment.OrderTypeSubscription, PaymentType: payment.TypeAlipay, PresaleMonth: NextPresalePeriod(time.Now()).Month, ExpectedSubscriptionBonusActivityID: a.ID, ExpectedPresaleBonusVersion: quote.BalanceBonus.Version}
	_, err = s.createOrderInTx(ctx, req, &User{ID: u.ID}, p, &PaymentConfig{BalanceRechargeMultiplier: .3}, 300, 300, 0, 300, nil)
	require.Equal(t, "ACTIVITY_BENEFIT_CHANGED", infraerrors.Reason(err))
	require.Zero(t, s.entClient.PaymentOrder.Query().CountX(ctx))
	order, err := s.createOrderInTx(ctx, req, &User{ID: u.ID}, p, &PaymentConfig{BalanceRechargeMultiplier: .2}, 300, 300, 0, 300, nil)
	require.NoError(t, err)
	require.Equal(t, "CNY", order.PresaleBalanceBonusCurrency)
	require.Equal(t, 20.0, order.PresaleBalanceBonusFaceAmount)
	require.Equal(t, 4.0, order.PresaleBalanceBonusAmount) // Never multiply gifts by 3.
	for range 2 {
		require.NoError(t, s.toPaid(ctx, order, "paid-cny", 300, payment.TypeAlipay))
	}
	require.Equal(t, 4.0, s.entClient.User.GetX(ctx, u.ID).Balance)
	require.Equal(t, 4.0, s.entClient.PromotionActivityParticipation.Query().OnlyX(ctx).BonusBalance)
	s.entClient.User.UpdateOneID(u.ID).SetBalance(3).ExecX(ctx)
	settings.values[SettingBalanceRechargeMult] = "0.1"
	order = s.entClient.PaymentOrder.GetX(ctx, order.ID)
	refund, err := s.GetPresaleRefundQuote(ctx, order, time.Now())
	require.NoError(t, err)
	require.Equal(t, 3.0, refund.BalanceBonusReclaim)
	require.Equal(t, 5.0, refund.BalanceBonusDeduction) // $1 / locked .2, not the new rate.
	require.NoError(t, s.RequestRefund(ctx, order.ID, u.ID, "test", refund.GatewayAmount))
	require.Zero(t, s.entClient.User.GetX(ctx, u.ID).Balance)
	update.BonusCurrency = "USD"
	_, err = s.configService.UpdatePromotionActivity(ctx, a.ID, update)
	require.Equal(t, "PROMOTION_ACTIVITY_IMMUTABLE", infraerrors.Reason(err))
}

func TestPresaleBalanceGiftConversionBounds(t *testing.T) {
	for _, rate := range []float64{1e-12, 1e12, 1000000} {
		b := &PresaleBalanceBenefit{Currency: "CNY", Amount: 20, Version: "terms"}
		require.Equal(t, "PRESALE_BALANCE_BONUS_RATE_INVALID", infraerrors.Reason(convertPresaleBalanceBenefit(b, rate)))
	}
	b := &PresaleBalanceBenefit{Currency: "CNY", Amount: 20, Version: "terms"}
	require.NoError(t, convertPresaleBalanceBenefit(b, .14285714))
	require.Equal(t, 2.8571428, b.CreditedAmount)
}
