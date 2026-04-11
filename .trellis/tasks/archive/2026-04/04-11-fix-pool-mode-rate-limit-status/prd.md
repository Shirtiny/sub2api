# Fix pool mode rate limit status

## Goal
Prevent accounts from entering the "rate limiting" state in account management when account pool mode is enabled.

## Requirements
- When account pool mode is enabled, account status evaluation must not mark the account as rate limited solely from the existing rate-limit path.
- Keep the fix minimal and scoped to this incorrect status behavior.
- Preserve existing non-pool-mode behavior.

## Acceptance Criteria
- [ ] Accounts in pool mode no longer show or enter the rate-limited state incorrectly.
- [ ] Accounts not in pool mode keep the current rate-limit behavior.
- [ ] The affected code path has validation coverage or a targeted verification step.

## Technical Notes
- Prefer changing status computation or its immediate guard rather than broader scheduler or rate-limit logic.
- Read the relevant account management and status-related guidelines before editing implementation files.
