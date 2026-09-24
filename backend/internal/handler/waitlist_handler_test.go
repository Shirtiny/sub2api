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
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type waitlistTestRepo struct {
	service.WaitlistRepository
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

func (waitlistTestBranding) GetDefaultBalance(context.Context) float64 { return 0 }

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
	h := &WaitlistHandler{waitlist: service.NewWaitlistService(repo, &waitlistTestMailer{}, waitlistTestBranding{}, nil, nil), challenge: challenge}
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
	h := &WaitlistHandler{waitlist: service.NewWaitlistService(repo, &waitlistTestMailer{}, waitlistTestBranding{}, nil, nil)}
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
	h := &WaitlistHandler{waitlist: service.NewWaitlistService(repo, mailer, waitlistTestBranding{}, nil, nil), challenge: &waitlistTestChallenge{}}
	failed := waitlistRequest(h, `{"email":"a@example.com"}`)
	require.Equal(t, http.StatusServiceUnavailable, failed.Code)
	require.Contains(t, failed.Body.String(), "WAITLIST_CONFIRMATION_FAILED")
	require.NotContains(t, failed.Body.String(), "private SMTP failure")
	require.False(t, repo.sent)
	mailer.err = nil
	require.Equal(t, http.StatusOK, waitlistRequest(h, `{"email":"a@example.com"}`).Code)
	require.True(t, repo.sent)
}

func (waitlistTestBranding) GetFrontendURL(context.Context) string { return "https://example.com" }

type waitlistApproveRepo struct {
	waitlistTestRepo
	id, adminID int64
}

func (r *waitlistApproveRepo) Approve(_ context.Context, id, adminID int64, _ float64) (*service.WaitlistEntry, error) {
	r.id, r.adminID = id, adminID
	if r.err != nil {
		return nil, r.err
	}
	return &service.WaitlistEntry{ID: id, Email: "approved@example.com"}, nil
}
func (*waitlistApproveRepo) ClaimApprovalNotice(context.Context, int64, time.Time) (bool, error) {
	return true, nil
}
func (*waitlistApproveRepo) FinishApprovalNotice(context.Context, int64, time.Time, bool) error {
	return nil
}

func TestWaitlistHandlerApprove(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &waitlistApproveRepo{}
	mailer := &waitlistTestMailer{}
	h := NewWaitlistHandler(service.NewWaitlistService(repo, mailer, waitlistTestBranding{}, nil, nil), nil)
	r := gin.New()
	r.POST("/waitlist/:id/approve", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		h.Approve(c)
	})
	request := func(id string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		// A client-supplied administrator ID must never override the authenticated subject.
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/waitlist/"+id+"/approve", strings.NewReader(`{"approved_by":1}`)))
		return w
	}
	for _, id := range []string{"0", "-1", "abc", "9223372036854775808"} {
		require.Equal(t, http.StatusBadRequest, request(id).Code)
	}
	require.Zero(t, repo.id)
	w := request("12")
	require.Equal(t, http.StatusOK, w.Code)
	require.EqualValues(t, 12, repo.id)
	require.EqualValues(t, 42, repo.adminID)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	repo.err = service.ErrWaitlistNotFound
	require.Equal(t, http.StatusNotFound, request("99").Code)
	repo.err = nil
	mailer.err = errors.New("private SMTP failure")
	w = request("12")
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), "WAITLIST_APPROVAL_NOTICE_FAILED")
	require.NotContains(t, w.Body.String(), "private SMTP")
}
