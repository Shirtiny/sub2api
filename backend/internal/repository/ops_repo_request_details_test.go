package repository

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestListRequestDetailsGroupFilterIncludesCustomSubscriptionGroups(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	gid := int64(42)
	start := time.Unix(100, 0)
	end := time.Unix(200, 0)

	mock.ExpectQuery("custom_source_group_id").
		WithArgs(start.UTC(), end.UTC(), gid).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery("custom_source_group_id").
		WithArgs(start.UTC(), end.UTC(), gid, 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"kind", "created_at", "request_id", "platform", "model", "duration_ms", "status_code", "error_id",
			"phase", "severity", "message", "user_id", "api_key_id", "account_id", "group_id", "stream",
		}))

	items, total, err := repo.ListRequestDetails(context.Background(), &service.OpsRequestDetailFilter{
		StartTime: &start,
		EndTime:   &end,
		GroupID:   &gid,
		Page:      1,
		PageSize:  10,
	})

	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, items)
	require.NoError(t, mock.ExpectationsWereMet())
}
