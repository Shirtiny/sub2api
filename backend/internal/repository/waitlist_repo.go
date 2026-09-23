package repository

import (
	"context"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/waitlistentry"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type waitlistRepository struct{ client *dbent.Client }

func NewWaitlistRepository(client *dbent.Client) service.WaitlistRepository {
	return &waitlistRepository{client: client}
}

func (r *waitlistRepository) Join(ctx context.Context, email string) error {
	err := r.client.WaitlistEntry.Create().SetEmail(email).OnConflictColumns(waitlistentry.FieldEmail).DoNothing().Exec(ctx)
	if isSQLNoRowsError(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("insert waitlist entry: %w", err)
	}
	return nil
}

func (r *waitlistRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.WaitlistEntry, int64, error) {
	count, err := r.client.WaitlistEntry.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count waitlist: %w", err)
	}
	entries, err := r.client.WaitlistEntry.Query().Order(dbent.Desc(waitlistentry.FieldID)).Offset(params.Offset()).Limit(params.Limit()).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list waitlist: %w", err)
	}
	result := make([]service.WaitlistEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, service.WaitlistEntry{ID: entry.ID, Email: entry.Email, CreatedAt: entry.CreatedAt})
	}
	return result, int64(count), nil
}

// ClaimConfirmation returns false only after a previously successful delivery.
// An in-progress claim is retryable, never misreported as an already sent email.
func (r *waitlistRepository) ClaimConfirmation(ctx context.Context, email string, attempt time.Time) (bool, error) {
	affected, err := r.client.WaitlistEntry.Update().Where(
		waitlistentry.EmailEQ(email), waitlistentry.ConfirmationSentAtIsNil(),
		waitlistentry.Or(waitlistentry.ConfirmationAttemptedAtIsNil(), waitlistentry.ConfirmationAttemptedAtLT(attempt.Add(-service.WaitlistConfirmationLease))),
	).SetConfirmationAttemptedAt(attempt).Save(ctx)
	if err != nil {
		return false, fmt.Errorf("claim waitlist confirmation: %w", err)
	}
	if affected == 1 {
		return true, nil
	}
	sent, err := r.client.WaitlistEntry.Query().Where(waitlistentry.EmailEQ(email), waitlistentry.ConfirmationSentAtNotNil()).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("check waitlist confirmation: %w", err)
	}
	if sent {
		return false, nil
	}
	return false, service.ErrWaitlistConfirmationFailed
}

func (r *waitlistRepository) FinishConfirmation(ctx context.Context, email string, attempt time.Time, sent bool) error {
	update := r.client.WaitlistEntry.Update().Where(waitlistentry.EmailEQ(email), waitlistentry.ConfirmationAttemptedAtEQ(attempt), waitlistentry.ConfirmationSentAtIsNil())
	if sent {
		update.SetConfirmationSentAt(time.Now().UTC())
	} else {
		update.ClearConfirmationAttemptedAt()
	}
	affected, err := update.Save(ctx)
	if err != nil {
		return fmt.Errorf("finish waitlist confirmation: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("waitlist confirmation claim lost")
	}
	return nil
}
