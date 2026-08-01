package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivity"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivityparticipation"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	entuser "github.com/Wei-Shaw/sub2api/ent/user"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type PromotionActivityRecordListParams struct {
	Page     int
	PageSize int
	Keyword  string
	Status   string
}

type PromotionActivityParticipantListParams struct {
	Page     int
	PageSize int
	Keyword  string
}

type PromotionActivityParticipationListParams struct {
	Page     int
	PageSize int
	UserID   int64
	Keyword  string
	Status   string
}

type PromotionActivityRecordView struct {
	PromotionActivityView
	ParticipantCount   int `json:"participant_count"`
	ParticipationCount int `json:"participation_count"`
	ReservedCount      int `json:"reserved_count"`
	GrantedCount       int `json:"granted_count"`
	ReleasedCount      int `json:"released_count"`
	GrantedBonusDays   int `json:"granted_bonus_days"`
}

type PromotionActivityParticipantView struct {
	UserID              int64     `json:"user_id"`
	UserEmail           string    `json:"user_email"`
	UserName            string    `json:"user_name"`
	ParticipationCount  int       `json:"participation_count"`
	ReservedCount       int       `json:"reserved_count"`
	GrantedCount        int       `json:"granted_count"`
	ReleasedCount       int       `json:"released_count"`
	GrantedBonusDays    int       `json:"granted_bonus_days"`
	FirstParticipatedAt time.Time `json:"first_participated_at"`
	LastParticipatedAt  time.Time `json:"last_participated_at"`
}

type PromotionActivityParticipationView struct {
	ID                    int64      `json:"id"`
	ActivityID            int64      `json:"activity_id"`
	UserID                int64      `json:"user_id"`
	UserEmail             string     `json:"user_email"`
	UserName              string     `json:"user_name"`
	OrderID               int64      `json:"order_id"`
	OutTradeNo            string     `json:"out_trade_no"`
	OrderStatus           string     `json:"order_status"`
	PaymentType           string     `json:"payment_type"`
	Amount                float64    `json:"amount"`
	PayAmount             float64    `json:"pay_amount"`
	PlanID                int64      `json:"plan_id"`
	PlanName              string     `json:"plan_name"`
	SubscriptionDays      *int       `json:"subscription_days,omitempty"`
	SubscriptionBonusDays int        `json:"subscription_bonus_days"`
	Status                string     `json:"status"`
	BonusDays             int        `json:"bonus_days"`
	ReservedAt            time.Time  `json:"reserved_at"`
	GrantedAt             *time.Time `json:"granted_at,omitempty"`
	ReleasedAt            *time.Time `json:"released_at,omitempty"`
	ReleaseReason         *string    `json:"release_reason,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	OrderCreatedAt        *time.Time `json:"order_created_at,omitempty"`
	PaidAt                *time.Time `json:"paid_at,omitempty"`
	CompletedAt           *time.Time `json:"completed_at,omitempty"`
	FailedAt              *time.Time `json:"failed_at,omitempty"`
	FailedReason          *string    `json:"failed_reason,omitempty"`
	RefundAmount          float64    `json:"refund_amount"`
	RefundAt              *time.Time `json:"refund_at,omitempty"`
}

type promotionActivityTotalRow struct {
	ActivityID         int64 `json:"activity_id"`
	ParticipantCount   int   `json:"participant_count"`
	ParticipationCount int   `json:"participation_count"`
}

type promotionActivityStatusRow struct {
	ActivityID int64  `json:"activity_id"`
	UserID     int64  `json:"user_id"`
	Status     string `json:"status"`
	Count      int    `json:"record_count"`
	BonusDays  int    `json:"bonus_days"`
}

type promotionActivityParticipantRow struct {
	UserID             int64 `json:"user_id"`
	ParticipationCount int   `json:"participation_count"`
}

func promotionCountDistinct(field string) dbent.AggregateFunc {
	return func(selector *entsql.Selector) string {
		return fmt.Sprintf("COUNT(DISTINCT %s)", selector.C(field))
	}
}

func (s *PaymentConfigService) AdminListPromotionActivityRecords(ctx context.Context, params PromotionActivityRecordListParams) ([]PromotionActivityRecordView, int, error) {
	if s == nil || s.entClient == nil {
		return nil, 0, fmt.Errorf("list promotion activity records: service is unavailable")
	}
	query := s.entClient.PromotionActivity.Query().WithPlanBonuses(func(q *dbent.PromotionActivityPlanQuery) {
		q.Order(dbent.Asc("plan_id"))
	})
	if keyword := strings.TrimSpace(params.Keyword); keyword != "" {
		query = query.Where(promotionactivity.NameContainsFold(keyword))
	}
	if status := strings.TrimSpace(params.Status); status != "" {
		statusPredicate, err := promotionActivityRecordStatusPredicate(status, time.Now())
		if err != nil {
			return nil, 0, err
		}
		query = query.Where(statusPredicate)
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count promotion activity records: %w", err)
	}
	pageSize, page := applyPagination(params.PageSize, params.Page)
	activities, err := query.Order(dbent.Desc(promotionactivity.FieldCreatedAt), dbent.Desc(promotionactivity.FieldID)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list promotion activity records: %w", err)
	}
	if len(activities) == 0 {
		return []PromotionActivityRecordView{}, total, nil
	}

	activityIDs := make([]int64, 0, len(activities))
	for _, activity := range activities {
		activityIDs = append(activityIDs, activity.ID)
	}
	totalRows := make([]promotionActivityTotalRow, 0, len(activityIDs))
	if err := s.entClient.PromotionActivityParticipation.Query().
		Where(promotionactivityparticipation.ActivityIDIn(activityIDs...)).
		GroupBy(promotionactivityparticipation.FieldActivityID).
		Aggregate(
			dbent.As(promotionCountDistinct(promotionactivityparticipation.FieldUserID), "participant_count"),
			dbent.As(dbent.Count(), "participation_count"),
		).
		Scan(ctx, &totalRows); err != nil {
		return nil, 0, fmt.Errorf("aggregate promotion activity participants: %w", err)
	}
	statusRows := make([]promotionActivityStatusRow, 0, len(activityIDs)*3)
	if err := s.entClient.PromotionActivityParticipation.Query().
		Where(promotionactivityparticipation.ActivityIDIn(activityIDs...)).
		GroupBy(promotionactivityparticipation.FieldActivityID, promotionactivityparticipation.FieldStatus).
		Aggregate(
			dbent.As(dbent.Count(), "record_count"),
			dbent.As(dbent.Sum(promotionactivityparticipation.FieldBonusDays), "bonus_days"),
		).
		Scan(ctx, &statusRows); err != nil {
		return nil, 0, fmt.Errorf("aggregate promotion activity statuses: %w", err)
	}

	resultByID := make(map[int64]*PromotionActivityRecordView, len(activities))
	result := make([]PromotionActivityRecordView, 0, len(activities))
	now := time.Now()
	for _, activity := range activities {
		item := PromotionActivityRecordView{PromotionActivityView: promotionActivityView(activity, now)}
		result = append(result, item)
		resultByID[activity.ID] = &result[len(result)-1]
	}
	for _, row := range totalRows {
		if item := resultByID[row.ActivityID]; item != nil {
			item.ParticipantCount = row.ParticipantCount
			item.ParticipationCount = row.ParticipationCount
		}
	}
	for _, row := range statusRows {
		item := resultByID[row.ActivityID]
		if item == nil {
			continue
		}
		switch row.Status {
		case PromotionParticipationStatusReserved:
			item.ReservedCount = row.Count
		case PromotionParticipationStatusGranted:
			item.GrantedCount = row.Count
			item.GrantedBonusDays = row.BonusDays
		case PromotionParticipationStatusReleased:
			item.ReleasedCount = row.Count
		}
	}
	return result, total, nil
}

func promotionActivityRecordStatusPredicate(status string, now time.Time) (predicate.PromotionActivity, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case PromotionActivityStatusDisabled:
		return promotionactivity.EnabledEQ(false), nil
	case PromotionActivityStatusScheduled:
		return promotionactivity.And(promotionactivity.EnabledEQ(true), promotionactivity.StartsAtGT(now)), nil
	case PromotionActivityStatusActive:
		return promotionactivity.And(promotionactivity.EnabledEQ(true), promotionactivity.StartsAtLTE(now), promotionactivity.EndsAtGT(now)), nil
	case PromotionActivityStatusEnded:
		return promotionactivity.And(promotionactivity.EnabledEQ(true), promotionactivity.EndsAtLTE(now)), nil
	default:
		return nil, infraerrors.BadRequest("PROMOTION_ACTIVITY_STATUS_INVALID", "invalid promotion activity status")
	}
}

func (s *PaymentConfigService) AdminListPromotionActivityParticipants(ctx context.Context, activityID int64, params PromotionActivityParticipantListParams) ([]PromotionActivityParticipantView, int, error) {
	if err := s.ensurePromotionActivityExists(ctx, activityID); err != nil {
		return nil, 0, err
	}
	query := s.entClient.PromotionActivityParticipation.Query().
		Where(promotionactivityparticipation.ActivityIDEQ(activityID))
	if keyword := strings.TrimSpace(params.Keyword); keyword != "" {
		matches, err := s.findPromotionActivityOrderMatches(ctx, activityID, 0, keyword)
		if err != nil {
			return nil, 0, err
		}
		userIDs := uniquePromotionMatchUserIDs(matches)
		if len(userIDs) == 0 {
			return []PromotionActivityParticipantView{}, 0, nil
		}
		query = query.Where(promotionactivityparticipation.UserIDIn(userIDs...))
	}

	totalRows := []struct {
		Total int `json:"total"`
	}{}
	if err := query.Clone().Aggregate(dbent.As(promotionCountDistinct(promotionactivityparticipation.FieldUserID), "total")).Scan(ctx, &totalRows); err != nil {
		return nil, 0, fmt.Errorf("count promotion activity participants: %w", err)
	}
	total := 0
	if len(totalRows) > 0 {
		total = totalRows[0].Total
	}
	pageSize, page := applyPagination(params.PageSize, params.Page)
	rows := make([]promotionActivityParticipantRow, 0, pageSize)
	if err := query.Clone().
		Order(dbent.Desc(promotionactivityparticipation.FieldUserID)).
		Offset((page-1)*pageSize).
		Limit(pageSize).
		GroupBy(promotionactivityparticipation.FieldUserID).
		Aggregate(dbent.As(dbent.Count(), "participation_count")).
		Scan(ctx, &rows); err != nil {
		return nil, 0, fmt.Errorf("list promotion activity participants: %w", err)
	}
	if len(rows) == 0 {
		return []PromotionActivityParticipantView{}, total, nil
	}
	userIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.UserID)
	}
	statusRows := make([]promotionActivityStatusRow, 0, len(rows)*3)
	if err := s.entClient.PromotionActivityParticipation.Query().
		Where(
			promotionactivityparticipation.ActivityIDEQ(activityID),
			promotionactivityparticipation.UserIDIn(userIDs...),
		).
		GroupBy(promotionactivityparticipation.FieldUserID, promotionactivityparticipation.FieldStatus).
		Aggregate(
			dbent.As(dbent.Count(), "record_count"),
			dbent.As(dbent.Sum(promotionactivityparticipation.FieldBonusDays), "bonus_days"),
		).
		Scan(ctx, &statusRows); err != nil {
		return nil, 0, fmt.Errorf("aggregate promotion participant statuses: %w", err)
	}
	participationDates, err := s.entClient.PromotionActivityParticipation.Query().
		Where(
			promotionactivityparticipation.ActivityIDEQ(activityID),
			promotionactivityparticipation.UserIDIn(userIDs...),
		).
		Select(promotionactivityparticipation.FieldUserID, promotionactivityparticipation.FieldCreatedAt).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("load promotion participant dates: %w", err)
	}
	timeRanges := make(map[int64][2]time.Time, len(userIDs))
	for _, participation := range participationDates {
		current := timeRanges[participation.UserID]
		if current[0].IsZero() || participation.CreatedAt.Before(current[0]) {
			current[0] = participation.CreatedAt
		}
		if current[1].IsZero() || participation.CreatedAt.After(current[1]) {
			current[1] = participation.CreatedAt
		}
		timeRanges[participation.UserID] = current
	}

	identityMap, err := s.loadPromotionParticipantIdentities(ctx, activityID, userIDs)
	if err != nil {
		return nil, 0, err
	}
	resultByUserID := make(map[int64]*PromotionActivityParticipantView, len(rows))
	result := make([]PromotionActivityParticipantView, 0, len(rows))
	for _, row := range rows {
		identity := identityMap[row.UserID]
		timeRange := timeRanges[row.UserID]
		item := PromotionActivityParticipantView{
			UserID: row.UserID, UserEmail: identity.email, UserName: identity.name,
			ParticipationCount:  row.ParticipationCount,
			FirstParticipatedAt: timeRange[0], LastParticipatedAt: timeRange[1],
		}
		result = append(result, item)
		resultByUserID[row.UserID] = &result[len(result)-1]
	}
	for _, row := range statusRows {
		item := resultByUserID[row.UserID]
		if item == nil {
			continue
		}
		switch row.Status {
		case PromotionParticipationStatusReserved:
			item.ReservedCount = row.Count
		case PromotionParticipationStatusGranted:
			item.GrantedCount = row.Count
			item.GrantedBonusDays = row.BonusDays
		case PromotionParticipationStatusReleased:
			item.ReleasedCount = row.Count
		}
	}
	return result, total, nil
}

type promotionParticipantIdentity struct {
	email string
	name  string
}

func (s *PaymentConfigService) loadPromotionParticipantIdentities(ctx context.Context, activityID int64, userIDs []int64) (map[int64]promotionParticipantIdentity, error) {
	result := make(map[int64]promotionParticipantIdentity, len(userIDs))
	users, err := s.entClient.User.Query().Where(entuser.IDIn(userIDs...)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load promotion participant users: %w", err)
	}
	for _, current := range users {
		result[current.ID] = promotionParticipantIdentity{email: current.Email, name: current.Username}
	}
	missing := make([]int64, 0)
	for _, userID := range userIDs {
		if _, ok := result[userID]; !ok {
			missing = append(missing, userID)
		}
	}
	if len(missing) == 0 {
		return result, nil
	}
	orders, err := s.entClient.PaymentOrder.Query().
		Where(paymentorder.SubscriptionBonusActivityIDEQ(activityID), paymentorder.UserIDIn(missing...)).
		Order(dbent.Desc(paymentorder.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load promotion participant order snapshots: %w", err)
	}
	for _, order := range orders {
		if _, ok := result[order.UserID]; ok {
			continue
		}
		result[order.UserID] = promotionParticipantIdentity{email: order.UserEmail, name: order.UserName}
	}
	return result, nil
}

func (s *PaymentConfigService) AdminListPromotionActivityParticipations(ctx context.Context, activityID int64, params PromotionActivityParticipationListParams) ([]PromotionActivityParticipationView, int, error) {
	if err := s.ensurePromotionActivityExists(ctx, activityID); err != nil {
		return nil, 0, err
	}
	query := s.entClient.PromotionActivityParticipation.Query().
		Where(promotionactivityparticipation.ActivityIDEQ(activityID))
	if params.UserID > 0 {
		query = query.Where(promotionactivityparticipation.UserIDEQ(params.UserID))
	}
	if status := strings.TrimSpace(params.Status); status != "" {
		if !validPromotionParticipationStatus(status) {
			return nil, 0, infraerrors.BadRequest("PROMOTION_PARTICIPATION_STATUS_INVALID", "invalid promotion participation status")
		}
		query = query.Where(promotionactivityparticipation.StatusEQ(status))
	}
	if keyword := strings.TrimSpace(params.Keyword); keyword != "" {
		matches, err := s.findPromotionActivityOrderMatches(ctx, activityID, params.UserID, keyword)
		if err != nil {
			return nil, 0, err
		}
		orderIDs := uniquePromotionMatchOrderIDs(matches)
		if len(orderIDs) == 0 {
			return []PromotionActivityParticipationView{}, 0, nil
		}
		query = query.Where(promotionactivityparticipation.OrderIDIn(orderIDs...))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count promotion activity participations: %w", err)
	}
	pageSize, page := applyPagination(params.PageSize, params.Page)
	participations, err := query.Order(dbent.Desc(promotionactivityparticipation.FieldCreatedAt), dbent.Desc(promotionactivityparticipation.FieldID)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list promotion activity participations: %w", err)
	}
	if len(participations) == 0 {
		return []PromotionActivityParticipationView{}, total, nil
	}

	orderIDs := make([]int64, 0, len(participations))
	planIDs := make([]int64, 0, len(participations))
	for _, participation := range participations {
		orderIDs = append(orderIDs, participation.OrderID)
		planIDs = append(planIDs, participation.PlanID)
	}
	orders, err := s.entClient.PaymentOrder.Query().Where(paymentorder.IDIn(orderIDs...)).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("load promotion participation orders: %w", err)
	}
	orderMap := make(map[int64]*dbent.PaymentOrder, len(orders))
	for _, order := range orders {
		orderMap[order.ID] = order
	}
	plans, err := s.entClient.SubscriptionPlan.Query().Where(subscriptionplan.IDIn(planIDs...)).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("load promotion participation plans: %w", err)
	}
	planNames := make(map[int64]string, len(plans))
	for _, plan := range plans {
		planNames[plan.ID] = plan.Name
	}

	result := make([]PromotionActivityParticipationView, 0, len(participations))
	for _, participation := range participations {
		item := PromotionActivityParticipationView{
			ID: participation.ID, ActivityID: participation.ActivityID,
			UserID: participation.UserID, OrderID: participation.OrderID,
			PlanID: participation.PlanID, PlanName: planNames[participation.PlanID],
			Status: participation.Status, BonusDays: participation.BonusDays,
			ReservedAt: participation.ReservedAt, GrantedAt: participation.GrantedAt,
			ReleasedAt: participation.ReleasedAt, ReleaseReason: participation.ReleaseReason,
			CreatedAt: participation.CreatedAt,
		}
		if order := orderMap[participation.OrderID]; order != nil {
			createdAt := order.CreatedAt
			item.UserEmail = order.UserEmail
			item.UserName = order.UserName
			item.OutTradeNo = order.OutTradeNo
			item.OrderStatus = order.Status
			item.PaymentType = order.PaymentType
			item.Amount = order.Amount
			item.PayAmount = order.PayAmount
			item.SubscriptionDays = order.SubscriptionDays
			item.SubscriptionBonusDays = order.SubscriptionBonusDays
			item.OrderCreatedAt = &createdAt
			item.PaidAt = order.PaidAt
			item.CompletedAt = order.CompletedAt
			item.FailedAt = order.FailedAt
			item.FailedReason = order.FailedReason
			item.RefundAmount = order.RefundAmount
			item.RefundAt = order.RefundAt
		}
		result = append(result, item)
	}
	return result, total, nil
}

func (s *PaymentConfigService) ensurePromotionActivityExists(ctx context.Context, activityID int64) error {
	if activityID <= 0 {
		return infraerrors.BadRequest("PROMOTION_ACTIVITY_ID_INVALID", "invalid promotion activity id")
	}
	exists, err := s.entClient.PromotionActivity.Query().Where(promotionactivity.IDEQ(activityID)).Exist(ctx)
	if err != nil {
		return fmt.Errorf("check promotion activity: %w", err)
	}
	if !exists {
		return infraerrors.NotFound("PROMOTION_ACTIVITY_NOT_FOUND", "promotion activity not found")
	}
	return nil
}

func validPromotionParticipationStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case PromotionParticipationStatusReserved, PromotionParticipationStatusGranted, PromotionParticipationStatusReleased:
		return true
	default:
		return false
	}
}

type promotionActivityOrderMatch struct {
	OrderID int64 `json:"id"`
	UserID  int64 `json:"user_id"`
}

func (s *PaymentConfigService) findPromotionActivityOrderMatches(ctx context.Context, activityID, userID int64, keyword string) ([]promotionActivityOrderMatch, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}
	query := s.entClient.PaymentOrder.Query().Where(paymentorder.SubscriptionBonusActivityIDEQ(activityID))
	if userID > 0 {
		query = query.Where(paymentorder.UserIDEQ(userID))
	}
	predicates := []predicate.PaymentOrder{
		paymentorder.UserEmailContainsFold(keyword),
		paymentorder.UserNameContainsFold(keyword),
		paymentorder.OutTradeNoContainsFold(keyword),
	}
	if numeric, err := strconv.ParseInt(keyword, 10, 64); err == nil && numeric > 0 {
		predicates = append(predicates, paymentorder.IDEQ(numeric), paymentorder.UserIDEQ(numeric))
	}
	rows := make([]promotionActivityOrderMatch, 0)
	if err := query.Where(paymentorder.Or(predicates...)).
		Select(paymentorder.FieldID, paymentorder.FieldUserID).
		Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("search promotion participation orders: %w", err)
	}
	return rows, nil
}

func uniquePromotionMatchUserIDs(matches []promotionActivityOrderMatch) []int64 {
	seen := make(map[int64]struct{}, len(matches))
	result := make([]int64, 0, len(matches))
	for _, match := range matches {
		if _, ok := seen[match.UserID]; ok {
			continue
		}
		seen[match.UserID] = struct{}{}
		result = append(result, match.UserID)
	}
	return result
}

func uniquePromotionMatchOrderIDs(matches []promotionActivityOrderMatch) []int64 {
	result := make([]int64, 0, len(matches))
	for _, match := range matches {
		result = append(result, match.OrderID)
	}
	return result
}
