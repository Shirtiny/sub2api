package service

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivity"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivityplan"
)

// PublicPresaleActivity is a marketing view, never proof of personal eligibility.
// Only rewards fulfilled by calendar-month presales may be advertised here.
// Legacy subscription_bonus_days activities belong to immediate subscriptions.
type PublicPresaleActivity struct {
	BonusCurrency  string                       `json:"bonus_currency"`
	ID             int64                        `json:"id"`
	Name           string                       `json:"name"`
	Type           string                       `json:"type"`
	StartsAt       time.Time                    `json:"starts_at"`
	EndsAt         time.Time                    `json:"ends_at"`
	MaxUsesPerUser int                          `json:"max_uses_per_user"`
	PlanBonuses    []PromotionActivityPlanInput `json:"plan_bonuses"`
}

func (s *PaymentConfigService) PublicPresaleActivities(ctx context.Context, visiblePlanIDs []int64, now time.Time) ([]PublicPresaleActivity, error) {
	result := make([]PublicPresaleActivity, 0)
	if len(visiblePlanIDs) == 0 {
		return result, nil
	}
	activities, err := s.entClient.PromotionActivity.Query().Where(
		promotionactivity.ActivityTypeEQ(PromotionActivityTypePresaleBalance),
		promotionactivity.EnabledEQ(true), promotionactivity.EndsAtGT(now),
		// Do not advertise an offer that starts after this presale cycle closes.
		promotionactivity.StartsAtLT(NextPresalePeriod(now).StartsAt),
		promotionactivity.HasPlanBonusesWith(promotionactivityplan.PlanIDIn(visiblePlanIDs...)),
	).WithPlanBonuses(func(q *dbent.PromotionActivityPlanQuery) {
		q.Where(promotionactivityplan.PlanIDIn(visiblePlanIDs...)).Order(dbent.Asc(promotionactivityplan.FieldPlanID))
	}).Order(dbent.Asc(promotionactivity.FieldStartsAt), dbent.Asc(promotionactivity.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range activities {
		bonuses := make([]PromotionActivityPlanInput, 0, len(a.Edges.PlanBonuses))
		for _, p := range a.Edges.PlanBonuses {
			bonuses = append(bonuses, PromotionActivityPlanInput{PlanID: p.PlanID, BonusBalance: p.BonusBalance})
		}
		result = append(result, PublicPresaleActivity{BonusCurrency: a.BonusCurrency, ID: a.ID, Name: a.Name, Type: a.ActivityType,
			StartsAt: a.StartsAt, EndsAt: a.EndsAt, MaxUsesPerUser: a.MaxUsesPerUser, PlanBonuses: bonuses})
	}
	return result, nil
}
