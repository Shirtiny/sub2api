package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type UserSubscriptionRepository interface {
	Create(ctx context.Context, sub *UserSubscription) error
	GetByID(ctx context.Context, id int64) (*UserSubscription, error)
	GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*UserSubscription, error)
	GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*UserSubscription, error)
	Update(ctx context.Context, sub *UserSubscription) error
	Delete(ctx context.Context, id int64) error

	ListByUserID(ctx context.Context, userID int64) ([]UserSubscription, error)
	ListActiveByUserID(ctx context.Context, userID int64) ([]UserSubscription, error)
	ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error)
	List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status, platform, sortBy, sortOrder string) ([]UserSubscription, *pagination.PaginationResult, error)

	ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error)
	ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error
	UpdateStatus(ctx context.Context, subscriptionID int64, status string) error
	UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error

	ActivateWindows(ctx context.Context, id int64, start time.Time) error
	ResetUsageWindows(ctx context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, newWindowStart time.Time) error
	ResetDailyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error
	ResetWeeklyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error
	ResetMonthlyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error
	ResetActiveUsage(ctx context.Context, resetDaily, resetWeekly, resetMonthly bool, newWindowStart time.Time) (int64, error)
	ShiftUsageWindows(ctx context.Context, input ShiftWindowQuery) (ShiftWindowRows, error)
	IncrementUsage(ctx context.Context, id int64, costUSD float64) error

	ListUsageDaily(ctx context.Context, subscriptionID int64, from, to time.Time) ([]SubscriptionUsageDaily, error)

	BatchUpdateExpiredStatus(ctx context.Context) (int64, error)
}

type EarlyResetSubscriptionParams struct {
	ID                      int64
	UserID                  int64
	ExpectedExpiresAt       time.Time
	ExpectedCustomExpiresAt *time.Time
	NewExpiresAt            time.Time
	NewCustomExpiresAt      *time.Time
	WindowStart             time.Time
}

// ShiftWindowQuery 描述一次批量窗口平移的目标范围与位移量。
type ShiftWindowQuery struct {
	Daily    bool
	Weekly   bool
	Monthly  bool
	Offset   time.Duration
	DryRun   bool
	Status   string
	UserID   *int64
	GroupID  *int64
	Platform string
}

// ShiftWindowRow 是一条被平移条件命中的订阅。
type ShiftWindowRow struct {
	ID      int64
	UserID  int64
	GroupID int64
	// Future 表示平移后至少有一个目标窗口的起点会落到未来，该行整行跳过。
	Future bool
}

// ShiftWindowRows 汇总一次平移的命中明细与实际写库条数。
type ShiftWindowRows struct {
	Rows    []ShiftWindowRow
	Updated int64
}

// SubscriptionUsageDaily 是 subscription_usage_daily 的一行：订阅在某个自然日的用量与当日限额快照。
type SubscriptionUsageDaily struct {
	BucketDate      time.Time
	CostUSD         float64
	RequestCount    int64
	DailyLimitUSD   *float64
	WeeklyLimitUSD  *float64
	MonthlyLimitUSD *float64
}
