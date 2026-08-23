package service

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"
)

const ContentModerationActionCyberPolicy = "cyber_policy"

// CyberPolicyRecordInput is the bounded request context captured after an
// upstream cyber-policy response has already been sent to the client.
type CyberPolicyRecordInput struct {
	RequestID       string
	UserID          int64
	UserEmail       string
	APIKeyID        int64
	APIKeyName      string
	GroupID         *int64
	GroupName       string
	Endpoint        string
	Provider        string
	Model           string
	UpstreamMessage string
	UpstreamBody    string
	UpstreamStatus  int
}

// RecordCyberPolicyEvent records a cyber_policy hit independently of the
// content-audit Enabled/Mode/scope switches. The global risk-control switch
// remains authoritative. Existing email, rolling violation, and auto-ban
// settings are deliberately reused so administrators manage one policy in the
// risk-control center.
func (s *ContentModerationService) RecordCyberPolicyEvent(ctx context.Context, in CyberPolicyRecordInput) {
	if s == nil || s.repo == nil || !s.isRiskControlEnabled(ctx) {
		return
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		slog.Warn("content_moderation.cyber_load_config_failed", "error", err)
		cfg = defaultContentModerationConfig()
		cfg.normalize()
	}
	userID := (*int64)(nil)
	if in.UserID > 0 {
		userID = &in.UserID
	}
	apiKeyID := (*int64)(nil)
	if in.APIKeyID > 0 {
		apiKeyID = &in.APIKeyID
	}
	provider := strings.TrimSpace(in.Provider)
	if provider == "" {
		provider = PlatformOpenAI
	}
	errorText := strings.TrimSpace(in.UpstreamMessage)
	if body := strings.TrimSpace(in.UpstreamBody); body != "" {
		errorText = strings.TrimSpace(errorText + "\n" + body)
	}
	log := &ContentModerationLog{
		RequestID:       strings.TrimSpace(in.RequestID),
		UserID:          userID,
		UserEmail:       strings.TrimSpace(in.UserEmail),
		APIKeyID:        apiKeyID,
		APIKeyName:      strings.TrimSpace(in.APIKeyName),
		GroupID:         cloneInt64Ptr(in.GroupID),
		GroupName:       strings.TrimSpace(in.GroupName),
		Endpoint:        strings.TrimSpace(in.Endpoint),
		Provider:        provider,
		Model:           strings.TrimSpace(in.Model),
		Mode:            "post_upstream",
		Action:          ContentModerationActionCyberPolicy,
		Flagged:         true,
		HighestCategory: ContentModerationActionCyberPolicy,
		HighestScore:    1,
		CategoryScores:  map[string]float64{ContentModerationActionCyberPolicy: 1},
		Error:           trimRunes(redactContentModerationSecrets(errorText), maxModerationExcerptRunes*4),
		CreatedAt:       time.Now(),
	}
	// Persist the audit row before attempting SMTP. A slow or unavailable mail
	// server must never hide the security event from the risk-control center.
	autoBanned := false
	if !cfg.CyberPolicyExcludeFromBanCount {
		autoBanned = s.applyFlaggedAccountSideEffects(ctx, cfg, log)
		log.SideEffectsApplied = log.ViolationCount > 0
	}
	log.EmailSent = false
	persisted := s.repo.CreateLog(ctx, log) == nil
	if !persisted {
		slog.Warn("content_moderation.cyber_create_log_failed", "user_id", in.UserID)
		return
	}
	emailSent := false
	if s.emailService != nil && strings.TrimSpace(log.UserEmail) != "" {
		if err := s.sendCyberPolicyEmail(ctx, cfg, log); err != nil {
			slog.Warn("content_moderation.cyber_email_failed", "user_id", in.UserID, "error", err)
		} else {
			emailSent = true
		}
		if autoBanned {
			if err := s.sendAccountDisabledEmail(ctx, cfg, log); err != nil {
				slog.Warn("content_moderation.cyber_ban_email_failed", "user_id", in.UserID, "error", err)
			} else {
				emailSent = true
			}
		}
	}
	if emailSent {
		if updater, ok := s.repo.(ContentModerationEmailAuditRepository); ok {
			if err := updater.UpdateLogEmailSent(ctx, log.ID, true); err != nil {
				slog.Warn("content_moderation.cyber_update_email_sent_failed", "log_id", log.ID, "error", err)
			}
		}
	}
}

func (s *ContentModerationService) sendCyberPolicyEmail(ctx context.Context, cfg *ContentModerationConfig, log *ContentModerationLog) error {
	if s == nil || s.emailService == nil || log == nil {
		return nil
	}
	siteName := s.siteName(ctx)
	banThreshold := defaultContentModerationBanThreshold
	if cfg != nil && cfg.BanThreshold > 0 {
		banThreshold = cfg.BanThreshold
	}
	if s.emailService.notificationEmailService != nil {
		err := s.emailService.notificationEmailService.Send(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventCyberPolicyNotice,
			RecipientEmail: log.UserEmail,
			RecipientName:  emailRecipientName(log.UserEmail),
			UserID:         contentModerationEmailUserID(log),
			SourceType:     "content_moderation",
			SourceID:       contentModerationEmailSourceID(log),
			Variables: map[string]string{
				"triggered_at":     log.CreatedAt.UTC().Format(time.RFC3339),
				"model":            defaultContentModerationString(log.Model, "-"),
				"group_name":       defaultContentModerationString(log.GroupName, "-"),
				"upstream_message": defaultContentModerationString(log.Error, "-"),
				"violation_count":  fmt.Sprintf("%d", log.ViolationCount),
				"ban_threshold":    fmt.Sprintf("%d", banThreshold),
			},
		})
		if err == nil {
			return nil
		}
		if !shouldFallbackNotificationEmail(err) {
			return err
		}
		slog.Warn("template cyber policy email failed; using built-in body", "error", err)
	}
	subject := fmt.Sprintf("[%s] 网络安全策略拦截提醒 / Cyber Policy Notice", sanitizeEmailHeader(siteName))
	return s.emailService.SendEmail(ctx, log.UserEmail, subject, buildCyberPolicyNoticeEmailBody(siteName, log))
}

func buildCyberPolicyNoticeEmailBody(siteName string, log *ContentModerationLog) string {
	if log == nil {
		return ""
	}
	name := strings.TrimSpace(log.UserEmail)
	if name == "" && log.UserID != nil {
		name = fmt.Sprintf("UID %d", *log.UserID)
	}
	return fmt.Sprintf(`<!doctype html><html><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Arial,sans-serif;background:#f5f6fb;color:#222;padding:32px"><div style="max-width:680px;margin:auto;background:#fff;padding:36px;border-top:8px solid #ef4444"><p style="color:#888;letter-spacing:2px">RISK CONTROL / 网络安全策略</p><h1>请求被网络安全策略拦截</h1><p>尊敬的用户 <strong>%s</strong>，您的请求被上游网络安全策略（cyber policy）拦截。</p><table style="width:100%%"><tr><td>触发时间</td><td>%s</td></tr><tr><td>模型</td><td>%s</td></tr><tr><td>上游说明</td><td>%s</td></tr></table><p>请调整请求内容后重试；如认为系误判，请联系管理员。</p><p style="color:#777">此邮件由 %s 自动发送，请勿回复。</p></div></body></html>`,
		html.EscapeString(name),
		html.EscapeString(log.CreatedAt.Format("2006-01-02 15:04:05")),
		html.EscapeString(defaultContentModerationString(log.Model, "-")),
		html.EscapeString(defaultContentModerationString(log.Error, "-")),
		html.EscapeString(siteName))
}
