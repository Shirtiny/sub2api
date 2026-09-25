# Preserve usage through presale renewal

## Requirement
Do not reset usage when a presale term activates. Existing daily/weekly/monthly
counters and window anchors must remain intact; normal rolling-window rules
continue to apply. Do not add user-facing explanation text.

## Scope
Remove only the activation mutation's forced zeroing/window clearing. Keep term
dates, multiplier/concurrency, historical reset-card grants, normal maintenance,
explicit resets and refund rules unchanged. Preserve the previous uncommitted
fourteen-day renewal-window changes. No production changes or data repair.

## Validation
Cover first purchase defaults, unchanged counters/anchors before and after
renewal, weekly-limit enforcement/reset timestamp, and activation retries. Run
presale and normal usage-window regressions and backend lint.
