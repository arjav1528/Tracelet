# Engineering decisions

## 2026-11-26 — T00/T01 foundation

- Local author and committer: Arjav Patel <arjav1528@gmail.com>. The initial documentation commit is `695c49a`.
- Work on `feat/tracelet-core` in the new checkout. `main` retains the initial planning baseline. There was no existing application work requiring a second checkout.
- GitHub owner, visibility and license remain open. Local work uses `tracelet.local/tracelet`; replace it and all imports with the verified GitHub module path before publication.
- Pin Go 1.27.4 as the build toolchain (module language floor 1.27.0) and Cobra v1.10.2. Cobra is Apache-2.0 licensed; Go uses its BSD-style license. The installed system Go was 1.27.0, so Go's toolchain management supplies the selected patch version without changing the system installation.
- CI actions are pinned to reviewed release commits: checkout v7.0.1 and setup-go v7.1.0. CI is read-only; no release credentials or publication workflow are introduced at this stage.
- No frontend or ingestion capability is claimed by the version command. Those arrive in their owning tasks.

References: [Go releases](https://go.dev/dl/), [Cobra release](https://github.com/spf13/cobra/releases/tag/v1.10.2), [checkout](https://github.com/actions/checkout/releases/tag/v7.0.1), [setup-go](https://github.com/actions/setup-go/releases/tag/v7.1.0).

## 2026-11-26 — T02 storage and protocol

- Real Go SDK gzip exports and fresh-process persistence passed using OpenTelemetry v1.47.0 and generated OTLP v1.11.1. Use official generated Protobuf decoding in-process; a bundled collector process is unnecessary for the current scope.
- Select modernc SQLite v1.63.0 with its required libc v1.80.1, bounded scalar indexes plus Protobuf details, one writer, WAL and synchronous FULL as the production starting point. The synthetic comparison favored this representation for write cost and storage size; list-query latency was similar. This does not establish production NFR performance.
- T03 configuration and T04 model work may be prepared in parallel with the isolated spike because neither depends on benchmark-selected schema details. Commits and task completion remain in plan order. Cost if a spike invalidates these independent contracts: revisit their uncommitted changes before accepting them.
- Project YAML uses the maintained security-fix branch `go.yaml.in/yaml/v3` v3.0.6 with strict field decoding and domain validation. Its distribution contains Apache-2.0 and MIT-licensed portions. The parser dependency is prepared alongside the spike dependencies for the immediately following T03.

References: [OTel exporter module](https://github.com/open-telemetry/opentelemetry-go/tree/v1.47.0/exporters/otlp/otlptrace/otlptracehttp), [OTLP generated types](https://github.com/open-telemetry/opentelemetry-proto-go/tree/v1.11.1), [SQLite driver](https://pkg.go.dev/modernc.org/sqlite), [YAML parser](https://github.com/yaml/go-yaml/tree/v3.0.6).
