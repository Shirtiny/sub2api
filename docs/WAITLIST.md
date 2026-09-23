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
or inbox placement is not guaranteed. The feature sends the application receipt
only: the wording about future availability does not add a broadcast/opening
notification trigger.

## Administrator access

The **Waiting list / 候补名单** sidebar item opens `/admin/waitlist`. Its paginated
email/join-time list uses `GET /api/v1/admin/waitlist`, behind the existing admin
middleware (not just a frontend guard). Results use `Cache-Control: no-store`.
Ordinary users and anonymous visitors cannot read the collection. Administrators
are reminded that collected addresses are not ownership-verified.

## Release and preview boundaries

`backend/migrations/198_waitlist_entries.sql` adds the table and
`199_waitlist_confirmation.sql` adds nullable delivery-state columns without
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
