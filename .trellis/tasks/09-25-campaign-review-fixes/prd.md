# Fix shared café campaign review findings

Keep the first verified payment facts immutable across callback retries, including
after consumption/audit failures. Preserve expiry and once-per-account safeguards.
Restore desktop campaign-table scrolling with accessible pagination; preserve
mobile flow. Display order-detail loading/errors/retry even without loaded data.
Use the same 100-character campaign-name limit in the UI, service and Ent schema.

Add focused regression tests and use isolated browser/SQL tests. No production
changes, live payments, coupon issuance, dependency additions, or release actions.
