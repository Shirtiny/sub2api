package service

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID             int64
	Email          string
	Username       string
	Notes          string
	AvatarURL      string
	AvatarSource   string
	AvatarMIME     string
	AvatarByteSize int
	AvatarSHA256   string
	PasswordHash   string
	Role           string
	Balance        float64
	Concurrency    int
	// PlanConcurrencyEntitlements are transient auth-time snapshots. They are
	// not stored on users; active subscription terms raise the effective limit
	// above Concurrency and automatically stop applying at their own expiration
	// time. They never lower it — see EffectiveConcurrencyAt.
	PlanConcurrencyEntitlements []PlanConcurrencyEntitlement
	Status                      string
	AllowedGroups               []int64
	TokenVersion                int64 // Incremented on password change to invalidate existing tokens
	// TokenVersionResolved indicates TokenVersion already contains the fingerprint-derived
	// value expected in JWT claims and refresh-token state.
	TokenVersionResolved bool
	SignupSource         string
	LastLoginAt          *time.Time
	LastActiveAt         *time.Time
	LastUsedAt           *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time // 非 nil 表示用户已软删除

	// GroupRates 用户专属分组倍率配置
	// map[groupID]rateMultiplier
	GroupRates map[int64]float64

	// TOTP 双因素认证字段
	TotpSecretEncrypted *string    // AES-256-GCM 加密的 TOTP 密钥
	TotpEnabled         bool       // 是否启用 TOTP
	TotpEnabledAt       *time.Time // TOTP 启用时间

	// 余额不足通知
	BalanceNotifyEnabled       bool
	BalanceNotifyThresholdType string // "fixed" (default) | "percentage"
	BalanceNotifyThreshold     *float64
	BalanceNotifyExtraEmails   []NotifyEmailEntry
	TotalRecharged             float64

	// RPMLimit 用户级每分钟请求数上限（0 = 不限制）。仅在所用分组未设置 rpm_limit
	// 且该 (用户, 分组) 无 rpm_override 时作为全局兜底生效，计数键 rpm:u:{userID}:{min}。
	RPMLimit int

	// UserGroupRPMOverride 来自 auth cache snapshot 的 (user, group) RPM 覆盖值。
	// nil = 该 API Key 对应的 (user, group) 无 override；非 nil 时 checkRPM 直接使用，
	// 避免每请求查 DB。字段不持久化到数据库。
	UserGroupRPMOverride         *int
	UserGroupRPMOverrideResolved bool

	APIKeys       []APIKey
	Subscriptions []UserSubscription
}

type PlanConcurrencyEntitlement struct {
	SubscriptionID int64     `json:"subscription_id,omitempty"`
	Concurrency    int       `json:"concurrency"`
	StartsAt       time.Time `json:"starts_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// EffectiveConcurrencyAt returns the highest of the user's persisted
// concurrency and every active plan entitlement. Plan entitlements are a floor,
// not a cap: admin adjustments and concurrency redeem codes write the persisted
// value, so a plan must never shrink a limit that was granted per user.
func (u *User) EffectiveConcurrencyAt(now time.Time) int {
	if u == nil {
		return 1
	}
	effective := 0
	type currentSubscriptionEntitlement struct {
		startsAt    time.Time
		concurrency int
	}
	currentBySubscription := make(map[int64]currentSubscriptionEntitlement)
	for _, entitlement := range u.PlanConcurrencyEntitlements {
		if entitlement.Concurrency <= 0 || now.Before(entitlement.StartsAt) || !now.Before(entitlement.ExpiresAt) {
			continue
		}
		if entitlement.SubscriptionID <= 0 {
			if entitlement.Concurrency > effective {
				effective = entitlement.Concurrency
			}
			continue
		}
		current, ok := currentBySubscription[entitlement.SubscriptionID]
		if !ok || entitlement.StartsAt.After(current.startsAt) ||
			(entitlement.StartsAt.Equal(current.startsAt) && entitlement.Concurrency > current.concurrency) {
			currentBySubscription[entitlement.SubscriptionID] = currentSubscriptionEntitlement{
				startsAt:    entitlement.StartsAt,
				concurrency: entitlement.Concurrency,
			}
		}
	}
	for _, entitlement := range currentBySubscription {
		if entitlement.concurrency > effective {
			effective = entitlement.concurrency
		}
	}
	if u.Concurrency > effective {
		effective = u.Concurrency
	}
	if effective > 0 {
		return effective
	}
	return 1
}

func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

func (u *User) IsActive() bool {
	return u.Status == StatusActive
}

// CanBindGroup checks whether a user can bind to a given group.
// For standard groups:
// - Public groups (non-exclusive): all users can bind
// - Exclusive groups: only users with the group in AllowedGroups can bind
func (u *User) CanBindGroup(groupID int64, isExclusive bool) bool {
	// 公开分组（非专属）：所有用户都可以绑定
	if !isExclusive {
		return true
	}
	// 专属分组：需要在 AllowedGroups 中
	for _, id := range u.AllowedGroups {
		if id == groupID {
			return true
		}
	}
	return false
}

func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	return nil
}

func (u *User) CheckPassword(password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil
}
