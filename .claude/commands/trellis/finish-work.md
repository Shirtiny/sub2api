# Finish Work - Executable Pre-Submit Check

Before submitting or committing, run this workflow to execute the required validation commands, then complete the final readiness checklist.

**Timing**: After code is written and before commit

---

## Core Rule

> **This is not a passive checklist. Run the required commands first. If any required command fails, stop and fix the issue before committing.**

---

## Step 1: Inspect Changed Areas

Start by checking what changed:

```bash
git status
git diff --name-only
```

Classify the change set using the rules below.

### Decision Priority

Use the **first matching stricter rule** and do not downgrade afterward:

1. **Build-affecting / dependency / config changes**
2. **Backend source/runtime changes**
3. **Frontend source/runtime changes**
4. **Fullstack / uncertain changes**
5. **Tests-only changes**
6. **Docs / metadata only**

If multiple categories match, choose the stricter command set.

Examples:
- `frontend/package.json` + docs change -> frontend runtime checks still required
- `backend/go.mod` + backend tests change -> backend runtime checks still required
- `backend/**` + `frontend/**` -> run both
- `Dockerfile` changed -> treat as build-affecting; do not skip owning runtime checks

Do not rely on intuition alone. Base the decision on changed paths.

---

## Step 1.5: Change Classification Matrix

### A. Backend source or backend dependency/runtime changes

Examples:
- `backend/internal/**`
- `backend/cmd/**`
- `backend/ent/**`
- `backend/migrations/**`
- `backend/go.mod`
- `backend/go.sum`
- `backend/Makefile`
- `.github/workflows/backend-ci.yml`

### B. Frontend source or frontend dependency/runtime changes

Examples:
- `frontend/src/**`
- `frontend/package.json`
- `frontend/pnpm-lock.yaml`
- `frontend/vite.config.*`
- `frontend/tsconfig*.json`
- `Dockerfile`
- `.github/workflows/release.yml`
- `.github/workflows/custom-prod-image.yml`

### C. Tests-only changes

Examples:
- backend tests only: `backend/**/*_test.go`
- frontend tests only: `frontend/**/*.spec.ts`

### D. Docs / workflow / metadata only

Examples:
- `.trellis/**`
- `.claude/commands/**`
- markdown/docs only, with no runtime/build-affecting file changes

### E. Fullstack / uncertain

If both backend and frontend source/runtime files changed, or if you are unsure whether a change affects release/build behavior, treat it as fullstack.

---

## Step 1.6: Non-Downgrade Rule

Never downgrade a required command set just because the change looks small.

Examples:
- A one-line frontend type change still requires `cd frontend && pnpm run build`
- A tiny backend repository fix still requires backend lint plus `cd backend && go test ./...`
- A lockfile-only change still requires the relevant runtime/build check

The goal is to catch build and type failures before commit, not to optimize for the shortest command list.

---

## Step 2: Select Commands Deterministically

Use this order:

1. Detect whether any build-affecting files changed
2. Detect whether backend runtime files changed
3. Detect whether frontend runtime files changed
4. If both backend and frontend are touched, run both
5. Only allow docs-only skip when no runtime/build-affecting files changed

Once the command set is selected, state it explicitly before running anything.

Example report lines:
- `Detected frontend runtime changes in frontend/src/** -> running cd frontend && pnpm run build`
- `Detected backend + frontend changes -> running backend golangci-lint, backend go test, and frontend pnpm run build`

---

## Step 2.5: Hard Rule For Build-Affecting Files

Always run checks if any of these changed:

- `backend/go.mod`
- `backend/go.sum`
- `backend/Makefile`
- `frontend/package.json`
- `frontend/pnpm-lock.yaml`
- `frontend/tsconfig*.json`
- `frontend/vite.config.*`
- `Dockerfile`
- CI workflow files that affect build/test behavior

These files are high-leverage and can break CI, Docker, or release builds even when application source changes look small.

---

## Step 3: Run Required Validation Commands

Run commands in this order:
1. Backend command first (if required)
2. Frontend command second (if required)

### Backend source/runtime changes

First confirm the linter is available. If it is missing, stop and install it before committing:

```bash
command -v golangci-lint
```

Then run:

```bash
cd backend && golangci-lint run --path-mode=abs --timeout=30m ./...
cd backend && go test ./...
```

This is the required backend validation path for pre-submit review:
- `golangci-lint run --path-mode=abs --timeout=30m ./...`
- `go test ./...`

Do not treat backend lint as optional or as something CI will catch later.

### Frontend source/runtime changes

Run:

```bash
cd frontend && pnpm run build
```

This is the minimum reliable frontend gate because it runs:
- `vue-tsc -b`
- `vite build`

This catches real release/build failures that lint-only checks may miss.

### Fullstack or uncertain changes

Run all required commands:

```bash
command -v golangci-lint
cd backend && golangci-lint run --path-mode=abs --timeout=30m ./...
cd backend && go test ./...
cd frontend && pnpm run build
```

### Tests-only changes

Use judgment, but default to the owning runtime command unless you are certain the change is isolated.

Recommended default:
- backend tests changed -> `command -v golangci-lint && cd backend && golangci-lint run --path-mode=abs --timeout=30m ./... && cd backend && go test ./...`
- frontend tests changed -> `cd frontend && pnpm run build`

### Docs / metadata only changes

Heavy runtime checks may be skipped **only if** no backend/frontend source, dependency, config, or build-affecting files changed.

If uncertain, do not skip — run the owning command set.

If the first required command fails, stop immediately and report the failure before running any further readiness steps.

Do not mark the work as ready while a required command is still pending.

---

## Step 4: Guideline Review By Changed Area

After commands pass, review the changed code against project guidance.

### Backend files changed

Reference:
- `.trellis/spec/backend/index.md`
- relevant backend spec files
- `/trellis:check-backend`

### Frontend files changed

Reference:
- `.trellis/spec/frontend/index.md`
- relevant frontend spec files
- `/trellis:check-frontend`

### Cross-layer changes

Reference:
- `.trellis/spec/guides/cross-layer-thinking-guide.md`
- `/trellis:check-cross-layer`

---

## Step 5: Hard Stop Rules

Do **not** proceed to commit if any of these are true:

- [ ] `golangci-lint run --path-mode=abs --timeout=30m ./...` failed
- [ ] `go test ./...` failed
- [ ] `pnpm run build` failed
- [ ] Required checks were skipped
- [ ] Code-spec / contract changes are not synced
- [ ] Known regressions remain unresolved

If a command fails, report:
- command run
- exact failure
- whether the failure is backend, frontend, or cross-layer

Then fix the issue before retrying.

---

## Step 6: Final Readiness Checklist

### 1. Validation Commands

- [ ] Required backend/frontend commands were run based on changed files
- [ ] Backend lint was run whenever backend validation was required
- [ ] All required commands passed
- [ ] No failing build, typecheck, lint, or test issues remain

### 2. Code-Spec Sync

**Code-Spec Docs**:
- [ ] Does `.trellis/spec/backend/` need updates?
- [ ] Does `.trellis/spec/frontend/` need updates?
- [ ] Does `.trellis/spec/guides/` need updates?

**Key Question**:
> "If I fixed a bug or discovered something non-obvious, should I document it so future me (or others) won't hit the same issue?"

If yes -> update the relevant spec doc.

### 3. API Changes

If you modified API endpoints:

- [ ] Input schema updated?
- [ ] Output schema updated?
- [ ] API documentation updated?
- [ ] Client code updated to match?

### 4. Database Changes

If you modified database schema:

- [ ] Migration file created?
- [ ] Schema file updated?
- [ ] Related queries updated?
- [ ] Seed data updated (if applicable)?

### 5. Cross-Layer Verification

If the change spans multiple layers:

- [ ] Data flows correctly through all layers?
- [ ] Error handling works at each boundary?
- [ ] Types are consistent across layers?
- [ ] Loading states handled?

### 6. Manual Testing

- [ ] Feature works in browser/app?
- [ ] Edge cases tested?
- [ ] Error states tested?
- [ ] Works after page refresh?

---

## Minimum Reliable Command Set

Use this table when in doubt:

| Changed area | Required command |
|--------------|------------------|
| `backend/**` | `command -v golangci-lint && cd backend && golangci-lint run --path-mode=abs --timeout=30m ./... && cd backend && go test ./...` |
| `frontend/**` | `cd frontend && pnpm run build` |
| Both / uncertain | Run both commands |
| Docs / metadata only | May skip heavy checks only when clearly isolated |
| Dependency / build config files | Never skip relevant checks |

---

## Output Format

When this workflow is run, always report in this format:

```markdown
## Pre-Submit Check

### Changed Files Summary
- [key changed paths or groups]

### Detected Change Class
- [build-affecting / backend runtime / frontend runtime / fullstack / tests-only / docs-only]

### Commands Selected
- [why each command was selected]

### Commands Run
- [command]: [pass/fail]

### Guideline Review
- [backend/frontend/cross-layer checks performed]

### Blocking Issues
- [none / list]

### Ready To Commit
- [yes/no]
```

If the answer is `Ready To Commit: no`, explicitly say what must be fixed before rerunning `/trellis:finish-work`.

---

## Relationship to Other Commands

```bash
Development Flow:
  Write code -> Test -> /trellis:finish-work -> git commit -> /trellis:record-session
                          |
                 Execute real validation first
```

- `/trellis:finish-work` - executable pre-submit validation (this command)
- `/trellis:record-session` - record session and commits
- `/trellis:check-backend` - backend guideline review
- `/trellis:check-frontend` - frontend guideline review
- `/trellis:check-cross-layer` - cross-layer verification

---

## Core Principle

> **Readiness to commit is proven by executed checks, not inferred from confidence.**

Complete work = Passing commands + Guideline review + Docs sync + Verification
