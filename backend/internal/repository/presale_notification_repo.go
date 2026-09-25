package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type presaleNotificationRepository struct{ db *sql.DB }

func NewPresaleNotificationRepository(db *sql.DB) service.PresaleNotificationRepository {
	return &presaleNotificationRepository{db: db}
}

func (r *presaleNotificationRepository) Recipients(ctx context.Context, includeRestricted bool) ([]service.PresaleNoticeRecipient, error) {
	// An existing account's access state takes precedence over its waitlist row.
	// Deleted accounts must not be reintroduced through historical waitlist data.
	rows, err := r.db.QueryContext(ctx, `
 SELECT MIN(user_id), email FROM (
   SELECT id AS user_id, lower(trim(email)) AS email FROM users
   WHERE deleted_at IS NULL AND (status = 'active' OR $1)
   UNION ALL
   SELECT 0 AS user_id, lower(trim(w.email)) AS email FROM waitlist_entries w
   WHERE (w.approved_at IS NOT NULL OR $1) AND NOT EXISTS (
     SELECT 1 FROM users u WHERE lower(trim(u.email)) = lower(trim(w.email))
   )
 ) recipients GROUP BY email ORDER BY email`, includeRestricted)
	if err != nil {
		return nil, fmt.Errorf("list presale notice recipients: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make([]service.PresaleNoticeRecipient, 0)
	for rows.Next() {
		var recipient service.PresaleNoticeRecipient
		if err := rows.Scan(&recipient.UserID, &recipient.Email); err != nil {
			return nil, err
		}
		result = append(result, recipient)
	}
	return result, rows.Err()
}

func (r *presaleNotificationRepository) States(ctx context.Context, campaign string) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT recipient_hash, status FROM presale_email_deliveries WHERE campaign_key=$1`, campaign)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make(map[string]string)
	for rows.Next() {
		var hash, status string
		if err := rows.Scan(&hash, &status); err != nil {
			return nil, err
		}
		result[hash] = status
	}
	return result, rows.Err()
}

func (r *presaleNotificationRepository) Claim(ctx context.Context, campaign, hash string, userID, adminID int64, version string, test bool) (bool, error) {
	// Formal notices are never automatically retried, including ambiguous sends.
	// Explicit test sends have a shared per-mailbox 60-second cooldown.
	result, err := r.db.ExecContext(ctx, `INSERT INTO presale_email_deliveries
 (campaign_key, recipient_hash, user_id, admin_id, status, preview_version)
 VALUES ($1,$2,$3,$4,'sending',$5)
 ON CONFLICT (campaign_key,recipient_hash) DO UPDATE SET
 status='sending', admin_id=EXCLUDED.admin_id, preview_version=EXCLUDED.preview_version, updated_at=NOW()
 WHERE $6 AND presale_email_deliveries.status <> 'sending' AND presale_email_deliveries.updated_at < NOW() - INTERVAL '60 seconds'`, campaign, hash, userID, adminID, version, test)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (r *presaleNotificationRepository) Finish(ctx context.Context, campaign, hash, status string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE presale_email_deliveries SET status=$3, updated_at=NOW()
 WHERE campaign_key=$1 AND recipient_hash=$2 AND status='sending'`, campaign, hash, status)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("presale notice claim is no longer active")
	}
	return nil
}
