<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

Use the `/trellis:start` command when starting a new session to:
- Initialize your developer identity
- Understand current project context
- Read relevant guidelines

Use `@/.trellis/` to learn:
- Development workflow (`workflow.md`)
- Project structure guidelines (`spec/`)
- Developer workspace (`workspace/`)

Keep this managed block so 'trellis update' can refresh the instructions.

<!-- TRELLIS:END -->

## Local Test Environment

- After completing changes, update and verify the isolated test environment at
  `/opt/stacks/sub2api-test` (`http://152.53.90.186:4178`) by default. The user has
  authorized keeping it current; do not ask again whether to update this environment.
- Preserve existing test data and the no-real-payment/no-email isolation. Apply
  necessary test migrations and update only the test app/frontend; verify health
  and the changed flows before reporting completion.
- This standing authorization does **not** cover production. Never update the
  local production stack or Netherlands production without explicit authorization.
