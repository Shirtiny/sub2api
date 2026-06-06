package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const (
	affiliateCodeLength      = 12
	affiliateCodeMaxAttempts = 12
)

var affiliateCodeCharset = []byte("ABCDEFGHJKLMNPQRSTUVWXYZ23456789")

const affiliateUserOverviewSQL = `
SELECT ua.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ua.aff_code,
       COALESCE(ua.aff_rebate_rate_percent, 0)::double precision,
       (ua.aff_rebate_rate_percent IS NOT NULL) AS has_custom_rate,
       ua.aff_count,
       COALESCE(rebated.rebated_invitee_count, 0),
       (ua.aff_quota + COALESCE(matured.matured_frozen_quota, 0))::double precision,
       ua.aff_history_quota::double precision
FROM user_affiliates ua
JOIN users u ON u.id = ua.user_id
LEFT JOIN (
    SELECT user_id, COUNT(DISTINCT source_user_id)::integer AS rebated_invitee_count
    FROM user_affiliate_ledger
    WHERE action = 'accrue' AND source_user_id IS NOT NULL
    GROUP BY user_id
) rebated ON rebated.user_id = ua.user_id
LEFT JOIN (
    SELECT user_id, COALESCE(SUM(amount), 0)::double precision AS matured_frozen_quota
    FROM user_affiliate_ledger
    WHERE action = 'accrue' AND frozen_until IS NOT NULL AND frozen_until <= NOW()
    GROUP BY user_id
) matured ON matured.user_id = ua.user_id
WHERE ua.user_id = $1
LIMIT 1`

type affiliateQueryExecer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type affiliateRepository struct {
	client *dbent.Client
}

func NewAffiliateRepository(client *dbent.Client, _ *sql.DB) service.AffiliateRepository {
	return &affiliateRepository{client: client}
}

func (r *affiliateRepository) EnsureUserAffiliate(ctx context.Context, userID int64) (*service.AffiliateSummary, error) {
	if userID <= 0 {
		return nil, service.ErrUserNotFound
	}
	client := clientFromContext(ctx, r.client)
	return ensureUserAffiliateWithClient(ctx, client, userID)
}

func (r *affiliateRepository) GetAffiliateByCode(ctx context.Context, code string) (*service.AffiliateSummary, error) {
	client := clientFromContext(ctx, r.client)
	return queryAffiliateByCode(ctx, client, code)
}

func (r *affiliateRepository) BindInviter(ctx context.Context, userID, inviterID int64, inviteLimit int) (bool, error) {
	if inviteLimit <= 0 {
		return false, service.ErrAffiliateInviteLimitReached
	}

	var bound bool
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, inviterID); err != nil {
			return err
		}

		res, err := txClient.ExecContext(txCtx,
			"UPDATE user_affiliates SET inviter_id = $1, updated_at = NOW() WHERE user_id = $2 AND inviter_id IS NULL",
			inviterID, userID,
		)
		if err != nil {
			return fmt.Errorf("bind inviter: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			bound = false
			return nil
		}

		res, err = txClient.ExecContext(txCtx,
			"UPDATE user_affiliates SET aff_count = aff_count + 1, updated_at = NOW() WHERE user_id = $1 AND aff_count < $2",
			inviterID, inviteLimit,
		)
		if err != nil {
			return fmt.Errorf("increment inviter aff_count: %w", err)
		}
		affected, _ = res.RowsAffected()
		if affected == 0 {
			return service.ErrAffiliateInviteLimitReached
		}
		bound = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return bound, nil
}

func (r *affiliateRepository) AccrueQuota(ctx context.Context, inviterID, inviteeUserID int64, amount float64, freezeHours int, sourceOrderID *int64) (bool, error) {
	if amount <= 0 {
		return false, nil
	}

	var applied bool
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		// freezeHours > 0: add to frozen quota; == 0: add to available quota directly
		var updateSQL string
		if freezeHours > 0 {
			updateSQL = "UPDATE user_affiliates SET aff_frozen_quota = aff_frozen_quota + $1, aff_history_quota = aff_history_quota + $1, updated_at = NOW() WHERE user_id = $2"
		} else {
			updateSQL = "UPDATE user_affiliates SET aff_quota = aff_quota + $1, aff_history_quota = aff_history_quota + $1, updated_at = NOW() WHERE user_id = $2"
		}
		res, err := txClient.ExecContext(txCtx, updateSQL, amount, inviterID)
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			applied = false
			return nil
		}

		if freezeHours > 0 {
			if _, err = txClient.ExecContext(txCtx, `
INSERT INTO user_affiliate_ledger (user_id, action, amount, source_user_id, source_order_id, frozen_until, created_at, updated_at)
VALUES ($1, 'accrue', $2, $3, $4, NOW() + make_interval(hours => $5), NOW(), NOW())`,
				inviterID, amount, inviteeUserID, nullableInt64Arg(sourceOrderID), freezeHours); err != nil {
				return fmt.Errorf("insert affiliate accrue ledger: %w", err)
			}
		} else {
			if _, err = txClient.ExecContext(txCtx, `
INSERT INTO user_affiliate_ledger (user_id, action, amount, source_user_id, source_order_id, created_at, updated_at)
VALUES ($1, 'accrue', $2, $3, $4, NOW(), NOW())`, inviterID, amount, inviteeUserID, nullableInt64Arg(sourceOrderID)); err != nil {
				return fmt.Errorf("insert affiliate accrue ledger: %w", err)
			}
		}

		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}

func (r *affiliateRepository) AccrueSubscriptionRebate(ctx context.Context, inviterID, inviteeUserID, groupID int64, days, freezeHours int, sourceOrderID *int64) (bool, error) {
	if days <= 0 || inviterID <= 0 || inviteeUserID <= 0 || groupID <= 0 {
		return false, nil
	}

	var applied bool
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		var rows *sql.Rows
		var err error
		if freezeHours > 0 {
			rows, err = txClient.QueryContext(txCtx, `
	INSERT INTO user_affiliate_ledger (user_id, action, amount, source_user_id, source_order_id, subscription_group_id, frozen_until, created_at, updated_at)
	SELECT $1, 'accrue_subscription', $2, $3, $4, $5, NOW() + make_interval(hours => $6), NOW(), NOW()
	WHERE NOT EXISTS (
		SELECT 1
		FROM user_affiliate_ledger
		WHERE action = 'accrue_subscription'
		  AND user_id = $1
		  AND source_user_id = $3
		  AND source_order_id IS NOT DISTINCT FROM $4
		  AND subscription_group_id = $5
	)
	RETURNING id`, inviterID, days, inviteeUserID, nullableInt64Arg(sourceOrderID), groupID, freezeHours)
		} else {
			rows, err = txClient.QueryContext(txCtx, `
	INSERT INTO user_affiliate_ledger (user_id, action, amount, source_user_id, source_order_id, subscription_group_id, created_at, updated_at)
	SELECT $1, 'accrue_subscription', $2, $3, $4, $5, NOW(), NOW()
	WHERE NOT EXISTS (
		SELECT 1
		FROM user_affiliate_ledger
		WHERE action = 'accrue_subscription'
		  AND user_id = $1
		  AND source_user_id = $3
		  AND source_order_id IS NOT DISTINCT FROM $4
		  AND subscription_group_id = $5
	)
	RETURNING id`, inviterID, days, inviteeUserID, nullableInt64Arg(sourceOrderID), groupID)
		}
		if err != nil {
			return fmt.Errorf("insert affiliate subscription accrue ledger: %w", err)
		}
		defer func() { _ = rows.Close() }()
		if rows.Next() {
			applied = true
		}
		return rows.Err()
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}

func (r *affiliateRepository) GetAccruedRebateFromInvitee(ctx context.Context, inviterID, inviteeUserID int64) (float64, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx,
		`SELECT COALESCE(SUM(amount), 0)::double precision FROM user_affiliate_ledger WHERE user_id = $1 AND source_user_id = $2 AND action = 'accrue'`,
		inviterID, inviteeUserID)
	if err != nil {
		return 0, fmt.Errorf("query accrued rebate from invitee: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var total float64
	if rows.Next() {
		if err := rows.Scan(&total); err != nil {
			return 0, err
		}
	}
	return total, rows.Close()
}

func (r *affiliateRepository) ThawFrozenQuota(ctx context.Context, userID int64) (float64, error) {
	var thawed float64
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		var err error
		thawed, err = thawFrozenQuotaTx(txCtx, txClient, userID)
		return err
	})
	return thawed, err
}

// thawFrozenQuotaTx moves matured frozen quota to available quota within an existing tx.
func thawFrozenQuotaTx(txCtx context.Context, txClient *dbent.Client, userID int64) (float64, error) {
	rows, err := txClient.QueryContext(txCtx, `
WITH matured AS (
    UPDATE user_affiliate_ledger
    SET frozen_until = NULL, updated_at = NOW()
    WHERE user_id = $1
      AND action = 'accrue'
      AND frozen_until IS NOT NULL
      AND frozen_until <= NOW()
    RETURNING amount
)
SELECT COALESCE(SUM(amount), 0) FROM matured`, userID)
	if err != nil {
		return 0, fmt.Errorf("thaw frozen quota: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var thawed float64
	if rows.Next() {
		if err := rows.Scan(&thawed); err != nil {
			return 0, err
		}
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if thawed <= 0 {
		return 0, nil
	}

	_, err = txClient.ExecContext(txCtx, `
UPDATE user_affiliates
SET aff_quota = aff_quota + $1,
    aff_frozen_quota = GREATEST(aff_frozen_quota - $1, 0),
    updated_at = NOW()
WHERE user_id = $2`, thawed, userID)
	if err != nil {
		return 0, fmt.Errorf("move thawed quota: %w", err)
	}
	return thawed, nil
}

func (r *affiliateRepository) TransferQuotaToBalance(ctx context.Context, userID int64, points float64, balanceMultiplier float64) (*service.AffiliateBalanceRedeemResult, error) {
	var redeemedPoints float64
	var creditedBalance float64
	var newBalance float64
	balanceMultiplier = normalizeAffiliateBalanceMultiplier(balanceMultiplier)
	points = roundAffiliateAmount(points)

	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}

		// Thaw any matured frozen point rows before redemption.
		if _, err := thawFrozenQuotaTx(txCtx, txClient, userID); err != nil {
			return fmt.Errorf("thaw before transfer: %w", err)
		}

		rows, err := txClient.QueryContext(txCtx, `
	SELECT aff_quota::double precision
	FROM user_affiliates
	WHERE user_id = $1
	FOR UPDATE`, userID)
		if err != nil {
			return fmt.Errorf("lock rebate amount: %w", err)
		}
		var available float64
		if rows.Next() {
			if err := rows.Scan(&available); err != nil {
				_ = rows.Close()
				return err
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if available <= 0 {
			return service.ErrAffiliateQuotaEmpty
		}

		redeemedPoints = points
		if redeemedPoints <= 0 {
			redeemedPoints = available
		}
		redeemedPoints = roundAffiliateAmount(redeemedPoints)
		if redeemedPoints <= 0 {
			return service.ErrAffiliateQuotaEmpty
		}
		if available+1e-8 < redeemedPoints {
			return service.ErrAffiliateQuotaInsufficient
		}

		res, err := txClient.ExecContext(txCtx, `
	UPDATE user_affiliates
	SET aff_quota = GREATEST(aff_quota - $1, 0),
	    updated_at = NOW()
	WHERE user_id = $2
	  AND aff_quota + 0.00000001 >= $1`, redeemedPoints, userID)
		if err != nil {
			return fmt.Errorf("deduct rebate amount: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrAffiliateQuotaInsufficient
		}

		creditedBalance = roundAffiliateAmount(redeemedPoints * balanceMultiplier)
		if creditedBalance <= 0 {
			return service.ErrAffiliateQuotaEmpty
		}

		affectedUsers, err := txClient.User.Update().
			Where(user.IDEQ(userID)).
			AddBalance(creditedBalance).
			Save(txCtx)
		if err != nil {
			return fmt.Errorf("credit user balance by rebate amount: %w", err)
		}
		if affectedUsers == 0 {
			return service.ErrUserNotFound
		}

		newBalance, err = queryUserBalance(txCtx, txClient, userID)
		if err != nil {
			return err
		}

		snapshot, err := queryAffiliateTransferSnapshot(txCtx, txClient, userID)
		if err != nil {
			return err
		}

		if _, err = txClient.ExecContext(txCtx, `
	INSERT INTO user_affiliate_ledger (
	    user_id,
	    action,
	    amount,
	    source_user_id,
	    balance_after,
	    aff_quota_after,
	    aff_frozen_quota_after,
	    aff_history_quota_after,
	    created_at,
	    updated_at
	)
	VALUES ($1, 'transfer', $2, NULL, $3, $4, $5, $6, NOW(), NOW())`,
			userID,
			redeemedPoints,
			snapshot.BalanceAfter,
			snapshot.AvailableQuotaAfter,
			snapshot.FrozenQuotaAfter,
			snapshot.HistoryQuotaAfter,
		); err != nil {
			return fmt.Errorf("insert affiliate transfer ledger: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &service.AffiliateBalanceRedeemResult{
		RedeemedPoints:  redeemedPoints,
		CreditedBalance: creditedBalance,
		Balance:         newBalance,
	}, nil
}

func (r *affiliateRepository) TransferQuotaToSubscription(ctx context.Context, userID, groupID, planID int64, points float64) (*service.AffiliateSubscriptionTransferResult, error) {
	if userID <= 0 || groupID <= 0 || planID <= 0 {
		return nil, service.ErrAffiliateQuotaEmpty
	}

	result := &service.AffiliateSubscriptionTransferResult{GroupID: groupID}
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		if _, err := thawFrozenQuotaTx(txCtx, txClient, userID); err != nil {
			return fmt.Errorf("thaw before subscription transfer: %w", err)
		}

		var available float64
		rows, err := txClient.QueryContext(txCtx, `
	SELECT aff_quota::double precision
	FROM user_affiliates
	WHERE user_id = $1
	FOR UPDATE`, userID)
		if err != nil {
			return fmt.Errorf("lock rebate amount: %w", err)
		}
		if rows.Next() {
			if err := rows.Scan(&available); err != nil {
				_ = rows.Close()
				return err
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if available <= 0 {
			return service.ErrAffiliateQuotaEmpty
		}

		var planPrice float64
		var validityDays int
		planRows, err := txClient.QueryContext(txCtx, `
	SELECT p.price::double precision,
	       p.validity_days,
	       COALESCE(g.name, '')
	FROM subscription_plans p
	JOIN groups g ON g.id = p.group_id AND g.deleted_at IS NULL
	WHERE p.id = $1
	  AND p.group_id = $2
	  AND p.for_sale = true
	  AND p.price > 0
	  AND p.validity_days > 0
	  AND g.status = 'active'
	  AND g.subscription_type = $3
	LIMIT 1`, planID, groupID, service.SubscriptionTypeSubscription)
		if err != nil {
			return fmt.Errorf("query affiliate subscription target: %w", err)
		}
		if planRows.Next() {
			if err := planRows.Scan(&planPrice, &validityDays, &result.GroupName); err != nil {
				_ = planRows.Close()
				return err
			}
		}
		if err := planRows.Close(); err != nil {
			return err
		}
		if planPrice <= 0 || math.IsNaN(planPrice) || math.IsInf(planPrice, 0) || validityDays <= 0 {
			return service.ErrSubscriptionNotFound
		}

		redeemedPoints := roundAffiliateAmount(planPrice)
		if redeemedPoints <= 0 {
			return service.ErrAffiliateQuotaEmpty
		}
		if available+1e-8 < redeemedPoints {
			return service.ErrAffiliateQuotaInsufficient
		}
		days := float64(validityDays)

		res, err := txClient.ExecContext(txCtx, `
	UPDATE user_affiliates
	SET aff_quota = GREATEST(aff_quota - $1, 0),
	    updated_at = NOW()
	WHERE user_id = $2
	  AND aff_quota + 0.00000001 >= $1`, redeemedPoints, userID)
		if err != nil {
			return fmt.Errorf("deduct rebate amount for subscription: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrAffiliateQuotaInsufficient
		}

		var newExpiresAt time.Time
		extendRows, err := txClient.QueryContext(txCtx, `
	WITH updated AS (
		UPDATE user_subscriptions
		SET starts_at = CASE
				WHEN expires_at <= NOW() THEN NOW()
				ELSE starts_at
			END,
			expires_at = CASE
				WHEN expires_at > NOW() THEN expires_at + ($3 * INTERVAL '1 day')
				ELSE NOW() + ($3 * INTERVAL '1 day')
			END,
			status = 'active',
			updated_at = NOW(),
			notes = CASE
				WHEN COALESCE(notes, '') = '' THEN 'affiliate point subscription redemption'
				ELSE notes || E'\n' || 'affiliate point subscription redemption'
			END
		WHERE user_id = $1
		  AND group_id = $2
		  AND deleted_at IS NULL
		RETURNING expires_at
	), inserted AS (
		INSERT INTO user_subscriptions (
			user_id,
			group_id,
			starts_at,
			expires_at,
			status,
			assigned_at,
			notes,
			created_at,
			updated_at
		)
		SELECT $1, $2, NOW(), NOW() + ($3 * INTERVAL '1 day'), 'active', NOW(), 'affiliate point subscription redemption', NOW(), NOW()
		WHERE NOT EXISTS (SELECT 1 FROM updated)
		RETURNING expires_at
	)
	SELECT expires_at FROM updated
	UNION ALL
	SELECT expires_at FROM inserted
	LIMIT 1`, userID, groupID, days)
		if err != nil {
			return fmt.Errorf("extend subscription by rebate amount: %w", err)
		}
		if !extendRows.Next() {
			_ = extendRows.Close()
			return service.ErrSubscriptionNotFound
		}
		if err := extendRows.Scan(&newExpiresAt); err != nil {
			_ = extendRows.Close()
			return err
		}
		if err := extendRows.Close(); err != nil {
			return err
		}

		snapshot, err := queryAffiliateTransferSnapshot(txCtx, txClient, userID)
		if err != nil {
			return err
		}
		if _, err = txClient.ExecContext(txCtx, `
	INSERT INTO user_affiliate_ledger (
	    user_id,
	    action,
	    amount,
	    source_user_id,
	    subscription_group_id,
	    transferred_at,
	    balance_after,
	    aff_quota_after,
	    aff_frozen_quota_after,
	    aff_history_quota_after,
	    created_at,
	    updated_at
	)
	VALUES ($1, 'transfer_subscription', $2, NULL, $3, NOW(), $4, $5, $6, $7, NOW(), NOW())`,
			userID,
			redeemedPoints,
			groupID,
			snapshot.BalanceAfter,
			snapshot.AvailableQuotaAfter,
			snapshot.FrozenQuotaAfter,
			snapshot.HistoryQuotaAfter,
		); err != nil {
			return fmt.Errorf("insert affiliate subscription point transfer ledger: %w", err)
		}

		result.TransferredDays = roundAffiliateAmount(days)
		result.RedeemedPoints = redeemedPoints
		result.ExpiresAt = &newExpiresAt
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *affiliateRepository) TransferSubscriptionRebateToSubscription(ctx context.Context, userID, groupID int64) (*service.AffiliateSubscriptionTransferResult, error) {
	if userID <= 0 || groupID <= 0 {
		return nil, service.ErrAffiliateSubscriptionQuotaEmpty
	}

	result := &service.AffiliateSubscriptionTransferResult{GroupID: groupID}
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}

		groupRows, err := txClient.QueryContext(txCtx, `SELECT COALESCE(name, '') FROM groups WHERE id = $1 AND deleted_at IS NULL LIMIT 1`, groupID)
		if err != nil {
			return fmt.Errorf("query subscription rebate group: %w", err)
		}
		if groupRows.Next() {
			if err := groupRows.Scan(&result.GroupName); err != nil {
				_ = groupRows.Close()
				return err
			}
		}
		if err := groupRows.Close(); err != nil {
			return err
		}

		rows, err := txClient.QueryContext(txCtx, `
	WITH claimed AS (
		UPDATE user_affiliate_ledger
		SET transferred_at = NOW(), updated_at = NOW()
		WHERE user_id = $1
		  AND subscription_group_id = $2
		  AND action = 'accrue_subscription'
		  AND transferred_at IS NULL
		  AND (frozen_until IS NULL OR frozen_until <= NOW())
		RETURNING amount
	)
	SELECT COALESCE(SUM(amount), 0)::integer FROM claimed`, userID, groupID)
		if err != nil {
			return fmt.Errorf("claim affiliate subscription rebate: %w", err)
		}
		if rows.Next() {
			if err := rows.Scan(&result.TransferredDays); err != nil {
				_ = rows.Close()
				return err
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if result.TransferredDays <= 0 {
			return service.ErrAffiliateSubscriptionQuotaEmpty
		}

		var expiresAt time.Time
		expiresRows, err := txClient.QueryContext(txCtx, `
	INSERT INTO user_subscriptions (
		user_id,
		group_id,
		starts_at,
		expires_at,
		status,
		assigned_at,
		notes,
		created_at,
		updated_at
	)
	VALUES ($1, $2, NOW(), NOW() + ($3 || ' days')::interval, 'active', NOW(), 'affiliate subscription rebate transfer', NOW(), NOW())
	ON CONFLICT (user_id, group_id) DO UPDATE
	SET starts_at = CASE
			WHEN user_subscriptions.expires_at <= NOW() OR user_subscriptions.deleted_at IS NOT NULL THEN NOW()
			ELSE user_subscriptions.starts_at
		END,
		expires_at = CASE
			WHEN user_subscriptions.expires_at > NOW() AND user_subscriptions.deleted_at IS NULL THEN user_subscriptions.expires_at + ($3 || ' days')::interval
			ELSE NOW() + ($3 || ' days')::interval
		END,
		status = 'active',
		deleted_at = NULL,
		updated_at = NOW(),
		notes = CASE
			WHEN COALESCE(user_subscriptions.notes, '') = '' THEN 'affiliate subscription rebate transfer'
			ELSE user_subscriptions.notes || E'\n' || 'affiliate subscription rebate transfer'
		END
	RETURNING expires_at`, userID, groupID, result.TransferredDays)
		if err != nil {
			return fmt.Errorf("apply affiliate subscription rebate: %w", err)
		}
		if !expiresRows.Next() {
			_ = expiresRows.Close()
			if err := expiresRows.Err(); err != nil {
				return fmt.Errorf("apply affiliate subscription rebate: %w", err)
			}
			return fmt.Errorf("apply affiliate subscription rebate: no subscription returned")
		}
		if err := expiresRows.Scan(&expiresAt); err != nil {
			_ = expiresRows.Close()
			return fmt.Errorf("apply affiliate subscription rebate: %w", err)
		}
		if err := expiresRows.Close(); err != nil {
			return fmt.Errorf("apply affiliate subscription rebate: %w", err)
		}
		result.ExpiresAt = &expiresAt

		if _, err = txClient.ExecContext(txCtx, `
	INSERT INTO user_affiliate_ledger (user_id, action, amount, source_user_id, subscription_group_id, transferred_at, created_at, updated_at)
	VALUES ($1, 'transfer_subscription', $2, NULL, $3, NOW(), NOW(), NOW())`, userID, result.TransferredDays, groupID); err != nil {
			return fmt.Errorf("insert affiliate subscription transfer ledger: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *affiliateRepository) ListSubscriptionRebateBalances(ctx context.Context, userID int64) ([]service.AffiliateSubscriptionRebateBalance, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
	SELECT ual.subscription_group_id,
	       COALESCE(g.name, ''),
	       COALESCE(SUM(CASE WHEN ual.transferred_at IS NULL AND (ual.frozen_until IS NULL OR ual.frozen_until <= NOW()) THEN ual.amount ELSE 0 END), 0)::integer AS available_days,
	       COALESCE(SUM(CASE WHEN ual.transferred_at IS NULL AND ual.frozen_until > NOW() THEN ual.amount ELSE 0 END), 0)::integer AS frozen_days
	FROM user_affiliate_ledger ual
	LEFT JOIN groups g ON g.id = ual.subscription_group_id
	WHERE ual.user_id = $1
	  AND ual.action = 'accrue_subscription'
	  AND ual.subscription_group_id IS NOT NULL
	GROUP BY ual.subscription_group_id, g.name
	HAVING COALESCE(SUM(CASE WHEN ual.transferred_at IS NULL THEN ual.amount ELSE 0 END), 0) > 0
	ORDER BY g.name, ual.subscription_group_id`, userID)
	if err != nil {
		return nil, fmt.Errorf("query affiliate subscription rebate balances: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AffiliateSubscriptionRebateBalance, 0)
	for rows.Next() {
		var item service.AffiliateSubscriptionRebateBalance
		if err := rows.Scan(&item.GroupID, &item.GroupName, &item.AvailableDays, &item.FrozenDays); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *affiliateRepository) ListInvitees(ctx context.Context, inviterID int64, limit int) ([]service.AffiliateInvitee, error) {
	if limit <= 0 {
		limit = 100
	}
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
SELECT ua.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ua.created_at,
       COALESCE(SUM(CASE WHEN ual.action = 'accrue' THEN ual.amount ELSE 0 END), 0)::double precision AS total_rebate,
       0::integer AS total_subscription_rebate_days
FROM user_affiliates ua
LEFT JOIN users u ON u.id = ua.user_id
LEFT JOIN user_affiliate_ledger ual
       ON ual.user_id = $1
      AND ual.source_user_id = ua.user_id
      AND ual.action = 'accrue'
WHERE ua.inviter_id = $1
GROUP BY ua.user_id, u.email, u.username, ua.created_at
ORDER BY ua.created_at DESC
LIMIT $2`, inviterID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	invitees := make([]service.AffiliateInvitee, 0)
	for rows.Next() {
		var item service.AffiliateInvitee
		var createdAt time.Time
		if err := rows.Scan(&item.UserID, &item.Email, &item.Username, &createdAt, &item.TotalRebate, &item.TotalSubscriptionRebateDays); err != nil {
			return nil, err
		}
		item.CreatedAt = &createdAt
		item.TotalRebatePoints = item.TotalRebate
		invitees = append(invitees, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return invitees, nil
}

func (r *affiliateRepository) ListAffiliateInviteRecords(ctx context.Context, filter service.AffiliateRecordFilter) ([]service.AffiliateInviteRecord, int64, error) {
	client := clientFromContext(ctx, r.client)
	where, args := buildAffiliateRecordWhere(filter, "ua.created_at", []string{
		"inviter.email", "inviter.username", "invitee.email", "invitee.username",
		"ua.inviter_id::text", "ua.user_id::text", "inviter_aff.aff_code",
	})

	total, err := queryAffiliateRecordCount(ctx, client, `
SELECT COUNT(*)
FROM user_affiliates ua
JOIN users invitee ON invitee.id = ua.user_id
JOIN users inviter ON inviter.id = ua.inviter_id
JOIN user_affiliates inviter_aff ON inviter_aff.user_id = ua.inviter_id
`+where, args...)
	if err != nil {
		return nil, 0, err
	}

	orderBy := buildAffiliateRecordOrderBy(filter, map[string]string{
		"inviter":      "inviter.email",
		"invitee":      "invitee.email",
		"aff_code":     "inviter_aff.aff_code",
		"total_rebate": "total_rebate",
		"created_at":   "ua.created_at",
	}, "ua.created_at")
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := client.QueryContext(ctx, `
SELECT ua.inviter_id,
       COALESCE(inviter.email, ''),
       COALESCE(inviter.username, ''),
       ua.user_id,
       COALESCE(invitee.email, ''),
       COALESCE(invitee.username, ''),
       COALESCE(inviter_aff.aff_code, ''),
       COALESCE(SUM(CASE WHEN ual.action = 'accrue' THEN ual.amount ELSE 0 END), 0)::double precision AS total_rebate,
       0::integer AS total_subscription_rebate_days,
       ua.created_at
FROM user_affiliates ua
JOIN users invitee ON invitee.id = ua.user_id
JOIN users inviter ON inviter.id = ua.inviter_id
JOIN user_affiliates inviter_aff ON inviter_aff.user_id = ua.inviter_id
LEFT JOIN user_affiliate_ledger ual
       ON ual.user_id = ua.inviter_id
      AND ual.source_user_id = ua.user_id
      AND ual.action = 'accrue'
`+where+`
GROUP BY ua.inviter_id, inviter.email, inviter.username, ua.user_id, invitee.email, invitee.username, inviter_aff.aff_code, ua.created_at
`+orderBy+`
LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AffiliateInviteRecord, 0)
	for rows.Next() {
		var item service.AffiliateInviteRecord
		if err := rows.Scan(
			&item.InviterID,
			&item.InviterEmail,
			&item.InviterUsername,
			&item.InviteeID,
			&item.InviteeEmail,
			&item.InviteeUsername,
			&item.AffCode,
			&item.TotalRebate,
			&item.TotalSubscriptionRebateDays,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		item.TotalRebatePoints = item.TotalRebate
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *affiliateRepository) ListAffiliateLedgerRecords(ctx context.Context, userID int64, filter service.AffiliateRecordFilter) ([]service.AffiliateLedgerRecord, int64, error) {
	client := clientFromContext(ctx, r.client)
	where, args := buildAffiliateRecordWhere(filter, "ual.created_at", []string{
		"ual.action", "ual.source_order_id::text", "po.out_trade_no", "source.email", "source.username",
	})
	args = append(args, userID)
	userArg := len(args)
	baseJoin := fmt.Sprintf(`
FROM user_affiliate_ledger ual
LEFT JOIN users source ON source.id = ual.source_user_id
LEFT JOIN payment_orders po ON po.id = ual.source_order_id
LEFT JOIN groups g ON g.id = ual.subscription_group_id
WHERE ual.user_id = $%d
  AND ual.action IN ('accrue', 'transfer', 'transfer_subscription')`, userArg)
	if where != "" {
		where = strings.Replace(where, "WHERE ", " AND ", 1)
	}

	total, err := queryAffiliateRecordCount(ctx, client, "SELECT COUNT(*) "+baseJoin+where, args...)
	if err != nil {
		return nil, 0, err
	}

	orderBy := buildAffiliateRecordOrderBy(filter, map[string]string{
		"action":                 "ual.action",
		"amount":                 "ual.amount",
		"available_points_after": "ual.aff_quota_after",
		"frozen_points_after":    "ual.aff_frozen_quota_after",
		"history_points_after":   "ual.aff_history_quota_after",
		"created_at":             "ual.created_at",
	}, "ual.created_at")
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := client.QueryContext(ctx, `
SELECT ual.id,
       ual.action,
       ual.amount::double precision,
       ual.source_user_id,
       COALESCE(source.email, ''),
       COALESCE(source.username, ''),
       ual.source_order_id,
       COALESCE(po.out_trade_no, ''),
       ual.subscription_group_id,
       COALESCE(g.name, ''),
       ual.balance_after::double precision,
       ual.aff_quota_after::double precision,
       ual.aff_frozen_quota_after::double precision,
       ual.aff_history_quota_after::double precision,
       ual.frozen_until,
       ual.transferred_at,
       ual.created_at
`+baseJoin+where+`
`+orderBy+`
LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AffiliateLedgerRecord, 0)
	for rows.Next() {
		var item service.AffiliateLedgerRecord
		var sourceUserID sql.NullInt64
		var sourceOrderID sql.NullInt64
		var groupID sql.NullInt64
		var balanceAfter sql.NullFloat64
		var availablePointsAfter sql.NullFloat64
		var frozenPointsAfter sql.NullFloat64
		var historyPointsAfter sql.NullFloat64
		var frozenUntil pq.NullTime
		var transferredAt pq.NullTime
		if err := rows.Scan(
			&item.LedgerID,
			&item.Action,
			&item.Amount,
			&sourceUserID,
			&item.SourceUserEmail,
			&item.SourceUsername,
			&sourceOrderID,
			&item.OutTradeNo,
			&groupID,
			&item.SubscriptionGroupName,
			&balanceAfter,
			&availablePointsAfter,
			&frozenPointsAfter,
			&historyPointsAfter,
			&frozenUntil,
			&transferredAt,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		if sourceUserID.Valid {
			item.SourceUserID = &sourceUserID.Int64
		}
		if sourceOrderID.Valid {
			item.SourceOrderID = &sourceOrderID.Int64
		}
		if groupID.Valid {
			item.SubscriptionGroupID = &groupID.Int64
		}
		item.BalanceAfter = nullableFloat64Ptr(balanceAfter)
		item.AvailablePointsAfter = nullableFloat64Ptr(availablePointsAfter)
		item.FrozenPointsAfter = nullableFloat64Ptr(frozenPointsAfter)
		item.HistoryPointsAfter = nullableFloat64Ptr(historyPointsAfter)
		if frozenUntil.Valid {
			item.FrozenUntil = &frozenUntil.Time
		}
		if transferredAt.Valid {
			item.TransferredAt = &transferredAt.Time
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *affiliateRepository) ListAffiliateRebateRecords(ctx context.Context, filter service.AffiliateRecordFilter) ([]service.AffiliateRebateRecord, int64, error) {
	client := clientFromContext(ctx, r.client)
	where, args := buildAffiliateRecordWhere(filter, "ual.created_at", []string{
		"inviter.email", "inviter.username", "invitee.email", "invitee.username",
		"po.id::text", "po.out_trade_no", "po.payment_type", "po.status",
	})
	baseJoin := `
FROM user_affiliate_ledger ual
JOIN payment_orders po ON po.id = ual.source_order_id
JOIN users invitee ON invitee.id = ual.source_user_id
JOIN users inviter ON inviter.id = ual.user_id
LEFT JOIN groups g ON g.id = ual.subscription_group_id
WHERE ual.action = 'accrue'
  AND ual.source_order_id IS NOT NULL`
	if where != "" {
		where = strings.Replace(where, "WHERE ", " AND ", 1)
	}

	total, err := queryAffiliateRecordCount(ctx, client, "SELECT COUNT(*) "+baseJoin+where, args...)
	if err != nil {
		return nil, 0, err
	}

	orderBy := buildAffiliateRecordOrderBy(filter, map[string]string{
		"order":         "po.id",
		"inviter":       "inviter.email",
		"invitee":       "invitee.email",
		"order_amount":  "po.amount",
		"pay_amount":    "po.pay_amount",
		"rebate_amount": "ual.amount",
		"payment_type":  "po.payment_type",
		"order_status":  "po.status",
		"created_at":    "ual.created_at",
	}, "ual.created_at")
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := client.QueryContext(ctx, `
SELECT po.id,
       po.out_trade_no,
       ual.user_id,
       COALESCE(inviter.email, ''),
       COALESCE(inviter.username, ''),
       ual.source_user_id,
       COALESCE(invitee.email, ''),
       COALESCE(invitee.username, ''),
       po.amount::double precision,
       po.pay_amount::double precision,
       ual.amount::double precision,
       ual.action,
       NULL::bigint,
       '',
       0::integer,
       po.payment_type,
       po.status,
       ual.created_at
`+baseJoin+where+`
`+orderBy+`
LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AffiliateRebateRecord, 0)
	for rows.Next() {
		var item service.AffiliateRebateRecord
		var groupID sql.NullInt64
		if err := rows.Scan(
			&item.OrderID,
			&item.OutTradeNo,
			&item.InviterID,
			&item.InviterEmail,
			&item.InviterUsername,
			&item.InviteeID,
			&item.InviteeEmail,
			&item.InviteeUsername,
			&item.OrderAmount,
			&item.PayAmount,
			&item.RebateAmount,
			&item.RebateAction,
			&groupID,
			&item.SubscriptionGroupName,
			&item.SubscriptionRebateDays,
			&item.PaymentType,
			&item.OrderStatus,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		if groupID.Valid {
			item.SubscriptionGroupID = &groupID.Int64
		}
		item.RebatePoints = item.RebateAmount
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *affiliateRepository) ListAffiliateTransferRecords(ctx context.Context, filter service.AffiliateRecordFilter) ([]service.AffiliateTransferRecord, int64, error) {
	client := clientFromContext(ctx, r.client)
	where, args := buildAffiliateRecordWhere(filter, "ual.created_at", []string{
		"u.email", "u.username", "u.id::text",
	})
	baseJoin := `
FROM user_affiliate_ledger ual
JOIN users u ON u.id = ual.user_id
LEFT JOIN groups g ON g.id = ual.subscription_group_id
WHERE ual.action IN ('transfer', 'transfer_subscription')`
	if where != "" {
		where = strings.Replace(where, "WHERE ", " AND ", 1)
	}

	total, err := queryAffiliateRecordCount(ctx, client, "SELECT COUNT(*) "+baseJoin+where, args...)
	if err != nil {
		return nil, 0, err
	}

	orderBy := buildAffiliateRecordOrderBy(filter, map[string]string{
		"user":                  "u.email",
		"action":                "ual.action",
		"amount":                "ual.amount",
		"balance_after":         "ual.balance_after",
		"available_quota_after": "ual.aff_quota_after",
		"frozen_quota_after":    "ual.aff_frozen_quota_after",
		"history_quota_after":   "ual.aff_history_quota_after",
		"created_at":            "ual.created_at",
	}, "ual.created_at")
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := client.QueryContext(ctx, `
SELECT ual.id,
       ual.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ual.action,
       ual.amount::double precision,
       ual.subscription_group_id,
       COALESCE(g.name, ''),
       ual.balance_after::double precision,
       ual.aff_quota_after::double precision,
       ual.aff_frozen_quota_after::double precision,
       ual.aff_history_quota_after::double precision,
       ual.created_at
`+baseJoin+where+`
`+orderBy+`
LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AffiliateTransferRecord, 0)
	for rows.Next() {
		var item service.AffiliateTransferRecord
		var balanceAfter sql.NullFloat64
		var availableQuotaAfter sql.NullFloat64
		var frozenQuotaAfter sql.NullFloat64
		var historyQuotaAfter sql.NullFloat64
		var groupID sql.NullInt64
		if err := rows.Scan(
			&item.LedgerID,
			&item.UserID,
			&item.UserEmail,
			&item.Username,
			&item.Action,
			&item.Amount,
			&groupID,
			&item.SubscriptionGroupName,
			&balanceAfter,
			&availableQuotaAfter,
			&frozenQuotaAfter,
			&historyQuotaAfter,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		if groupID.Valid {
			item.SubscriptionGroupID = &groupID.Int64
		}
		item.BalanceAfter = nullableFloat64Ptr(balanceAfter)
		item.AvailableQuotaAfter = nullableFloat64Ptr(availableQuotaAfter)
		item.AvailablePointsAfter = item.AvailableQuotaAfter
		item.FrozenQuotaAfter = nullableFloat64Ptr(frozenQuotaAfter)
		item.FrozenPointsAfter = item.FrozenQuotaAfter
		item.HistoryQuotaAfter = nullableFloat64Ptr(historyQuotaAfter)
		item.HistoryPointsAfter = item.HistoryQuotaAfter
		item.SnapshotAvailable = balanceAfter.Valid &&
			availableQuotaAfter.Valid &&
			frozenQuotaAfter.Valid &&
			historyQuotaAfter.Valid
		item.RedeemedPoints = item.Amount
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *affiliateRepository) GetAffiliateUserOverview(ctx context.Context, userID int64) (*service.AffiliateUserOverview, error) {
	if userID <= 0 {
		return nil, service.ErrUserNotFound
	}
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, affiliateUserOverviewSQL, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrUserNotFound
	}

	var overview service.AffiliateUserOverview
	var customRate float64
	var hasCustomRate bool
	if err := rows.Scan(
		&overview.UserID,
		&overview.Email,
		&overview.Username,
		&overview.AffCode,
		&customRate,
		&hasCustomRate,
		&overview.InvitedCount,
		&overview.RebatedInviteeCount,
		&overview.AvailableQuota,
		&overview.HistoryQuota,
	); err != nil {
		return nil, err
	}
	if hasCustomRate {
		overview.RebateRatePercent = customRate
		overview.RebateRateCustom = true
	}
	overview.AvailableRebatePoints = overview.AvailableQuota
	overview.TotalRebatePoints = overview.HistoryQuota
	return &overview, rows.Err()
}

func buildAffiliateRecordWhere(filter service.AffiliateRecordFilter, timeColumn string, searchColumns []string) (string, []any) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if filter.StartAt != nil {
		args = append(args, *filter.StartAt)
		clauses = append(clauses, fmt.Sprintf("%s >= $%d", timeColumn, len(args)))
	}
	if filter.EndAt != nil {
		args = append(args, *filter.EndAt)
		clauses = append(clauses, fmt.Sprintf("%s <= $%d", timeColumn, len(args)))
	}
	search := strings.TrimSpace(filter.Search)
	if search != "" && len(searchColumns) > 0 {
		args = append(args, "%"+strings.ToLower(search)+"%")
		parts := make([]string, 0, len(searchColumns))
		for _, col := range searchColumns {
			parts = append(parts, fmt.Sprintf("LOWER(%s) LIKE $%d", col, len(args)))
		}
		clauses = append(clauses, "("+strings.Join(parts, " OR ")+")")
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func buildAffiliateRecordOrderBy(filter service.AffiliateRecordFilter, sortColumns map[string]string, fallbackColumn string) string {
	column := sortColumns[filter.SortBy]
	if column == "" {
		column = fallbackColumn
	}
	direction := "DESC"
	if !filter.SortDesc {
		direction = "ASC"
	}
	return "ORDER BY " + column + " " + direction + " NULLS LAST"
}

func queryAffiliateRecordCount(ctx context.Context, client affiliateQueryExecer, query string, args ...any) (int64, error) {
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return 0, rows.Err()
	}
	var total int64
	if err := rows.Scan(&total); err != nil {
		return 0, err
	}
	return total, rows.Err()
}

func (r *affiliateRepository) withTx(ctx context.Context, fn func(txCtx context.Context, txClient *dbent.Client) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx, tx.Client())
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin affiliate transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx, tx.Client()); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit affiliate transaction: %w", err)
	}
	return nil
}

func ensureUserAffiliateWithClient(ctx context.Context, client affiliateQueryExecer, userID int64) (*service.AffiliateSummary, error) {
	summary, err := queryAffiliateByUserID(ctx, client, userID)
	if err == nil {
		return summary, nil
	}
	if !errors.Is(err, service.ErrAffiliateProfileNotFound) {
		return nil, err
	}

	for i := 0; i < affiliateCodeMaxAttempts; i++ {
		code, codeErr := generateAffiliateCode()
		if codeErr != nil {
			return nil, codeErr
		}
		_, insertErr := client.ExecContext(ctx, `
INSERT INTO user_affiliates (user_id, aff_code, created_at, updated_at)
VALUES ($1, $2, NOW(), NOW())
ON CONFLICT (user_id) DO NOTHING`, userID, code)
		if insertErr == nil {
			break
		}
		if isAffiliateUniqueViolation(insertErr) {
			continue
		}
		return nil, insertErr
	}

	return queryAffiliateByUserID(ctx, client, userID)
}

func queryAffiliateByUserID(ctx context.Context, client affiliateQueryExecer, userID int64) (*service.AffiliateSummary, error) {
	rows, err := client.QueryContext(ctx, `
SELECT ua.user_id,
       ua.aff_code,
       ua.aff_code_custom,
       ua.aff_rebate_rate_percent,
       ua.aff_invite_limit,
       ua.inviter_id,
       ua.aff_count,
       ua.aff_quota::double precision,
       ua.aff_frozen_quota::double precision,
       ua.aff_history_quota::double precision,
       u.total_recharged::double precision,
       ua.created_at,
       ua.updated_at
FROM user_affiliates ua
JOIN users u ON u.id = ua.user_id
WHERE ua.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAffiliateProfileNotFound
	}

	var out service.AffiliateSummary
	var inviterID sql.NullInt64
	var rebateRate sql.NullFloat64
	var inviteLimit sql.NullInt64
	if err := rows.Scan(
		&out.UserID,
		&out.AffCode,
		&out.AffCodeCustom,
		&rebateRate,
		&inviteLimit,
		&inviterID,
		&out.AffCount,
		&out.AffQuota,
		&out.AffFrozenQuota,
		&out.AffHistoryQuota,
		&out.TotalRecharged,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if inviterID.Valid {
		out.InviterID = &inviterID.Int64
	}
	if rebateRate.Valid {
		v := rebateRate.Float64
		out.AffRebateRatePercent = &v
	}
	if inviteLimit.Valid {
		v := int(inviteLimit.Int64)
		out.AffInviteLimit = &v
	}
	return &out, nil
}

func queryAffiliateByCode(ctx context.Context, client affiliateQueryExecer, code string) (*service.AffiliateSummary, error) {
	rows, err := client.QueryContext(ctx, `
SELECT ua.user_id,
       ua.aff_code,
       ua.aff_code_custom,
       ua.aff_rebate_rate_percent,
       ua.aff_invite_limit,
       ua.inviter_id,
       ua.aff_count,
       ua.aff_quota::double precision,
       ua.aff_frozen_quota::double precision,
       ua.aff_history_quota::double precision,
       u.total_recharged::double precision,
       ua.created_at,
       ua.updated_at
FROM user_affiliates ua
JOIN users u ON u.id = ua.user_id
WHERE ua.aff_code = $1
LIMIT 1`, strings.ToUpper(strings.TrimSpace(code)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAffiliateProfileNotFound
	}

	var out service.AffiliateSummary
	var inviterID sql.NullInt64
	var rebateRate sql.NullFloat64
	var inviteLimit sql.NullInt64
	if err := rows.Scan(
		&out.UserID,
		&out.AffCode,
		&out.AffCodeCustom,
		&rebateRate,
		&inviteLimit,
		&inviterID,
		&out.AffCount,
		&out.AffQuota,
		&out.AffFrozenQuota,
		&out.AffHistoryQuota,
		&out.TotalRecharged,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if inviterID.Valid {
		out.InviterID = &inviterID.Int64
	}
	if rebateRate.Valid {
		v := rebateRate.Float64
		out.AffRebateRatePercent = &v
	}
	if inviteLimit.Valid {
		v := int(inviteLimit.Int64)
		out.AffInviteLimit = &v
	}
	return &out, nil
}

func queryUserBalance(ctx context.Context, client affiliateQueryExecer, userID int64) (float64, error) {
	rows, err := client.QueryContext(ctx,
		"SELECT balance::double precision FROM users WHERE id = $1 LIMIT 1",
		userID,
	)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, service.ErrUserNotFound
	}
	var balance float64
	if err := rows.Scan(&balance); err != nil {
		return 0, err
	}
	return balance, nil
}

type affiliateTransferSnapshot struct {
	BalanceAfter        float64
	AvailableQuotaAfter float64
	FrozenQuotaAfter    float64
	HistoryQuotaAfter   float64
}

func queryAffiliateTransferSnapshot(ctx context.Context, client affiliateQueryExecer, userID int64) (*affiliateTransferSnapshot, error) {
	rows, err := client.QueryContext(ctx, `
SELECT u.balance::double precision,
       ua.aff_quota::double precision,
       ua.aff_frozen_quota::double precision,
       ua.aff_history_quota::double precision
FROM users u
JOIN user_affiliates ua ON ua.user_id = u.id
WHERE u.id = $1
LIMIT 1`, userID)
	if err != nil {
		return nil, fmt.Errorf("query affiliate transfer snapshot: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrUserNotFound
	}

	var snapshot affiliateTransferSnapshot
	if err := rows.Scan(
		&snapshot.BalanceAfter,
		&snapshot.AvailableQuotaAfter,
		&snapshot.FrozenQuotaAfter,
		&snapshot.HistoryQuotaAfter,
	); err != nil {
		return nil, err
	}
	return &snapshot, rows.Err()
}

func normalizeAffiliateBalanceMultiplier(multiplier float64) float64 {
	if multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return 1
	}
	return multiplier
}

func roundAffiliateAmount(v float64) float64 {
	return math.Round(v*1e8) / 1e8
}

func nullableFloat64Ptr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func generateAffiliateCode() (string, error) {
	buf := make([]byte, affiliateCodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate affiliate code: %w", err)
	}
	for i := range buf {
		buf[i] = affiliateCodeCharset[int(buf[i])%len(affiliateCodeCharset)]
	}
	return string(buf), nil
}

func isAffiliateUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == "23505"
	}
	return false
}

// UpdateUserAffCode 改写用户的邀请码（自定义专属邀请码）。
// 唯一性冲突返回 ErrAffiliateCodeTaken。
func (r *affiliateRepository) UpdateUserAffCode(ctx context.Context, userID int64, newCode string) error {
	if userID <= 0 {
		return service.ErrUserNotFound
	}
	code := strings.ToUpper(strings.TrimSpace(newCode))
	if code == "" {
		return service.ErrAffiliateCodeInvalid
	}

	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		res, err := txClient.ExecContext(txCtx, `
UPDATE user_affiliates
SET aff_code = $1,
    aff_code_custom = true,
    updated_at = NOW()
WHERE user_id = $2`, code, userID)
		if err != nil {
			if isAffiliateUniqueViolation(err) {
				return service.ErrAffiliateCodeTaken
			}
			return fmt.Errorf("update aff_code: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrUserNotFound
		}
		return nil
	})
}

// ResetUserAffCode 把 aff_code 还原为系统随机码，并清除 aff_code_custom 标记。
func (r *affiliateRepository) ResetUserAffCode(ctx context.Context, userID int64) (string, error) {
	if userID <= 0 {
		return "", service.ErrUserNotFound
	}
	var newCode string
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		for i := 0; i < affiliateCodeMaxAttempts; i++ {
			candidate, codeErr := generateAffiliateCode()
			if codeErr != nil {
				return codeErr
			}
			res, err := txClient.ExecContext(txCtx, `
UPDATE user_affiliates
SET aff_code = $1,
    aff_code_custom = false,
    updated_at = NOW()
WHERE user_id = $2`, candidate, userID)
			if err != nil {
				if isAffiliateUniqueViolation(err) {
					continue
				}
				return fmt.Errorf("reset aff_code: %w", err)
			}
			affected, _ := res.RowsAffected()
			if affected == 0 {
				return service.ErrUserNotFound
			}
			newCode = candidate
			return nil
		}
		return fmt.Errorf("reset aff_code: exhausted attempts")
	})
	if err != nil {
		return "", err
	}
	return newCode, nil
}

// SetUserRebateRate 设置或清除用户专属返利比例。ratePercent==nil 表示清除（沿用全局）。
func (r *affiliateRepository) SetUserRebateRate(ctx context.Context, userID int64, ratePercent *float64) error {
	if userID <= 0 {
		return service.ErrUserNotFound
	}
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		// nullableArg lets us use a single UPDATE for both "set value" and
		// "clear" cases — database/sql converts nil interface{} to SQL NULL.
		res, err := txClient.ExecContext(txCtx, `
UPDATE user_affiliates
SET aff_rebate_rate_percent = $1,
    updated_at = NOW()
WHERE user_id = $2`, nullableArg(ratePercent), userID)
		if err != nil {
			return fmt.Errorf("set aff_rebate_rate_percent: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrUserNotFound
		}
		return nil
	})
}

func (r *affiliateRepository) SetUserInviteLimit(ctx context.Context, userID int64, limit *int) error {
	if userID <= 0 {
		return service.ErrUserNotFound
	}
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		res, err := txClient.ExecContext(txCtx, `
UPDATE user_affiliates
SET aff_invite_limit = $1,
    updated_at = NOW()
WHERE user_id = $2`, nullableIntArg(limit), userID)
		if err != nil {
			return fmt.Errorf("set aff_invite_limit: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrUserNotFound
		}
		return nil
	})
}

// BatchSetUserRebateRate 批量为多个用户设置专属比例（nil 清除）。
func (r *affiliateRepository) BatchSetUserRebateRate(ctx context.Context, userIDs []int64, ratePercent *float64) error {
	if len(userIDs) == 0 {
		return nil
	}
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		for _, uid := range userIDs {
			if uid <= 0 {
				continue
			}
			if _, err := ensureUserAffiliateWithClient(txCtx, txClient, uid); err != nil {
				return err
			}
		}
		_, err := txClient.ExecContext(txCtx, `
UPDATE user_affiliates
SET aff_rebate_rate_percent = $1,
    updated_at = NOW()
WHERE user_id = ANY($2)`, nullableArg(ratePercent), pq.Array(userIDs))
		if err != nil {
			return fmt.Errorf("batch set aff_rebate_rate_percent: %w", err)
		}
		return nil
	})
}

// nullableArg unwraps a *float64 into an interface{} suitable for SQL parameter
// binding: nil pointer → SQL NULL, non-nil → the float value.
func nullableArg(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableIntArg(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableInt64Arg(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

// ListUsersWithCustomSettings 列出有专属配置（自定义码或专属比例）的用户。
//
// 单一查询同时处理"无搜索"与"按邮箱/用户名模糊搜索"：
// 空 search 时拼接出的 LIKE 模式为 "%%"，匹配所有行；非空时按 ILIKE 子串匹配。
// 这避免了为两种情况维护两份 SQL 模板。
func (r *affiliateRepository) ListUsersWithCustomSettings(ctx context.Context, filter service.AffiliateAdminFilter) ([]service.AffiliateAdminEntry, int64, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	likePattern := "%" + strings.TrimSpace(filter.Search) + "%"

	const baseFrom = `
FROM user_affiliates ua
JOIN users u ON u.id = ua.user_id
WHERE (ua.aff_code_custom = true OR ua.aff_rebate_rate_percent IS NOT NULL OR ua.aff_invite_limit IS NOT NULL)
  AND (u.email ILIKE $1 OR u.username ILIKE $1)`

	client := clientFromContext(ctx, r.client)

	total, err := scanInt64(ctx, client, "SELECT COUNT(*)"+baseFrom, likePattern)
	if err != nil {
		return nil, 0, fmt.Errorf("count affiliate admin entries: %w", err)
	}

	listQuery := `
SELECT ua.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ua.aff_code,
       ua.aff_code_custom,
       ua.aff_rebate_rate_percent,
       ua.aff_invite_limit,
       ua.aff_count` + baseFrom + `
ORDER BY ua.updated_at DESC
LIMIT $2 OFFSET $3`

	rows, err := client.QueryContext(ctx, listQuery, likePattern, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list affiliate admin entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	entries := make([]service.AffiliateAdminEntry, 0)
	for rows.Next() {
		var e service.AffiliateAdminEntry
		var rebate sql.NullFloat64
		var inviteLimit sql.NullInt64
		if err := rows.Scan(&e.UserID, &e.Email, &e.Username, &e.AffCode,
			&e.AffCodeCustom, &rebate, &inviteLimit, &e.AffCount); err != nil {
			return nil, 0, err
		}
		if rebate.Valid {
			v := rebate.Float64
			e.AffRebateRatePercent = &v
		}
		if inviteLimit.Valid {
			v := int(inviteLimit.Int64)
			e.AffInviteLimit = &v
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

// scanInt64 runs a query expected to return a single int64 column (e.g. COUNT).
func scanInt64(ctx context.Context, client affiliateQueryExecer, query string, args ...any) (int64, error) {
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, nil
	}
	var v int64
	if err := rows.Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}
