package repository

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBuildContentModerationLogWhere_BlockedIncludesAllBlockActions(t *testing.T) {
	where, args := buildContentModerationLogWhere(service.ContentModerationLogFilter{Result: "blocked"})

	require.Empty(t, args)
	sql := strings.Join(where, " AND ")
	require.Contains(t, sql, "l.action IN ('block', 'keyword_block', 'hash_block')")
	require.NotContains(t, sql, "l.action = 'block'")
}

func TestBuildContentModerationLogWhereGroupFilterIncludesCustomSubscriptionGroups(t *testing.T) {
	groupID := int64(42)
	where, args := buildContentModerationLogWhere(service.ContentModerationLogFilter{GroupID: &groupID})
	sql := strings.Join(where, " AND ")
	for _, want := range []string{"l.group_id = $1", "custom_source_group_id = $1", "is_custom_subscription_group = TRUE"} {
		require.Contains(t, sql, want)
	}
	require.Equal(t, []any{groupID}, args)
}

func TestContentModerationRepositoryCountFlaggedByUserSince_CountsOnlyAppliedViolations(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	since := time.Now().Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("AND side_effects_applied = TRUE")).
		WithArgs(int64(1001), since).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count, err := repo.CountFlaggedByUserSince(context.Background(), 1001, since)

	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryCreateLogPersistsSideEffectIdentity(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	userID := int64(1001)
	createdAt := time.Now().UTC()
	inputHash := strings.Repeat("a", 64)
	log := &service.ContentModerationLog{
		RequestID:          "request-1",
		UserID:             &userID,
		UserEmail:          "user@example.com",
		Endpoint:           "/v1/responses",
		Provider:           "openai",
		Model:              "gpt-test",
		Mode:               service.ContentModerationModePreBlock,
		Action:             service.ContentModerationActionKeywordBlock,
		Flagged:            true,
		HighestCategory:    "keyword",
		HighestScore:       1,
		CategoryScores:     map[string]float64{"keyword": 1},
		ThresholdSnapshot:  map[string]float64{},
		InputExcerpt:       "blocked excerpt",
		InputHash:          inputHash,
		MatchedKeyword:     "blocked",
		ViolationCount:     1,
		SideEffectsApplied: true,
	}

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO content_moderation_logs")).
		WithArgs(
			"request-1", userID, "user@example.com", nil, "", nil, "",
			"/v1/responses", "openai", "gpt-test", service.ContentModerationModePreBlock,
			service.ContentModerationActionKeywordBlock, true, "keyword", float64(1),
			`{"keyword":1}`, `{}`, "blocked excerpt", inputHash, "blocked", nil, "",
			1, true, false, false, nil,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(42), createdAt))

	require.NoError(t, repo.CreateLog(context.Background(), log))
	require.Equal(t, int64(42), log.ID)
	require.Equal(t, createdAt, log.CreatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}
