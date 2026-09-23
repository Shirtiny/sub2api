package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
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
		result = append(result, waitlistEntryDTO(entry))
	}
	return result, int64(count), nil
}

func waitlistEntryDTO(entry *dbent.WaitlistEntry) service.WaitlistEntry {
	return service.WaitlistEntry{
		ID: entry.ID, Email: entry.Email, CreatedAt: entry.CreatedAt,
		ApprovedAt: entry.ApprovedAt, ApprovedBy: entry.ApprovedBy,
		GrantedUserID: entry.GrantedUserID, ApprovalNoticeSentAt: entry.ApprovalNoticeSentAt,
	}
}

func (r *waitlistRepository) Approve(ctx context.Context, id, adminID int64) (*service.WaitlistEntry, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin waitlist approval: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	entry, err := tx.WaitlistEntry.Query().Where(waitlistentry.IDEQ(id)).ForUpdate().Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrWaitlistNotFound, nil)
	}
	if entry.ApprovedAt == nil {
		// Use the same normalized mailbox as signup. Never alter credentials,
		// balances, roles or subscriptions, or restore a soft-deleted account.
		users, err := tx.User.Query().Where(userEmailLookupPredicate(entry.Email), dbuser.DeletedAtIsNil()).Limit(2).ForUpdate().All(ctx)
		if err != nil {
			return nil, fmt.Errorf("find waitlist account: %w", err)
		}
		if len(users) > 1 {
			return nil, service.ErrWaitlistAccountConflict
		}
		update := tx.WaitlistEntry.UpdateOneID(id).SetApprovedAt(time.Now().UTC()).SetApprovedBy(adminID)
		if len(users) == 1 {
			affected, err := tx.User.Update().Where(dbuser.IDEQ(users[0].ID), dbuser.DeletedAtIsNil()).SetStatus(service.StatusActive).Save(ctx)
			if err != nil {
				return nil, fmt.Errorf("enable waitlist account: %w", err)
			}
			if affected != 1 {
				return nil, service.ErrWaitlistAccountConflict
			}
			update.SetGrantedUserID(users[0].ID)
		}
		entry, err = update.Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("save waitlist approval: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit waitlist approval: %w", err)
	}
	result := waitlistEntryDTO(entry)
	return &result, nil
}

func (r *waitlistRepository) HasRegistrationApproval(ctx context.Context, email string) (bool, error) {
	return r.client.WaitlistEntry.Query().Where(
		waitlistentry.EmailEQ(strings.ToLower(strings.TrimSpace(email))),
		waitlistentry.ApprovedAtNotNil(), waitlistentry.GrantedUserIDIsNil(),
	).Exist(ctx)
}

// A grant can only be consumed inside the same transaction that creates the user.
func (r *waitlistRepository) ConsumeRegistrationApproval(ctx context.Context, email string, userID int64) error {
	tx := dbent.TxFromContext(ctx)
	if tx == nil || userID <= 0 {
		return fmt.Errorf("consume waitlist approval requires a registration transaction")
	}
	affected, err := tx.WaitlistEntry.Update().Where(
		waitlistentry.EmailEQ(strings.ToLower(strings.TrimSpace(email))),
		waitlistentry.ApprovedAtNotNil(), waitlistentry.GrantedUserIDIsNil(),
	).SetGrantedUserID(userID).Save(ctx)
	if err != nil {
		return fmt.Errorf("consume waitlist approval: %w", err)
	}
	if affected != 1 {
		return service.ErrRegDisabled
	}
	return nil
}

func (r *waitlistRepository) ClaimApprovalNotice(ctx context.Context, id int64, attempt time.Time) (bool, error) {
	affected, err := r.client.WaitlistEntry.Update().Where(
		waitlistentry.IDEQ(id), waitlistentry.ApprovedAtNotNil(), waitlistentry.ApprovalNoticeSentAtIsNil(),
		waitlistentry.Or(waitlistentry.ApprovalNoticeAttemptedAtIsNil(), waitlistentry.ApprovalNoticeAttemptedAtLT(attempt.Add(-service.WaitlistConfirmationLease))),
	).SetApprovalNoticeAttemptedAt(attempt).Save(ctx)
	if err != nil {
		return false, fmt.Errorf("claim waitlist approval notice: %w", err)
	}
	if affected == 1 {
		return true, nil
	}
	sent, err := r.client.WaitlistEntry.Query().Where(waitlistentry.IDEQ(id), waitlistentry.ApprovalNoticeSentAtNotNil()).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("check waitlist approval notice: %w", err)
	}
	if sent {
		return false, nil
	}
	return false, service.ErrWaitlistApprovalNoticeFailed
}

func (r *waitlistRepository) FinishApprovalNotice(ctx context.Context, id int64, attempt time.Time, sent bool) error {
	update := r.client.WaitlistEntry.Update().Where(waitlistentry.IDEQ(id), waitlistentry.ApprovalNoticeAttemptedAtEQ(attempt), waitlistentry.ApprovalNoticeSentAtIsNil())
	if sent {
		update.SetApprovalNoticeSentAt(time.Now().UTC())
	} else {
		update.ClearApprovalNoticeAttemptedAt()
	}
	affected, err := update.Save(ctx)
	if err != nil {
		return fmt.Errorf("finish waitlist approval notice: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("waitlist approval notice claim lost")
	}
	return nil
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
