package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type waitlistTestRepo struct {
	emails []string
	err    error
	sent   bool
}

func (r *waitlistTestRepo) Join(_ context.Context, email string) error {
	r.emails = append(r.emails, email)
	return r.err
}
func (r *waitlistTestRepo) ClaimConfirmation(context.Context, string, time.Time) (bool, error) {
	return !r.sent, nil
}
func (r *waitlistTestRepo) FinishConfirmation(_ context.Context, _ string, _ time.Time, sent bool) error {
	r.sent = sent
	return nil
}

type waitlistTestBranding struct{}

func (waitlistTestBranding) GetSiteName(context.Context) string { return "Test site" }

type waitlistTestMailer struct{ err error }

func (m *waitlistTestMailer) SendEmail(context.Context, string, string, string) error { return m.err }

func (r *waitlistTestRepo) List(context.Context, pagination.PaginationParams) ([]service.WaitlistEntry, int64, error) {
	return []service.WaitlistEntry{{ID: 1, Email: "a@example.com"}}, 1, r.err
}

type waitlistTestChallenge struct {
	err   error
	token string
	calls int
}

func (s *waitlistTestChallenge) VerifyToken(_ context.Context, token, _ string) error {
	s.token = token
	s.calls++
	return s.err
}
func waitlistRequest(h *WaitlistHandler, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/waitlist", h.Join)
	req := httptest.NewRequest(http.MethodPost, "/waitlist", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
func TestWaitlistHandlerJoin(t *testing.T) {
	repo := &waitlistTestRepo{}
	challenge := &waitlistTestChallenge{}
	h := &WaitlistHandler{waitlist: service.NewWaitlistService(repo, &waitlistTestMailer{}, waitlistTestBranding{}), challenge: challenge}
	for _, body := range []string{`{}`, `{"email":"bad"}`, `{"email":3}`, `{`, `{"email":"` + strings.Repeat("a", 5000) + `"}`} {
		require.Equal(t, http.StatusBadRequest, waitlistRequest(h, body).Code)
	}
	require.Empty(t, repo.emails)
	require.Zero(t, challenge.calls)
	first := waitlistRequest(h, `{"email":" A@Example.com ","turnstile_token":"proof"}`)
	second := waitlistRequest(h, `{"email":"a@example.com","turnstile_token":"proof"}`)
	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, first.Body.String(), second.Body.String())
	require.NotContains(t, first.Body.String(), "example.com")
	require.Equal(t, "proof", challenge.token)
	require.Equal(t, []string{"a@example.com", "a@example.com"}, repo.emails)
	challenge.err = service.ErrTurnstileVerificationFailed
	require.NotEqual(t, http.StatusOK, waitlistRequest(h, `{"email":"b@example.com"}`).Code)
	require.Len(t, repo.emails, 2)
	challenge.err = nil
	repo.err = errors.New("database unavailable")
	result := waitlistRequest(h, `{"email":"b@example.com"}`)
	require.Equal(t, http.StatusInternalServerError, result.Code)
	require.NotContains(t, result.Body.String(), "database unavailable")
}
func TestWaitlistHandlerList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &waitlistTestRepo{}
	h := &WaitlistHandler{waitlist: service.NewWaitlistService(repo, &waitlistTestMailer{}, waitlistTestBranding{})}
	r := gin.New()
	r.GET("/waitlist", h.List)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/waitlist?page=1&page_size=20", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"email":"a@example.com"`)
	require.Contains(t, w.Body.String(), `"total":1`)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}

func TestWaitlistHandlerMailFailureIsRetryable(t *testing.T) {
	repo := &waitlistTestRepo{}
	mailer := &waitlistTestMailer{err: errors.New("private SMTP failure")}
	h := &WaitlistHandler{waitlist: service.NewWaitlistService(repo, mailer, waitlistTestBranding{}), challenge: &waitlistTestChallenge{}}
	failed := waitlistRequest(h, `{"email":"a@example.com"}`)
	require.Equal(t, http.StatusServiceUnavailable, failed.Code)
	require.Contains(t, failed.Body.String(), "WAITLIST_CONFIRMATION_FAILED")
	require.NotContains(t, failed.Body.String(), "private SMTP failure")
	require.False(t, repo.sent)
	mailer.err = nil
	require.Equal(t, http.StatusOK, waitlistRequest(h, `{"email":"a@example.com"}`).Code)
	require.True(t, repo.sent)
}
