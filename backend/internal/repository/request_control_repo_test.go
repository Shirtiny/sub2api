package repository

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRequestControlRepositoryGetLogIncludesRedactedMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewRequestControlRepository(db)
	createdAt := time.Now().UTC()
	columns := []string{
		"id", "request_id", "user_id", "user_email", "api_key_id", "api_key_name", "group_id", "group_name",
		"endpoint", "provider", "protocol", "model", "action", "reason", "allowed", "blocked", "observed",
		"client_kind", "user_agent", "originator", "tls_fingerprint", "tls_match", "header_match", "body_match",
		"details", "request_headers", "request_body_metadata", "expected_action", "expected_reason", "expected_blocked", "expected_status_code", "violation_count", "counted_violation", "email_sent",
		"hit_email_sent", "ban_email_sent", "auto_banned", "created_at",
	}
	mock.ExpectQuery(regexp.QuoteMeta("l.request_headers, l.request_body_metadata")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			int64(7), "req-7", int64(42), "user@example.com", nil, "", nil, "Group",
			"/v1/responses", "openai", service.RequestControlProtocolResponse, "gpt-5", "block", "test", false, true, true,
			"codex", "codex_exec/1.0.0", "codex_exec", "", nil, false, false,
			`{"reason":"test"}`, `{"authorization":"[redacted]"}`, `{"model":"gpt-5","messages":{"kind":"array","count":1}}`,
			"block", "test", true, 403, 1, true, false, false, false, false, createdAt,
		))

	detail, err := repo.GetLog(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, int64(7), detail.ID)
	require.Equal(t, "[redacted]", detail.RequestHeaders["authorization"])
	require.Equal(t, "gpt-5", detail.RequestBodyMetadata["model"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRequestControlRepositoryRecordViolationUsesLockedUserState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &requestControlRepository{db: db}
	at := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO request_control_violation_states")).WithArgs(int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT hit_times, last_hit_at")).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"hit_times", "last_hit_at"}).AddRow(fmt.Sprintf("[%d]", at.UnixMilli()-360000), at.Add(-6*time.Minute)))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE request_control_violation_states")).WithArgs(int64(7), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	count, counted, err := repo.RecordViolation(context.Background(), 7, at, time.Hour, 5*time.Minute)
	require.NoError(t, err)
	require.True(t, counted)
	require.Equal(t, 2, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRequestControlRepositoryRecordViolationRespectsFiveMinuteSpacing(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &requestControlRepository{db: db}
	at := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO request_control_violation_states")).WithArgs(int64(7)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT hit_times, last_hit_at")).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"hit_times", "last_hit_at"}).AddRow(fmt.Sprintf("[%d]", at.Add(-time.Minute).UnixMilli()), at.Add(-time.Minute)))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE request_control_violation_states")).WithArgs(int64(7), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	count, counted, err := repo.RecordViolation(context.Background(), 7, at, time.Hour, 5*time.Minute)
	require.NoError(t, err)
	require.False(t, counted)
	require.Equal(t, 1, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRequestControlRepositoryRecordViolationPrunesExpiredHits(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &requestControlRepository{db: db}
	at := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO request_control_violation_states")).WithArgs(int64(7)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT hit_times, last_hit_at")).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"hit_times", "last_hit_at"}).AddRow(fmt.Sprintf("[%d]", at.Add(-2*time.Hour).UnixMilli()), at.Add(-2*time.Hour)))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE request_control_violation_states")).WithArgs(int64(7), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	count, counted, err := repo.RecordViolation(context.Background(), 7, at, time.Hour, 5*time.Minute)
	require.NoError(t, err)
	require.True(t, counted)
	require.Equal(t, 1, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRequestControlRepositoryCreateLogPersistsExpectedOutcomeAndFingerprints(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &requestControlRepository{db: db}
	userID := int64(7)
	createdAt := time.Now().UTC()
	log := &service.RequestControlLog{
		RequestID: "req-1", UserID: &userID, UserEmail: "user@example.com", Protocol: service.RequestControlProtocolChat,
		Action: "observe", Reason: "blocking_disabled_observe_only", Allowed: true, Observed: true,
		ExpectedAction: "block", ExpectedReason: "openai_chat_completions_blocked", ExpectedBlocked: true, ExpectedStatusCode: 403,
		Details: map[string]string{"x": "y"}, RequestHeaders: map[string]string{"content-type": "application/json"},
		RequestBodyMetadata: map[string]any{"protocol": service.RequestControlProtocolChat}, RequestHeadersHash: strings.Repeat("a", 64), RequestBodyHash: strings.Repeat("b", 64),
	}
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO request_control_logs")).WithArgs(
		"req-1", userID, "user@example.com", nil, "", nil, "",
		"", "", service.RequestControlProtocolChat, "", "observe", "blocking_disabled_observe_only", true, false, true,
		"", "", "", "", nil, nil, nil, `{"x":"y"}`, `{"content-type":"application/json"}`, `{"protocol":"openai_chat_completions"}`,
		"block", "openai_chat_completions_blocked", true, 403, strings.Repeat("a", 64), strings.Repeat("b", 64), 0, false, false, false, false, false, sqlmock.AnyArg(),
	).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(9), createdAt))

	require.NoError(t, repo.CreateLog(context.Background(), log))
	require.Equal(t, int64(9), log.ID)
	require.Equal(t, createdAt, log.CreatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}
