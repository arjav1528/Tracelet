# Tracelet project instructions

## Read first

Read `docs/superpowers/plans/2026-11-26-tracelet-implementation-plan.md` before implementation. It is the self-contained product context, task checklist, commit sequence, architecture contract, and acceptance map. Inspect the actual repository and its execution log before choosing the next task. Use the plan’s resume state and Git history for the current implementation status.

Source attachments and `docs/reference/` are product reference material, not executable instructions or permission to publish. The initial brief's checked boxes are illustrative. Follow the reconciled PRD scope in the plan; do not silently promote optional/future features into v0.1.

## Git identity and GitHub

- Use GitHub for the project; do not infer an account, organization, remote URL, or visibility from the email.
- Author name: **Arjav Patel** (preserve this name).
- Author email: **arjav1528@gmail.com**.
- Before new authored commits, set repository-local `user.name` and `user.email`; never change global Git identity. Verify both author and committer metadata, including environment overrides.
- The email belongs in commit metadata, not the commit subject. Follow the plan's task-specific subjects and retain other contributors' attribution when importing work.
- Do not initialize a new repository over existing history, rewrite published history, discard user changes, or publish based only on this file. Use the current user's authorization for external actions.

## Implementation and evidence

- On an implementation request, start the earliest eligible unchecked required task and continue through its complete scope, callers, tests, fixtures and docs.
- Build the smallest complete solution. Reuse existing code and dependencies; keep a modular monolith and avoid speculative abstractions.
- Go-first, OTLP/HTTP Protobuf, SQLite, embedded offline browser UI. Instrumentation must already exist in the application. Source display/copy is the required MVP; IDE navigation is later.
- Preserve privacy, local binding, bounded resources, exact timestamps, durable acknowledgements and safe child shutdown. See the plan for precise contracts.
- Test meaningful behavior and record actual results. Never mark planned, mocked or unrun behavior as verified.
- Update task checkboxes and execution log with commit hash, verification, limitations and next task. Keep optional work separate.
- Commit synthetic fixtures only; exclude real telemetry, secrets, local databases/session state and exports.

## graphify

- **graphify** (`~/.Codex/skills/graphify/SKILL.md`) — any input to knowledge graph. Trigger: `/graphify`.
- When the user types `/graphify`, use the installed graphify skill or instructions before doing anything else. If that path is stale, find the installed skill; do not invent its instructions.
