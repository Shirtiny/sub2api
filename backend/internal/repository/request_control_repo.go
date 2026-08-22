package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type requestControlRepository struct {
	db *sql.DB
}

func NewRequestControlRepository(db *sql.DB) service.RequestControlRepository {
	return &requestControlRepository{db: db}
}

func (r *requestControlRepository) CreateLog(ctx context.Context, log *service.RequestControlLog) error {
	if r == nil || r.db == nil || log == nil {
		return nil
	}
	details, err := json.Marshal(log.Details)
	if err != nil {
		return fmt.Errorf("marshal request control details: %w", err)
	}
	var userID, apiKeyID, groupID any
	if log.UserID != nil {
		userID = *log.UserID
	}
	if log.APIKeyID != nil {
		apiKeyID = *log.APIKeyID
	}
	if log.GroupID != nil {
		groupID = *log.GroupID
	}
	err = r.db.QueryRowContext(ctx, `
INSERT INTO request_control_logs (
    request_id, user_id, user_email, api_key_id, api_key_name, group_id, group_name,
    endpoint, provider, protocol, model, action, reason, allowed, blocked, observed,
    client_kind, user_agent, originator, tls_fingerprint, tls_match, header_match,
    body_match, details
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, $14, $15, $16,
    $17, $18, $19, $20, $21, $22, $23, $24::jsonb
) RETURNING id, created_at`,
		log.RequestID, userID, log.UserEmail, apiKeyID, log.APIKeyName, groupID, log.GroupName,
		log.Endpoint, log.Provider, log.Protocol, log.Model, log.Action, log.Reason,
		log.Allowed, log.Blocked, log.Observed, log.ClientKind, log.UserAgent, log.Originator,
		log.TLSFingerprint, nullableBoolPtr(log.TLSMatch), nullableBoolPtr(log.HeaderMatch),
		nullableBoolPtr(log.BodyMatch), string(details),
	).Scan(&log.ID, &log.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert request control log: %w", err)
	}
	return nil
}

func (r *requestControlRepository) ListLogs(ctx context.Context, filter service.RequestControlLogFilter) ([]service.RequestControlLog, *pagination.PaginationResult, error) {
	where, args := buildRequestControlLogWhere(filter)
	whereSQL := "WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM request_control_logs l "+whereSQL, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count request control logs: %w", err)
	}
	params := filter.Pagination
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, params.Limit(), params.Offset())
	rows, err := r.db.QueryContext(ctx, `
SELECT
    l.id, l.request_id, l.user_id, l.user_email, l.api_key_id, l.api_key_name,
    l.group_id, l.group_name, l.endpoint, l.provider, l.protocol, l.model,
    l.action, l.reason, l.allowed, l.blocked, l.observed, l.client_kind,
    l.user_agent, l.originator, l.tls_fingerprint, l.tls_match, l.header_match,
    l.body_match, l.details, l.created_at
FROM request_control_logs l `+whereSQL+`
ORDER BY l.created_at DESC, l.id DESC
LIMIT $`+fmt.Sprint(len(queryArgs)-1)+` OFFSET $`+fmt.Sprint(len(queryArgs)), queryArgs...)
	if err != nil {
		return nil, nil, fmt.Errorf("list request control logs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.RequestControlLog, 0)
	for rows.Next() {
		var item service.RequestControlLog
		var userID, apiKeyID, groupID sql.NullInt64
		var tlsMatch, headerMatch, bodyMatch sql.NullBool
		var details []byte
		if err := rows.Scan(
			&item.ID, &item.RequestID, &userID, &item.UserEmail, &apiKeyID, &item.APIKeyName,
			&groupID, &item.GroupName, &item.Endpoint, &item.Provider, &item.Protocol, &item.Model,
			&item.Action, &item.Reason, &item.Allowed, &item.Blocked, &item.Observed, &item.ClientKind,
			&item.UserAgent, &item.Originator, &item.TLSFingerprint, &tlsMatch, &headerMatch,
			&bodyMatch, &details, &item.CreatedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("scan request control log: %w", err)
		}
		if userID.Valid {
			value := userID.Int64
			item.UserID = &value
		}
		if apiKeyID.Valid {
			value := apiKeyID.Int64
			item.APIKeyID = &value
		}
		if groupID.Valid {
			value := groupID.Int64
			item.GroupID = &value
		}
		if tlsMatch.Valid {
			value := tlsMatch.Bool
			item.TLSMatch = &value
		}
		if headerMatch.Valid {
			value := headerMatch.Bool
			item.HeaderMatch = &value
		}
		if bodyMatch.Valid {
			value := bodyMatch.Bool
			item.BodyMatch = &value
		}
		item.Details = map[string]string{}
		_ = json.Unmarshal(details, &item.Details)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate request control logs: %w", err)
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *requestControlRepository) CleanupLogs(ctx context.Context, before time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	result, err := r.db.ExecContext(ctx, `DELETE FROM request_control_logs WHERE created_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("cleanup request control logs: %w", err)
	}
	deleted, _ := result.RowsAffected()
	return deleted, nil
}

func buildRequestControlLogWhere(filter service.RequestControlLogFilter) ([]string, []any) {
	where := []string{"l.id IS NOT NULL"}
	args := make([]any, 0)
	add := func(expr string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(expr, len(args)))
	}
	switch strings.ToLower(strings.TrimSpace(filter.Action)) {
	case "block", "observe", "allow", "ua_whitelist":
		add("l.action = $%d", strings.ToLower(strings.TrimSpace(filter.Action)))
	}
	if protocol := strings.TrimSpace(filter.Protocol); protocol != "" {
		add("l.protocol = $%d", protocol)
	}
	if filter.GroupID != nil {
		add("l.group_id = $%d", *filter.GroupID)
	}
	if filter.UserID != nil {
		add("l.user_id = $%d", *filter.UserID)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		base := len(args) + 1
		args = append(args, search, search, search, search)
		where = append(where, fmt.Sprintf("(l.user_email ILIKE '%%' || $%d || '%%' OR l.user_agent ILIKE '%%' || $%d || '%%' OR l.reason ILIKE '%%' || $%d || '%%' OR l.request_id ILIKE '%%' || $%d || '%%')", base, base+1, base+2, base+3))
	}
	if filter.From != nil {
		add("l.created_at >= $%d", *filter.From)
	}
	if filter.To != nil {
		add("l.created_at <= $%d", *filter.To)
	}
	return where, args
}

func nullableBoolPtr(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}
