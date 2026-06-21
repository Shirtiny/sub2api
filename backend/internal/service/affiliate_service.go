package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	ErrAffiliateProfileNotFound        = infraerrors.NotFound("AFFILIATE_PROFILE_NOT_FOUND", "affiliate profile not found")
	ErrAffiliateCodeInvalid            = infraerrors.BadRequest("AFFILIATE_CODE_INVALID", "invalid affiliate code")
	ErrAffiliateCodeTaken              = infraerrors.Conflict("AFFILIATE_CODE_TAKEN", "affiliate code already in use")
	ErrAffiliateAlreadyBound           = infraerrors.Conflict("AFFILIATE_ALREADY_BOUND", "affiliate inviter already bound")
	ErrAffiliateInviteLimitReached     = infraerrors.Conflict("AFFILIATE_INVITE_LIMIT_REACHED", "affiliate invite limit reached")
	ErrAffiliateQuotaEmpty             = infraerrors.BadRequest("AFFILIATE_QUOTA_EMPTY", "no affiliate rebate amount available to redeem")
	ErrAffiliateQuotaInsufficient      = infraerrors.BadRequest("AFFILIATE_QUOTA_INSUFFICIENT", "affiliate rebate amount is insufficient")
	ErrAffiliateRedeemTargetInvalid    = infraerrors.BadRequest("AFFILIATE_REDEEM_TARGET_INVALID", "invalid affiliate redeem target")
	ErrAffiliateRedeemPointsTooLarge   = infraerrors.BadRequest("AFFILIATE_REDEEM_POINTS_TOO_LARGE", "affiliate rebate amount exceeds the single redemption limit")
	ErrAffiliateSubscriptionQuotaEmpty = infraerrors.BadRequest("AFFILIATE_SUBSCRIPTION_QUOTA_EMPTY", "no affiliate subscription rebate available to transfer")
)

const (
	affiliateInviteesLimit         = 100
	AffiliateRedeemPointsSingleMax = 100.0
	// AffiliateCodeMinLength / AffiliateCodeMaxLength bound both system-generated
	// 12-char codes and admin-customized codes (e.g. "VIP2026").
	AffiliateCodeMinLength = 4
	AffiliateCodeMaxLength = 32
)

// affiliateCodeValidChar accepts uppercase letters, digits, underscore and dash.
// All input passes through strings.ToUpper before validation, so lowercase from
// users is normalized — admins may supply mixed case in their UI.
var affiliateCodeValidChar = func() [256]bool {
	var tbl [256]bool
	for c := byte('A'); c <= 'Z'; c++ {
		tbl[c] = true
	}
	for c := byte('0'); c <= '9'; c++ {
		tbl[c] = true
	}
	tbl['_'] = true
	tbl['-'] = true
	return tbl
}()

// isValidAffiliateCodeFormat validates code format for both binding (user input)
// and admin updates. Caller is expected to upper-case the input first.
func isValidAffiliateCodeFormat(code string) bool {
	if len(code) < AffiliateCodeMinLength || len(code) > AffiliateCodeMaxLength {
		return false
	}
	for i := 0; i < len(code); i++ {
		if !affiliateCodeValidChar[code[i]] {
			return false
		}
	}
	return true
}

type AffiliateSummary struct {
	UserID               int64      `json:"user_id"`
	AffCode              string     `json:"aff_code"`
	AffCodeCustom        bool       `json:"aff_code_custom"`
	AffRebateRatePercent *float64   `json:"aff_rebate_rate_percent,omitempty"`
	AffInviteLimit       *int       `json:"aff_invite_limit,omitempty"`
	InviterID            *int64     `json:"inviter_id,omitempty"`
	InviterBoundAt       *time.Time `json:"inviter_bound_at,omitempty"`
	AffCount             int        `json:"aff_count"`
	AffQuota             float64    `json:"aff_quota"`
	AffFrozenQuota       float64    `json:"aff_frozen_quota"`
	AffHistoryQuota      float64    `json:"aff_history_quota"`
	TotalRecharged       float64    `json:"total_recharged"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type AffiliateInvitee struct {
	UserID                      int64      `json:"user_id"`
	Email                       string     `json:"email"`
	Username                    string     `json:"username"`
	CreatedAt                   *time.Time `json:"created_at,omitempty"`
	TotalRebate                 float64    `json:"total_rebate"`
	TotalRebatePoints           float64    `json:"total_rebate_points"`
	TotalSubscriptionRebateDays int        `json:"total_subscription_rebate_days"`
}

type AffiliateSubscriptionRebateBalance struct {
	GroupID       int64  `json:"group_id"`
	GroupName     string `json:"group_name"`
	AvailableDays int    `json:"available_days"`
	FrozenDays    int    `json:"frozen_days"`
}

type AffiliateSubscriptionTransferResult struct {
	GroupID         int64      `json:"group_id"`
	GroupName       string     `json:"group_name"`
	TransferredDays float64    `json:"transferred_days"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	RedeemedPoints  float64    `json:"-"`
}

type AffiliateRedeemTargetType string

const (
	AffiliateRedeemTargetBalance      AffiliateRedeemTargetType = "balance"
	AffiliateRedeemTargetSubscription AffiliateRedeemTargetType = "subscription"
)

type AffiliateRedeemRequest struct {
	TargetType AffiliateRedeemTargetType
	Points     float64
	GroupID    int64
	PlanID     int64
}

type AffiliateRedeemResult struct {
	TargetType      AffiliateRedeemTargetType `json:"target_type"`
	RedeemedPoints  float64                   `json:"redeemed_points"`
	CreditedBalance float64                   `json:"credited_balance,omitempty"`
	Balance         float64                   `json:"balance,omitempty"`
	GroupID         int64                     `json:"group_id,omitempty"`
	GroupName       string                    `json:"group_name,omitempty"`
	TransferredDays float64                   `json:"transferred_days,omitempty"`
	ExpiresAt       *time.Time                `json:"expires_at,omitempty"`
}

type AffiliateBalanceRedeemResult struct {
	RedeemedPoints  float64
	CreditedBalance float64
	Balance         float64
}

type AffiliateDetail struct {
	UserID                       int64                                `json:"user_id"`
	AffCode                      string                               `json:"aff_code,omitempty"`
	InviterID                    *int64                               `json:"inviter_id,omitempty"`
	AffCount                     int                                  `json:"aff_count"`
	AffQuota                     float64                              `json:"aff_quota"`
	AffFrozenQuota               float64                              `json:"aff_frozen_quota"`
	AffHistoryQuota              float64                              `json:"aff_history_quota"`
	AvailableRebatePoints        float64                              `json:"available_rebate_points"`
	AvailablePoints              float64                              `json:"available_points"`
	FrozenRebatePoints           float64                              `json:"frozen_rebate_points"`
	TotalRebatePoints            float64                              `json:"total_rebate_points"`
	AffiliateRebatePerInviteeCap float64                              `json:"affiliate_rebate_per_invitee_cap"`
	SubscriptionRebateBalances   []AffiliateSubscriptionRebateBalance `json:"subscription_rebate_balances"`
	MembershipLevel              int                                  `json:"membership_level"`
	CanInvite                    bool                                 `json:"can_invite"`
	EffectiveInviteLimit         int                                  `json:"effective_invite_limit"`
	// EffectiveRebateRatePercent 是当前用户作为邀请人时实际生效的返利比例：
	// 优先用户自己的专属比例（aff_rebate_rate_percent），否则回退到全局比例。
	// 用于在用户的 /affiliate 页面直观展示「分享后能拿到多少」。
	EffectiveRebateRatePercent float64            `json:"effective_rebate_rate_percent"`
	Invitees                   []AffiliateInvitee `json:"invitees"`
}

type AffiliateSubscriptionRebate struct {
	InviterID  int64
	RebateDays int
	Reason     string
}

type AffiliateRepository interface {
	EnsureUserAffiliate(ctx context.Context, userID int64) (*AffiliateSummary, error)
	GetAffiliateByCode(ctx context.Context, code string) (*AffiliateSummary, error)
	BindInviter(ctx context.Context, userID, inviterID int64, inviteLimit int) (bool, error)
	AccrueQuota(ctx context.Context, inviterID, inviteeUserID int64, amount float64, freezeHours int, sourceOrderID *int64, perInviteeCap float64) (float64, error)
	AccrueSubscriptionRebate(ctx context.Context, inviterID, inviteeUserID, groupID int64, days, freezeHours int, sourceOrderID *int64) (bool, error)
	GetAccruedRebateFromInvitee(ctx context.Context, inviterID, inviteeUserID int64) (float64, error)
	ThawFrozenQuota(ctx context.Context, userID int64) (float64, error)
	TransferQuotaToBalance(ctx context.Context, userID int64, points float64, balanceMultiplier float64) (*AffiliateBalanceRedeemResult, error)
	TransferQuotaToSubscription(ctx context.Context, userID, groupID, planID int64, points float64) (*AffiliateSubscriptionTransferResult, error)
	TransferSubscriptionRebateToSubscription(ctx context.Context, userID, groupID int64) (*AffiliateSubscriptionTransferResult, error)
	ClawbackQuotaForOrder(ctx context.Context, sourceOrderID int64, ratio float64) (float64, error)
	ListInvitees(ctx context.Context, inviterID int64, limit int) ([]AffiliateInvitee, error)
	ListSubscriptionRebateBalances(ctx context.Context, userID int64) ([]AffiliateSubscriptionRebateBalance, error)
	ListAffiliateLedgerRecords(ctx context.Context, userID int64, filter AffiliateRecordFilter) ([]AffiliateLedgerRecord, int64, error)

	// 管理端：用户级专属配置
	UpdateUserAffCode(ctx context.Context, userID int64, newCode string) error
	ResetUserAffCode(ctx context.Context, userID int64) (string, error)
	SetUserRebateRate(ctx context.Context, userID int64, ratePercent *float64) error
	SetUserInviteLimit(ctx context.Context, userID int64, limit *int) error
	BatchSetUserRebateRate(ctx context.Context, userIDs []int64, ratePercent *float64) error
	ListUsersWithCustomSettings(ctx context.Context, filter AffiliateAdminFilter) ([]AffiliateAdminEntry, int64, error)
	ListAffiliateInviteRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateInviteRecord, int64, error)
	ListAffiliateRebateRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateRebateRecord, int64, error)
	ListAffiliateTransferRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateTransferRecord, int64, error)
	GetAffiliateUserOverview(ctx context.Context, userID int64) (*AffiliateUserOverview, error)
}

// AffiliateAdminFilter 列表筛选条件
type AffiliateAdminFilter struct {
	Search   string
	Page     int
	PageSize int
}

// AffiliateAdminEntry 专属用户列表条目
type AffiliateAdminEntry struct {
	UserID               int64    `json:"user_id"`
	Email                string   `json:"email"`
	Username             string   `json:"username"`
	AffCode              string   `json:"aff_code"`
	AffCodeCustom        bool     `json:"aff_code_custom"`
	AffRebateRatePercent *float64 `json:"aff_rebate_rate_percent,omitempty"`
	AffInviteLimit       *int     `json:"aff_invite_limit,omitempty"`
	AffCount             int      `json:"aff_count"`
}

type AffiliateRecordFilter struct {
	Search   string
	Page     int
	PageSize int
	StartAt  *time.Time
	EndAt    *time.Time
	SortBy   string
	SortDesc bool
}

type AffiliateInviteRecord struct {
	InviterID                   int64     `json:"inviter_id"`
	InviterEmail                string    `json:"inviter_email"`
	InviterUsername             string    `json:"inviter_username"`
	InviteeID                   int64     `json:"invitee_id"`
	InviteeEmail                string    `json:"invitee_email"`
	InviteeUsername             string    `json:"invitee_username"`
	AffCode                     string    `json:"aff_code"`
	TotalRebate                 float64   `json:"total_rebate"`
	TotalRebatePoints           float64   `json:"total_rebate_points"`
	TotalSubscriptionRebateDays int       `json:"total_subscription_rebate_days"`
	CreatedAt                   time.Time `json:"created_at"`
}

type AffiliateRebateRecord struct {
	OrderID                int64     `json:"order_id"`
	OutTradeNo             string    `json:"out_trade_no"`
	InviterID              int64     `json:"inviter_id"`
	InviterEmail           string    `json:"inviter_email"`
	InviterUsername        string    `json:"inviter_username"`
	InviteeID              int64     `json:"invitee_id"`
	InviteeEmail           string    `json:"invitee_email"`
	InviteeUsername        string    `json:"invitee_username"`
	OrderAmount            float64   `json:"order_amount"`
	PayAmount              float64   `json:"pay_amount"`
	RebateAmount           float64   `json:"rebate_amount"`
	RebatePoints           float64   `json:"rebate_points"`
	RebateAction           string    `json:"rebate_action"`
	SubscriptionGroupID    *int64    `json:"subscription_group_id,omitempty"`
	SubscriptionGroupName  string    `json:"subscription_group_name,omitempty"`
	SubscriptionRebateDays int       `json:"subscription_rebate_days"`
	PaymentType            string    `json:"payment_type"`
	OrderStatus            string    `json:"order_status"`
	CreatedAt              time.Time `json:"created_at"`
}

type AffiliateTransferRecord struct {
	LedgerID              int64     `json:"ledger_id"`
	UserID                int64     `json:"user_id"`
	UserEmail             string    `json:"user_email"`
	Username              string    `json:"username"`
	Action                string    `json:"action"`
	Amount                float64   `json:"amount"`
	RedeemedPoints        float64   `json:"redeemed_points,omitempty"`
	SubscriptionGroupID   *int64    `json:"subscription_group_id,omitempty"`
	SubscriptionGroupName string    `json:"subscription_group_name,omitempty"`
	BalanceAfter          *float64  `json:"balance_after,omitempty"`
	AvailableQuotaAfter   *float64  `json:"available_quota_after,omitempty"`
	AvailablePointsAfter  *float64  `json:"available_points_after,omitempty"`
	FrozenQuotaAfter      *float64  `json:"frozen_quota_after,omitempty"`
	FrozenPointsAfter     *float64  `json:"frozen_points_after,omitempty"`
	HistoryQuotaAfter     *float64  `json:"history_quota_after,omitempty"`
	HistoryPointsAfter    *float64  `json:"history_points_after,omitempty"`
	SnapshotAvailable     bool      `json:"snapshot_available"`
	CurrentBalance        float64   `json:"-"`
	RemainingQuota        float64   `json:"-"`
	FrozenQuota           float64   `json:"-"`
	HistoryQuota          float64   `json:"-"`
	CreatedAt             time.Time `json:"created_at"`
}

type AffiliateLedgerRecord struct {
	LedgerID              int64      `json:"ledger_id"`
	Action                string     `json:"action"`
	Amount                float64    `json:"amount"`
	SourceUserID          *int64     `json:"source_user_id,omitempty"`
	SourceUserEmail       string     `json:"source_user_email,omitempty"`
	SourceUsername        string     `json:"source_username,omitempty"`
	SourceOrderID         *int64     `json:"source_order_id,omitempty"`
	OutTradeNo            string     `json:"out_trade_no,omitempty"`
	SubscriptionGroupID   *int64     `json:"subscription_group_id,omitempty"`
	SubscriptionGroupName string     `json:"subscription_group_name,omitempty"`
	BalanceAfter          *float64   `json:"balance_after,omitempty"`
	AvailablePointsAfter  *float64   `json:"available_points_after,omitempty"`
	FrozenPointsAfter     *float64   `json:"frozen_points_after,omitempty"`
	HistoryPointsAfter    *float64   `json:"history_points_after,omitempty"`
	FrozenUntil           *time.Time `json:"frozen_until,omitempty"`
	TransferredAt         *time.Time `json:"transferred_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
}

type AffiliateUserOverview struct {
	UserID                int64   `json:"user_id"`
	Email                 string  `json:"email"`
	Username              string  `json:"username"`
	AffCode               string  `json:"aff_code"`
	RebateRatePercent     float64 `json:"rebate_rate_percent"`
	RebateRateCustom      bool    `json:"-"`
	InvitedCount          int     `json:"invited_count"`
	RebatedInviteeCount   int     `json:"rebated_invitee_count"`
	AvailableQuota        float64 `json:"available_quota"`
	AvailableRebatePoints float64 `json:"available_rebate_points"`
	HistoryQuota          float64 `json:"history_quota"`
	TotalRebatePoints     float64 `json:"total_rebate_points"`
}

type AffiliateService struct {
	repo                 AffiliateRepository
	settingService       *SettingService
	authCacheInvalidator APIKeyAuthCacheInvalidator
	billingCacheService  *BillingCacheService
	subscriptionService  *SubscriptionService
}

func NewAffiliateService(repo AffiliateRepository, settingService *SettingService, authCacheInvalidator APIKeyAuthCacheInvalidator, billingCacheService *BillingCacheService, subscriptionService *SubscriptionService) *AffiliateService {
	return &AffiliateService{
		repo:                 repo,
		settingService:       settingService,
		authCacheInvalidator: authCacheInvalidator,
		billingCacheService:  billingCacheService,
		subscriptionService:  subscriptionService,
	}
}

// IsEnabled reports whether the affiliate (邀请返利) feature is turned on.
func (s *AffiliateService) IsEnabled(ctx context.Context) bool {
	if s == nil || s.settingService == nil {
		return AffiliateEnabledDefault
	}
	return s.settingService.IsAffiliateEnabled(ctx)
}

func (s *AffiliateService) EnsureUserAffiliate(ctx context.Context, userID int64) (*AffiliateSummary, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "invalid user")
	}
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	return s.repo.EnsureUserAffiliate(ctx, userID)
}

func (s *AffiliateService) GetAffiliateDetail(ctx context.Context, userID int64) (*AffiliateDetail, error) {
	// Lazy thaw: move any matured frozen quota to available before reading.
	if s != nil && s.repo != nil {
		// best-effort: thaw failure is non-fatal
		_, _ = s.repo.ThawFrozenQuota(ctx, userID)
	}

	summary, err := s.EnsureUserAffiliate(ctx, userID)
	if err != nil {
		return nil, err
	}
	invitees, err := s.listInvitees(ctx, userID)
	if err != nil {
		return nil, err
	}
	subscriptionRebateBalances, err := s.repo.ListSubscriptionRebateBalances(ctx, userID)
	if err != nil {
		return nil, err
	}
	level := CalculateMembershipLevel(summary.TotalRecharged)
	affiliateEnabled := s.IsEnabled(ctx)
	canInvite := affiliateEnabled && level > 0
	affCode := ""
	rebateRatePercent := 0.0
	effectiveInviteLimit := 0
	if canInvite {
		affCode = summary.AffCode
		rebateRatePercent = s.resolveRebateRatePercent(ctx, summary)
		effectiveInviteLimit = s.resolveInviteLimit(ctx, summary)
	}
	return &AffiliateDetail{
		UserID:                       summary.UserID,
		AffCode:                      affCode,
		InviterID:                    summary.InviterID,
		AffCount:                     summary.AffCount,
		AffQuota:                     summary.AffQuota,
		AffFrozenQuota:               summary.AffFrozenQuota,
		AffHistoryQuota:              summary.AffHistoryQuota,
		AvailableRebatePoints:        summary.AffQuota,
		AvailablePoints:              summary.AffQuota,
		FrozenRebatePoints:           summary.AffFrozenQuota,
		TotalRebatePoints:            summary.AffHistoryQuota,
		AffiliateRebatePerInviteeCap: s.affiliateRebatePerInviteeCap(ctx, summary),
		SubscriptionRebateBalances:   subscriptionRebateBalances,
		MembershipLevel:              level,
		CanInvite:                    canInvite,
		EffectiveInviteLimit:         effectiveInviteLimit,
		EffectiveRebateRatePercent:   rebateRatePercent,
		Invitees:                     invitees,
	}, nil
}

func (s *AffiliateService) CanUseCodeForSignup(ctx context.Context, rawCode string) error {
	code := strings.ToUpper(strings.TrimSpace(rawCode))
	if code == "" {
		return ErrAffiliateCodeInvalid
	}
	if s == nil || s.repo == nil {
		return infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	if !s.IsEnabled(ctx) {
		return ErrAffiliateCodeInvalid
	}
	if !isValidAffiliateCodeFormat(code) {
		return ErrAffiliateCodeInvalid
	}

	inviterSummary, err := s.repo.GetAffiliateByCode(ctx, code)
	if err != nil {
		if errors.Is(err, ErrAffiliateProfileNotFound) {
			return ErrAffiliateCodeInvalid
		}
		return err
	}
	if inviterSummary == nil || inviterSummary.UserID <= 0 {
		return ErrAffiliateCodeInvalid
	}
	if !canInviteByMembership(inviterSummary) {
		return ErrAffiliateCodeInvalid
	}
	inviteLimit := s.resolveInviteLimit(ctx, inviterSummary)
	if inviteLimit <= 0 || inviterSummary.AffCount >= inviteLimit {
		return ErrAffiliateInviteLimitReached
	}
	return nil
}

func (s *AffiliateService) BindInviterByCode(ctx context.Context, userID int64, rawCode string) error {
	code := strings.ToUpper(strings.TrimSpace(rawCode))
	if code == "" {
		return nil
	}
	if s == nil || s.repo == nil {
		return infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	// 总开关关闭时，注册阶段静默忽略 aff 参数（不报错，避免阻断注册流程）
	if !s.IsEnabled(ctx) {
		return nil
	}
	if !isValidAffiliateCodeFormat(code) {
		return ErrAffiliateCodeInvalid
	}

	selfSummary, err := s.repo.EnsureUserAffiliate(ctx, userID)
	if err != nil {
		return err
	}
	if selfSummary.InviterID != nil {
		return nil
	}

	inviterSummary, err := s.repo.GetAffiliateByCode(ctx, code)
	if err != nil {
		if errors.Is(err, ErrAffiliateProfileNotFound) {
			return ErrAffiliateCodeInvalid
		}
		return err
	}
	if inviterSummary == nil || inviterSummary.UserID <= 0 || inviterSummary.UserID == userID {
		return ErrAffiliateCodeInvalid
	}
	if !canInviteByMembership(inviterSummary) {
		return ErrAffiliateCodeInvalid
	}
	inviteLimit := s.resolveInviteLimit(ctx, inviterSummary)
	if inviteLimit <= 0 || inviterSummary.AffCount >= inviteLimit {
		return ErrAffiliateInviteLimitReached
	}

	bound, err := s.repo.BindInviter(ctx, userID, inviterSummary.UserID, inviteLimit)
	if err != nil {
		return err
	}
	if !bound {
		return ErrAffiliateAlreadyBound
	}
	return nil
}

func (s *AffiliateService) ResolveSubscriptionInviteRebate(ctx context.Context, inviteeUserID int64, subscriptionDays int) (*AffiliateSubscriptionRebate, error) {
	result := &AffiliateSubscriptionRebate{Reason: "subscription rebate skipped"}
	if s == nil || s.repo == nil {
		result.Reason = "affiliate service unavailable"
		return result, nil
	}
	if inviteeUserID <= 0 || subscriptionDays <= 0 {
		result.Reason = "invalid invitee or subscription days"
		return result, nil
	}
	if subscriptionDays < AffiliateSubscriptionRebateMinDays {
		result.Reason = "subscription days below monthly rebate threshold"
		return result, nil
	}
	if !s.IsEnabled(ctx) {
		result.Reason = "affiliate disabled"
		return result, nil
	}

	inviteeSummary, err := s.repo.EnsureUserAffiliate(ctx, inviteeUserID)
	if err != nil {
		return nil, err
	}
	if inviteeSummary.InviterID == nil || *inviteeSummary.InviterID <= 0 {
		result.Reason = "no inviter bound"
		return result, nil
	}
	if s.settingService != nil {
		if s.affiliateRebateExpired(ctx, inviteeSummary) {
			result.Reason = "affiliate rebate duration expired"
			return result, nil
		}
	}

	inviterSummary, err := s.repo.EnsureUserAffiliate(ctx, *inviteeSummary.InviterID)
	if err != nil {
		return nil, err
	}
	if !canInviteByMembership(inviterSummary) {
		result.Reason = "inviter membership level not eligible"
		return result, nil
	}
	result.InviterID = *inviteeSummary.InviterID
	result.RebateDays = calculateSubscriptionRebateDaysByMembership(CalculateMembershipLevel(inviterSummary.TotalRecharged))
	result.Reason = ""
	return result, nil
}

func CalculateMembershipLevel(totalRecharged float64) int {
	switch {
	case totalRecharged > MembershipLevel3Threshold:
		return 3
	case totalRecharged > MembershipLevel2Threshold:
		return 2
	case totalRecharged > MembershipLevel1Threshold:
		return 1
	default:
		return 0
	}
}

func canInviteByMembership(inviter *AffiliateSummary) bool {
	return inviter != nil && CalculateMembershipLevel(inviter.TotalRecharged) > 0
}

func calculateSubscriptionRebateDaysByMembership(level int) int {
	if level <= 0 {
		return 0
	}
	if level >= 3 {
		return AffiliateSubscriptionRebateDaysL3
	}
	if level == 2 {
		return AffiliateSubscriptionRebateDaysL2
	}
	return AffiliateSubscriptionRebateDaysBase
}

func (s *AffiliateService) affiliateRebateExpired(ctx context.Context, invitee *AffiliateSummary) bool {
	if s == nil || s.settingService == nil || invitee == nil {
		return false
	}
	durationDays := s.settingService.GetAffiliateRebateDurationDays(ctx)
	if durationDays <= 0 {
		return false
	}
	boundAt := invitee.CreatedAt
	if invitee.InviterBoundAt != nil {
		boundAt = *invitee.InviterBoundAt
	}
	return time.Now().After(boundAt.AddDate(0, 0, durationDays))
}

func (s *AffiliateService) AccrueSubscriptionRebateForOrder(ctx context.Context, inviteeUserID, groupID int64, subscriptionDays int, sourceOrderID *int64) (*AffiliateSubscriptionRebate, error) {
	if s == nil || s.repo == nil {
		return &AffiliateSubscriptionRebate{Reason: "affiliate service unavailable"}, nil
	}
	rebate, err := s.ResolveSubscriptionInviteRebate(ctx, inviteeUserID, subscriptionDays)
	if err != nil {
		return nil, err
	}
	if rebate == nil || rebate.RebateDays <= 0 || rebate.InviterID <= 0 || groupID <= 0 {
		return rebate, nil
	}
	freezeHours := 0
	if s.settingService != nil {
		freezeHours = s.settingService.GetAffiliateRebateFreezeHours(ctx)
	}
	applied, err := s.repo.AccrueSubscriptionRebate(ctx, rebate.InviterID, inviteeUserID, groupID, rebate.RebateDays, freezeHours, sourceOrderID)
	if err != nil {
		return nil, err
	}
	if !applied {
		rebate.Reason = "subscription rebate already accrued"
	}
	return rebate, nil
}

func (s *AffiliateService) AccrueInviteRebateForOrder(ctx context.Context, inviteeUserID int64, baseRechargeAmount float64, sourceOrderID *int64) (float64, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	if inviteeUserID <= 0 || baseRechargeAmount <= 0 || math.IsNaN(baseRechargeAmount) || math.IsInf(baseRechargeAmount, 0) {
		return 0, nil
	}
	// 总开关关闭时，新充值不再产生返利
	if !s.IsEnabled(ctx) {
		return 0, nil
	}

	inviteeSummary, err := s.repo.EnsureUserAffiliate(ctx, inviteeUserID)
	if err != nil {
		return 0, err
	}
	if inviteeSummary.InviterID == nil || *inviteeSummary.InviterID <= 0 {
		return 0, nil
	}

	// 加载邀请人 profile，优先使用专属比例（覆盖全局）
	inviterSummary, err := s.repo.EnsureUserAffiliate(ctx, *inviteeSummary.InviterID)
	if err != nil {
		return 0, err
	}
	if !canInviteByMembership(inviterSummary) {
		return 0, nil
	}
	// 有效期检查：超过返利有效期后不再产生返利
	if s.affiliateRebateExpired(ctx, inviteeSummary) {
		return 0, nil
	}

	rebateRatePercent := s.resolveRebateRatePercent(ctx, inviterSummary)
	rebate := roundTo(baseRechargeAmount*(rebateRatePercent/100), 8)
	if rebate <= 0 {
		return 0, nil
	}

	// 单人上限检查在 repository 事务内重算并截断，避免并发订单突破 cap。
	perInviteeCap := s.affiliateRebatePerInviteeCap(ctx, inviterSummary)
	if perInviteeCap <= 0 {
		return 0, nil
	}

	var freezeHours int
	if s.settingService != nil {
		freezeHours = s.settingService.GetAffiliateRebateFreezeHours(ctx)
	}

	appliedAmount, err := s.repo.AccrueQuota(ctx, *inviteeSummary.InviterID, inviteeUserID, rebate, freezeHours, sourceOrderID, perInviteeCap)
	if err != nil {
		return 0, err
	}
	return appliedAmount, nil
}

func (s *AffiliateService) ClawbackInviteRebateForRefund(ctx context.Context, sourceOrderID int64, refundAmount, orderAmount float64) (float64, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	if sourceOrderID <= 0 || refundAmount <= 0 || orderAmount <= 0 || math.IsNaN(refundAmount) || math.IsInf(refundAmount, 0) || math.IsNaN(orderAmount) || math.IsInf(orderAmount, 0) {
		return 0, nil
	}
	ratio := refundAmount / orderAmount
	if ratio > 1 {
		ratio = 1
	}
	if ratio <= 0 {
		return 0, nil
	}
	return s.repo.ClawbackQuotaForOrder(ctx, sourceOrderID, ratio)
}

func (s *AffiliateService) affiliateRebatePerInviteeCap(ctx context.Context, inviter *AffiliateSummary) float64 {
	level := 0
	if inviter != nil {
		level = CalculateMembershipLevel(inviter.TotalRecharged)
	}
	if s == nil || s.settingService == nil {
		return defaultAffiliateRebatePerInviteeCapByMembership(level)
	}
	return s.settingService.GetAffiliateRebatePerInviteeCapByLevel(ctx, level)
}

func defaultAffiliateRebatePerInviteeCapByMembership(level int) float64 {
	switch {
	case level >= 3:
		return AffiliateRebatePerInviteeCapLevel3Default
	case level == 2:
		return AffiliateRebatePerInviteeCapLevel2Default
	case level == 1:
		return AffiliateRebatePerInviteeCapLevel1Default
	default:
		return AffiliateRebatePerInviteeCapLevel0Default
	}
}

func (s *AffiliateService) resolveInviteLimit(ctx context.Context, inviter *AffiliateSummary) int {
	if inviter != nil && inviter.AffInviteLimit != nil {
		return clampAffiliateInviteLimit(*inviter.AffInviteLimit)
	}
	if s == nil || s.settingService == nil {
		return defaultAffiliateInviteLimitByMembership(CalculateMembershipLevel(inviter.TotalRecharged))
	}
	return s.settingService.GetAffiliateInviteLimitByLevel(ctx, CalculateMembershipLevel(inviter.TotalRecharged))
}

// resolveRebateRatePercent returns the inviter's exclusive rate when set,
// otherwise the membership-level default rate.
func defaultAffiliateInviteLimitByMembership(level int) int {
	switch {
	case level >= 3:
		return AffiliateInviteLimitLevel3Default
	case level == 2:
		return AffiliateInviteLimitLevel2Default
	case level == 1:
		return AffiliateInviteLimitLevel1Default
	default:
		return AffiliateInviteLimitLevel0Default
	}
}

func (s *AffiliateService) resolveRebateRatePercent(ctx context.Context, inviter *AffiliateSummary) float64 {
	if inviter != nil && inviter.AffRebateRatePercent != nil {
		v := *inviter.AffRebateRatePercent
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return s.membershipRebateRatePercent(ctx, inviter)
		}
		return clampAffiliateRebateRate(v)
	}
	return s.membershipRebateRatePercent(ctx, inviter)
}

func (s *AffiliateService) membershipRebateRatePercent(ctx context.Context, inviter *AffiliateSummary) float64 {
	level := 0
	if inviter != nil {
		level = CalculateMembershipLevel(inviter.TotalRecharged)
	}
	if s == nil || s.settingService == nil {
		return defaultAffiliateRebateRateByMembership(level)
	}
	return s.settingService.GetAffiliateRebateRatePercentByLevel(ctx, level)
}

func defaultAffiliateRebateRateByMembership(level int) float64 {
	switch {
	case level >= 3:
		return AffiliateRebateRateLevel3Default
	case level == 2:
		return AffiliateRebateRateLevel2Default
	case level == 1:
		return AffiliateRebateRateLevel1Default
	default:
		return AffiliateRebateRateLevel0Default
	}
}

func (s *AffiliateService) TransferAffiliateQuota(ctx context.Context, userID int64) (float64, float64, error) {
	if s == nil || s.repo == nil {
		return 0, 0, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	result, err := s.repo.TransferQuotaToBalance(ctx, userID, 0, s.balanceRechargeMultiplier(ctx))
	if err != nil {
		return 0, 0, err
	}
	if result != nil && result.RedeemedPoints > 0 {
		s.invalidateAffiliateCaches(ctx, userID)
	}
	return result.RedeemedPoints, result.Balance, nil
}

func (s *AffiliateService) RedeemAffiliatePoints(ctx context.Context, userID int64, req AffiliateRedeemRequest) (*AffiliateRedeemResult, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	if userID <= 0 {
		return nil, ErrAffiliateQuotaEmpty
	}
	points := req.Points
	if points < 0 || math.IsNaN(points) || math.IsInf(points, 0) {
		return nil, ErrAffiliateQuotaInsufficient
	}
	points = roundTo(points, 8)

	switch req.TargetType {
	case "", AffiliateRedeemTargetBalance:
		if points > AffiliateRedeemPointsSingleMax {
			return nil, ErrAffiliateRedeemPointsTooLarge
		}
		balanceResult, err := s.repo.TransferQuotaToBalance(ctx, userID, points, s.balanceRechargeMultiplier(ctx))
		if err != nil {
			return nil, err
		}
		if balanceResult != nil && balanceResult.RedeemedPoints > 0 {
			s.invalidateAffiliateCaches(ctx, userID)
		}
		return &AffiliateRedeemResult{
			TargetType:      AffiliateRedeemTargetBalance,
			RedeemedPoints:  balanceResult.RedeemedPoints,
			CreditedBalance: balanceResult.CreditedBalance,
			Balance:         balanceResult.Balance,
		}, nil
	case AffiliateRedeemTargetSubscription:
		if req.GroupID <= 0 || req.PlanID <= 0 {
			return nil, ErrAffiliateRedeemTargetInvalid
		}
		result, err := s.redeemAffiliateQuotaToSubscriptionWithPlan(ctx, userID, req.GroupID, req.PlanID, points)
		if err != nil {
			return nil, err
		}
		return &AffiliateRedeemResult{
			TargetType:      AffiliateRedeemTargetSubscription,
			RedeemedPoints:  result.RedeemedPoints,
			GroupID:         result.GroupID,
			GroupName:       result.GroupName,
			TransferredDays: result.TransferredDays,
			ExpiresAt:       result.ExpiresAt,
		}, nil
	default:
		return nil, ErrAffiliateRedeemTargetInvalid
	}
}

func (s *AffiliateService) TransferAffiliateQuotaToSubscription(ctx context.Context, userID, groupID, planID int64) (*AffiliateSubscriptionTransferResult, error) {
	if userID <= 0 || groupID <= 0 || planID <= 0 {
		return nil, ErrAffiliateQuotaEmpty
	}
	return s.redeemAffiliateQuotaToSubscriptionWithPlan(ctx, userID, groupID, planID, 0)
}

func (s *AffiliateService) redeemAffiliateQuotaToSubscriptionWithPlan(ctx context.Context, userID, groupID, planID int64, points float64) (*AffiliateSubscriptionTransferResult, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	result, err := s.repo.TransferQuotaToSubscription(ctx, userID, groupID, planID, points)
	if err != nil {
		return nil, err
	}
	if result != nil && result.TransferredDays > 0 {
		s.invalidateAffiliateSubscriptionCaches(ctx, userID, groupID)
	}
	return result, nil
}

func (s *AffiliateService) balanceRechargeMultiplier(ctx context.Context) float64 {
	if s == nil || s.settingService == nil {
		return defaultBalanceRechargeMultiplier
	}
	return s.settingService.GetBalanceRechargeMultiplier(ctx)
}

func (s *AffiliateService) TransferAffiliateSubscriptionRebate(ctx context.Context, userID, groupID int64) (*AffiliateSubscriptionTransferResult, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	if userID <= 0 || groupID <= 0 {
		return nil, ErrAffiliateSubscriptionQuotaEmpty
	}

	result, err := s.repo.TransferSubscriptionRebateToSubscription(ctx, userID, groupID)
	if err != nil {
		return nil, err
	}
	if result != nil && result.TransferredDays > 0 {
		s.invalidateAffiliateSubscriptionCaches(ctx, userID, groupID)
	}
	return result, nil
}

func (s *AffiliateService) listInvitees(ctx context.Context, inviterID int64) ([]AffiliateInvitee, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	invitees, err := s.repo.ListInvitees(ctx, inviterID, affiliateInviteesLimit)
	if err != nil {
		return nil, err
	}
	for i := range invitees {
		invitees[i].Email = maskEmail(invitees[i].Email)
	}
	return invitees, nil
}

func roundTo(v float64, scale int) float64 {
	factor := math.Pow10(scale)
	return math.Round(v*factor) / factor
}

func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	at := strings.Index(email, "@")
	if at <= 0 || at >= len(email)-1 {
		return "***"
	}

	local := email[:at]
	domain := email[at+1:]
	dot := strings.LastIndex(domain, ".")

	maskedLocal := maskSegment(local)
	if dot <= 0 || dot >= len(domain)-1 {
		return maskedLocal + "@" + maskSegment(domain)
	}

	domainName := domain[:dot]
	tld := domain[dot:]
	return maskedLocal + "@" + maskSegment(domainName) + tld
}

func maskSegment(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return "***"
	}
	if len(r) == 1 {
		return string(r[0]) + "***"
	}
	return string(r[0]) + "***"
}

func maskAccountIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return maskSegment(value)
}

func (s *AffiliateService) invalidateAffiliateCaches(ctx context.Context, userID int64) {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.billingCacheService != nil {
		if err := s.billingCacheService.InvalidateUserBalance(ctx, userID); err != nil {
			logger.LegacyPrintf("service.affiliate", "[Affiliate] Failed to invalidate billing cache for user %d: %v", userID, err)
		}
	}
}

func (s *AffiliateService) invalidateAffiliateSubscriptionCaches(ctx context.Context, userID, groupID int64) {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.subscriptionService != nil {
		s.subscriptionService.InvalidateSubCache(userID, groupID)
	}
	if s.billingCacheService != nil {
		if err := s.billingCacheService.InvalidateSubscription(ctx, userID, groupID); err != nil {
			logger.LegacyPrintf("service.affiliate", "[Affiliate] Failed to invalidate subscription cache for user %d group %d: %v", userID, groupID, err)
		}
	}
}

// =========================
// Admin: 专属配置管理
// =========================

// validateExclusiveRate ensures a per-user override is finite and within
// [Min, Max]. nil is always valid (means "clear / fall back to global").
func validateExclusiveRate(ratePercent *float64) error {
	if ratePercent == nil {
		return nil
	}
	v := *ratePercent
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return infraerrors.BadRequest("INVALID_RATE", "invalid rebate rate")
	}
	if v < AffiliateRebateRateMin || v > AffiliateRebateRateMax {
		return infraerrors.BadRequest("INVALID_RATE", "rebate rate out of range")
	}
	return nil
}

func validateInviteLimit(limit *int) error {
	if limit == nil {
		return nil
	}
	if *limit < 0 || *limit > AffiliateInviteLimitMax {
		return infraerrors.BadRequest("INVALID_INVITE_LIMIT", "invite limit out of range")
	}
	return nil
}

// AdminUpdateUserAffCode 管理员改写用户的邀请码（专属邀请码）。
func (s *AffiliateService) AdminUpdateUserAffCode(ctx context.Context, userID int64, rawCode string) error {
	if s == nil || s.repo == nil {
		return infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	code := strings.ToUpper(strings.TrimSpace(rawCode))
	if !isValidAffiliateCodeFormat(code) {
		return ErrAffiliateCodeInvalid
	}
	return s.repo.UpdateUserAffCode(ctx, userID, code)
}

// AdminResetUserAffCode 重置用户邀请码为系统随机码。
func (s *AffiliateService) AdminResetUserAffCode(ctx context.Context, userID int64) (string, error) {
	if s == nil || s.repo == nil {
		return "", infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	return s.repo.ResetUserAffCode(ctx, userID)
}

// AdminSetUserRebateRate 设置/清除用户专属返利比例。ratePercent==nil 表示清除。
func (s *AffiliateService) AdminSetUserRebateRate(ctx context.Context, userID int64, ratePercent *float64) error {
	if s == nil || s.repo == nil {
		return infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	if err := validateExclusiveRate(ratePercent); err != nil {
		return err
	}
	return s.repo.SetUserRebateRate(ctx, userID, ratePercent)
}

func (s *AffiliateService) AdminSetUserInviteLimit(ctx context.Context, userID int64, limit *int) error {
	if s == nil || s.repo == nil {
		return infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	if err := validateInviteLimit(limit); err != nil {
		return err
	}
	return s.repo.SetUserInviteLimit(ctx, userID, limit)
}

// AdminBatchSetUserRebateRate 批量设置/清除用户专属返利比例。
func (s *AffiliateService) AdminBatchSetUserRebateRate(ctx context.Context, userIDs []int64, ratePercent *float64) error {
	if s == nil || s.repo == nil {
		return infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	if err := validateExclusiveRate(ratePercent); err != nil {
		return err
	}
	cleaned := make([]int64, 0, len(userIDs))
	for _, uid := range userIDs {
		if uid > 0 {
			cleaned = append(cleaned, uid)
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	return s.repo.BatchSetUserRebateRate(ctx, cleaned, ratePercent)
}

// AdminListCustomUsers 列出有专属配置的用户。
func (s *AffiliateService) AdminListCustomUsers(ctx context.Context, filter AffiliateAdminFilter) ([]AffiliateAdminEntry, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	return s.repo.ListUsersWithCustomSettings(ctx, filter)
}

func (s *AffiliateService) ListAffiliateLedgerRecords(ctx context.Context, userID int64, filter AffiliateRecordFilter) ([]AffiliateLedgerRecord, int64, error) {
	if userID <= 0 {
		return nil, 0, infraerrors.BadRequest("INVALID_USER", "invalid user")
	}
	if s == nil || s.repo == nil {
		return nil, 0, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	items, total, err := s.repo.ListAffiliateLedgerRecords(ctx, userID, normalizeAffiliateRecordFilter(filter))
	if err != nil {
		return nil, 0, err
	}
	for i := range items {
		items[i].SourceUserEmail = maskEmail(items[i].SourceUserEmail)
		items[i].SourceUsername = maskAccountIdentifier(items[i].SourceUsername)
	}
	return items, total, nil
}

func (s *AffiliateService) AdminListInviteRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateInviteRecord, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	return s.repo.ListAffiliateInviteRecords(ctx, normalizeAffiliateRecordFilter(filter))
}

func (s *AffiliateService) AdminListRebateRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateRebateRecord, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	return s.repo.ListAffiliateRebateRecords(ctx, normalizeAffiliateRecordFilter(filter))
}

func (s *AffiliateService) AdminListTransferRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateTransferRecord, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	return s.repo.ListAffiliateTransferRecords(ctx, normalizeAffiliateRecordFilter(filter))
}

func (s *AffiliateService) AdminGetUserOverview(ctx context.Context, userID int64) (*AffiliateUserOverview, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "invalid user")
	}
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	overview, err := s.repo.GetAffiliateUserOverview(ctx, userID)
	if err != nil {
		return nil, err
	}
	if overview != nil {
		if !overview.RebateRateCustom {
			inviter, err := s.repo.EnsureUserAffiliate(ctx, userID)
			if err != nil {
				return nil, err
			}
			overview.RebateRatePercent = s.membershipRebateRatePercent(ctx, inviter)
		}
		overview.RebateRatePercent = clampAffiliateRebateRate(overview.RebateRatePercent)
	}
	return overview, nil
}

func normalizeAffiliateRecordFilter(filter AffiliateRecordFilter) AffiliateRecordFilter {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	filter.Search = strings.TrimSpace(filter.Search)
	filter.SortBy = strings.TrimSpace(filter.SortBy)
	return filter
}
