package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type waitlistBrandingStub string

func (b waitlistBrandingStub) GetSiteName(context.Context) string { return string(b) }

type waitlistRepoStub struct {
	WaitlistRepository
	emails    []string
	err       error
	sent      bool
	inFlight  bool
	finishErr error
	claims    int
}

func (r *waitlistRepoStub) Join(_ context.Context, email string) error {
	r.emails = append(r.emails, email)
	return r.err
}
func (r *waitlistRepoStub) ClaimConfirmation(_ context.Context, _ string, _ time.Time) (bool, error) {
	r.claims++
	if r.sent {
		return false, nil
	}
	if r.inFlight {
		return false, ErrWaitlistConfirmationFailed
	}
	r.inFlight = true
	return true, nil
}
func (r *waitlistRepoStub) FinishConfirmation(ctx context.Context, _ string, _ time.Time, sent bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.finishErr != nil {
		return r.finishErr
	}
	r.sent = sent
	r.inFlight = false
	return nil
}

type waitlistMailerStub struct {
	to, subject, body string
	calls             int
	err               error
	send              func(context.Context) error
}

func (m *waitlistMailerStub) SendEmail(ctx context.Context, to, subject, body string) error {
	m.calls++
	m.to, m.subject, m.body = to, subject, body
	if m.send != nil {
		return m.send(ctx)
	}
	return m.err
}

func (r *waitlistRepoStub) List(context.Context, pagination.PaginationParams) ([]WaitlistEntry, int64, error) {
	return []WaitlistEntry{}, 0, r.err
}

func TestNormalizeWaitlistEmail(t *testing.T) {
	for _, email := range []string{"person@example.com", "  Person+launch@EXAMPLE.COM \n", "a.b@sub.example.co.uk"} {
		got, err := NormalizeWaitlistEmail(email)
		require.NoError(t, err)
		require.Equal(t, strings.ToLower(strings.TrimSpace(email)), got)
	}
	for _, email := range []string{"", "   ", "a", "a@", "@example.com", "a@localhost", "a@@example.com", "a b@example.com", "Name <a@example.com>", ".a@example.com", "a.@example.com", "a..b@example.com", "a@-example.com", "a@example-.com", "a@example..com", "a@example.com.", "a@127.0.0.1", "a@example.c\nom", "a@exa_mple.com", "a@例子.cn", strings.Repeat("a", 65) + "@example.com", "a@" + strings.Repeat("x", 64) + ".com", "a@" + strings.Repeat("a.", 126) + "com"} {
		t.Run(email, func(t *testing.T) {
			_, err := NormalizeWaitlistEmail(email)
			require.ErrorIs(t, err, ErrWaitlistEmailInvalid)
		})
	}
}

func TestWaitlistServiceValidationAndPersistence(t *testing.T) {
	repo := &waitlistRepoStub{}
	mailer := &waitlistMailerStub{}
	svc := NewWaitlistService(repo, mailer, waitlistBrandingStub("Café Shop"), nil)
	require.ErrorIs(t, svc.Join(context.Background(), "bad-email"), ErrWaitlistEmailInvalid)
	require.Empty(t, repo.emails)
	require.Zero(t, mailer.calls)
	require.NoError(t, svc.Join(context.Background(), " A+tag@Example.COM "))
	require.Equal(t, []string{"a+tag@example.com"}, repo.emails)
	require.Equal(t, "a+tag@example.com", mailer.to)
	require.Equal(t, "Waiting List 申请成功", mailer.subject)
	require.Contains(t, mailer.body, "欢迎，您已经加入到Waiting List，请耐心等待。关注邮件消息，开放后会即时通知。")
	require.Contains(t, mailer.body, "Café Shop")
	require.True(t, repo.sent)
	require.NoError(t, svc.Join(context.Background(), "A+tag@EXAMPLE.COM"))
	require.Equal(t, 1, mailer.calls)
	repo.err = errors.New("database unavailable")
	require.ErrorIs(t, svc.Join(context.Background(), "a@example.com"), repo.err)
}

func TestWaitlistConfirmationRetryAfterSMTPFailure(t *testing.T) {
	repo := &waitlistRepoStub{}
	mailer := &waitlistMailerStub{err: ErrEmailNotConfigured}
	svc := NewWaitlistService(repo, mailer, waitlistBrandingStub("Café Shop"), nil)
	err := svc.Join(context.Background(), "a@example.com")
	require.ErrorIs(t, err, ErrWaitlistConfirmationFailed)
	require.ErrorIs(t, err, ErrEmailNotConfigured)
	require.Len(t, repo.emails, 1) // Application survives a delivery failure.
	require.False(t, repo.sent)
	require.False(t, repo.inFlight)
	mailer.err = nil
	require.NoError(t, svc.Join(context.Background(), "a@example.com"))
	require.True(t, repo.sent)
	require.Equal(t, 2, mailer.calls)
	require.NoError(t, svc.Join(context.Background(), "a@example.com"))
	require.Equal(t, 2, mailer.calls)
}

func TestWaitlistConfirmationSkipsActiveClaim(t *testing.T) {
	repo := &waitlistRepoStub{inFlight: true}
	mailer := &waitlistMailerStub{}
	err := NewWaitlistService(repo, mailer, waitlistBrandingStub("Café Shop"), nil).Join(context.Background(), "a@example.com")
	require.ErrorIs(t, err, ErrWaitlistConfirmationFailed)
	require.Zero(t, mailer.calls)
}

func TestWaitlistConfirmationPreservesSentMarkerAfterBrowserDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo := &waitlistRepoStub{}
	mailer := &waitlistMailerStub{send: func(mailCtx context.Context) error {
		cancel()
		require.NoError(t, mailCtx.Err())
		_, bounded := mailCtx.Deadline()
		require.True(t, bounded)
		return nil
	}}
	require.NoError(t, NewWaitlistService(repo, mailer, waitlistBrandingStub("Café Shop"), nil).Join(ctx, "a@example.com"))
	require.True(t, repo.sent)
}

func TestWaitlistConfirmationDoesNotHidePersistenceFailure(t *testing.T) {
	repo := &waitlistRepoStub{finishErr: errors.New("database unavailable")}
	mailer := &waitlistMailerStub{}
	err := NewWaitlistService(repo, mailer, waitlistBrandingStub("Café Shop"), nil).Join(context.Background(), "a@example.com")
	require.ErrorIs(t, err, ErrWaitlistConfirmationFailed)
	require.ErrorIs(t, err, repo.finishErr)
	require.False(t, repo.sent)
}

func (b waitlistBrandingStub) GetFrontendURL(context.Context) string { return "https://example.com" }
