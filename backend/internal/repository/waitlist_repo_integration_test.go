//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/waitlistentry"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestWaitlistRepositoryConcurrentDuplicate(t *testing.T) {
	ctx := context.Background()
	email := fmt.Sprintf("waitlist-%d@example.com", time.Now().UnixNano())
	repo := NewWaitlistRepository(integrationEntClient)
	t.Cleanup(func() {
		_, err := integrationEntClient.WaitlistEntry.Delete().Where(waitlistentry.EmailEQ(email)).Exec(ctx)
		require.NoError(t, err)
	})
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- repo.Join(ctx, email) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	first, err := integrationEntClient.WaitlistEntry.Query().Where(waitlistentry.EmailEQ(email)).Only(ctx)
	require.NoError(t, err)
	require.NoError(t, repo.Join(ctx, email))
	after, err := integrationEntClient.WaitlistEntry.Query().Where(waitlistentry.EmailEQ(email)).Only(ctx)
	require.NoError(t, err)
	require.Equal(t, first.ID, after.ID)
	require.Equal(t, first.CreatedAt, after.CreatedAt)
}

func TestWaitlistRepositoryConfirmationLeaseAndRetry(t *testing.T) {
	ctx := context.Background()
	email := fmt.Sprintf("confirmation-%d@example.com", time.Now().UnixNano())
	repo := NewWaitlistRepository(integrationEntClient)
	require.NoError(t, repo.Join(ctx, email))
	t.Cleanup(func() {
		_, err := integrationEntClient.WaitlistEntry.Delete().Where(waitlistentry.EmailEQ(email)).Exec(ctx)
		require.NoError(t, err)
	})
	attempt := time.Now().UTC().Truncate(time.Microsecond)
	var wg sync.WaitGroup
	claims := make(chan bool, 12)
	errs := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := repo.ClaimConfirmation(ctx, email, attempt)
			claims <- claimed
			errs <- err
		}()
	}
	wg.Wait()
	close(claims)
	close(errs)
	won := 0
	for claimed := range claims {
		if claimed {
			won++
		}
	}
	require.Equal(t, 1, won)
	for err := range errs {
		if err != nil {
			require.ErrorIs(t, err, service.ErrWaitlistConfirmationFailed)
		}
	}
	// A failed SMTP send releases the row for a later retry, preserving the signup.
	require.NoError(t, repo.FinishConfirmation(ctx, email, attempt, false))
	retry := attempt.Add(time.Second)
	claimed, err := repo.ClaimConfirmation(ctx, email, retry)
	require.NoError(t, err)
	require.True(t, claimed)
	// An abandoned attempt expires, and the old owner cannot clear the new lease.
	takeover := retry.Add(service.WaitlistConfirmationLease + time.Second)
	claimed, err = repo.ClaimConfirmation(ctx, email, takeover)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Error(t, repo.FinishConfirmation(ctx, email, retry, false))
	require.NoError(t, repo.FinishConfirmation(ctx, email, takeover, true))
	claimed, err = repo.ClaimConfirmation(ctx, email, takeover.Add(time.Second))
	require.NoError(t, err)
	require.False(t, claimed)
	entry, err := integrationEntClient.WaitlistEntry.Query().Where(waitlistentry.EmailEQ(email)).Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, entry.ConfirmationSentAt)
}
