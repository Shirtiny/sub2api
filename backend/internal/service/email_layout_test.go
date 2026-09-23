package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	xhtml "golang.org/x/net/html"
)

func requireEmailLetter(t *testing.T, body string) {
	t.Helper()
	require.Equal(t, 1, strings.Count(body, "<!doctype html>"))
	require.Equal(t, 1, strings.Count(body, `class="masthead"`))
	require.Equal(t, 1, strings.Count(body, `class="paper"`))
	require.Contains(t, body, "#30251d")
	require.Contains(t, body, "#faf7f2")
	require.Contains(t, body, "prefers-color-scheme: dark")
	require.Contains(t, body, "max-width: 480px")
	require.NotContains(t, body, "linear-gradient")
	require.NotContains(t, body, "cafecode.work")
	require.NotContains(t, body, "%!")
	require.Less(t, len(body), 32*1024)
	doc, err := xhtml.Parse(strings.NewReader(body))
	require.NoError(t, err)
	var visit func(*xhtml.Node)
	visit = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			require.NotContains(t, []string{"script", "img", "svg", "iframe", "link", "form"}, n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
}

func TestEmailLayoutAllOfficialTemplates(t *testing.T) {
	ctx := context.Background()
	repo := newNotificationEmailMemorySettingRepo()
	require.NoError(t, repo.Set(ctx, SettingKeySiteName, "Café Shop & Team"))
	svc := NewNotificationEmailService(repo, nil)
	for _, event := range svc.ListEventInfos() {
		for _, locale := range []string{"en", "zh"} {
			t.Run(event.Event+"/"+locale, func(t *testing.T) {
				preview, err := svc.PreviewTemplate(ctx, NotificationEmailPreviewInput{
					Event: event.Event, Locale: locale,
					Variables: map[string]string{
						"recipient_name":    "Reader <script> & Team",
						"reset_url":         "https://cafeshop.ai/reset-password?token=example&email=reader%40example.com",
						"recharge_url":      "https://cafeshop.ai/balance",
						"unsubscribe_url":   "https://cafeshop.ai/api/v1/notifications/unsubscribe?token=example",
						"verification_code": "654321",
					},
				})
				require.NoError(t, err)
				requireEmailLetter(t, preview.HTML)
				require.Contains(t, preview.HTML, "Café Shop &amp; Team")
				require.NotContains(t, preview.HTML, "{{")
				require.NotContains(t, preview.HTML, "<script>")
				if locale == "zh" {
					require.Contains(t, preview.HTML, `<html lang="zh-CN">`)
					require.Contains(t, preview.HTML, "此邮件由系统自动发送")
				} else {
					require.Contains(t, preview.HTML, `<html lang="en">`)
					require.Contains(t, preview.HTML, "This is an automated message.")
				}
				if event.Optional {
					require.Contains(t, preview.HTML, `href="https://cafeshop.ai/api/v1/notifications/unsubscribe?token=example"`)
				}
				if event.Event == NotificationEmailEventAuthPasswordReset {
					require.Contains(t, preview.HTML, `href="https://cafeshop.ai/reset-password?token=example&amp;email=reader%40example.com"`)
					require.Contains(t, preview.HTML, `class="button" style="`)
				}
				if event.Event == NotificationEmailEventAuthVerifyCode || event.Event == NotificationEmailEventNotificationEmailVerifyCode {
					require.Equal(t, 1, strings.Count(preview.HTML, "654321"))
					require.Contains(t, preview.HTML, `class="code receipt ink"`)
				}
			})
		}
	}
}

func TestEmailLayoutFallbacksShareBrandAndEscapeData(t *testing.T) {
	brand := "Café Shop & <Team>"
	mail := &EmailService{}
	balance := &BalanceNotifyService{}
	log := &ContentModerationLog{
		UserEmail: "reader@example.com", GroupName: "Astra <Group>",
		Model: "gpt-6", HighestCategory: "<risk>", HighestScore: .95,
		ViolationCount: 10, AutoBanned: true, BlockMessage: "Check <content>",
		CreatedAt: time.Now(),
	}
	cases := map[string]string{
		"waitlist": waitlistConfirmationHTML(brand),
		"access":   waitlistApprovalHTML(brand, "https://cafeshop.ai", false),
		"verify":   mail.buildVerifyCodeEmailBody("654321", brand),
		"reset":    mail.buildPasswordResetEmailBody("https://cafeshop.ai/reset?token=example&email=reader", brand),
		"notify":   buildNotifyVerifyEmailBody("654321", brand),
		"balance":  balance.buildBalanceLowEmailBody("<Reader>", 3.14, 10, brand, "https://cafeshop.ai/balance"),
		"quota":    balance.buildQuotaAlertEmailBody(42, "<Account>", "<Platform>", "Daily", 3, 10, 7, "30%", brand),
		"risk":     buildContentModerationViolationEmailBody(brand, log, nil),
		"disabled": buildContentModerationAccountDisabledEmailBody(brand, log, nil),
		"cyber":    buildCyberPolicyNoticeEmailBody(brand, nil, log),
		"smtp":     BuildSMTPTestEmailBody(brand),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			requireEmailLetter(t, body)
			require.Contains(t, body, "Café Shop &amp; &lt;Team&gt;")
			require.NotContains(t, body, "&amp;amp;")
			for _, raw := range []string{"<Team>", "<Reader>", "<Account>", "<Platform>", "<Group>", "<risk>", "<content>"} {
				require.NotContains(t, body, raw)
			}
		})
	}
	require.Contains(t, cases["verify"], "15 minutes")
	require.Contains(t, cases["reset"], "30 分钟")
	require.Contains(t, cases["reset"], "token=example&amp;email=reader")
	require.Contains(t, cases["access"], `href="https://cafeshop.ai/register?waitlist=1"`)
	require.Contains(t, cases["access"], `href="https://cafeshop.ai/login"`)
	require.Contains(t, cases["disabled"], "所有 API 请求将被拒绝")
	require.Contains(t, cases["quota"], "30%")
}

func TestEmailLayoutLiteralPlaceholdersAndUnsafeLinks(t *testing.T) {
	mail := &EmailService{}
	body := mail.buildVerifyCodeEmailBody(`<654321>`, "{{CONTENT}} & {{site_name}}")
	require.Contains(t, body, "{{CONTENT}} &amp; {{site_name}}")
	require.Contains(t, body, "&lt;654321&gt;")
	require.NotContains(t, body, "<654321>")
	for _, link := range []string{"javascript:alert(1)", "data:text/html,unsafe"} {
		require.NotContains(t, mail.buildPasswordResetEmailBody(link, "Site"), link)
		require.NotContains(t, (&BalanceNotifyService{}).buildBalanceLowEmailBody("Reader", 1, 10, "Site", link), "立即充值")
	}
}

func TestEmailLayoutCustomTemplateIsNotRewrapped(t *testing.T) {
	ctx := context.Background()
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	const custom = `<article style="background:#fff">Custom {{recipient_name}}</article>`
	_, err := svc.UpdateTemplate(ctx, NotificationEmailEventAuthVerifyCode, "en", "Code", custom)
	require.NoError(t, err)
	template, err := svc.GetTemplate(ctx, NotificationEmailEventAuthVerifyCode, "en")
	require.NoError(t, err)
	require.True(t, template.IsCustom)
	require.Equal(t, custom, template.HTML)
	preview, err := svc.PreviewTemplate(ctx, NotificationEmailPreviewInput{
		Event: NotificationEmailEventAuthVerifyCode, Locale: "en", Variables: map[string]string{"recipient_name": "Reader"},
	})
	require.NoError(t, err)
	require.Equal(t, `<article style="background:#fff">Custom Reader</article>`, preview.HTML)
}

func TestEmailLayoutOperationsFallback(t *testing.T) {
	ctx := context.Background()
	repo := newNotificationEmailMemorySettingRepo()
	require.NoError(t, repo.Set(ctx, SettingKeySiteName, "Café Shop & Ops"))
	mail := &EmailService{settingRepo: repo}
	body := mail.renderOpsEmail(ctx, "Ops report", `<p>Report period</p>`+emailDetails([2]string{"Requests", "123"}))
	requireEmailLetter(t, body)
	require.Contains(t, body, "Café Shop &amp; Ops")
	require.Contains(t, body, "Report period")
	require.Contains(t, body, "123")
	require.Contains(t, (&EmailService{}).renderOpsEmail(ctx, "Ops alert", "<p>Warning</p>"), ">Sub2API</p>")
}
