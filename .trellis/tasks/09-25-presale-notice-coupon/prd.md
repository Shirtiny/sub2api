# Optional coupon in presale opening notices

Add a saved, optional public café coupon code in subscription plan management's
presale notice dialog. Store it for the reviewed presale month using existing
settings; do not carry it into the following month. Empty means no coupon block.
Only existing enabled public presale campaigns whose window overlaps the remaining
presale period may be configured. Render the actual discount, inclusive dates and
per-account limit in the existing email style. Disabled/expired codes are omitted.

Preview, test and formal mail must use the same server-derived coupon. Changes
invalidate the reviewed send version; unsaved UI edits cannot be sent. Admin-only
configuration must not create, redeem, enable or send anything by itself. No new
database schema, production deployment or real email send is required.

Verification: persistence/clear/month rollover, invalid/private/expired codes,
HTML/locale handling, changed-review rejection, dialog save/dirty/stale behavior,
admin route protection and frontend typecheck/build.
