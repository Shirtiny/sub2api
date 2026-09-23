package service

import (
	_ "embed"
	"html"
	"net/url"
	"strings"
)

//go:embed templates/waitlist_confirmation.html
var waitlistConfirmationTemplate string

func waitlistConfirmationHTML(siteName string) string {
	return renderWaitlistLetter(siteName,
		"{{SUBJECT}}", waitlistConfirmationSubject,
		"{{MESSAGE}}", waitlistConfirmationMessage,
		"{{PREHEADER}}", "申请已收到。感谢你的耐心与期待，开放后会通过邮件通知。",
		"{{KICKER}}", "YOU’RE ON THE LIST",
		"{{HEADLINE}}", "好事，正在酝酿。",
		"{{STATUS}}", "已加入 Waiting List",
		"{{SIGNOFF}}", "感谢你的耐心与期待。",
		"{{NEXT_STEPS}}", "",
	)
}

func waitlistApprovalHTML(siteName, frontendURL string) string {
	next := `<p class="body-copy" style="margin:24px 0 0;color:#685648;font-size:14px;line-height:2;">已有账号？使用原账号登录即可。尚未注册？请前往本站注册页，选择候补审批注册，使用收到本邮件的邮箱完成验证。</p>`
	// Never derive email links from an untrusted request Host. Missing or invalid
	// configured URLs fall back to instructions rather than a broken CTA.
	base, err := url.Parse(strings.TrimSpace(frontendURL))
	if err == nil && (base.Scheme == "https" || base.Scheme == "http") && base.Hostname() != "" && base.User == nil {
		base.RawQuery, base.Fragment, base.RawPath = "", "", ""
		base.Path = strings.TrimRight(base.Path, "/") + "/"
		registerURL := base.ResolveReference(&url.URL{Path: "register", RawQuery: "waitlist=1"}).String()
		loginURL := base.ResolveReference(&url.URL{Path: "login"}).String()
		next += `<p style="margin:24px 0 0;font-size:14px;line-height:2;"><a style="display:inline-block;padding:10px 22px;border-radius:6px;background:#865630;color:#ffffff;text-decoration:none;" href="` + html.EscapeString(registerURL) + `">注册账号</a> &nbsp; <a class="accent" style="display:inline-block;padding:10px 14px;color:#865630;" href="` + html.EscapeString(loginURL) + `">进入控制台</a></p>`
	}
	return renderWaitlistLetter(siteName,
		"{{SUBJECT}}", waitlistApprovalSubject,
		"{{MESSAGE}}", waitlistApprovalMessage,
		"{{PREHEADER}}", "候补申请已通过，欢迎开始使用。",
		"{{KICKER}}", "YOUR ACCESS IS READY",
		"{{HEADLINE}}", "欢迎，现在开始。",
		"{{STATUS}}", "已通过 · 访问权限已开通",
		"{{SIGNOFF}}", "无论你何时需要，它都在这里。",
		"{{NEXT_STEPS}}", next,
	)
}

// The replacements below are trusted copy / locally built markup. Only the site
// name and configured URLs are dynamic; escape those before the single pass.
func renderWaitlistLetter(siteName string, replacements ...string) string {
	siteName = strings.TrimSpace(siteName)
	if siteName == "" {
		siteName = "Sub2API"
	}
	replacements = append(replacements, "{{SITE_NAME}}", html.EscapeString(siteName))
	return strings.NewReplacer(replacements...).Replace(waitlistConfirmationTemplate)
}
