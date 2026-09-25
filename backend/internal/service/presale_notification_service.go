package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type PresaleNoticeRecipient struct {
	UserID int64
	Email  string
}
type PresaleNotificationRepository interface {
	Recipients(context.Context, bool) ([]PresaleNoticeRecipient, error)
	States(context.Context, string) (map[string]string, error)
	Claim(context.Context, string, string, int64, int64, string, bool) (bool, error)
	Finish(context.Context, string, string, string) error
}
type presaleNoticeCatalog interface {
	ListPresalePlans(context.Context) ([]*dbent.SubscriptionPlan, error)
	ListPlans(context.Context) ([]*dbent.SubscriptionPlan, error)
	GetPaymentConfig(context.Context) (*PaymentConfig, error)
	PublicPresaleActivities(context.Context, []int64, time.Time) ([]PublicPresaleActivity, error)
	GetPresaleNoticeCoupon(context.Context, time.Time) (string, *PresaleNoticeCoupon, error)
	SavePresaleNoticeCoupon(context.Context, string, time.Time) (string, error)
}
type presaleNoticeSettings interface {
	GetSiteName(context.Context) string
	GetFrontendURL(context.Context) string
}
type presaleNoticeMailer interface {
	SendEmail(context.Context, string, string, string) error
	GetSMTPConfig(context.Context) (*SMTPConfig, error)
}

type PresaleNoticeCounts struct {
	Eligible  int `json:"eligible"`
	Sent      int `json:"sent"`
	Skipped   int `json:"skipped"`
	Uncertain int `json:"uncertain"`
	Sending   int `json:"sending"`
	Pending   int `json:"pending"`
}
type PresaleNoticePreview struct {
	CouponCode     string              `json:"coupon_code"`
	CouponIncluded bool                `json:"coupon_included"`
	Month          string              `json:"month"`
	Subject        string              `json:"subject"`
	HTML           string              `json:"html"`
	Version        string              `json:"version"`
	Ready          bool                `json:"ready"`
	Draft          bool                `json:"draft"`
	PublishedPlans int                 `json:"published_plans"`
	Counts         PresaleNoticeCounts `json:"counts"`
}
type PresaleNoticeRequest struct {
	IncludeRestricted bool   `json:"include_restricted"`
	Locale            string `json:"locale"`
	Month             string `json:"month"`
	Version           string `json:"version"`
	Confirmed         bool   `json:"confirmed"`
	Email             string `json:"email"`
}
type PresaleNoticeSendResult struct {
	Status string `json:"status"`
	Done   bool   `json:"done"`
}

type PresaleNotificationService struct {
	repo          PresaleNotificationRepository
	catalog       presaleNoticeCatalog
	settings      presaleNoticeSettings
	mailer        presaleNoticeMailer
	notifications *NotificationEmailService
	now           func() time.Time
}

func NewPresaleNotificationService(repo PresaleNotificationRepository, catalog *PaymentConfigService, settings *SettingService, mailer *EmailService, notifications *NotificationEmailService) *PresaleNotificationService {
	return &PresaleNotificationService{repo: repo, catalog: catalog, settings: settings, mailer: mailer, notifications: notifications, now: time.Now}
}

type presaleNoticeDraft struct {
	preview    *PresaleNoticePreview
	recipients []PresaleNoticeRecipient
	period     PresalePeriod
	plans      []*dbent.SubscriptionPlan
	activities []PublicPresaleActivity
	coupon     *PresaleNoticeCoupon
	base       *url.URL
	template   NotificationEmailTemplate
	variables  map[string]string
	locale     string
	now        time.Time
}

func (s *PresaleNotificationService) draft(ctx context.Context, locale string) (*presaleNoticeDraft, error) {
	locale = normalizeNotificationLocale(locale)
	now := s.now()
	period := NextPresalePeriod(now)
	base, err := presaleNoticeBaseURL(s.settings.GetFrontendURL(ctx))
	if err != nil {
		return nil, infraerrors.BadRequest("PRESALE_NOTICE_URL_INVALID", "请先配置正确的前端地址")
	}
	cfg, err := s.catalog.GetPaymentConfig(ctx)
	if err != nil {
		return nil, err
	}
	plans, err := s.catalog.ListPresalePlans(ctx)
	if err != nil {
		return nil, err
	}
	published := len(plans)
	// A draft is available before publication, but cannot be broadcast.
	if published == 0 {
		all, err := s.catalog.ListPlans(ctx)
		if err != nil {
			return nil, err
		}
		for _, p := range all {
			if p.PresaleEnabled {
				plans = append(plans, p)
			}
		}
	}
	ids := make([]int64, 0, len(plans))
	for _, p := range plans {
		ids = append(ids, p.ID)
	}
	activities, err := s.catalog.PublicPresaleActivities(ctx, ids, now)
	if err != nil {
		return nil, err
	}
	tmpl, err := s.notifications.GetTemplate(ctx, NotificationEmailEventPresaleOpening, locale)
	if err != nil {
		return nil, err
	}
	monthLabel := period.StartsAt.Format("January 2006")
	if locale == "zh" {
		monthLabel = fmt.Sprintf("%d 年 %d 月", period.StartsAt.Year(), period.StartsAt.Month())
	}
	variables := map[string]string{"site_name": s.settings.GetSiteName(ctx), "month_label": monthLabel}
	couponCode, coupon, err := s.catalog.GetPresaleNoticeCoupon(ctx, now)
	if err != nil {
		return nil, err
	}
	content := presaleNoticeContent(locale, period, plans, activities, base, now, "", coupon)
	rendered, err := renderNotificationEmail(NotificationEmailEventPresaleOpening, tmpl.Subject, tmpl.HTML, variables, map[string]string{"presale_content": content})
	if err != nil {
		return nil, err
	}
	ready := cfg.Enabled && published > 0
	// Bind the coupon even when a custom template omits presale_content.
	couponReview, err := json.Marshal(struct {
		Code   string
		Coupon *PresaleNoticeCoupon
	}{couponCode, coupon})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%t\x00%s\x00%s\x00%s", period.Month, ready, rendered.Subject, rendered.HTML, couponReview)))
	preview := &PresaleNoticePreview{CouponCode: couponCode, CouponIncluded: coupon != nil, Month: period.Month, Subject: rendered.Subject, HTML: rendered.HTML, Version: hex.EncodeToString(digest[:]), Ready: ready, Draft: published == 0, PublishedPlans: published}
	return &presaleNoticeDraft{preview: preview, period: period, plans: plans, activities: activities, coupon: coupon, base: base, template: tmpl, variables: variables, locale: locale, now: now}, nil
}

func (s *PresaleNotificationService) recipients(ctx context.Context, includeRestricted bool) ([]PresaleNoticeRecipient, error) {
	rows, err := s.repo.Recipients(ctx, includeRestricted)
	if err != nil {
		return nil, err
	}
	result := make([]PresaleNoticeRecipient, 0, len(rows))
	seen := make(map[string]bool)
	for _, r := range rows {
		normalized, err := NormalizeWaitlistEmail(r.Email)
		if err != nil || strings.HasSuffix(normalized, ".local") || strings.HasSuffix(normalized, ".invalid") || seen[normalized] {
			continue
		}
		r.Email = normalized
		result = append(result, r)
		seen[normalized] = true
	}
	return result, nil
}

func (s *PresaleNotificationService) Preview(ctx context.Context, includeRestricted bool, locale string) (*PresaleNoticePreview, error) {
	draft, err := s.draft(ctx, locale)
	if err != nil {
		return nil, err
	}
	recipients, err := s.recipients(ctx, includeRestricted)
	if err != nil {
		return nil, err
	}
	states, err := s.repo.States(ctx, draft.period.Month)
	if err != nil {
		return nil, err
	}
	counts := PresaleNoticeCounts{Eligible: len(recipients)}
	for _, r := range recipients {
		switch states[notificationEmailHash(r.Email)] {
		case "sent":
			counts.Sent++
		case "skipped":
			counts.Skipped++
		case "uncertain":
			counts.Uncertain++
		case "sending":
			counts.Sending++
		default:
			counts.Pending++
		}
	}
	draft.preview.Version = presaleNoticeReviewVersion(draft.preview.Version, includeRestricted, recipients)
	draft.preview.Counts = counts
	return draft.preview, nil
}

func (s *PresaleNotificationService) validateSend(ctx context.Context, req PresaleNoticeRequest, adminID int64, test bool) (*presaleNoticeDraft, error) {
	if adminID <= 0 || !req.Confirmed {
		return nil, infraerrors.BadRequest("PRESALE_NOTICE_CONFIRM_REQUIRED", "请先确认通知内容与收件范围")
	}
	draft, err := s.draft(ctx, req.Locale)
	if err != nil {
		return nil, err
	}
	recipients, err := s.recipients(ctx, req.IncludeRestricted)
	if err != nil {
		return nil, err
	}
	draft.recipients = recipients
	draft.preview.Version = presaleNoticeReviewVersion(draft.preview.Version, req.IncludeRestricted, recipients)
	if req.Month != draft.period.Month || req.Version != draft.preview.Version {
		return nil, infraerrors.Conflict("PRESALE_NOTICE_CHANGED", "通知内容或预售月份已变化，请刷新预览后重新确认")
	}
	if !test && !draft.preview.Ready {
		return nil, infraerrors.Conflict("PRESALE_NOTICE_NOT_OPEN", "预售尚未开放，请先上架套餐；现在可以发送测试邮件")
	}
	smtp, err := s.mailer.GetSMTPConfig(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(smtp.Host) == "" || strings.TrimSpace(smtp.From) == "" {
		return nil, ErrEmailNotConfigured
	}
	return draft, nil
}

func (s *PresaleNotificationService) SendTest(ctx context.Context, req PresaleNoticeRequest, adminID int64) (*PresaleNoticeSendResult, error) {
	email, err := NormalizeWaitlistEmail(req.Email)
	if err != nil {
		return nil, infraerrors.BadRequest("PRESALE_NOTICE_EMAIL_INVALID", "请填写一个有效的收件邮箱")
	}
	draft, err := s.validateSend(ctx, req, adminID, true)
	if err != nil {
		return nil, err
	}
	hash := notificationEmailHash(email)
	claimed, err := s.repo.Claim(ctx, "test", hash, 0, adminID, req.Version, true)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, infraerrors.TooManyRequests("PRESALE_NOTICE_TEST_COOLDOWN", "请等待 60 秒后再发送测试邮件")
	}
	// Test mail is requested explicitly; it neither consumes formal recipients nor changes preferences.
	err = s.mailer.SendEmail(ctx, email, "[预览 / Preview] "+draft.preview.Subject, draft.preview.HTML)
	return s.finish("test", hash, err)
}

func (s *PresaleNotificationService) SendNext(ctx context.Context, req PresaleNoticeRequest, adminID int64) (*PresaleNoticeSendResult, error) {
	draft, err := s.validateSend(ctx, req, adminID, false)
	if err != nil {
		return nil, err
	}
	recipients := draft.recipients
	states, err := s.repo.States(ctx, draft.period.Month)
	if err != nil {
		return nil, err
	}
	for _, r := range recipients {
		hash := notificationEmailHash(r.Email)
		if states[hash] != "" {
			continue
		}
		unsubscribed, err := s.notifications.IsUnsubscribed(ctx, r.Email, NotificationEmailEventPresaleOpening)
		if err != nil {
			return nil, err
		}
		var body string
		if !unsubscribed {
			// Derive every link from the trusted frontend setting, never the request Host
			// or API proxy hostname. Token/config failures stop BEFORE claiming/sending.
			token, err := s.notifications.createUnsubscribeToken(ctx, r.Email, NotificationEmailEventPresaleOpening)
			if err != nil {
				return nil, err
			}
			link := draft.base.ResolveReference(&url.URL{Path: "api/v1/settings/email-unsubscribe", RawQuery: "token=" + url.QueryEscape(token)}).String()
			content := presaleNoticeContent(draft.locale, draft.period, draft.plans, draft.activities, draft.base, draft.now, link, draft.coupon)
			rendered, err := renderNotificationEmail(NotificationEmailEventPresaleOpening, draft.template.Subject, draft.template.HTML, draft.variables, map[string]string{"presale_content": content})
			if err != nil {
				return nil, err
			}
			body = rendered.HTML
		}
		claimed, err := s.repo.Claim(ctx, draft.period.Month, hash, r.UserID, adminID, req.Version, false)
		if err != nil {
			return nil, err
		}
		if !claimed {
			continue
		}
		if unsubscribed {
			if err := s.repo.Finish(ctx, draft.period.Month, hash, "skipped"); err != nil {
				return nil, err
			}
			return &PresaleNoticeSendResult{Status: "skipped"}, nil
		}
		err = s.mailer.SendEmail(ctx, r.Email, draft.preview.Subject, body)
		return s.finish(draft.period.Month, hash, err)
	}
	return &PresaleNoticeSendResult{Status: "complete", Done: true}, nil
}

func (s *PresaleNotificationService) finish(campaign, hash string, sendErr error) (*PresaleNoticeSendResult, error) {
	status := "sent"
	if sendErr != nil {
		status = "uncertain"
	}
	// A disconnected browser must not roll back a claimed/accepted delivery.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.repo.Finish(ctx, campaign, hash, status); err != nil {
		return nil, infraerrors.ServiceUnavailable("PRESALE_NOTICE_RECEIPT_FAILED", "发送记录暂未确认，请检查后再操作，勿重复发送")
	}
	if sendErr != nil {
		return nil, infraerrors.ServiceUnavailable("PRESALE_NOTICE_SEND_UNCONFIRMED", "邮件发送结果未确认，已停止发送，请检查邮件服务记录，系统不会自动重发")
	}
	return &PresaleNoticeSendResult{Status: status}, nil
}

func presaleNoticeReviewVersion(contentVersion string, includeRestricted bool, recipients []PresaleNoticeRecipient) string {
	payload, _ := json.Marshal(struct {
		Content           string
		IncludeRestricted bool
		Recipients        []PresaleNoticeRecipient
	}{contentVersion, includeRestricted, recipients})
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}
