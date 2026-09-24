# Homepage waiting list

The homepage's secondary hero action is **Join waiting list**. It opens the shared
accessible dialog with an email-only form. This is independent of account signup:
closed registration does not prevent a visitor from joining the waiting list.

## Collection contract

- `POST /api/v1/waitlist` accepts JSON with `email` and, when Turnstile is enabled,
  `turnstile_token`. No authentication or account creation is required.
- Client and service validate common ASCII email syntax, strip surrounding
  whitespace, lowercase the address, enforce the 254-character total / 64-character
  local-part limits, and reject invalid labels and display-name forms. Plus aliases
  are preserved. This does **not** verify mailbox ownership or deliverability.
- The existing Redis limiter allows five attempts per client IP per minute and
  fails closed if Redis is unavailable. The body limit is 4 KiB. Existing Turnstile
  settings and verifier are reused; no new runtime configuration is introduced.
- PostgreSQL stores the normalized email and original join time, a row ID, and
  internal confirmation-attempt/sent timestamps. A unique constraint and
  `ON CONFLICT DO NOTHING` make concurrent/repeated submissions idempotent without
  changing the original timestamp.
- New submissions and already-confirmed duplicates return the same
  `{ accepted: true }` payload in the shared success envelope.
  Database failures return an error, never a fake success. The form preserves the
  entered address on failure, supports retry, and prevents concurrent submissions.
- The consent note covers waiting-list collection and availability updates. A
  successful application sends the confirmation below through the existing SMTP
  service/settings. It does not create an account, add other campaigns, or store
  the email in browser storage.

## Application confirmation email

Subject: **Waiting List 申请成功**

Body (kept exactly as requested, including punctuation):

> 欢迎，您已经加入到Waiting List，请耐心等待。关注邮件消息，开放后会即时通知。

The original paragraph is preserved inside a self-contained, table-based HTML
letter: an espresso masthead, warm paper background, editorial heading, application
status and a quiet sign-off. The brand is read from the existing administrator
`site_name` setting (HTML-escaped, with the normal `Sub2API` fallback), rather than
hardcoded or taken from a potentially outdated SMTP display name. Responsive and
dark-mode styles enhance an inline-styled baseline; no external fonts, tracking
pixels, images, links or dynamic recipient markup are included. The receipt does
not claim email ownership verification, a guaranteed place or an opening date.
SMTP sender settings and the delivery workflow below are unchanged.

The receipt and approval letter now share the [transactional email layout](EMAIL_TEMPLATES.md)
with verification, billing and service notifications, rather than maintaining a
separate visual theme.

1. Persist the application before attempting delivery. No database transaction is
   held while SMTP runs.
2. Atomically claim an unsent confirmation. A two-minute database-backed lease
   prevents concurrent submissions from sending multiple copies. Already-sent
   confirmations are not resent.
3. Send synchronously using the existing SMTP sender/configuration and its network
   deadlines, then persist the sent marker. Completion uses a separately bounded
   context so a browser disconnect cannot cancel the marker write.
4. On a known delivery failure, keep the application, release the claim, and return
   `WAITLIST_CONFIRMATION_FAILED`. The dialog says the application was saved but
   confirmation is incomplete, retaining the email for retry. Missing SMTP
   configuration is an error, not a fake success. An in-progress attempt also
   returns a retryable response; abandoned attempts become eligible after the
   lease expires when the visitor retries.

This is retry-on-submission, not a new background queue or automatic mailing
campaign. In the unavoidable crash window between SMTP acceptance and persistence
of the sent marker, a retry may deliver another copy; exactly-once SMTP delivery
or inbox placement is not guaranteed. Joining sends the application receipt only; administrator approval now triggers a
separate access notification (below). There is no automatic broadcast when public
registration is opened.

## Administrator access

The **Waiting list / 候补名单** sidebar item opens `/admin/waitlist`. Its paginated
email/join-time list uses `GET /api/v1/admin/waitlist`, behind the existing admin
middleware (not just a frontend guard). Results use `Cache-Control: no-store`.
Ordinary users and anonymous visitors cannot read the collection. Administrators
are reminded that collected addresses are not ownership-verified.

## Administrator approval

`POST /api/v1/admin/waitlist/:id/approve` is protected by the same administrator
middleware as the list (JWT or admin API key). The ID must be positive. The acting
administrator comes from the authenticated subject, never the request body. The
admin page offers **通过申请 / Approve**, a confirmation of its effects, the approval
time, notification state, and **重试通知 / Retry notification** after mail failure.

- First approval locks the application row. For one matching non-deleted account,
  set its status to `active`, add the configured default balance gift, and
  invalidate its API-key auth and billing balance caches. Match the same trimmed,
  case-insensitive mailbox as signup. Ambiguous legacy account matches require
  manual resolution. Never change passwords, roles,
  subscription entitlements, or moderation history, and never restore deleted users.
- If no account exists, persist a single-email signup allowance. This does **not**
  change `registration_enabled`, create an account, or verify mailbox ownership.
- Approval time and administrator ID are recorded alongside the granted/consuming
  user ID. Account creation and allowance consumption use **one transaction**;
  any admission failure rolls both back. Consumed allowances stay consumed after
  account deletion. Concurrent approvals of the same entry are idempotent.
- Already-approved entries do not change account status again. Retrying mail must
  not repeat the gift or undo a suspension imposed after the initial approval.
  “Approved” describes this application decision, not a live guarantee that an
  account is still active.

### Default balance gift and recharge history

- Existing accounts receive `SettingService.GetDefaultBalance` at first approval,
  added to their current balance (never replacing it). A zero default grants
  access without a monetary history entry; invalid non-finite/negative amounts
  are rejected without changing the application or account.
- Approval, the balance increment and its history entry commit together. A history
  write failure rolls everything back. The application row lock prevents duplicate
  credits from concurrent clicks; later notification retries ignore changed defaults
  and do not top up a gift the user has already spent.
- For applicants without an account, no account or balance is created at approval.
  Email signup grants its existing resolved signup balance (including a configured
  email-source override) exactly once. The signup transaction now records that
  amount together with consuming the approval; it does not add a second gift.
- History reuses a **used** `admin_balance` entry in `redeem_codes`, with the note
  **候补名单通过赠送**, the amount, recipient and time. `WL-GIFT-<application-id>`
  is a unique audit identifier, not a redeemable coupon. The entry is visible in
  existing user recharge history and administrator balance history. It is not a
  paid payment order and does not increase paid recharge totals or affiliate rewards.
- This change needs no schema migration and does not retroactively credit already
  approved existing accounts. Mail failure leaves both the approval and gift intact;
  retrying the notification never repeats the gift.

### Approval notification

After committing access, send a separate branded HTML letter through the existing
SMTP settings/sender (no new email-provider configuration):

Subject: **访问权限已开通**

> 您已获得访问权限，可以注册或进入控制台了。

This paragraph is used for a new applicant without a linked account. The primary
action is **验证邮箱并注册**; a secondary sign-in link covers applicants who have
already completed signup by the time they read the notice.

For an application with `granted_user_id` (an existing account activated by
approval, or a grant consumed before a notification retry), the message is:

> 您已获得访问权限，请使用原账号登录并进入控制台，无需重新注册。

That notice has **one sign-in action**, not a registration CTA. It tells the
recipient to use the original login method; approval does not set or replace a
password and an email link does not authenticate the recipient.

The letter uses the same responsive espresso/cream design as the receipt and
includes registration/login links derived **only** from the existing configured
frontend URL. Invalid/missing URLs fall back to instructions, never the request
Host or a hardcoded production domain. Set the existing frontend URL correctly to
include usable buttons; existing SMTP sender settings determine the From address.

Approval-notice delivery has its own two-minute lease and sent timestamp, separate
from the application receipt. A known SMTP failure releases that lease, leaves
access intact, and returns `WAITLIST_APPROVAL_NOTICE_FAILED`; the admin reloads the
saved state and can retry notification. Concurrent/in-progress attempts also return
a retryable result. Successful notices are not resent. Browser disconnects do not
cancel the bounded SMTP/marker completion. As with the receipt, SMTP acceptance
followed by a crash before marker persistence may cause a duplicate on retry; no
exactly-once or inbox-placement claim is made.

### Existing-account recovery during signup

When public registration is closed, the existing registration/verification-code
endpoints return `WAITLIST_SIGN_IN_REQUIRED` if the reviewed application is
already linked to an account, instead of the generic `REGISTRATION_DISABLED`.
This is a sign-in hint only: it grants no fresh signup allowance, changes no user
status/password and issues no login token. Unreviewed or missing applications
keep the neutral closed-registration response without querying arbitrary user
accounts. Public signup retains its ordinary existence/verification rules, and
unconsumed approved grants still require mailbox verification.

The signup and email-verification pages turn that reason (or `EMAIL_EXISTS`) into
an explicit **no need to register again / sign in** panel. It clears temporary
registration credentials and stops the OTP form/countdown instead of trapping
the user behind a disappearing error toast. The waiting-list signup form also
offers direct sign-in before submission. Going back from mailbox verification
preserves the waiting-list form selector, which is never an admission credential.
OAuth pending-session binding/recovery continues to use its separate flow.

### Registration while public signup is closed

- Email links open `/register?waitlist=1`; the closed-registration page also has an
  **已通过候补审批？继续注册** entry. The query flag only selects the form: it is not
  authorization and carries no credential or email address.
- Both verification-code sending and email/password registration independently
  check the approved, unconsumed email grant. Unapproved emails remain rejected
  while global registration is closed. Database lookup failures fail closed.
- Approved applicants **must verify ownership with the existing email OTP flow**,
  even if ordinary email verification is disabled. The approval replaces the need
  for an admission invitation code, not mailbox proof. Turnstile, reserved-email
  and allowed-domain policies still apply; the registration stage does not reuse
  the one-time Turnstile token already checked when the code was sent.
- The allowance applies to email/password signup. Approval-mode UI omits new-account
  OAuth shortcuts; it does not globally enable third-party registration. Existing
  reactivated accounts continue to use their existing login methods. New users can
  bind supported third-party methods after signup using the normal account flow.

## Release and preview boundaries

`backend/migrations/198_waitlist_entries.sql` adds the table and
`199_waitlist_confirmation.sql` adds nullable delivery-state columns, and
`200_waitlist_approval.sql` adds nullable approval/audit/notification state without
modifying existing users or migration history. Ent schema and generated code,
repository, service, handler, routes and Wire providers are included. The normal migration
runner applies these migrations only when a later authorized backend deployment
starts; developing or building the frontend does not apply it.

The isolated homepage preview on port 4178 serves static assets only. It does not
proxy backend requests, access production data, or accept real waiting-list
submissions. The form must report an unsuccessful submission there rather than
claim the email was saved. Real collection requires deploying the backend and its
migrations with the matching frontend and configuring the existing SMTP settings. No production deployment is implied by this
change.

## Verification

- Frontend tests cover mailbox syntax, form focus/errors/retry, success only after
  acknowledgement, duplicate-submit protection, Turnstile, the API contract,
  hero action, and admin list pagination/cancellation/access metadata.
- Go tests cover validation, persistence/error propagation, the bounded handler,
  uniform success responses, Turnstile rejection, admin authentication, Redis
  fail-close behavior, exact email copy/recipient, SMTP failure/retry, persisted
  deduplication, lease ownership/expiry, browser disconnects, SQL queries and the
  additive migrations. Template tests also cover configured branding, HTML escaping,
  literal placeholder-like site names, original-copy preservation and self-contained
  presentation. Mailers are fakes in unit tests; no live SMTP is contacted.
- The PostgreSQL integration test races twelve inserts of the same email and
  verifies the original ID/timestamp survives another join. It uses the repository
  integration harness, **never production**. Under a restricted environment this
  test may be compiled without running its container-based harness; report that
  limitation explicitly rather than treating compilation as an integration pass.

Approval verification also covers admin-only access, invalid IDs and trusted actor
identity, SMTP retries and disconnects, URL/HTML escaping, closed-signup and OTP
requirements, and registration-view behavior. The PostgreSQL harness tests initial
activation, concurrent approval and SMTP leases, rollback of both account and grant,
actual approved signup with public registration and ordinary verification disabled,
consumed-grant reuse rejection, and preservation of later suspensions. These tests
use disposable PostgreSQL/Redis containers and fake mail/codes, never production
accounts or real SMTP.
