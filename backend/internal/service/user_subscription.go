package service

import "time"

type UserSubscription struct {
	ID      int64
	UserID  int64
	GroupID int64

	StartsAt  time.Time
	ExpiresAt time.Time
	Status    string

	DailyWindowStart   *time.Time
	WeeklyWindowStart  *time.Time
	MonthlyWindowStart *time.Time

	DailyUsageUSD   float64
	WeeklyUsageUSD  float64
	MonthlyUsageUSD float64
	// ResetCount is the number of user-initiated daily/weekly quota resets
	// remaining for this subscription.
	ResetCount             int
	EarlyResetEnabled      bool
	EarlyResetDurationDays int

	AssignedBy               *int64
	AssignedAt               time.Time
	Notes                    string
	PlanConcurrency          *int
	PlanConcurrencyExpiresAt *time.Time

	CustomMultiplier    *int
	CustomSourcePlanID  *int64
	CustomSourceGroupID *int64
	CustomExpiresAt     *time.Time
	CustomDisplayName   string

	CreatedAt time.Time
	UpdatedAt time.Time

	User           *User
	Group          *Group
	AssignedByUser *User
}

func (s *UserSubscription) ActivePlanConcurrencyEntitlementAt(now time.Time) (PlanConcurrencyEntitlement, bool) {
	if s == nil || s.PlanConcurrency == nil || *s.PlanConcurrency <= 0 || s.Status != SubscriptionStatusActive || now.Before(s.StartsAt) || !now.Before(s.ExpiresAt) {
		return PlanConcurrencyEntitlement{}, false
	}
	expiresAt := s.ExpiresAt
	if s.PlanConcurrencyExpiresAt != nil && s.PlanConcurrencyExpiresAt.Before(expiresAt) {
		expiresAt = *s.PlanConcurrencyExpiresAt
	}
	if !now.Before(expiresAt) {
		return PlanConcurrencyEntitlement{}, false
	}
	return PlanConcurrencyEntitlement{Concurrency: *s.PlanConcurrency, StartsAt: s.StartsAt, ExpiresAt: expiresAt}, true
}

func (s *UserSubscription) VirtualCustomMultiplier() int {
	if !s.HasActiveVirtualCustomEntitlementAt(time.Now()) {
		return 0
	}
	return *s.CustomMultiplier
}

func (s *UserSubscription) HasVirtualCustomEntitlement() bool {
	return s != nil && s.CustomMultiplier != nil && *s.CustomMultiplier >= 1 && s.CustomSourcePlanID != nil && s.CustomSourceGroupID != nil && s.CustomExpiresAt != nil
}

func (s *UserSubscription) HasActiveVirtualCustomEntitlementAt(now time.Time) bool {
	if !s.HasVirtualCustomEntitlement() {
		return false
	}
	return s.CustomExpiresAt == nil || s.CustomExpiresAt.After(now)
}

func (s *UserSubscription) IsVirtualCustomSubscription() bool {
	return s.HasActiveVirtualCustomEntitlementAt(time.Now())
}

func (s *UserSubscription) DisplayCustomMultiplier() int {
	if s == nil {
		return 0
	}
	if m := s.VirtualCustomMultiplier(); m >= 1 {
		return m
	}
	if s.Group != nil && s.Group.IsCustomSubscriptionGroup && s.Group.CustomMultiplier != nil && *s.Group.CustomMultiplier >= 1 {
		return *s.Group.CustomMultiplier
	}
	return 0
}

func (s *UserSubscription) DisplayCustomSourcePlanID() *int64 {
	if s == nil {
		return nil
	}
	if s.IsVirtualCustomSubscription() && s.CustomSourcePlanID != nil {
		return s.CustomSourcePlanID
	}
	if s.Group != nil && s.Group.IsCustomSubscriptionGroup {
		return s.Group.CustomSourcePlanID
	}
	return nil
}

func (s *UserSubscription) DisplayCustomSourceGroupID() *int64 {
	if s == nil {
		return nil
	}
	if s.IsVirtualCustomSubscription() && s.CustomSourceGroupID != nil {
		return s.CustomSourceGroupID
	}
	if s.Group != nil && s.Group.IsCustomSubscriptionGroup {
		return s.Group.CustomSourceGroupID
	}
	return nil
}

func (s *UserSubscription) DisplayName(group *Group) string {
	if s != nil && s.IsVirtualCustomSubscription() {
		groupName := ""
		if group != nil {
			groupName = group.Name
		}
		return customSubscriptionGroupName(groupName, s.VirtualCustomMultiplier())
	}
	if group != nil {
		return group.Name
	}
	return ""
}

func EffectiveSubscriptionGroup(sub *UserSubscription, group *Group) *Group {
	if group == nil {
		return nil
	}
	if sub == nil || !sub.IsVirtualCustomSubscription() || group.IsCustomSubscriptionGroup {
		return group
	}
	multiplier := float64(sub.VirtualCustomMultiplier())
	if multiplier <= 0 {
		return group
	}
	cp := *group
	cp.DailyLimitUSD = multiplyOptionalLimit(group.DailyLimitUSD, multiplier)
	cp.WeeklyLimitUSD = multiplyOptionalLimit(group.WeeklyLimitUSD, multiplier)
	cp.MonthlyLimitUSD = multiplyOptionalLimit(group.MonthlyLimitUSD, multiplier)
	if name := sub.DisplayName(group); name != "" {
		cp.Name = name
	}
	return &cp
}

func (s *UserSubscription) IsActive() bool {
	return s.Status == SubscriptionStatusActive && time.Now().Before(s.ExpiresAt)
}

func (s *UserSubscription) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

func (s *UserSubscription) DaysRemaining() int {
	if s.IsExpired() {
		return 0
	}
	return int(time.Until(s.ExpiresAt).Hours() / 24)
}

func (s *UserSubscription) IsWindowActivated() bool {
	return s.DailyWindowStart != nil || s.WeeklyWindowStart != nil || s.MonthlyWindowStart != nil
}

func (s *UserSubscription) HasOneTimeDailyQuota() bool {
	if s == nil || s.StartsAt.IsZero() || s.ExpiresAt.IsZero() {
		return false
	}
	return !s.ExpiresAt.After(s.StartsAt.AddDate(0, 0, 1))
}

// HasLimitedQuota reports whether quota is a one-time allowance for the
// subscription term. These plans can only refresh quota through an explicit
// early reset; extending the expiry must not create another quota allowance.
func (s *UserSubscription) HasLimitedQuota() bool {
	return s != nil && s.EarlyResetEnabled
}

func (s *UserSubscription) quotaExpiresAt() time.Time {
	if s == nil {
		return time.Time{}
	}
	if s.HasActiveVirtualCustomEntitlementAt(time.Now()) && s.CustomExpiresAt.Before(s.ExpiresAt) {
		return *s.CustomExpiresAt
	}
	return s.ExpiresAt
}

func (s *UserSubscription) NeedsDailyReset() bool {
	return s.NeedsDailyResetAt(time.Now())
}

func (s *UserSubscription) NeedsDailyResetAt(now time.Time) bool {
	if s.DailyWindowStart == nil {
		return false
	}
	if s.HasLimitedQuota() || s.HasOneTimeDailyQuota() {
		return false
	}
	return !now.Before(s.DailyWindowStart.Add(24 * time.Hour))
}

func (s *UserSubscription) NeedsWeeklyReset() bool {
	if s.WeeklyWindowStart == nil || s.HasLimitedQuota() {
		return false
	}
	return time.Since(*s.WeeklyWindowStart) >= 7*24*time.Hour
}

func (s *UserSubscription) NeedsMonthlyReset() bool {
	if s.MonthlyWindowStart == nil || s.HasLimitedQuota() {
		return false
	}
	return time.Since(*s.MonthlyWindowStart) >= 30*24*time.Hour
}

func (s *UserSubscription) DailyResetTime() *time.Time {
	if s.DailyWindowStart == nil {
		return nil
	}
	if s.HasLimitedQuota() || s.HasOneTimeDailyQuota() {
		t := s.quotaExpiresAt()
		return &t
	}
	t := s.DailyWindowStart.Add(24 * time.Hour)
	return &t
}

func (s *UserSubscription) WeeklyResetTime() *time.Time {
	if s.WeeklyWindowStart == nil {
		return nil
	}
	if s.HasLimitedQuota() {
		t := s.quotaExpiresAt()
		return &t
	}
	t := s.WeeklyWindowStart.Add(7 * 24 * time.Hour)
	return &t
}

func (s *UserSubscription) MonthlyResetTime() *time.Time {
	if s.MonthlyWindowStart == nil {
		return nil
	}
	if s.HasLimitedQuota() {
		t := s.quotaExpiresAt()
		return &t
	}
	t := s.MonthlyWindowStart.Add(30 * 24 * time.Hour)
	return &t
}

func (s *UserSubscription) CheckDailyLimit(group *Group, additionalCost float64) bool {
	group = EffectiveSubscriptionGroup(s, group)
	if group == nil || !group.HasDailyLimit() {
		return true
	}
	return s.DailyUsageUSD+additionalCost <= *group.DailyLimitUSD
}

func (s *UserSubscription) CheckWeeklyLimit(group *Group, additionalCost float64) bool {
	group = EffectiveSubscriptionGroup(s, group)
	if group == nil || !group.HasWeeklyLimit() {
		return true
	}
	return s.WeeklyUsageUSD+additionalCost <= *group.WeeklyLimitUSD
}

func (s *UserSubscription) CheckMonthlyLimit(group *Group, additionalCost float64) bool {
	group = EffectiveSubscriptionGroup(s, group)
	if group == nil || !group.HasMonthlyLimit() {
		return true
	}
	return s.MonthlyUsageUSD+additionalCost <= *group.MonthlyLimitUSD
}

func (s *UserSubscription) CheckAllLimits(group *Group, additionalCost float64) (daily, weekly, monthly bool) {
	daily = s.CheckDailyLimit(group, additionalCost)
	weekly = s.CheckWeeklyLimit(group, additionalCost)
	monthly = s.CheckMonthlyLimit(group, additionalCost)
	return
}
