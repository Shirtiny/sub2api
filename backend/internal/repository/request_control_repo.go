package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
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

const requestControlMaxStoredHitTimestamps = 10000

// RecordViolation updates the compact per-user rolling hit state. The
// deduplicated request row must not be used as the source of truth for this
// state because one fingerprint can be observed many times.
func (r *requestControlRepository) RecordViolation(ctx context.Context, userID int64, at time.Time, window, spacing time.Duration) (int, bool, error) {
	if r == nil || r.db == nil || userID <= 0 {
		return 0, false, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, fmt.Errorf("begin request control violation state: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO request_control_violation_states (user_id)
VALUES ($1)
ON CONFLICT (user_id) DO NOTHING`, userID); err != nil {
		return 0, false, fmt.Errorf("initialize request control violation state: %w", err)
	}
	var raw []byte
	var last sql.NullTime
	if err := tx.QueryRowContext(ctx, `
SELECT hit_times, last_hit_at
FROM request_control_violation_states
WHERE user_id = $1
FOR UPDATE`, userID).Scan(&raw, &last); err != nil {
		return 0, false, fmt.Errorf("load request control violation state: %w", err)
	}
	var hits []int64
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &hits); err != nil {
			return 0, false, fmt.Errorf("decode request control violation state: %w", err)
		}
	}
	effectiveAt := at
	if last.Valid && last.Time.After(effectiveAt) {
		effectiveAt = last.Time
	}
	cutoff := effectiveAt.Add(-window).UnixMilli()
	filtered := hits[:0]
	for _, timestamp := range hits {
		if timestamp >= cutoff {
			filtered = append(filtered, timestamp)
		}
	}
	hits = filtered
	counted := !last.Valid || (at.After(last.Time) && at.Sub(last.Time) > spacing)
	if counted {
		hits = append(hits, at.UnixMilli())
	}
	// Requests may finish out of order across workers. Keep timestamps ordered
	// so the bounded tail retains the newest counted hits.
	sort.Slice(hits, func(i, j int) bool { return hits[i] < hits[j] })
	if len(hits) > requestControlMaxStoredHitTimestamps {
		hits = hits[len(hits)-requestControlMaxStoredHitTimestamps:]
	}
	latest := effectiveAt
	encoded, err := json.Marshal(hits)
	if err != nil {
		return 0, false, fmt.Errorf("encode request control violation state: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE request_control_violation_states
SET hit_times = $2::jsonb, last_hit_at = $3, updated_at = NOW()
WHERE user_id = $1`, userID, string(encoded), latest); err != nil {
		return 0, false, fmt.Errorf("save request control violation state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("commit request control violation state: %w", err)
	}
	return len(hits), counted, nil
}

func (r *requestControlRepository) CreateLog(ctx context.Context, log *service.RequestControlLog) error {
	if r == nil || r.db == nil || log == nil {
		return nil
	}
	eventAt := requestControlLogEventAt(log)
	if eventAt.IsZero() {
		eventAt = time.Now().UTC()
		log.EventAt = eventAt
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = eventAt
	}
	details, err := json.Marshal(log.Details)
	if err != nil {
		return fmt.Errorf("marshal request control details: %w", err)
	}
	requestHeaders, err := json.Marshal(log.RequestHeaders)
	if err != nil {
		return fmt.Errorf("marshal request control request headers: %w", err)
	}
	requestBody, err := json.Marshal(log.RequestBodyMetadata)
	if err != nil {
		return fmt.Errorf("marshal request control request body metadata: %w", err)
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
INSERT INTO request_control_logs AS existing (
    request_id, user_id, user_email, api_key_id, api_key_name, group_id, group_name,
    endpoint, provider, protocol, model, action, reason, allowed, blocked, observed,
	client_kind, user_agent, originator, tls_fingerprint, tls_match, header_match,
	body_match, details, request_headers, request_body_metadata,
    expected_action, expected_reason, expected_blocked, expected_status_code,
    request_headers_hash, request_body_hash, violation_count,
    counted_violation, email_sent, hit_email_sent, ban_email_sent, auto_banned, created_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8, $9, $10, $11, $12, $13, $14, $15, $16,
	$17, $18, $19, $20, $21, $22, $23, $24::jsonb, $25::jsonb, $26::jsonb,
	$27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, $39
	) ON CONFLICT (user_id, protocol, request_headers_hash, request_body_hash)
	WHERE user_id IS NOT NULL AND request_headers_hash <> '' AND request_body_hash <> ''
DO UPDATE SET
    request_id = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.request_id ELSE existing.request_id END,
    user_email = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.user_email ELSE existing.user_email END,
    api_key_id = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.api_key_id ELSE existing.api_key_id END,
    api_key_name = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.api_key_name ELSE existing.api_key_name END,
    group_id = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.group_id ELSE existing.group_id END,
    group_name = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.group_name ELSE existing.group_name END,
    endpoint = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.endpoint ELSE existing.endpoint END,
    provider = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.provider ELSE existing.provider END,
    model = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.model ELSE existing.model END,
    action = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.action ELSE existing.action END,
    reason = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.reason ELSE existing.reason END,
    allowed = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.allowed ELSE existing.allowed END,
    blocked = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.blocked ELSE existing.blocked END,
    observed = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.observed ELSE existing.observed END,
    client_kind = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.client_kind ELSE existing.client_kind END,
    user_agent = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.user_agent ELSE existing.user_agent END,
    originator = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.originator ELSE existing.originator END,
    tls_fingerprint = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.tls_fingerprint ELSE existing.tls_fingerprint END,
    tls_match = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.tls_match ELSE existing.tls_match END,
    header_match = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.header_match ELSE existing.header_match END,
    body_match = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.body_match ELSE existing.body_match END,
    details = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.details ELSE existing.details END,
    request_headers = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.request_headers ELSE existing.request_headers END,
    request_body_metadata = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.request_body_metadata ELSE existing.request_body_metadata END,
    expected_action = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.expected_action ELSE existing.expected_action END,
    expected_reason = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.expected_reason ELSE existing.expected_reason END,
    expected_blocked = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.expected_blocked ELSE existing.expected_blocked END,
    expected_status_code = CASE WHEN EXCLUDED.created_at >= existing.created_at THEN EXCLUDED.expected_status_code ELSE existing.expected_status_code END,
    violation_count = CASE WHEN EXCLUDED.created_at >= existing.created_at AND EXCLUDED.violation_count > 0 THEN EXCLUDED.violation_count ELSE existing.violation_count END,
	-- The state/count side effect is calculated after this upsert. Preserve a
	-- previous counted marker while the transient insert carries its default
	-- false value; UpdateLogSideEffects records a new true marker afterward.
	counted_violation = existing.counted_violation OR EXCLUDED.counted_violation,
    email_sent = existing.email_sent OR EXCLUDED.email_sent,
    hit_email_sent = existing.hit_email_sent OR EXCLUDED.hit_email_sent,
    ban_email_sent = existing.ban_email_sent OR EXCLUDED.ban_email_sent,
    auto_banned = existing.auto_banned OR EXCLUDED.auto_banned,
	created_at = GREATEST(existing.created_at, EXCLUDED.created_at)
RETURNING id, created_at`,
		log.RequestID, userID, log.UserEmail, apiKeyID, log.APIKeyName, groupID, log.GroupName,
		log.Endpoint, log.Provider, log.Protocol, log.Model, log.Action, log.Reason,
		log.Allowed, log.Blocked, log.Observed, log.ClientKind, log.UserAgent, log.Originator,
		log.TLSFingerprint, nullableBoolPtr(log.TLSMatch), nullableBoolPtr(log.HeaderMatch),
		nullableBoolPtr(log.BodyMatch), string(details), string(requestHeaders), string(requestBody),
		log.ExpectedAction, log.ExpectedReason, log.ExpectedBlocked, log.ExpectedStatusCode,
		log.RequestHeadersHash, log.RequestBodyHash, log.ViolationCount, log.Counted, log.EmailSent,
		log.HitEmailSent, log.BanEmailSent, log.AutoBanned, eventAt,
	).Scan(&log.ID, &log.CreatedAt)
	if err == sql.ErrNoRows {
		log.ID = 0
		return nil
	}
	if err != nil {
		return fmt.Errorf("insert request control log: %w", err)
	}
	return nil
}

// GetViolationState returns counted hits and the latest blocked hit in the
// window. The latest timestamp intentionally includes uncounted hits so a
// burst cannot become multiple violations merely because the count is stale.
func (r *requestControlRepository) GetViolationState(ctx context.Context, userID int64, since time.Time) (int, *time.Time, error) {
	if r == nil || r.db == nil || userID <= 0 {
		return 0, nil, nil
	}
	var count int
	var last sql.NullTime
	err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*) FILTER (WHERE counted_violation = TRUE), MAX(created_at)
FROM request_control_logs
WHERE user_id = $1 AND blocked = TRUE AND created_at >= $2`, userID, since).Scan(&count, &last)
	if err != nil {
		return 0, nil, fmt.Errorf("get request control violation state: %w", err)
	}
	if !last.Valid {
		return count, nil, nil
	}
	value := last.Time
	return count, &value, nil
}

func (r *requestControlRepository) UpdateLogSideEffects(ctx context.Context, log *service.RequestControlLog) error {
	if r == nil || r.db == nil || log == nil || log.ID <= 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE request_control_logs
	SET violation_count = CASE WHEN created_at <= $8::timestamptz + INTERVAL '1 microsecond' AND $2 > 0 THEN $2 ELSE violation_count END,
	    counted_violation = counted_violation OR (created_at <= $8::timestamptz + INTERVAL '1 microsecond' AND $3),
	    email_sent = email_sent OR $4,
	    hit_email_sent = hit_email_sent OR $5,
	    ban_email_sent = ban_email_sent OR $6,
	    auto_banned = auto_banned OR $7
	WHERE id = $1`, log.ID, log.ViolationCount, log.Counted, log.EmailSent,
		log.HitEmailSent, log.BanEmailSent, log.AutoBanned, requestControlLogEventAt(log))
	if err != nil {
		return fmt.Errorf("update request control log side effects: %w", err)
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
	 l.body_match, l.details, l.expected_action, l.expected_reason, l.expected_blocked, l.expected_status_code,
	 l.violation_count, l.counted_violation, l.email_sent,
	 l.hit_email_sent, l.ban_email_sent, l.auto_banned, l.created_at
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
		var violationCount int
		var countedViolation, emailSent, hitEmailSent, banEmailSent, autoBanned bool
		var expectedBlocked bool
		var expectedStatusCode int
		var details []byte
		if err := rows.Scan(
			&item.ID, &item.RequestID, &userID, &item.UserEmail, &apiKeyID, &item.APIKeyName,
			&groupID, &item.GroupName, &item.Endpoint, &item.Provider, &item.Protocol, &item.Model,
			&item.Action, &item.Reason, &item.Allowed, &item.Blocked, &item.Observed, &item.ClientKind,
			&item.UserAgent, &item.Originator, &item.TLSFingerprint, &tlsMatch, &headerMatch,
			&bodyMatch, &details, &item.ExpectedAction, &item.ExpectedReason, &expectedBlocked, &expectedStatusCode,
			&violationCount, &countedViolation, &emailSent, &hitEmailSent, &banEmailSent, &autoBanned, &item.CreatedAt,
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
		item.ExpectedBlocked = expectedBlocked
		item.ExpectedStatusCode = expectedStatusCode
		item.ViolationCount = violationCount
		item.Counted = countedViolation
		item.EmailSent = emailSent
		item.HitEmailSent = hitEmailSent
		item.BanEmailSent = banEmailSent
		item.AutoBanned = autoBanned
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate request control logs: %w", err)
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *requestControlRepository) GetLog(ctx context.Context, id int64) (*service.RequestControlLogDetail, error) {
	if r == nil || r.db == nil || id <= 0 {
		return nil, service.ErrRequestControlLogNotFound
	}
	var item service.RequestControlLog
	var userID, apiKeyID, groupID sql.NullInt64
	var tlsMatch, headerMatch, bodyMatch sql.NullBool
	var violationCount int
	var countedViolation, emailSent, hitEmailSent, banEmailSent, autoBanned bool
	var expectedBlocked bool
	var expectedStatusCode int
	var details, requestHeaders, requestBody []byte
	err := r.db.QueryRowContext(ctx, `
SELECT
    l.id, l.request_id, l.user_id, l.user_email, l.api_key_id, l.api_key_name,
    l.group_id, l.group_name, l.endpoint, l.provider, l.protocol, l.model,
    l.action, l.reason, l.allowed, l.blocked, l.observed, l.client_kind,
    l.user_agent, l.originator, l.tls_fingerprint, l.tls_match, l.header_match,
	 l.body_match, l.details, l.request_headers, l.request_body_metadata,
	 l.expected_action, l.expected_reason, l.expected_blocked, l.expected_status_code,
	 l.violation_count, l.counted_violation, l.email_sent, l.hit_email_sent,
    l.ban_email_sent, l.auto_banned, l.created_at
FROM request_control_logs l WHERE l.id = $1`, id).Scan(
		&item.ID, &item.RequestID, &userID, &item.UserEmail, &apiKeyID, &item.APIKeyName,
		&groupID, &item.GroupName, &item.Endpoint, &item.Provider, &item.Protocol, &item.Model,
		&item.Action, &item.Reason, &item.Allowed, &item.Blocked, &item.Observed, &item.ClientKind,
		&item.UserAgent, &item.Originator, &item.TLSFingerprint, &tlsMatch, &headerMatch,
		&bodyMatch, &details, &requestHeaders, &requestBody, &item.ExpectedAction, &item.ExpectedReason, &expectedBlocked, &expectedStatusCode,
		&violationCount, &countedViolation,
		&emailSent, &hitEmailSent, &banEmailSent, &autoBanned, &item.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, service.ErrRequestControlLogNotFound
		}
		return nil, fmt.Errorf("get request control log: %w", err)
	}
	applyRequestControlLogNullableFields(&item, userID, apiKeyID, groupID, tlsMatch, headerMatch, bodyMatch, details, violationCount, countedViolation, emailSent, hitEmailSent, banEmailSent, autoBanned)
	item.ExpectedBlocked = expectedBlocked
	item.ExpectedStatusCode = expectedStatusCode
	var headers map[string]string
	if len(requestHeaders) > 0 {
		_ = json.Unmarshal(requestHeaders, &headers)
	}
	if headers == nil {
		headers = map[string]string{}
	}
	bodyMetadata := map[string]any{}
	if len(requestBody) > 0 {
		_ = json.Unmarshal(requestBody, &bodyMetadata)
	}
	if len(bodyMetadata) == 0 {
		bodyMetadata = map[string]any{
			"metadata_available": false,
			"reason":             "recorded_before_metadata_capture",
		}
	}
	return &service.RequestControlLogDetail{RequestControlLog: item, RequestHeaders: headers, RequestBodyMetadata: bodyMetadata}, nil
}

func applyRequestControlLogNullableFields(item *service.RequestControlLog, userID, apiKeyID, groupID sql.NullInt64, tlsMatch, headerMatch, bodyMatch sql.NullBool, details []byte, violationCount int, countedViolation, emailSent, hitEmailSent, banEmailSent, autoBanned bool) {
	if item == nil {
		return
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
	item.ViolationCount = violationCount
	item.Counted = countedViolation
	item.EmailSent = emailSent
	item.HitEmailSent = hitEmailSent
	item.BanEmailSent = banEmailSent
	item.AutoBanned = autoBanned
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
	stateResult, err := r.db.ExecContext(ctx, `DELETE FROM request_control_violation_states WHERE updated_at < $1`, before)
	if err != nil {
		return deleted, fmt.Errorf("cleanup request control violation states: %w", err)
	}
	stateDeleted, _ := stateResult.RowsAffected()
	return deleted + stateDeleted, nil
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

func requestControlLogEventAt(log *service.RequestControlLog) time.Time {
	if log == nil {
		return time.Time{}
	}
	if !log.EventAt.IsZero() {
		return log.EventAt
	}
	return log.CreatedAt
}
