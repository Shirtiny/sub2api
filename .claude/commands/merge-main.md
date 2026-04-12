# Merge Main Into custom-prod

Safely merge the latest `main` updates into `custom-prod`, preserve both upstream behavior and branch-specific behavior, and verify the result before pushing.

---

## Quick Start

If you just need the shortest safe flow, use this:

```bash
# 1. Check whether a merge is already in progress
git status

# 2. If clean, fetch and merge
git fetch origin main
git merge origin/main

# 3. Resolve conflicts carefully
# 4. Verify no conflict markers remain
grep -R "^<<<<<<< \|^=======\|^>>>>>>> " backend frontend

# 5. Run focused checks
cd backend && go test ./internal/service ./internal/repository ./internal/handler/dto

# 6. Complete the merge
git commit -m "Merge branch 'main' into custom-prod"
```

---

## When to Use

Use this when upstream `main` has moved forward and you need to sync those changes into `custom-prod`.

This workflow is especially important when the branch contains long-lived custom changes in high-conflict areas such as:

- `backend/internal/repository/account_repo.go`
- `backend/internal/repository/account_repo_integration_test.go`
- `frontend/pnpm-lock.yaml`

---

## Core Lessons From Previous Merges

### 1. Check for unfinished merges first

Before starting a new merge, always verify whether Git is already in the middle of one:

```bash
git status
```

If you see:
- `All conflicts fixed but you are still merging.`

then **do not start another merge**. Finish the existing merge first.

### 2. Empty merge commits can happen

If a merge was already resolved and only a commit remained, creating the merge commit may produce a merge commit with no effective diff relative to the current branch tip.

Check whether the merge commit is actually meaningful:

```bash
git show --stat --summary HEAD
git diff --stat HEAD^1 HEAD
```

If the merge commit is truly empty and you want to redo it:

```bash
git reset --hard HEAD^
```

Only do this when you are certain the merge commit is disposable and local.

### 3. High-risk conflict files need deliberate resolution

These files are conflict hotspots:

| File | Why it conflicts often |
|------|-------------------------|
| `backend/internal/repository/account_repo.go` | Shared repository filtering/query logic changes frequently |
| `backend/internal/repository/account_repo_integration_test.go` | Many features append tests into the same large suite |
| `frontend/pnpm-lock.yaml` | Lockfiles are regenerated and reorder often |

### 4. Merge resolution should preserve both dimensions of behavior

When resolving code conflicts, do not blindly choose `ours` or `theirs`.

Instead, ask:
- What new upstream behavior must be preserved?
- What branch-specific custom behavior must be preserved?
- Can both coexist in the same function/test block?

Example pattern:
- Keep upstream filtering improvements
- Re-apply `custom-prod` pool-mode semantics on top

### 5. Lockfile conflicts should usually follow the source of truth

For `frontend/pnpm-lock.yaml`, if upstream changed frontend dependencies and your branch did not intentionally curate a different lock state, prefer upstream lockfile content:

```bash
git checkout --theirs frontend/pnpm-lock.yaml
git add frontend/pnpm-lock.yaml
```

Then verify the frontend still installs/tests correctly later.

---

## Recommended Merge Procedure

### Step 1: Check current state

```bash
git status --short --branch
git remote -v
```

### Step 2: Detect unfinished merge state

```bash
git status
git rev-parse --verify MERGE_HEAD
```

Interpretation:
- If no `MERGE_HEAD` → safe to start a merge
- If `MERGE_HEAD` exists and conflicts are resolved → commit the merge
- If `MERGE_HEAD` exists and conflicts remain → resolve them first

### Step 3: Fetch upstream main

```bash
git fetch origin main
git log --oneline -3 origin/main
```

### Step 4: Merge main into custom-prod

```bash
git merge origin/main
```

### Step 5: Resolve conflicts deliberately

For each conflict:

1. Read the full conflict block
2. Identify:
   - upstream new behavior
   - branch custom behavior
3. Combine both where needed
4. Re-run focused tests for touched areas
5. Stage resolved files

Helpful checks:

```bash
git diff --name-only --diff-filter=U
git status
```

### Step 6: Verify no conflict markers remain

```bash
grep -R "^<<<<<<< \|^=======\|^>>>>>>> " backend frontend
```

### Step 7: Run focused validation

For backend repository/service conflicts:

```bash
cd backend && go test ./internal/service ./internal/repository ./internal/handler/dto
```

If the conflict is narrower, run focused tests first:

```bash
cd backend && go test ./internal/repository -run 'TestListWithFilters|TestListSchedulable' -count=1
cd backend && go test ./internal/service -run 'Test.*PoolMode.*' -count=1
```

### Step 8: Complete the merge commit

```bash
git commit -m "Merge branch 'main' into custom-prod"
```

### Step 9: Post-merge verification

```bash
git status --short --branch
git log --oneline -3
git show --stat --summary HEAD
```

---

## Special Case: Redoing a Bad Merge

If the most recent merge commit is empty or wrong, and it has not been pushed:

### 1. Inspect it

```bash
git show --stat --summary HEAD
git rev-list --parents -n 1 HEAD
git diff --stat HEAD^1 HEAD
```

### 2. Remove it

```bash
git reset --hard HEAD^
```

### 3. Re-fetch and merge again

```bash
git fetch origin main
git merge origin/main
```

---

## Review Checklist For Conflict Resolution

Before concluding the merge, check:

- [ ] Did I accidentally drop custom branch semantics?
- [ ] Did I accidentally discard upstream behavior?
- [ ] Did I preserve both logic and tests where both matter?
- [ ] Did I verify lockfile resolution strategy intentionally?
- [ ] Did I run focused tests for the touched conflict areas?
- [ ] Is the merge commit non-empty and meaningful?

---

## Output Format

When executing this workflow, report in this format:

```markdown
## Merge Status
- Base branch: `custom-prod`
- Merged branch: `origin/main`
- Merge state: [clean / conflicts resolved / unfinished merge detected]

## Conflicts
- [file path]: [how resolved]

## Verification
- [command]: [pass/fail]

## Final Result
- Merge commit: `<hash>`
- Working tree: [clean / not clean]
```

---

## Core Principle

> **A good merge is not "make Git happy" — it is preserving upstream intent and branch-specific intent at the same time.**
