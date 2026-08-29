//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRequestControlRepositoryPersistsAndRefreshesSnapshot(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, testEntClient(t), &service.User{
		Email: fmt.Sprintf("request-control-snapshot-%d@example.com", time.Now().UnixNano()),
	})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	repo := &requestControlRepository{db: integrationDB}
	at := time.Now().UTC().Truncate(time.Microsecond)
	newLog := func(requestID, body string, eventAt time.Time) *service.RequestControlLog {
		return &service.RequestControlLog{
			RequestID: requestID, UserID: &user.ID, Protocol: service.RequestControlProtocolResponse,
			Details: map[string]string{}, RequestHeaders: map[string]string{}, RequestBodyMetadata: map[string]any{},
			RequestHeadersHash: strings.Repeat("a", 64), RequestBodyHash: strings.Repeat("b", 64), EventAt: eventAt,
			RequestSnapshot: service.RequestControlRequestSnapshot{Available: true, Body: body, BodyBytes: len(body), CapturedAt: eventAt},
		}
	}

	first := newLog("snapshot-first", `{"model":"gpt-5","input":"first"}`, at)
	require.NoError(t, repo.CreateLog(ctx, first))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM request_control_logs WHERE id = $1", first.ID)
	})
	requireSnapshotBody(t, first.ID, first.RequestSnapshot.Body)

	second := newLog("snapshot-second", `{"model":"gpt-5","input":"second"}`, at.Add(16*time.Minute))
	require.NoError(t, repo.CreateLog(ctx, second))
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, int64(2), second.EventCount)
	requireSnapshotBody(t, second.ID, second.RequestSnapshot.Body)
}

func requireSnapshotBody(t *testing.T, logID int64, expected string) {
	t.Helper()
	var raw []byte
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), `
SELECT request_snapshot
FROM request_control_logs
WHERE id = $1`, logID).Scan(&raw))
	var snapshot service.RequestControlRequestSnapshot
	require.NoError(t, json.Unmarshal(raw, &snapshot))
	require.True(t, snapshot.Available)
	require.Equal(t, expected, snapshot.Body)
}
