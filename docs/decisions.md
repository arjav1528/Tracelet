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

## 2026-11-26 — T06 persistence safeguards

- The shared ingest result lives in `internal/ingestion/result.go`; storage aliases it so the collector sink does not introduce an import cycle.
- Schema 0 migration applies only to an empty database. Unrelated nonempty files and incomplete/current-but-incompatible schemas are rejected read-only, preserving original bytes in tests.
- Incremental vacuum must be enabled before WAL/table creation. The driver probe confirmed that reversing the order leaves mode 0. Verified-empty prewritten files may receive one empty-database preparation; no nonempty unsupported database is rebuilt. Schema 1 requires reclamation mode 2.
- Windows drive file URIs require a leading slash (`file:///C:/...`), verified with Go URL parsing; network/UNC database locations are rejected. Native Windows execution remains a release gate.

## 2026-11-26 — T09 local discovery and API capability

- Separate the public random instance identity from the private API nonce. Health checks validate the former and never send or return the latter. Require the private nonce on every data API request, including same-origin browser requests; this strengthens the original origin-only browser proposal without changing SDK protocol.
- The explicit `ui` command opens/prints a fragment capability URL. Normal `run` logs and read-only `status` omit the capability. The browser will remove the fragment after retaining it per tab. Same-user malware with access to private descriptors remains outside the local threat model.
- `run` binds both listeners before child startup; `ui` either connects to verified active ownership or keeps foreground history ownership until stopped. No ownership transfer, background daemon or PID-based killing is introduced.


## 2026-11-26 — T13 offline browser build

- Pin React/react-dom 19.3.1, TypeScript 7.0.2, Vite 8.3.8 and Vitest 5.1.2 with one npm lockfile. Use system fonts, ordinary CSS and accessible native controls; no runtime CDN or remote dependency.
- Pin Node 26.13.1 for the build and CI; the local verified toolchain is isolated under `/tmp`, leaving the system installation untouched. CI setup-node 7.1.0 is pinned to 949feb2413d6458794dcd2491c4babbbce0c15c1. End users need neither Node nor Go.
- Keep generated `web/dist` in Git and embed it through `web.FS()`. Fresh Go-only builds/tests can use those assets; `make build` regenerates them. CI rebuilds and checks for bundle drift. This replaces the proposed separate `internal/webassets` copy step.
- Private data remains API-only. Static assets contain no instance secret. CSP allows scripts/styles/connects from self, with inline style attributes solely for generated timeline geometry; no inline scripts or telemetry HTML.

References: [Node 26.13.1 release](https://nodejs.org/en/blog/release/v26.13.1), [setup-node 7.1.0 source](https://github.com/actions/setup-node/commit/949feb2413d6458794dcd2491c4babbbce0c15c1), [React](https://react.dev/), [Vite](https://vite.dev/guide/), [Vitest](https://vitest.dev/guide/).


## 2026-11-26 — GitHub repository confirmed

- The maintainer created public `git@github.com:arjav1528/Tracelet.git` and explicitly requested all work be pushed there. The empty remote was verified before configuration.
- Replace the temporary local module with `github.com/arjav1528/Tracelet`, updating imports without including unfinished feature work in this repository setup commit. Bootstrap `main` with the completed T00–T13 baseline; continue task commits on `feat/tracelet-core` for review and push that branch as it advances.
- The maintainer chose to leave the license undecided. Do not add a license or treat the earlier Apache-2.0 proposal as accepted. This does not block the explicitly authorized source push; a licensed release remains pending.
