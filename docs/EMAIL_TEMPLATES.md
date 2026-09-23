# Transactional email design

All built-in emails share the espresso-and-cream letter introduced for the
homepage waiting list. The shared shell is embedded from
`backend/internal/service/templates/email_layout.html`; rendering helpers live in
`email_layout.go`.

## Coverage

- Waiting-list receipt and access-approval notice.
- All 13 notification events in both English and Chinese: authentication and
  notification-address verification, password reset, subscription activation and
  expiry, balance/recharge/quota, moderation/Cyber notices, operations alerts and
  scheduled reports.
- Legacy fallback bodies for authentication, billing, moderation and operations.
- The administrator's SMTP test email.

The design uses a dark espresso masthead, serif heading, warm paper background,
consistent brown actions, quiet rules and receipt-style codes/details. It uses
table-based framing and inline baseline styles, with narrow-screen and dark-mode
enhancements. No remote fonts, images, tracking pixels or scripts are needed.

## Configuration and safety

- The displayed brand still comes from `site_name`, with the existing `Sub2API`
  fallback. There is no hardcoded production brand or domain in the layout.
- SMTP From/name/provider settings are unchanged. Styling does not switch email
  providers or send preview messages.
- Waiting-list registration/login links still use the configured `frontend_url`.
  A stale value must be corrected on the actual production instance; changing
  the HTML theme cannot correct a stale setting. API/unsubscribe URLs keep their
  existing configuration and signing behavior.
- The settings form submits `frontend_url` only when it differs from the value
  loaded or last saved. The update API preserves the current database value for
  an omitted/null field; an explicit empty string still clears the override.
  This prevents an older page from restoring its domain snapshot when saving an
  unrelated setting. Release the frontend/backend change together, and refresh
  already-open older settings pages before saving. This is not a conflict lock
  for deliberate concurrent domain edits. Already-delivered emails cannot be
  rewritten by a settings correction.
- Administrator-saved notification template overrides are preserved verbatim.
  The new shell applies to built-in defaults and explicit fallback bodies, not to
  every SMTP send. Restore a custom template to the official default only when
  the administrator intends to replace it.
- User-provided text is HTML-escaped. The letter renderer substitutes its slots
  in one pass; placeholder-like brand/text values are not expanded recursively.
  `styleBuiltinEmailContent` styles only locally generated, escaped markup; it
  is not a sanitizer for arbitrary HTML. Existing notification placeholder/URL
  validation and the trusted-report-HTML allowlist remain in place.
- OTP/reset validity, admission/approval, moderation decisions, notification
  preferences, deduplication and delivery/retry behavior are not changed.

## Verification

```sh
cd backend
go test -tags=unit ./internal/service ./internal/handler/admin \
  -run 'Test(EmailLayout|Waitlist|NotificationEmail|Build.*EmailBody|RequestControlBuiltInEmail)' \
  -count=1
```

The layout tests render both locales of every default, all fallback categories
and the SMTP test letter. They check the shared framing, escaping, language,
required links/expiry data and preservation of custom templates. Existing
waiting-list tests retain the exact requested confirmation copy, safe approval
URLs and the self-contained email contract.

For visual review, render with synthetic data locally and check desktop/mobile
light and dark schemes without sending SMTP. Browser previews are useful layout
checks, not a guarantee of identical rendering in every email client. Updating
the source alone does not change a running production image.
