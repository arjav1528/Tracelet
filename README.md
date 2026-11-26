# Tracelet

Local-first application observability for developers.

**Status: local capture, stored history and the browser trace list work; waterfall and detailed inspection are in progress.** Tracelet receives OpenTelemetry traces from an instrumented Go application and stores them locally.

Start with the **[complete implementation plan and agent handoff](docs/superpowers/plans/2026-11-26-tracelet-implementation-plan.md)**. That single Markdown file contains the product context, architecture, 25 required tasks, dependencies, tests, planned commits, GitHub workflow, optional work, future backlog, and full requirement-to-task mapping. It can be given to another model without the original attachments.

- [Project instructions](AGENTS.md)
- [Original PRD transcription](docs/reference/tracelet-prd.md)
- [Original product brief](docs/reference/initial-product-brief.md)

The source documents contain proposals and illustrative completion checkboxes; they are not evidence that features exist. The handoff resolves differences between them and records remaining decisions.

New authored commits must use **Arjav Patel <arjav1528@gmail.com>**, configured locally for this repository. Local Git is initialized with the requested identity. Implementation progress and verification are recorded in the handoff. The public repository is [arjav1528/Tracelet](https://github.com/arjav1528/Tracelet). The maintainer has authorized pushing completed work there. The project license remains undecided; no license file has been added.

## Development

Use the pinned Go toolchain and Node version in `.node-version`. Run `npm --prefix web ci`, `make check`, `make web-check` and `make build`. Node is a build dependency only; the installed binary embeds the UI. The current executable supports `./dist/tracelet version`, `init`, `run`, `ui`, `status`, and help. See [configuration](docs/configuration.md) for safe defaults and overwrite behavior. Follow the [Go sample guide](docs/instrumentation-go.md) to capture real traces.

See [sessions and history](docs/sessions.md) for local ownership and browser discovery. The browser reads real local traces and includes operation search, sorting, paging, session status and read-only settings.
