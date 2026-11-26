# Tracelet

Local-first application observability for developers.

**Status: CLI and project configuration implemented; capture and UI are not available yet.** Tracelet will receive OpenTelemetry traces from an instrumented Go application, store them locally, and display a browser waterfall with span details and available source locations.

Start with the **[complete implementation plan and agent handoff](docs/superpowers/plans/2026-11-26-tracelet-implementation-plan.md)**. That single Markdown file contains the product context, architecture, 25 required tasks, dependencies, tests, planned commits, GitHub workflow, optional work, future backlog, and full requirement-to-task mapping. It can be given to another model without the original attachments.

- [Project instructions](AGENTS.md)
- [Original PRD transcription](docs/reference/tracelet-prd.md)
- [Original product brief](docs/reference/initial-product-brief.md)

The source documents contain proposals and illustrative completion checkboxes; they are not evidence that features exist. The handoff resolves differences between them and records remaining decisions.

New authored commits must use **Arjav Patel <arjav1528@gmail.com>**, configured locally for this repository. Local Git is initialized with the requested identity. Implementation progress and verification are recorded in the handoff. No GitHub remote has been created. GitHub owner, visibility, license, and release targets remain decisions for implementation.

## Development

Use the pinned Go toolchain, then run `make check` and `make build`. The current executable supports `./dist/tracelet version`, `init`, and help. See [configuration](docs/configuration.md) for safe defaults and overwrite behavior. The full capture workflow will be added in subsequent planned tasks.
