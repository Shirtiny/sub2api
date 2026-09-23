package service

import (
	"html"
	"net/url"
	"strings"
)

func waitlistConfirmationHTML(siteName string) string {
	return renderEmailLetter(siteName, notificationEmailLocaleChinese, emailLetter{
		Subject:   waitlistConfirmationSubject,
		Preheader: "申请已收到。感谢你的耐心与期待，开放后会通过邮件通知。",
		Kicker:    "YOU’RE ON THE LIST",
		Headline:  "好事，正在酝酿。",
		Signoff:   "感谢你的耐心与期待。",
		Content:   waitlistLetterContent(waitlistConfirmationMessage, "已加入 Waiting List", ""),
	})
}

func waitlistApprovalHTML(siteName, frontendURL string, existingAccount bool) string {
	message := waitlistApprovalMessage
	instructions := "请前往候补审批注册页，使用收到本邮件的邮箱完成验证并创建账号。若你已完成注册，请直接登录。"
	actionPath, actionQuery, actionLabel := "register", "waitlist=1", "验证邮箱并注册"
	if existingAccount {
		message = waitlistExistingAccountApprovalMessage
		instructions = "此邮箱已关联账号，请使用原来的登录方式进入控制台。请勿再次注册；如忘记密码，可在登录页找回密码。"
		actionPath, actionQuery, actionLabel = "login", "", "登录并进入控制台"
	}
	next := `<p class="body-copy" style="margin:24px 0 0;color:#685648;font-size:14px;line-height:2;">` + html.EscapeString(instructions) + `</p>`
	// Never derive email links from an untrusted request Host. Missing or invalid
	// configured URLs fall back to instructions rather than a broken CTA.
	base, err := url.Parse(strings.TrimSpace(frontendURL))
	if err == nil && (base.Scheme == "https" || base.Scheme == "http") && base.Hostname() != "" && base.User == nil {
		base.RawQuery, base.Fragment, base.RawPath = "", "", ""
		base.Path = strings.TrimRight(base.Path, "/") + "/"
		actionURL := base.ResolveReference(&url.URL{Path: actionPath, RawQuery: actionQuery}).String()
		next += `<p style="margin:24px 0 0;font-size:14px;line-height:2;"><a class="button" style="display:inline-block;padding:11px 22px;border-radius:6px;background-color:#865630;color:#ffffff;text-decoration:none;font-weight:500;line-height:1.6;" href="` + html.EscapeString(actionURL) + `">` + actionLabel + `</a>`
		if !existingAccount {
			loginURL := base.ResolveReference(&url.URL{Path: "login"}).String()
			next += ` &nbsp; <a class="accent" style="display:inline-block;padding:10px 14px;color:#865630;" href="` + html.EscapeString(loginURL) + `">已有账号，直接登录</a>`
		}
		next += `</p>`
	}
	return renderEmailLetter(siteName, notificationEmailLocaleChinese, emailLetter{
		Subject:   waitlistApprovalSubject,
		Preheader: "候补申请已通过，欢迎开始使用。",
		Kicker:    "YOUR ACCESS IS READY",
		Headline:  "欢迎，现在开始。",
		Signoff:   "无论你何时需要，它都在这里。",
		Content:   waitlistLetterContent(message, "已通过 · 访问权限已开通", next),
	})
}

// next is trusted local markup with its dynamic URLs already escaped.
func waitlistLetterContent(message, status, next string) string {
	return `<p style="margin:0;">` + html.EscapeString(message) + `</p>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="width:100%;margin-top:30px;">
  <tr><td class="receipt" style="padding:16px 20px;background-color:#f0e9df;border:1px solid #e4d8c8;border-radius:6px;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="width:100%;">
      <tr>
        <td class="muted" width="64" style="width:64px;color:#796653;font-size:12px;line-height:22px;">申请状态</td>
        <td class="accent" align="right" style="color:#865630;font-size:13px;font-weight:500;line-height:22px;word-break:keep-all;">` + html.EscapeString(status) + `</td>
      </tr>
    </table>
  </td></tr>
</table>` + next
}
