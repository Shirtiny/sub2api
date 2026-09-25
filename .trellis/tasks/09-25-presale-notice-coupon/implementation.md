# Implementation and verification

- Added the optional public café code field to the presale notice dialog, with
  explicit save/preview and server-side persistence scoped to the presale month.
- Empty configuration removes the entire email block. Discount and inclusive
  dates come from the existing public campaign, not user-entered promises.
- Personal, unknown, disabled, expired or out-of-period campaigns cannot be
  configured; later disable/expiry suppresses the email block. No coupon use or
  purchase state is changed. No migration or environment variable was added.
- Admin route protection, month checks and the reviewed send version cover the
  configuration. Dirty input blocks test/formal sending; stale responses cannot
  overwrite a reopened dialog. Monthly delivery dedup is unchanged.
- Kept the shared letter branding, dark mode and asset-free email markup. Long
  codes wrap; empty codes do not leave a heading or blank card.

## Validation

- Focused service, layout, notification and admin route tests passed.
- Full backend unit suite passed.
- Frontend: 174 test files / 1419 tests passed; typecheck, build and changed-file
  ESLint passed.
- Backend changed-package golangci-lint: 0 issues.
- Local email previews cover configured, empty, English and maximum-length code
  cases at 760/390/320 pixels in light/dark mode, without horizontal overflow.
- No production service update, campaign/config mutation or broadcast performed.
- The user requested one individual sample and a commit/tag/push. SMTP acceptance
  and message identity are retained in protected operational artifacts, not here.
- The single authorized sample was accepted by SMTP (250). This confirms provider
  acceptance only, not mailbox delivery. Production configuration was not saved
  or changed to generate this sample, and no formal notice recipients were used.

Previously reviewed shared-notification issues are outside this coupon change.
