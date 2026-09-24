//go:build integration

package repository

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/waitlistentry"
	"github.com/Wei-Shaw/sub2api/internal/config"
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

// Approval runs entirely against the harness-owned disposable PostgreSQL database.
func TestWaitlistApprovalExistingAccountAndRetry(t *testing.T) {
	ctx := context.Background()
	email := fmt.Sprintf("approved-existing-%d@example.com", time.Now().UnixNano())
	user, err := integrationEntClient.User.Create().SetEmail(" " + strings.ToUpper(email) + " ").SetPasswordHash("unchanged-hash").SetBalance(12.5).SetRole(service.RoleUser).SetStatus(service.StatusDisabled).Save(ctx)
	require.NoError(t, err)
	repo := NewWaitlistRepository(integrationEntClient)
	require.NoError(t, repo.Join(ctx, email))
	entry, err := integrationEntClient.WaitlistEntry.Query().Where(waitlistentry.EmailEQ(email)).Only(ctx)
	require.NoError(t, err)
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := repo.Approve(ctx, entry.ID, 7, 3.75); results <- err }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	after, err := integrationEntClient.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, after.Status)
	require.Equal(t, user.PasswordHash, after.PasswordHash)
	require.Equal(t, user.Balance+3.75, after.Balance)
	require.Equal(t, user.Role, after.Role)
	require.Equal(t, user.TotalRecharged, after.TotalRecharged)
	history, err := NewRedeemCodeRepository(integrationEntClient).ListByUser(ctx, user.ID, 10)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, service.AdjustmentTypeAdminBalance, history[0].Type)
	require.Equal(t, service.StatusUsed, history[0].Status)
	require.Equal(t, 3.75, history[0].Value)
	require.Equal(t, "候补名单通过赠送", history[0].Notes)
	require.NotNil(t, history[0].UsedAt)
	require.Equal(t, user.ID, *history[0].UsedBy)
	approved, err := repo.HasRegistrationApproval(ctx, email)
	require.NoError(t, err)
	require.False(t, approved)
	linked, err := repo.GetApprovedEntryByEmail(ctx, " "+strings.ToUpper(email)+" ")
	require.NoError(t, err)
	require.NotNil(t, linked)
	require.Equal(t, user.ID, *linked.GrantedUserID)
	require.NotNil(t, linked.ApprovedAt)
	// Later suspension must survive an approval/notification retry.
	require.NoError(t, integrationEntClient.User.UpdateOneID(user.ID).SetStatus(service.StatusDisabled).SetBalance(1.25).Exec(ctx))
	retry, err := repo.Approve(ctx, entry.ID, 8, 99)
	require.NoError(t, err)
	require.Equal(t, int64(7), *retry.ApprovedBy)
	require.Equal(t, user.ID, *retry.GrantedUserID)
	after, err = integrationEntClient.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusDisabled, after.Status)
	require.Equal(t, 1.25, after.Balance)
	history, err = NewRedeemCodeRepository(integrationEntClient).ListByUser(ctx, user.ID, 10)
	require.NoError(t, err)
	require.Len(t, history, 1)
}

func TestWaitlistApprovalRegistrationTransaction(t *testing.T) {
	ctx := context.Background()
	email := fmt.Sprintf("approved-new-%d@example.com", time.Now().UnixNano())
	repo := NewWaitlistRepository(integrationEntClient)
	missing, err := repo.GetApprovedEntryByEmail(ctx, email)
	require.NoError(t, err)
	require.Nil(t, missing)
	require.NoError(t, repo.Join(ctx, email))
	pending, err := repo.GetApprovedEntryByEmail(ctx, email)
	require.NoError(t, err)
	require.Nil(t, pending)
	entry, err := integrationEntClient.WaitlistEntry.Query().Where(waitlistentry.EmailEQ(email)).Only(ctx)
	require.NoError(t, err)
	allowed, err := repo.HasRegistrationApproval(ctx, email)
	require.NoError(t, err)
	require.False(t, allowed)
	_, err = repo.Approve(ctx, entry.ID, 7, 3.75)
	require.NoError(t, err)
	approvedEntry, err := repo.GetApprovedEntryByEmail(ctx, strings.ToUpper(email))
	require.NoError(t, err)
	require.NotNil(t, approvedEntry)
	require.Nil(t, approvedEntry.GrantedUserID)
	allowed, err = repo.HasRegistrationApproval(ctx, strings.ToUpper(email))
	require.NoError(t, err)
	require.True(t, allowed)
	userRepo := NewUserRepository(integrationEntClient, integrationDB)
	require.Error(t, repo.ConsumeRegistrationApproval(ctx, email, 10, 0)) // no external transaction
	for _, commit := range []bool{false, true} {
		tx, err := integrationEntClient.Tx(ctx)
		require.NoError(t, err)
		txCtx := dbent.NewTxContext(ctx, tx)
		user := &service.User{Email: email, PasswordHash: "test", Balance: 3.75, Role: service.RoleUser, Status: service.StatusActive, Concurrency: 2}
		require.NoError(t, userRepo.Create(txCtx, user))
		// User creation must not commit a nested transaction ahead of admission.
		visible, err := integrationEntClient.User.Query().Where(dbuser.IDEQ(user.ID)).Exist(ctx)
		require.NoError(t, err)
		require.False(t, visible)
		require.NoError(t, repo.ConsumeRegistrationApproval(txCtx, email, user.ID, user.Balance))
		if commit {
			require.NoError(t, tx.Commit())
		} else {
			require.NoError(t, tx.Rollback())
		}
		visible, err = integrationEntClient.User.Query().Where(dbuser.IDEQ(user.ID)).Exist(ctx)
		require.NoError(t, err)
		require.Equal(t, commit, visible)
		history, err := NewRedeemCodeRepository(integrationEntClient).ListByUser(ctx, user.ID, 10)
		require.NoError(t, err)
		if commit {
			require.Len(t, history, 1)
			require.Equal(t, 3.75, history[0].Value)
			saved, err := integrationEntClient.User.Get(ctx, user.ID)
			require.NoError(t, err)
			require.Equal(t, 3.75, saved.Balance) // Record only, not a second gift.
		} else {
			require.Empty(t, history)
		}
		allowed, err = repo.HasRegistrationApproval(ctx, email)
		require.NoError(t, err)
		require.Equal(t, !commit, allowed)
	}
	// A consumed approval is never reusable, even after soft deletion and a retry.
	require.NoError(t, integrationEntClient.User.Update().Where(userEmailLookupPredicate(email)).SetDeletedAt(time.Now()).Exec(ctx))
	_, err = repo.Approve(ctx, entry.ID, 8, 0)
	require.NoError(t, err)
	allowed, err = repo.HasRegistrationApproval(ctx, email)
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestWaitlistApprovalNoticeConcurrentLease(t *testing.T) {
	ctx := context.Background()
	email := fmt.Sprintf("approval-notice-%d@example.com", time.Now().UnixNano())
	repo := NewWaitlistRepository(integrationEntClient)
	require.NoError(t, repo.Join(ctx, email))
	entry, err := integrationEntClient.WaitlistEntry.Query().Where(waitlistentry.EmailEQ(email)).Only(ctx)
	require.NoError(t, err)
	_, err = repo.Approve(ctx, entry.ID, 7, 0)
	require.NoError(t, err)
	attempt := time.Now().UTC().Truncate(time.Microsecond)
	var wg sync.WaitGroup
	claims := make(chan bool, 8)
	errors := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := repo.ClaimApprovalNotice(ctx, entry.ID, attempt)
			claims <- claimed
			errors <- err
		}()
	}
	wg.Wait()
	close(claims)
	close(errors)
	won := 0
	for claimed := range claims {
		if claimed {
			won++
		}
	}
	require.Equal(t, 1, won)
	for err := range errors {
		if err != nil {
			require.ErrorIs(t, err, service.ErrWaitlistApprovalNoticeFailed)
		}
	}
	require.NoError(t, repo.FinishApprovalNotice(ctx, entry.ID, attempt, false))
	retry := attempt.Add(time.Second)
	claimed, err := repo.ClaimApprovalNotice(ctx, entry.ID, retry)
	require.NoError(t, err)
	require.True(t, claimed)
	takeover := retry.Add(service.WaitlistConfirmationLease + time.Second)
	claimed, err = repo.ClaimApprovalNotice(ctx, entry.ID, takeover)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Error(t, repo.FinishApprovalNotice(ctx, entry.ID, retry, false))
	require.NoError(t, repo.FinishApprovalNotice(ctx, entry.ID, takeover, true))
	claimed, err = repo.ClaimApprovalNotice(ctx, entry.ID, takeover.Add(time.Second))
	require.NoError(t, err)
	require.False(t, claimed)
}

type waitlistRegistrationSettings struct {
	service.SettingRepository
	defaultBalance string
}

func (s waitlistRegistrationSettings) GetValue(_ context.Context, key string) (string, error) {
	if key == service.SettingKeyDefaultBalance && s.defaultBalance != "" {
		return s.defaultBalance, nil
	}
	if key == service.SettingKeyRegistrationEnabled || key == service.SettingKeyEmailVerifyEnabled {
		return "false", nil
	}
	if key == service.SettingKeyInvitationCodeEnabled {
		return "true", nil
	}
	return "", service.ErrSettingNotFound
}
func (waitlistRegistrationSettings) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

type waitlistRegistrationCache struct {
	service.EmailCache
	email string
}

func (c *waitlistRegistrationCache) GetVerificationCode(_ context.Context, email string) (*service.VerificationCodeData, error) {
	if email != c.email {
		return nil, nil
	}
	return &service.VerificationCodeData{Code: "123456", ExpiresAt: time.Now().Add(time.Minute)}, nil
}
func (c *waitlistRegistrationCache) DeleteVerificationCode(context.Context, string) error { return nil }
func (c *waitlistRegistrationCache) SetVerificationCode(context.Context, string, *service.VerificationCodeData, time.Duration) error {
	return nil
}

type waitlistConsumeFailure struct{ service.WaitlistRepository }

func (waitlistConsumeFailure) ConsumeRegistrationApproval(context.Context, string, int64, float64) error {
	return service.ErrRegDisabled
}

func TestWaitlistApprovedEmailSignupWhileRegistrationClosed(t *testing.T) {
	ctx := context.Background()
	email := fmt.Sprintf("signup-approved-%d@example.com", time.Now().UnixNano())
	repo := NewWaitlistRepository(integrationEntClient)
	require.NoError(t, repo.Join(ctx, email))
	entry, err := integrationEntClient.WaitlistEntry.Query().Where(waitlistentry.EmailEQ(email)).Only(ctx)
	require.NoError(t, err)
	_, err = repo.Approve(ctx, entry.ID, 7, 0)
	require.NoError(t, err)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "local-test-only-secret", ExpireHour: 1}, Default: config.DefaultConfig{UserConcurrency: 2, UserBalance: 123}}
	settingsRepo := waitlistRegistrationSettings{defaultBalance: "4.5"}
	settings := service.NewSettingService(settingsRepo, cfg)
	userRepo := NewUserRepository(integrationEntClient, integrationDB)
	emailService := service.NewEmailService(settingsRepo, &waitlistRegistrationCache{email: email})
	build := func(r service.WaitlistRepository) *service.AuthService {
		return service.NewAuthService(integrationEntClient, userRepo, nil, nil, cfg, settings, emailService, nil, nil, nil, nil, nil, nil, r)
	}
	auth := build(repo)
	_, _, err = auth.Register(ctx, email, "test-password")
	require.ErrorIs(t, err, service.ErrEmailVerifyRequired)
	_, _, err = auth.RegisterWithVerification(ctx, email, "test-password", "999999", "", "", "")
	require.ErrorIs(t, err, service.ErrInvalidVerifyCode)
	// Fail admission after Create: no orphan account or consumed grant can escape.
	_, _, err = build(waitlistConsumeFailure{repo}).RegisterWithVerification(ctx, email, "test-password", "123456", "", "", "")
	require.ErrorIs(t, err, service.ErrRegDisabled)
	exists, err := userRepo.ExistsByEmail(ctx, email)
	require.NoError(t, err)
	require.False(t, exists)
	allowed, err := repo.HasRegistrationApproval(ctx, email)
	require.NoError(t, err)
	require.True(t, allowed)
	// A failed history insert must also roll back the new user and admission.
	conflict, err := integrationEntClient.RedeemCode.Create().SetCode(fmt.Sprintf("WL-GIFT-%d", entry.ID)).SetStatus(service.StatusUsed).Save(ctx)
	require.NoError(t, err)
	_, _, err = auth.RegisterWithVerification(ctx, email, "test-password", "123456", "", "", "")
	require.ErrorContains(t, err, "record waitlist gift")
	exists, err = userRepo.ExistsByEmail(ctx, email)
	require.NoError(t, err)
	require.False(t, exists)
	allowed, err = repo.HasRegistrationApproval(ctx, email)
	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, integrationEntClient.RedeemCode.DeleteOneID(conflict.ID).Exec(ctx))
	token, user, err := auth.RegisterWithVerification(ctx, strings.ToUpper(email), "test-password", "123456", "", "", "")
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.Equal(t, email, user.Email)
	require.Equal(t, service.StatusActive, user.Status)
	require.Equal(t, 4.5, user.Balance)
	require.Zero(t, user.TotalRecharged)
	history, err := NewRedeemCodeRepository(integrationEntClient).ListByUser(ctx, user.ID, 10)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, 4.5, history[0].Value)
	require.Equal(t, "候补名单通过赠送", history[0].Notes)
	after, err := integrationEntClient.WaitlistEntry.Get(ctx, entry.ID)
	require.NoError(t, err)
	require.Equal(t, user.ID, *after.GrantedUserID)
	require.False(t, settings.IsRegistrationEnabled(ctx))
	_, _, err = auth.RegisterWithVerification(ctx, "unapproved@example.com", "test-password", "123456", "", "", "")
	require.ErrorIs(t, err, service.ErrRegDisabled)
	_, _, err = auth.RegisterWithVerification(ctx, email, "test-password", "123456", "", "", "")
	require.ErrorIs(t, err, service.ErrWaitlistSignInRequired)
	_, err = repo.Approve(ctx, entry.ID, 7, 99)
	require.NoError(t, err)
	saved, err := userRepo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, 4.5, saved.Balance)
	history, err = NewRedeemCodeRepository(integrationEntClient).ListByUser(ctx, user.ID, 10)
	require.NoError(t, err)
	require.Len(t, history, 1)
}

func TestWaitlistApprovalGiftHistoryFailureRollsBack(t *testing.T) {
	ctx := context.Background()
	email := fmt.Sprintf("gift-rollback-%d@example.com", time.Now().UnixNano())
	user, err := integrationEntClient.User.Create().SetEmail(email).SetPasswordHash("unchanged").SetBalance(2).SetStatus(service.StatusDisabled).Save(ctx)
	require.NoError(t, err)
	repo := NewWaitlistRepository(integrationEntClient)
	require.NoError(t, repo.Join(ctx, email))
	entry, err := integrationEntClient.WaitlistEntry.Query().Where(waitlistentry.EmailEQ(email)).Only(ctx)
	require.NoError(t, err)
	conflict, err := integrationEntClient.RedeemCode.Create().SetCode(fmt.Sprintf("WL-GIFT-%d", entry.ID)).SetStatus(service.StatusUsed).Save(ctx)
	require.NoError(t, err)
	_, err = repo.Approve(ctx, entry.ID, 7, 5)
	require.ErrorContains(t, err, "record waitlist gift")
	entry, err = integrationEntClient.WaitlistEntry.Get(ctx, entry.ID)
	require.NoError(t, err)
	require.Nil(t, entry.ApprovedAt)
	require.Nil(t, entry.GrantedUserID)
	after, err := integrationEntClient.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusDisabled, after.Status)
	require.Equal(t, 2.0, after.Balance)
	require.NoError(t, integrationEntClient.RedeemCode.DeleteOneID(conflict.ID).Exec(ctx))
	_, err = repo.Approve(ctx, entry.ID, 7, 5)
	require.NoError(t, err)
	after, err = integrationEntClient.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, after.Status)
	require.Equal(t, 7.0, after.Balance)
}

func TestWaitlistApprovalZeroAndInvalidGift(t *testing.T) {
	ctx := context.Background()
	for _, amount := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		t.Run(fmt.Sprint(amount), func(t *testing.T) {
			email := fmt.Sprintf("gift-amount-%d@example.com", time.Now().UnixNano())
			user, err := integrationEntClient.User.Create().SetEmail(email).SetPasswordHash("test").SetBalance(2).SetStatus(service.StatusDisabled).Save(ctx)
			require.NoError(t, err)
			repo := NewWaitlistRepository(integrationEntClient)
			require.NoError(t, repo.Join(ctx, email))
			entry, err := integrationEntClient.WaitlistEntry.Query().Where(waitlistentry.EmailEQ(email)).Only(ctx)
			require.NoError(t, err)
			_, err = repo.Approve(ctx, entry.ID, 7, amount)
			if amount == 0 {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "invalid waitlist gift balance")
			}
			after, err := integrationEntClient.User.Get(ctx, user.ID)
			require.NoError(t, err)
			require.Equal(t, 2.0, after.Balance)
			entry, err = integrationEntClient.WaitlistEntry.Get(ctx, entry.ID)
			require.NoError(t, err)
			if amount == 0 {
				require.Equal(t, service.StatusActive, after.Status)
				require.NotNil(t, entry.ApprovedAt)
			} else {
				require.Equal(t, service.StatusDisabled, after.Status)
				require.Nil(t, entry.ApprovedAt)
			}
			count, err := integrationEntClient.RedeemCode.Query().Where(redeemcode.UsedByEQ(user.ID)).Count(ctx)
			require.NoError(t, err)
			require.Zero(t, count)
		})
	}
}
