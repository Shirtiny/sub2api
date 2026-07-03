//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestUserGroupRateRepositoryGetByUserAndGroupFallsBackToCustomSourceGroup(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &userGroupRateRepository{sql: db}
	mock.ExpectQuery("custom_source_group_id").
		WithArgs(int64(101), int64(202)).
		WillReturnRows(sqlmock.NewRows([]string{"rate_multiplier"}).AddRow(1.75))

	got, err := repo.GetByUserAndGroup(context.Background(), 101, 202)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, 1.75, *got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserGroupRateRepositoryGetRPMOverrideByUserAndGroupFallsBackToCustomSourceGroup(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &userGroupRateRepository{sql: db}
	mock.ExpectQuery("custom_source_group_id").
		WithArgs(int64(101), int64(202)).
		WillReturnRows(sqlmock.NewRows([]string{"rpm_override"}).AddRow(60))

	got, err := repo.GetRPMOverrideByUserAndGroup(context.Background(), 101, 202)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, 60, *got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserGroupRateRepositoryGetByUserIDIncludesEffectiveCustomGroupRates(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &userGroupRateRepository{sql: db}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT cg.id AS group_id, src.rate_multiplier")).
		WithArgs(int64(101)).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "rate_multiplier"}).
			AddRow(int64(10), 1.25).
			AddRow(int64(20), 1.75))

	got, err := repo.GetByUserID(context.Background(), 101)
	require.NoError(t, err)
	require.Equal(t, map[int64]float64{10: 1.25, 20: 1.75}, got)
	require.NoError(t, mock.ExpectationsWereMet())
}
