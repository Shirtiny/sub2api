//go:build unit

package repository

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorRepositoryComputeAvailabilityScansRollupTotals(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &channelMonitorRepository{db: db}
	mock.ExpectQuery("WITH params AS").
		WithArgs(int64(7), 30).
		WillReturnRows(sqlmock.NewRows([]string{"model", "total", "ok", "avg_latency_ms"}).
			AddRow("gpt-5.3-codex-spark", int64(400), int64(300), 125.8))

	rows, err := repo.ComputeAvailability(context.Background(), 7, 30)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, rows, 1)
	require.Equal(t, "gpt-5.3-codex-spark", rows[0].Model)
	require.Equal(t, 30, rows[0].WindowDays)
	require.Equal(t, 400, rows[0].TotalChecks)
	require.Equal(t, 300, rows[0].OperationalChecks)
	require.Equal(t, 75.0, rows[0].AvailabilityPct)
	require.NotNil(t, rows[0].AvgLatencyMs)
	require.Equal(t, 125, *rows[0].AvgLatencyMs)
}

func TestChannelMonitorRepositoryComputeAvailabilityForMonitorsScansRollupTotals(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &channelMonitorRepository{db: db}
	mock.ExpectQuery("WITH params AS").
		WithArgs(sqlmock.AnyArg(), 15).
		WillReturnRows(sqlmock.NewRows([]string{"monitor_id", "model", "total", "ok", "avg_latency_ms"}).
			AddRow(int64(7), "gpt-5.3-codex-spark", int64(10), int64(9), nil).
			AddRow(int64(8), "gpt-5.4-mini", int64(20), int64(10), 200.0))

	rowsByMonitor, err := repo.ComputeAvailabilityForMonitors(context.Background(), []int64{7, 8}, 15)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, rowsByMonitor[7], 1)
	require.Len(t, rowsByMonitor[8], 1)
	require.Equal(t, 90.0, rowsByMonitor[7][0].AvailabilityPct)
	require.Nil(t, rowsByMonitor[7][0].AvgLatencyMs)
	require.Equal(t, 50.0, rowsByMonitor[8][0].AvailabilityPct)
	require.NotNil(t, rowsByMonitor[8][0].AvgLatencyMs)
	require.Equal(t, 200, *rowsByMonitor[8][0].AvgLatencyMs)
}
