package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
)

func (s *RequestControlService) applyRequestControlSideEffects(ctx context.Context, cfg *RequestControlConfig, log *RequestControlLog) {
	if s == nil || cfg == nil || log == nil || !log.Counted || log.UserID == nil || *log.UserID <= 0 {
		return
	}
	autoBanJustApplied := s.applyRequestControlAutoBan(ctx, cfg, log)
	if cfg.EmailOnHit && s.emailService != nil && strings.TrimSpace(log.UserEmail) != "" {
		if err := s.sendRequestControlEmail(ctx, cfg, log, false); err != nil {
			slog.Warn("request_control.hit_email_failed", "user_id", *log.UserID, "recipient_hash", notificationEmailHash(log.UserEmail), "error", err)
		} else {
			log.EmailSent = true
		}
	}
	if autoBanJustApplied && s.emailService != nil && strings.TrimSpace(log.UserEmail) != "" {
		if err := s.sendRequestControlEmail(ctx, cfg, log, true); err != nil {
			slog.Warn("request_control.ban_email_failed", "user_id", *log.UserID, "recipient_hash", notificationEmailHash(log.UserEmail), "error", err)
		} else {
			log.EmailSent = true
		}
	}
}

func (s *RequestControlService) applyRequestControlAutoBan(ctx context.Context, cfg *RequestControlConfig, log *RequestControlLog) bool {
	if s == nil || cfg == nil || log == nil || !cfg.AutoBanEnabled || cfg.BanThreshold <= 0 || log.ViolationCount < cfg.BanThreshold || log.UserID == nil || *log.UserID <= 0 || s.userRepo == nil {
		return false
	}
	user, err := s.userRepo.GetByID(ctx, *log.UserID)
	if err != nil {
		slog.Warn("request_control.ban_get_user_failed", "user_id", *log.UserID, "error", err)
		return false
	}
	if user == nil || user.IsAdmin() {
		if user != nil && user.IsAdmin() {
			slog.Warn("request_control.autoban_skipped_admin", "user_id", *log.UserID, "count", log.ViolationCount, "threshold", cfg.BanThreshold)
		}
		return false
	}
	if user.Status == StatusDisabled {
		log.AutoBanned = true
		return false
	}
	user.Status = StatusDisabled
	if err := s.userRepo.Update(ctx, user); err != nil {
		slog.Warn("request_control.ban_update_user_failed", "user_id", *log.UserID, "error", err)
		return false
	}
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, *log.UserID)
	}
	log.AutoBanned = true
	return true
}

func (s *RequestControlService) sendRequestControlEmail(ctx context.Context, cfg *RequestControlConfig, log *RequestControlLog, disabled bool) error {
	if s == nil || s.emailService == nil || cfg == nil || log == nil {
		return nil
	}
	moderationLog := &ContentModerationLog{
		UserID:          log.UserID,
		UserEmail:       log.UserEmail,
		GroupName:       log.GroupName,
		HighestCategory: "request_control",
		HighestScore:    0,
		ViolationCount:  log.ViolationCount,
		AutoBanned:      log.AutoBanned,
		CreatedAt:       log.CreatedAt,
	}
	moderationCfg := &ContentModerationConfig{BanThreshold: cfg.BanThreshold}
	event := NotificationEmailEventContentModerationViolation
	if disabled {
		event = NotificationEmailEventContentModerationDisabled
	}
	variables := contentModerationEmailVariables(moderationLog, moderationCfg)
	sourceID := requestControlEmailSourceID(log, disabled)
	if s.emailService.notificationEmailService != nil {
		err := s.emailService.notificationEmailService.Send(ctx, NotificationEmailSendInput{
			Event:          event,
			RecipientEmail: log.UserEmail,
			RecipientName:  emailRecipientName(log.UserEmail),
			UserID:         *log.UserID,
			SourceType:     "request_control",
			SourceID:       sourceID,
			Variables:      variables,
		})
		if err == nil {
			return nil
		}
		if !shouldFallbackNotificationEmail(err) {
			return err
		}
	}
	siteName := s.requestControlSiteName(ctx)
	if disabled {
		return s.emailService.SendEmail(ctx, log.UserEmail, fmt.Sprintf("[%s] 账户已被禁用 / Account Disabled", sanitizeEmailHeader(siteName)), buildContentModerationAccountDisabledEmailBody(siteName, moderationLog, moderationCfg))
	}
	return s.emailService.SendEmail(ctx, log.UserEmail, fmt.Sprintf("[%s] 账户风控提醒 / Risk Control Notice", sanitizeEmailHeader(siteName)), buildContentModerationViolationEmailBody(siteName, moderationLog, moderationCfg))
}

func requestControlEmailSourceID(log *RequestControlLog, disabled bool) string {
	if log == nil {
		return ""
	}
	kind := "hit"
	if disabled {
		kind = "disabled"
	}
	userID := int64(0)
	if log.UserID != nil {
		userID = *log.UserID
	}
	seed := fmt.Sprintf("%s:%d:%d:%s:%d", kind, userID, log.ViolationCount, log.RequestID, log.CreatedAt.UnixNano())
	digest := sha256.Sum256([]byte(seed))
	return "request-control-" + kind + "-" + hex.EncodeToString(digest[:16])
}

func (s *RequestControlService) requestControlSiteName(ctx context.Context) string {
	if s == nil || s.settingRepo == nil {
		return defaultSiteName
	}
	name, err := s.settingRepo.GetValue(ctx, SettingKeySiteName)
	if err != nil || strings.TrimSpace(name) == "" {
		return defaultSiteName
	}
	return strings.TrimSpace(name)
}
