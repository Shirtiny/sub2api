package repository

import (
	"context"
	"regexp"
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
		"details", "request_headers", "request_body_metadata", "violation_count", "counted_violation", "email_sent",
		"hit_email_sent", "ban_email_sent", "auto_banned", "created_at",
	}
	mock.ExpectQuery(regexp.QuoteMeta("l.request_headers, l.request_body_metadata")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			int64(7), "req-7", int64(42), "user@example.com", nil, "", nil, "Group",
			"/v1/responses", "openai", service.RequestControlProtocolResponse, "gpt-5", "block", "test", false, true, true,
			"codex", "codex_exec/1.0.0", "codex_exec", "", nil, false, false,
			`{"reason":"test"}`, `{"authorization":"[redacted]"}`, `{"model":"gpt-5","messages":{"kind":"array","count":1}}`,
			1, true, false, false, false, false, createdAt,
		))

	detail, err := repo.GetLog(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, int64(7), detail.ID)
	require.Equal(t, "[redacted]", detail.RequestHeaders["authorization"])
	require.Equal(t, "gpt-5", detail.RequestBodyMetadata["model"])
	require.NoError(t, mock.ExpectationsWereMet())
}
