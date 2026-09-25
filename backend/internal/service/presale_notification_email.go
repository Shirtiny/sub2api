package service

import (
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

const NotificationEmailEventPresaleOpening = "subscription.presale_opening"

func presaleNoticeOfficialTemplates() map[string]notificationEmailOfficialTemplate {
	result := make(map[string]notificationEmailOfficialTemplate)
	for _, locale := range []string{"zh", "en"} {
		subject, headline, preheader, signoff := "{{month_label}} subscription presale is open", "Your next month,\nready to begin.", "Reserve next month’s subscription. Your term begins on the first day of the month.", "Here whenever you need us. This is an automated message. Please do not reply."
		if locale == "zh" {
			subject = "{{month_label}}订阅预售已开启"
			headline = "下个月，提前安排。"
			preheader = "下个月的订阅已开放预订，月初正式生效。"
			signoff = "无论何时需要，它都在这里。此邮件由系统自动发送，请勿直接回复。"
		}
		result[locale] = notificationEmailOfficialTemplate{Subject: subject + " · {{site_name}}", HTML: renderEmailLetter("{{site_name}}", locale, emailLetter{Subject: subject, Headline: headline, Preheader: preheader, Kicker: "YOUR NEXT CHAPTER", Signoff: signoff, Content: "{{presale_content}}"})}
	}
	return result
}

func presaleNoticeBaseURL(raw string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || base == nil || (base.Scheme != "https" && base.Scheme != "http") || base.Hostname() == "" || base.User != nil {
		return nil, fmt.Errorf("configure a valid frontend URL before sending presale notices")
	}
	base.RawQuery, base.Fragment, base.RawPath = "", "", ""
	base.Path = strings.TrimRight(base.Path, "/") + "/"
	return base, nil
}

func presaleNoticeContent(locale string, period PresalePeriod, plans []*dbent.SubscriptionPlan, activities []PublicPresaleActivity, base *url.URL, now time.Time, unsubscribe string, coupon *PresaleNoticeCoupon) string {
	zh := locale == "zh"
	pick := func(cn, en string) string {
		if zh {
			return cn
		}
		return en
	}
	esc := html.EscapeString
	start := period.StartsAt.In(presaleLocation)
	last := period.ExpiresAt.In(presaleLocation).AddDate(0, 0, -1)
	content := `<p style="margin:0 0 24px;">` + pick("新一个月的工作节奏，从一份合适的订阅开始。现在预订，下个月月初正式生效。", "Make room for the month ahead. Reserve your subscription now; it starts on the first day of next month.") + `</p>`
	content += fmt.Sprintf(`<table role="presentation" class="receipt rule" width="100%%" cellpadding="0" cellspacing="0" style="width:100%%;margin:0 0 28px;border:1px solid #e4d8c8;border-radius:8px;background:#f0e9df;"><tr><td style="padding:22px 24px;"><p class="muted" style="margin:0 0 6px;color:#8b7662;font-size:10px;letter-spacing:2px;">%d / %s</p><p class="ink" style="margin:0;color:#382a20;font-family:Georgia,'Songti SC',serif;font-size:29px;line-height:1.4;">%s</p><p class="muted" style="margin:10px 0 0;color:#796653;font-size:12px;">%s · %s — %s</p></td></tr></table>`, start.Year(), strings.ToUpper(start.Format("Jan")), esc(pick(fmt.Sprintf("%d 月订阅", start.Month()), start.Format("January")+" subscription")), pick("使用周期", "Subscription term"), start.Format("01.02"), last.Format("01.02"))
	if len(plans) > 0 {
		content += `<p class="muted" style="margin:0 0 10px;font-size:11px;color:#8b7662;letter-spacing:1px;">` + pick("本期订阅", "THIS MONTH’S SELECTION") + `</p><table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="width:100%;border-collapse:collapse;">`
		for _, p := range plans {
			gift := ""
			for _, a := range activities {
				if a.StartsAt.After(now) || !a.EndsAt.After(now) {
					continue
				}
				for _, b := range a.PlanBonuses {
					if b.PlanID != p.ID {
						continue
					}
					currency := "$"
					if a.BonusCurrency == "CNY" {
						currency = "￥"
					}
					gift = pick("赠 ", "Gift ") + currency + fmt.Sprintf("%g", b.BonusBalance) + pick(" 余额", " balance")
				}
			}
			content += `<tr><td class="rule" style="padding:16px 0;border-bottom:1px solid #e4d9ca;vertical-align:top;"><strong class="ink" style="font-size:16px;font-weight:500;color:#382a20;">` + esc(p.Name) + `</strong>`
			if p.PresaleBadge != "" {
				content += `<span class="muted" style="font-size:11px;color:#8b7662;"> &nbsp; ` + esc(p.PresaleBadge) + `</span>`
			}
			if gift != "" {
				content += `<p class="accent" style="margin:6px 0 0;font-size:12px;color:#865630;">` + esc(gift) + `</p>`
			}
			content += fmt.Sprintf(`</td><td class="rule" align="right" style="padding:16px 0;border-bottom:1px solid #e4d9ca;vertical-align:top;white-space:nowrap;"><strong class="ink" style="font-family:Georgia,serif;color:#382a20;font-size:22px;font-weight:400;">￥%g</strong><p class="muted" style="margin:3px 0 0;font-size:10px;color:#8b7662;">%s</p></td></tr>`, p.Price, pick("/ 自然月 · 基础套餐", "/ calendar month · base plan"))
		}
		content += `</table>`
	}
	for _, a := range activities {
		if a.StartsAt.After(now) || !a.EndsAt.After(now) {
			continue
		}
		end := a.EndsAt.In(presaleLocation)
		content += `<p class="receipt muted" style="margin:20px 0 0;padding:15px 18px;border-left:2px solid #ad8158;background:#f2e8da;font-size:12px;color:#796653;">` + esc(a.Name) + ` · ` + a.StartsAt.In(presaleLocation).Format("01.02") + ` — ` + end.Add(-time.Second).Format("01.02") + `<br>` + esc(pick(fmt.Sprintf("每人限 %d 次，赠额不随订阅倍数增加，可叠加咖啡券。", a.MaxUsesPerUser), fmt.Sprintf("Up to %d per person. Fixed gift regardless of subscription multiplier; café coupons may be combined.", a.MaxUsesPerUser))) + `</p>`
	}
	if coupon != nil {
		content += presaleNoticeCouponContent(locale, coupon)
	}
	link := base.ResolveReference(&url.URL{Path: "presale"}).String()
	content += `<p style="margin:30px 0 24px;"><a class="button" href="` + esc(link) + `" style="display:inline-block;background:#865630;color:#ffffff;text-decoration:none;padding:13px 25px;border-radius:6px;font-size:14px;">` + pick("查看预售套餐", "Explore the presale") + ` &nbsp; →</a></p>`
	content += `<p class="muted" style="margin:0;color:#8b7662;font-size:12px;line-height:1.9;">` + pick("预售不会立即开通，生效日期与退款规则请在预售页查看。急需使用，可选择即时余额充值。", "Presales do not activate immediately. See the presale page for activation dates and refund rules. Need access sooner? Balance top-ups are available immediately.") + `</p>`
	content += `<p class="muted" style="margin:12px 0 0;color:#8b7662;font-size:11px;line-height:1.9;">` + pick("购买需具备站点访问权限。若仍在候补中，请等待权限开放邮件；本通知不代表候补申请已通过。", "Purchasing requires site access. If you are on the waiting list, please await your access approval email; this notice does not grant access.") + `</p>`
	if unsubscribe != "" {
		content += `<p style="margin:24px 0 0;font-size:11px;"><a href="` + esc(unsubscribe) + `" style="color:#8b7662;">` + pick("不再接收预售通知", "Unsubscribe from presale notices") + `</a></p>`
	}
	return content
}

func presaleNoticeCouponContent(locale string, coupon *PresaleNoticeCoupon) string {
	label, offer, instructions, datesLabel := "A CAFÉ TREAT", fmt.Sprintf("%d%% off your presale", coupon.DiscountPercent), "Enter this code at checkout. Presales only · Once per account.", "Valid"
	if locale == "zh" {
		label, offer, instructions, datesLabel = "一张咖啡券", fmt.Sprintf("预售减免 %d%%", coupon.DiscountPercent), "结算时填写券码 · 仅限预售 · 每人限用一次", "有效期"
	}
	// No script, copy button or external asset: the selectable code works in mail clients.
	return `<table role="presentation" class="receipt rule" width="100%" cellpadding="0" cellspacing="0" style="width:100%;margin:26px 0 0;background:#f0e9df;border:1px solid #d7b897;border-radius:8px;"><tr><td style="padding:22px 24px;">` +
		`<p class="accent" style="margin:0 0 8px;color:#865630;font-size:11px;letter-spacing:1.5px;">` + label + `</p>` +
		`<p class="ink" style="margin:0 0 14px;color:#382a20;font-size:18px;">` + offer + `</p>` +
		`<p class="ink" style="margin:0 0 12px;color:#382a20;font-family:Consolas,Menlo,monospace;font-size:18px;line-height:1.7;letter-spacing:0.4px;word-break:normal;overflow-wrap:anywhere;">` + html.EscapeString(coupon.Code) + `</p>` +
		`<p class="muted" style="margin:0 0 5px;color:#796653;font-size:12px;">` + datesLabel + ` ` + coupon.StartsAt.In(presaleLocation).Format("2006.01.02") + ` — ` + coupon.ExpiresAt.In(presaleLocation).Add(-time.Second).Format("2006.01.02") + `</p>` +
		`<p class="muted" style="margin:0;color:#796653;font-size:11px;line-height:1.9;">` + instructions + `</p></td></tr></table>`
}
