# Fix usage record worker pool panic test

## Goal
Fix the failing backend unit test around panic handling in the usage record worker pool so the worker execution path does not leak panics to callers.

## Requirements
- Preserve the worker pool's intended panic-handling behavior during task execution.
- Make the minimal backend code change needed to satisfy the failing test.
- Keep logging and error-handling consistent with existing backend conventions.

## Acceptance Criteria
- [ ] `TestUsageRecordWorkerPool_Execute_PanicAndTimeout` passes.
- [ ] The worker pool does not propagate task panics to the caller in the tested path.
- [ ] Focused backend tests for the touched area pass.

## Technical Notes
- Investigate `backend/internal/service/usage_record_worker_pool.go` and its tests.
- Prefer a targeted fix over refactoring unrelated worker-pool behavior.
