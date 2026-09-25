package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

type noticeRepoStub struct {
	mu         sync.Mutex
	rows       []PresaleNoticeRecipient
	restricted []PresaleNoticeRecipient
	states     map[string]map[string]string
	finishErr  error
}

func (r *noticeRepoStub) Recipients(_ context.Context, include bool) ([]PresaleNoticeRecipient, error) {
	if include {
		return append(append([]PresaleNoticeRecipient{}, r.rows...), r.restricted...), nil
	}
	return r.rows, nil
}
func (r *noticeRepoStub) States(_ context.Context, key string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := map[string]string{}
	for k, v := range r.states[key] {
		m[k] = v
	}
	return m, nil
}
func (r *noticeRepoStub) Claim(_ context.Context, key, hash string, _, _ int64, _ string, _ bool) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.states[key] == nil {
		r.states[key] = map[string]string{}
	}
	if r.states[key][hash] != "" {
		return false, nil
	}
	r.states[key][hash] = "sending"
	return true, nil
}
func (r *noticeRepoStub) Finish(_ context.Context, key, hash, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finishErr != nil {
		return r.finishErr
	}
	r.states[key][hash] = status
	return nil
}

type noticeCatalogStub struct {
	plans      []*dbent.SubscriptionPlan
	published  bool
	enabled    bool
	activities []PublicPresaleActivity
}

func (c *noticeCatalogStub) ListPresalePlans(context.Context) ([]*dbent.SubscriptionPlan, error) {
	if c.published {
		return c.plans, nil
	}
	return nil, nil
}
func (c *noticeCatalogStub) ListPlans(context.Context) ([]*dbent.SubscriptionPlan, error) {
	return c.plans, nil
}
func (c *noticeCatalogStub) GetPaymentConfig(context.Context) (*PaymentConfig, error) {
	return &PaymentConfig{Enabled: c.enabled}, nil
}
func (c *noticeCatalogStub) PublicPresaleActivities(context.Context, []int64, time.Time) ([]PublicPresaleActivity, error) {
	return c.activities, nil
}

type noticeSettingsStub struct{ name, url string }

func (s noticeSettingsStub) GetSiteName(context.Context) string    { return s.name }
func (s noticeSettingsStub) GetFrontendURL(context.Context) string { return s.url }

type noticeMailerStub struct {
	mu                sync.Mutex
	calls             int
	to, subject, body string
	err               error
}

func (m *noticeMailerStub) GetSMTPConfig(context.Context) (*SMTPConfig, error) {
	return &SMTPConfig{Host: "smtp.example.com", From: "support@cafeshop.ai"}, nil
}
func (m *noticeMailerStub) SendEmail(_ context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.to, m.subject, m.body = to, subject, body
	return m.err
}
func newNoticeTestService() (*PresaleNotificationService, *noticeRepoStub, *noticeCatalogStub, *noticeMailerStub) {
	repo := &noticeRepoStub{rows: []PresaleNoticeRecipient{{UserID: 1, Email: "person@example.com"}}, restricted: []PresaleNoticeRecipient{{UserID: 2, Email: "pending@example.com"}}, states: map[string]map[string]string{}}
	catalog := &noticeCatalogStub{published: true, enabled: true, plans: []*dbent.SubscriptionPlan{{ID: 2, Name: "小杯", Price: 139, PresaleEnabled: true, PresaleBadge: "Plus"}}}
	mailer := &noticeMailerStub{}
	svc := &PresaleNotificationService{repo: repo, catalog: catalog, settings: noticeSettingsStub{"Café Shop", "https://cafeshop.ai"}, mailer: mailer, notifications: NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil), now: func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, presaleLocation) }}
	return svc, repo, catalog, mailer
}
func noticeRequest(t *testing.T, s *PresaleNotificationService, include bool) PresaleNoticeRequest {
	t.Helper()
	p, err := s.Preview(context.Background(), include, "zh")
	require.NoError(t, err)
	return PresaleNoticeRequest{Month: p.Month, Version: p.Version, Locale: "zh", IncludeRestricted: include, Confirmed: true}
}
func TestPresaleNoticePreviewDraftAndTestOnly(t *testing.T) {
	s, repo, catalog, mailer := newNoticeTestService()
	catalog.published = false
	p, err := s.Preview(context.Background(), false, "zh")
	require.NoError(t, err)
	require.False(t, p.Ready)
	require.True(t, p.Draft)
	require.Contains(t, p.Subject, "2026 年 10 月")
	require.Contains(t, p.HTML, "￥139")
	require.Contains(t, p.HTML, `href="https://cafeshop.ai/presale"`)
	require.Zero(t, mailer.calls)
	req := noticeRequest(t, s, false)
	_, err = s.SendNext(context.Background(), req, 1)
	require.ErrorContains(t, err, "预售尚未开放")
	require.Empty(t, repo.states)
	req.Email = "owner@example.com"
	_, err = s.SendTest(context.Background(), req, 1)
	require.NoError(t, err)
	require.Contains(t, mailer.subject, "Preview")
	require.Empty(t, repo.states[req.Month])
	require.Equal(t, "sent", repo.states["test"][notificationEmailHash(req.Email)])
	_, err = s.SendTest(context.Background(), req, 1)
	require.ErrorContains(t, err, "60 秒")
	require.Equal(t, 1, mailer.calls)
}
func TestPresaleNoticeReviewBindings(t *testing.T) {
	cases := []struct {
		name   string
		change func(*PresaleNotificationService, *noticeCatalogStub, *PresaleNoticeRequest)
	}{
		{"confirmation", func(_ *PresaleNotificationService, _ *noticeCatalogStub, r *PresaleNoticeRequest) {
			r.Confirmed = false
		}},
		{"audience", func(_ *PresaleNotificationService, _ *noticeCatalogStub, r *PresaleNoticeRequest) {
			r.IncludeRestricted = true
		}},
		{"month", func(s *PresaleNotificationService, _ *noticeCatalogStub, _ *PresaleNoticeRequest) {
			s.now = func() time.Time { return time.Date(2026, 10, 1, 0, 0, 0, 0, presaleLocation) }
		}},
		{"price", func(_ *PresaleNotificationService, c *noticeCatalogStub, _ *PresaleNoticeRequest) {
			c.plans[0].Price = 150
		}},
		{"not open", func(_ *PresaleNotificationService, c *noticeCatalogStub, _ *PresaleNoticeRequest) { c.enabled = false }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, repo, c, mailer := newNoticeTestService()
			req := noticeRequest(t, s, false)
			tc.change(s, c, &req)
			_, err := s.SendNext(context.Background(), req, 1)
			require.Error(t, err)
			require.Zero(t, mailer.calls)
			require.Empty(t, repo.states)
		})
	}
}
func TestPresaleNoticeMonthlyDedupAndUnsubscribe(t *testing.T) {
	s, repo, _, mailer := newNoticeTestService()
	repo.rows = append(repo.rows, PresaleNoticeRecipient{UserID: 3, Email: " PERSON@EXAMPLE.COM "}, PresaleNoticeRecipient{Email: "invalid"})
	req := noticeRequest(t, s, false)
	_, err := s.SendNext(context.Background(), req, 1)
	require.NoError(t, err)
	require.Equal(t, 1, mailer.calls)
	require.Contains(t, mailer.body, "不再接收预售通知")
	require.Contains(t, mailer.body, "https://cafeshop.ai/api/v1/settings/email-unsubscribe?token=")
	result, err := s.SendNext(context.Background(), req, 1)
	require.NoError(t, err)
	require.True(t, result.Done)
	require.Equal(t, 1, mailer.calls)
	req = noticeRequest(t, s, true)
	require.NoError(t, s.notifications.settingRepo.Set(context.Background(), notificationEmailPreferenceKey(NotificationEmailEventPresaleOpening, "pending@example.com"), "unsubscribed"))
	result, err = s.SendNext(context.Background(), req, 1)
	require.NoError(t, err)
	require.Equal(t, "skipped", result.Status)
	require.Equal(t, 1, mailer.calls)
	p, err := s.Preview(context.Background(), true, "zh")
	require.NoError(t, err)
	require.Equal(t, PresaleNoticeCounts{Eligible: 2, Sent: 1, Skipped: 1}, p.Counts)
}
func TestPresaleNoticeConcurrentClaimsAndUncertainDelivery(t *testing.T) {
	s, _, _, mailer := newNoticeTestService()
	req := noticeRequest(t, s, false)
	// Initialize unsubscribe signing once; claims remain the concurrency boundary.
	_, err := s.notifications.createUnsubscribeToken(context.Background(), "person@example.com", NotificationEmailEventPresaleOpening)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = s.SendNext(context.Background(), req, 1) }()
	}
	wg.Wait()
	require.Equal(t, 1, mailer.calls)
	s, repo, _, mailer := newNoticeTestService()
	req = noticeRequest(t, s, false)
	mailer.err = errors.New("SMTP timeout, secret must not be exposed")
	_, err = s.SendNext(context.Background(), req, 1)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret")
	require.Equal(t, "uncertain", repo.states[req.Month][notificationEmailHash("person@example.com")])
	result, err := s.SendNext(context.Background(), req, 1)
	require.NoError(t, err)
	require.True(t, result.Done)
	require.Equal(t, 1, mailer.calls)
}
func TestPresaleNoticeHTMLAndURLSafety(t *testing.T) {
	s, _, catalog, _ := newNoticeTestService()
	s.settings = noticeSettingsStub{`<script>name</script>`, "https://cafeshop.ai/base/?old=1#x"}
	catalog.plans[0].Name = `<img src=x onerror=alert(1)>`
	p, err := s.Preview(context.Background(), false, "en")
	require.NoError(t, err)
	require.NotContains(t, p.HTML, "<script>name")
	require.Contains(t, p.HTML, "&lt;img")
	require.Contains(t, p.HTML, `href="https://cafeshop.ai/base/presale"`)
	for _, bad := range []string{"javascript:alert(1)", "https://user:pass@example.com", "//example.com", ""} {
		s.settings = noticeSettingsStub{"Cafe", bad}
		_, err = s.Preview(context.Background(), false, "zh")
		require.Error(t, err)
	}
}

func TestPresaleNoticeReceiptFailureDoesNotRetry(t *testing.T) {
	s, repo, _, mailer := newNoticeTestService()
	req := noticeRequest(t, s, false)
	repo.finishErr = errors.New("database unavailable")
	_, err := s.SendNext(context.Background(), req, 1)
	require.ErrorContains(t, err, "发送记录暂未确认")
	require.Equal(t, 1, mailer.calls)
	require.Equal(t, "sending", repo.states[req.Month][notificationEmailHash("person@example.com")])
	result, err := s.SendNext(context.Background(), req, 1)
	require.NoError(t, err)
	require.True(t, result.Done)
	require.Equal(t, 1, mailer.calls)
}
