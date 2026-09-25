//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/waitlistentry"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPresaleNoticeRepositoryConcurrentClaim(t *testing.T) {
	ctx := context.Background()
	repo := NewPresaleNotificationRepository(integrationDB)
	hash := fmt.Sprintf("%064x", time.Now().UnixNano())
	version := strings.Repeat("a", 64)
	t.Cleanup(func() {
		_, err := integrationDB.Exec(`DELETE FROM presale_email_deliveries WHERE recipient_hash=$1`, hash)
		require.NoError(t, err)
	})
	var won atomic.Int64
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := repo.Claim(ctx, "2026-10", hash, 1, 1, version, false)
			if err != nil {
				t.Error(err)
			}
			if ok {
				won.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, won.Load())
	require.NoError(t, repo.Finish(ctx, "2026-10", hash, "uncertain"))
	claimed, err := repo.Claim(ctx, "2026-10", hash, 1, 2, version, false)
	require.NoError(t, err)
	require.False(t, claimed)
	states, err := repo.States(ctx, "2026-10")
	require.NoError(t, err)
	require.Equal(t, "uncertain", states[hash])
	claimed, err = repo.Claim(ctx, "2026-11", hash, 1, 2, version, false)
	require.NoError(t, err)
	require.True(t, claimed)
	claimed, err = repo.Claim(ctx, "test", hash, 0, 1, version, true)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, repo.Finish(ctx, "test", hash, "sent"))
	claimed, err = repo.Claim(ctx, "test", hash, 0, 1, version, true)
	require.NoError(t, err)
	require.False(t, claimed)
	_, err = integrationDB.Exec(`UPDATE presale_email_deliveries SET updated_at=NOW()-INTERVAL '61 seconds' WHERE campaign_key='test' AND recipient_hash=$1`, hash)
	require.NoError(t, err)
	claimed, err = repo.Claim(ctx, "test", hash, 0, 1, version, true)
	require.NoError(t, err)
	require.True(t, claimed)
}
func TestPresaleNoticeRepositoryAudience(t *testing.T) {
	ctx := context.Background()
	prefix := fmt.Sprintf("presale-notice-%d-", time.Now().UnixNano())
	for _, state := range []string{service.StatusActive, service.StatusDisabled} {
		email := prefix + state + "@example.com"
		_, err := integrationEntClient.User.Create().SetEmail(email).SetPasswordHash("test").SetStatus(state).Save(ctx)
		require.NoError(t, err)
		// Approval must not override a disabled existing account.
		_, err = integrationEntClient.WaitlistEntry.Create().SetEmail(email).SetApprovedAt(time.Now()).Save(ctx)
		require.NoError(t, err)
	}
	_, err := integrationEntClient.WaitlistEntry.Create().SetEmail(prefix + "pending@example.com").Save(ctx)
	require.NoError(t, err)
	_, err = integrationEntClient.WaitlistEntry.Create().SetEmail(prefix + "approved@example.com").SetApprovedAt(time.Now()).Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := integrationEntClient.WaitlistEntry.Delete().Where(waitlistentry.EmailHasPrefix(prefix)).Exec(ctx)
		require.NoError(t, err)
		_, err = integrationEntClient.User.Delete().Where(dbuser.EmailHasPrefix(prefix)).Exec(ctx)
		require.NoError(t, err)
	})
	repo := NewPresaleNotificationRepository(integrationDB)
	get := func(include bool) []string {
		rows, err := repo.Recipients(ctx, include)
		require.NoError(t, err)
		emails := []string{}
		for _, r := range rows {
			if strings.HasPrefix(r.Email, prefix) {
				emails = append(emails, r.Email)
			}
		}
		return emails
	}
	require.ElementsMatch(t, []string{prefix + "active@example.com", prefix + "approved@example.com"}, get(false))
	require.Len(t, get(true), 4)
}
