# Activated-presale refund fee

User clarified that daily refunds after activation also charge 20%.
The shared user/admin/execution quote now prorates by remaining whole days, then
multiplies by 0.8, with fee_percent=20. The final-seven-day exclusion is unchanged.
Accounting and actual gateway payment are prorated independently, preserving
coupon/channel amounts and currency-specific rounding. Service-failure refunds
remain full. Previously accepted quotes retain their audited amounts; legacy
requests without full snapshots also retain their stored amount/gateway_amount,
rather than being charged a newly introduced fee during review/retry.

Synchronized Chinese/English landing copy, shared benefit descriptions, and refund
review fee labels. The review no longer labels every fee-bearing refund as a
pre-activation preparation-period refund. Updated SUBSCRIPTION_PRESALES.md.

Validation:
- Focused backend presale/refund/dev-payment unit tests passed.
- Boundaries: full/preparation/activation, partial used days, final seven days,
  failed activation, early-reset shortened terms and frozen retry quotes.
- CNY and JPY rounding checked; old full/legacy audited amounts preserved.
- 72 focused frontend tests passed; frontend lint/typecheck/build passed.
- Backend service golangci-lint: 0 issues; current backend build passed.
- Only the isolated sub2api-test-app was recreated with the new local binary;
  app healthy, migration count unchanged at 230. No migration/backfill introduced.
- Test preview browser displays the 20% daily fee and final-seven-day exclusion.
- Test API authentication passed; accepted QA refund #6 still quotes the original
  1.01 amount and zero fee after the test backend update.
- Evidence: /opt/stacks/sub2api-test/artifacts/presale-daily-fee-*.

No production writes, lifecycle changes, commit, tag or push.

## Follow-up: exclude the original payment-channel fee

New presale refund quotes now exclude the original payment-channel fee before
applying the calendar/refund ratio. The fee is reconstructed from the immutable
order amount, coupon discount and fee rate using checkout's currency-specific
upward rounding. Cancellation fees and accounting amounts keep their existing
meaning. All new quote policies, including unfulfilled terms, use the fee-exclusive
payment base. Previously accepted quotes retain their audited amounts.

No frontend copy was added. Tests cover coupons, 2x orders, fractional fee rounding,
CNY/JPY, all refund windows, stale-quote rejection, frozen retries, legacy accepted
amounts and simulated gateway execution retaining the payment fee. Focused service
and payment unit tests passed; service golangci-lint reports zero issues.

Only the isolated local test app was refreshed; health is OK and migration count
remains 230. User/admin APIs both quote 2.00 for test order #9 (paid 2.02), while the
previously accepted test order #6 still quotes 1.01. Browser review shows ¥2.00 with
no additional fee explanation. No refund was submitted; order #9's full database
row is unchanged. Production container IDs and start times are unchanged.
Evidence: /opt/stacks/sub2api-test/artifacts/presale-payment-fee-*.
