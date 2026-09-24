package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type waitlistApprovalRepoStub struct {
	waitlistRepoStub
	entry             WaitlistEntry
	approveErr        error
	approvals         int
	giftBalance       float64
	approvalSent      bool
	approvalInFlight  bool
	approvalFinishErr error
	allowedEmail      string
	lookupErr         error
}

func (r *waitlistApprovalRepoStub) Approve(_ context.Context, id, adminID int64, giftBalance float64) (*WaitlistEntry, error) {
	if r.approveErr != nil {
		return nil, r.approveErr
	}
	if r.entry.ApprovedAt == nil {
		now := time.Now()
		r.entry.ID, r.entry.ApprovedAt, r.entry.ApprovedBy = id, &now, &adminID
		r.approvals++
		r.giftBalance = giftBalance
	}
	return &r.entry, nil
}
func (r *waitlistApprovalRepoStub) ClaimApprovalNotice(context.Context, int64, time.Time) (bool, error) {
	if r.approvalSent {
		return false, nil
	}
	if r.approvalInFlight {
		return false, ErrWaitlistApprovalNoticeFailed
	}
	r.approvalInFlight = true
	return true, nil
}
func (r *waitlistApprovalRepoStub) FinishApprovalNotice(ctx context.Context, _ int64, _ time.Time, sent bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.approvalFinishErr != nil {
		return r.approvalFinishErr
	}
	r.approvalSent, r.approvalInFlight = sent, false
	return nil
}
func (r *waitlistApprovalRepoStub) HasRegistrationApproval(_ context.Context, email string) (bool, error) {
	return email != "" && email == r.allowedEmail, r.lookupErr
}

func (r *waitlistApprovalRepoStub) GetApprovedEntryByEmail(_ context.Context, email string) (*WaitlistEntry, error) {
	if r.lookupErr != nil {
		return nil, r.lookupErr
	}
	if email != "" && email == r.entry.Email && r.entry.ApprovedAt != nil {
		return &r.entry, nil
	}
	if email != "" && email == r.allowedEmail {
		now := time.Now()
		return &WaitlistEntry{Email: email, ApprovedAt: &now}, nil
	}
	return nil, nil
}

type waitlistCacheSpy struct {
	APIKeyAuthCacheInvalidator
	users []int64
}

type waitlistGiftSettings struct {
	waitlistBrandingStub
	balance float64
}

func (s *waitlistGiftSettings) GetDefaultBalance(context.Context) float64 { return s.balance }

type waitlistBalanceCacheSpy struct {
	BillingCache
	users []int64
	err   error
}

func (s *waitlistBalanceCacheSpy) InvalidateUserBalance(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.users = append(s.users, id)
	return s.err
}

func (s *waitlistCacheSpy) InvalidateAuthCacheByUserID(_ context.Context, id int64) {
	s.users = append(s.users, id)
}

func TestWaitlistApprovalNotificationRetry(t *testing.T) {
	id := int64(9)
	repo := &waitlistApprovalRepoStub{entry: WaitlistEntry{Email: "approved@example.com", GrantedUserID: &id}}
	mailer := &waitlistMailerStub{err: ErrEmailNotConfigured}
	cache := &waitlistCacheSpy{}
	settings := &waitlistGiftSettings{waitlistBrandingStub("Test & Site"), 5.25}
	balanceCache := &waitlistBalanceCacheSpy{}
	svc := NewWaitlistService(repo, mailer, settings, cache, &BillingCacheService{cache: balanceCache})
	require.ErrorIs(t, svc.Approve(context.Background(), 1, 7), ErrWaitlistApprovalNoticeFailed)
	require.NotNil(t, repo.entry.ApprovedAt)
	require.Equal(t, int64(7), *repo.entry.ApprovedBy)
	require.False(t, repo.approvalSent)
	require.False(t, repo.approvalInFlight)
	require.Equal(t, []int64{9}, cache.users)
	require.Equal(t, []int64{9}, balanceCache.users)
	require.Equal(t, 5.25, repo.giftBalance)
	mailer.err = nil
	settings.balance = 20 // Retrying notification must not credit the changed default.
	require.NoError(t, svc.Approve(context.Background(), 1, 8))
	require.True(t, repo.approvalSent)
	require.Equal(t, "approved@example.com", mailer.to)
	require.Equal(t, waitlistApprovalSubject, mailer.subject)
	require.Contains(t, mailer.body, waitlistExistingAccountApprovalMessage)
	require.NotContains(t, mailer.body, "/register")
	require.Contains(t, mailer.body, `href="https://example.com/login"`)
	require.Contains(t, mailer.body, "Test &amp; Site")
	require.NoError(t, svc.Approve(context.Background(), 1, 8))
	require.Equal(t, 1, repo.approvals)
	require.Equal(t, int64(7), *repo.entry.ApprovedBy)
	require.Equal(t, 2, mailer.calls)
	require.Equal(t, 5.25, repo.giftBalance)
	require.Equal(t, []int64{9, 9, 9}, balanceCache.users)
}

func TestWaitlistApprovalGiftCacheFailureDoesNotBlockNotice(t *testing.T) {
	for _, registered := range []bool{false, true} {
		t.Run(fmt.Sprint(registered), func(t *testing.T) {
			entry := WaitlistEntry{Email: "approved@example.com"}
			if registered {
				id := int64(9)
				entry.GrantedUserID = &id
			}
			repo := &waitlistApprovalRepoStub{entry: entry}
			cache := &waitlistBalanceCacheSpy{err: errors.New("cache unavailable")}
			svc := NewWaitlistService(repo, &waitlistMailerStub{}, waitlistBrandingStub("Site"), nil, &BillingCacheService{cache: cache})
			require.NoError(t, svc.Approve(context.Background(), 1, 7))
			require.True(t, repo.approvalSent)
			if registered {
				require.Equal(t, []int64{9}, cache.users)
			} else {
				require.Empty(t, cache.users)
			}
		})
	}
}

func TestWaitlistApprovalDoesNotEmailOnApprovalFailure(t *testing.T) {
	repo := &waitlistApprovalRepoStub{approveErr: ErrWaitlistNotFound}
	mailer := &waitlistMailerStub{}
	svc := NewWaitlistService(repo, mailer, waitlistBrandingStub("Site"), nil, nil)
	require.ErrorIs(t, svc.Approve(context.Background(), 1, 7), ErrWaitlistNotFound)
	require.Error(t, svc.Approve(context.Background(), 0, 7))
	require.Error(t, svc.Approve(context.Background(), 1, 0))
	require.Zero(t, mailer.calls)
}

func TestWaitlistApprovalNoticeClaimAndPersistenceErrors(t *testing.T) {
	for _, inFlight := range []bool{false, true} {
		repo := &waitlistApprovalRepoStub{entry: WaitlistEntry{Email: "a@example.com"}, approvalInFlight: inFlight, approvalFinishErr: errors.New("db failed")}
		mailer := &waitlistMailerStub{}
		svc := NewWaitlistService(repo, mailer, waitlistBrandingStub("Site"), nil, nil)
		require.ErrorIs(t, svc.Approve(context.Background(), 1, 7), ErrWaitlistApprovalNoticeFailed)
		require.False(t, repo.approvalSent)
		if inFlight {
			require.Zero(t, mailer.calls)
		} else {
			require.Equal(t, 1, mailer.calls)
		}
	}
}

func TestWaitlistApprovalNoticeSurvivesBrowserDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo := &waitlistApprovalRepoStub{entry: WaitlistEntry{Email: "a@example.com"}}
	mailer := &waitlistMailerStub{send: func(ctx context.Context) error {
		cancel()
		require.NoError(t, ctx.Err())
		_, bounded := ctx.Deadline()
		require.True(t, bounded)
		return nil
	}}
	require.NoError(t, NewWaitlistService(repo, mailer, waitlistBrandingStub("Site"), nil, nil).Approve(ctx, 1, 7))
	require.True(t, repo.approvalSent)
}

func TestWaitlistApprovalHTML(t *testing.T) {
	body := waitlistApprovalHTML(`Site <script> & "x"`, "https://example.com/app/?untrusted=1#fragment", false)
	require.Equal(t, 1, strings.Count(body, waitlistApprovalMessage))
	require.Contains(t, body, "Site &lt;script&gt; &amp; &#34;x&#34;")
	require.Contains(t, body, `href="https://example.com/app/register?waitlist=1"`)
	require.Contains(t, body, `href="https://example.com/app/login"`)
	require.NotContains(t, body, "untrusted")
	require.NotContains(t, body, "{{")
	require.NotContains(t, body, "<script>")
	for _, invalid := range []string{"", "javascript:alert(1)", "//example.com", "https://u:p@example.com", "/app", "https://"} {
		body := waitlistApprovalHTML("", invalid, false)
		require.NotContains(t, body, "href=")
		require.Contains(t, body, "候补审批注册")
		require.Contains(t, body, "Sub2API")
	}
}

func TestWaitlistApprovalHTMLExistingAccount(t *testing.T) {
	for _, base := range []string{"https://cafeshop.ai", ""} {
		body := waitlistApprovalHTML("Café Shop", base, true)
		require.Contains(t, body, waitlistExistingAccountApprovalMessage)
		require.Contains(t, body, "无需重新注册")
		require.NotContains(t, body, waitlistApprovalMessage)
		require.NotContains(t, body, "/register")
		require.NotContains(t, body, "验证邮箱并注册")
		if base != "" {
			require.Contains(t, body, `href="https://cafeshop.ai/login"`)
			require.Equal(t, 1, strings.Count(body, "href="))
		} else {
			require.NotContains(t, body, "href=")
		}
	}
}
