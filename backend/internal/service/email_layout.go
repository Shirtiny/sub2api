package service

import (
	"context"
	_ "embed"
	"html"
	"strings"
)

//go:embed templates/email_layout.html
var emailLayoutTemplate string

type emailLetter struct {
	Subject, Preheader, Kicker, Headline, Signoff string
	// Content is trusted, locally built markup. Escape dynamic values before
	// inserting them; administrator-authored templates do not pass through here.
	Content string
}

func renderEmailLetter(siteName, locale string, letter emailLetter) string {
	siteName = strings.TrimSpace(siteName)
	if siteName == "" {
		siteName = "Sub2API"
	}
	lang, tagline := "en", "Make inspiration happen."
	if locale == notificationEmailLocaleChinese {
		lang, tagline = "zh-CN", "让灵感，高效落地。"
	}
	if letter.Subject == "" {
		letter.Subject = letter.Headline
	}
	if letter.Preheader == "" {
		letter.Preheader = letter.Headline
	}
	if letter.Kicker == "" {
		letter.Kicker = "FROM YOUR WORKSPACE"
	}
	if letter.Signoff == "" {
		letter.Signoff = "This is an automated message. Please do not reply."
		if locale == notificationEmailLocaleChinese {
			letter.Signoff = "此邮件由系统自动发送，请勿直接回复。"
		}
	}
	// A single pass keeps placeholder-like text in a configured brand or a
	// recipient's data literal instead of interpreting it as another template.
	return strings.NewReplacer(
		"{{LANG}}", lang,
		"{{SITE_NAME}}", html.EscapeString(siteName),
		"{{SUBJECT}}", html.EscapeString(letter.Subject),
		"{{PREHEADER}}", html.EscapeString(letter.Preheader),
		"{{KICKER}}", html.EscapeString(letter.Kicker),
		"{{HEADLINE}}", html.EscapeString(letter.Headline),
		"{{CONTENT}}", letter.Content,
		"{{TAGLINE}}", tagline,
		"{{SIGNOFF}}", html.EscapeString(letter.Signoff),
	).Replace(emailLayoutTemplate)
}

func renderEmailCard(siteName, locale, title, content string) string {
	return renderEmailLetter(siteName, locale, emailLetter{
		Headline: title,
		Content:  styleBuiltinEmailContent(content),
	})
}

// Style the small, explicit markup vocabulary used by built-in email bodies.
// Inline styles remain usable when a mail client removes the head stylesheet.
// This is not an HTML sanitizer and must not be applied to custom templates.
func styleBuiltinEmailContent(content string) string {
	return strings.NewReplacer(
		`<p>`, `<p style="margin:0 0 18px;">`,
		`<p class="muted">`, `<p class="muted" style="margin:0 0 18px;color:#8b7662;font-size:12px;line-height:1.8;">`,
		`<h2>`, `<h2 class="ink" style="margin:24px 0 16px;color:#382a20;font-size:17px;font-weight:600;line-height:1.6;">`,
		`<h3>`, `<h3 class="ink" style="margin:20px 0 12px;color:#382a20;font-size:15px;font-weight:600;line-height:1.6;">`,
		`<ul>`, `<ul style="margin:16px 0;padding-left:22px;">`,
		`<li>`, `<li style="margin:0 0 8px;">`,
		`<a class="button"`, `<a class="button" style="display:inline-block;padding:11px 22px;border-radius:6px;background-color:#865630;color:#ffffff;text-decoration:none;font-size:14px;font-weight:500;line-height:1.6;"`,
		`<a href=`, `<a class="accent" style="color:#865630;text-decoration:underline;" href=`,
		`<table style="width:100%;border-collapse:collapse;">`, `<table width="100%" cellpadding="0" cellspacing="0" style="width:100%;border-collapse:collapse;table-layout:fixed;margin:24px 0;font-size:13px;">`,
		`<td>`, `<td class="rule" style="padding:12px 8px;border-bottom:1px solid #e4d9ca;vertical-align:top;overflow-wrap:anywhere;word-wrap:break-word;">`,
		`<th>`, `<th class="receipt ink rule" style="padding:12px 8px;border-bottom:1px solid #e4d9ca;background-color:#f0e9df;color:#382a20;text-align:left;font-weight:600;overflow-wrap:anywhere;word-wrap:break-word;">`,
	).Replace(content)
}

func emailVerificationCode(code string) string {
	return `<p class="code receipt ink" style="margin:24px 0;padding:20px 12px;border:1px solid #e4d8c8;border-radius:6px;background-color:#f0e9df;color:#382a20;font-family:'SFMono-Regular',Consolas,monospace;font-size:32px;font-weight:500;letter-spacing:8px;line-height:1.5;text-align:center;">` + html.EscapeString(code) + `</p>`
}

func emailWarning(message string) string {
	return `<p class="notice" style="margin:24px 0;padding:16px 20px;border-left:3px solid #a27057;background-color:#f2e5db;color:#7b3d2c;font-weight:600;line-height:1.8;">` + html.EscapeString(message) + `</p>`
}

// emailDetails accepts plain-text labels and values, never raw HTML.
func emailDetails(rows ...[2]string) string {
	content := `<table style="width:100%;border-collapse:collapse;">`
	for _, row := range rows {
		content += "<tr><td>" + html.EscapeString(row[0]) + "</td><td>" + html.EscapeString(row[1]) + "</td></tr>"
	}
	return content + "</table>"
}

// Legacy operations senders also use the configured brand when the centralized
// notification-template service is unavailable.
func (s *EmailService) renderOpsEmail(ctx context.Context, title, content string) string {
	siteName := "Sub2API"
	if s.settingRepo != nil {
		if value, err := s.settingRepo.GetValue(ctx, SettingKeySiteName); err == nil && strings.TrimSpace(value) != "" {
			siteName = value
		}
	}
	return renderEmailCard(siteName, notificationEmailDefaultLocale, title, content)
}

// BuildSMTPTestEmailBody uses the same letter as the transactional emails.
// Rendering it does not send mail or modify SMTP settings.
func BuildSMTPTestEmailBody(siteName string) string {
	return renderEmailLetter(siteName, notificationEmailDefaultLocale, emailLetter{
		Headline: "Email configuration successful",
		Kicker:   "DELIVERY CHECK",
		Signoff:  "This is an automated test message.",
		Content:  styleBuiltinEmailContent(`<p>This is a test email to verify your SMTP settings are working correctly.</p>`),
	})
}
