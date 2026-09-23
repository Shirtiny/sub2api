package service

import (
	"fmt"
	"html"
	"strings"
	"time"
)

func buildContentModerationViolationEmailBody(siteName string, log *ContentModerationLog, cfg *ContentModerationConfig) string {
	if log == nil {
		return ""
	}
	content := `<p>尊敬的用户 <strong>` + html.EscapeString(moderationEmailUserName(log)) + `</strong>，您的 API 请求在内容审计中触发平台风控策略。详情如下。</p><h2>触发详情</h2>`
	content += moderationEmailDetails(log, cfg, "触发时间")
	if log.AutoBanned {
		content += emailWarning("账户当前处于封禁状态，所有 API 请求将被拒绝")
	}
	return renderEmailCard(siteName, notificationEmailLocaleChinese, "账户触发内容审计规则", content)
}

func buildContentModerationAccountDisabledEmailBody(siteName string, log *ContentModerationLog, cfg *ContentModerationConfig) string {
	if log == nil {
		return ""
	}
	content := `<p>尊敬的用户 <strong>` + html.EscapeString(moderationEmailUserName(log)) + `</strong>，您的账户在计数周期内多次触发平台风控策略，系统已自动禁用该账户。详情如下。</p><h2>封禁详情</h2>`
	content += moderationEmailDetails(log, cfg, "封禁时间")
	content += emailWarning("账户当前处于封禁状态，所有 API 请求将被拒绝")
	content += `<p>如需申诉或恢复账号，请联系平台管理员处理。</p>`
	return renderEmailCard(siteName, notificationEmailLocaleChinese, "账户已被自动禁用", content)
}

func moderationEmailUserName(log *ContentModerationLog) string {
	name := strings.TrimSpace(log.UserEmail)
	if name == "" && log.UserID != nil {
		name = fmt.Sprintf("UID %d", *log.UserID)
	}
	return name
}

func moderationEmailDetails(log *ContentModerationLog, cfg *ContentModerationConfig, timeLabel string) string {
	threshold := defaultContentModerationBanThreshold
	if cfg != nil && cfg.BanThreshold > 0 {
		threshold = cfg.BanThreshold
	}
	return emailDetails(
		[2]string{timeLabel, time.Now().Format("2006-01-02 15:04:05")},
		[2]string{"触发来源", "内容审核"},
		[2]string{"所属分组", defaultContentModerationString(log.GroupName, "-")},
		[2]string{"命中类别", fmt.Sprintf("%s / %.3f", defaultContentModerationString(log.HighestCategory, "-"), log.HighestScore)},
		[2]string{"拦截提示", contentModerationBlockMessage(log)},
		[2]string{"累计触发次数", fmt.Sprintf("%d 次（阈值 %d）", log.ViolationCount, threshold)},
	)
}

func defaultContentModerationString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func contentModerationBlockMessage(log *ContentModerationLog) string {
	if log != nil && strings.TrimSpace(log.BlockMessage) != "" {
		return strings.TrimSpace(log.BlockMessage)
	}
	return defaultContentModerationBlockMessage
}
