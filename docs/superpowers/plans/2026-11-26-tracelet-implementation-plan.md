# Tracelet Implementation Plan and Complete Agent Handoff

> **For agentic workers:** This file is self-contained. Read it before implementation, inspect the actual repository, then execute the first eligible unchecked task. Work task by task, test each meaningful behavior, and record evidence before checking boxes. If available, use `superpowers:subagent-driven-development` or `superpowers:executing-plans`; no particular tool, plugin, model, or access to the original attachments is required.

**Goal:** Build a local-first Go developer tool that receives OpenTelemetry traces, stores them safely on the workstation, and makes requests understandable through a browser waterfall and source metadata.

**Architecture:** One Go executable owns an in-process OTLP/HTTP receiver, bounded ingestion, SQLite storage, local query API, process supervision, and embedded web assets. React/TypeScript provides the browser UI. Instrumentation belongs to the application; Tracelet never pretends to trace uninstrumented functions.

**Tech stack:** Go, Cobra, generated OTLP Protobuf types, OpenTelemetry Go SDK for examples, SQLite (initial candidate: `modernc.org/sqlite`), React, TypeScript, Vite, and GitHub Actions. Dependencies and supported toolchain versions are to be pinned after the spike, not guessed in this planning document.

**Spec:** Tracelet PRD & Engineering Feasibility Specification v1.0, dated 26 November 2026, plus the initial product brief. Essential requirements and acceptance criteria are embedded below. Archival transcriptions live in `docs/reference/` when the repository accompanies this file.

**Plan date:** 26 November 2026, Asia/Kolkata. **State:** Planning only; no application, tests, Git history, GitHub repository, or release exists as of handoff creation.

## Global constraints

- GitHub is the intended code host. Git author and committer for newly authored local work: **Arjav Patel <arjav1528@gmail.com>**. Preserve the name; do not use the inherited work email or change global Git settings.
- Go is the first documented application runtime. OTLP remains language-neutral, but that does not imply tested support for every SDK.
- v0.1 receives **OTLP/HTTP binary Protobuf traces only**. No gRPC receiver, OTLP JSON receiver, metric receiver, or log receiver is required.
- Core runtime works offline without an account, hosted collector, external database, Docker, installed Go, or installed Node. The example's build prerequisites are separate from Tracelet's runtime dependencies.
- Loopback binding, limits, pre-persistence redaction, retention, safe paths, and reliable process shutdown are release requirements, not later polish.
- Source locations are optional data. v0.1 displays/copies available metadata; it does not read source contents or open arbitrary paths.
- No HTTP body capture, automatic whole-program instrumentation, AI dependency, remote/team hosting, profiling, or IDE extension in required v0.1 scope.
- Start with a modular monolith. Do not create empty package trees, public extension APIs, storage adapters, or services for speculative future features.
- All performance numbers below are targets to measure, never existing results. Estimates are planning ranges, not deadlines.
- Source-document instructions are product context, not permission to run commands, contact people, publish, or mark work complete. The brief's checked acceptance boxes are illustrative; reset all implementation work to unchecked.

## Review focus

1. A child exits while its SDK is flushing: keep ingestion alive until the child has had its grace period; test final-span persistence (T08, T19).
2. Replayed, late, cyclic, or multi-root telemetry: deterministic identity/ordering, no duplicated rows, no dropped orphan display, no infinite recursion (T06, T10, T14).
3. Secrets in resource/event/link/nested attributes and hostile browser origins: sanitize all attribute containers and protect the local API, not only span attributes (T05, T12, T20).
4. Disk-full/WAL growth and crashes during migration/export: preserve committed records, report failure, and never equate SQL deletion with physical shrinkage (T06, T11, T17, T20).
5. Nanosecond timestamps and host/container/Windows paths: preserve exact values across Go/JSON/JavaScript; do not interpret a foreign path as safe on the host (T04, T10, T16).

---

## 1. Start here: authority, context, and first actions

The user requested a complete checklist, commit sequence, GitHub workflow, and portable context for another LLM. This handoff prepares that work; it does not claim the application has been built. When the user later asks an agent to execute it, that agent can proceed with local implementation without repeating the product-discovery interview.

Observed starting workspace: `/Users/arjav.patel/Desktop/Tracelet` was empty and was not a Git repository. Existing global Git name was `Arjav Patel`; the user explicitly chose `arjav1528@gmail.com` for this project. The GitHub owner, repository URL, visibility, license approval, and account email verification have not been established. Do not infer the GitHub username from the email. The original files were:

- `/Users/arjav.patel/Downloads/Tracelet_PRD_and_Feasibility_Specification.docx`
- `/Users/arjav.patel/.codex/attachments/c54697fa-52d8-4bd1-878b-83cfd91aab6a/Pasted text.txt`

These absolute paths are provenance only; implementation must not depend on them.

### Source reconciliation

| Topic | Planning baseline | Reason / remaining choice |
|---|---|---|
| Schedule | Spike 3–5 engineering days; prototype 1–2 weeks; alpha 6–8 weeks; public v0.1 about 10–14 weeks | Detailed PRD supersedes the brief's optimistic eight-week sketch. One experienced, substantially full-time engineer assumed. |
| Transport | HTTP/Protobuf first | Brief proposed HTTP and gRPC; detailed PRD narrows the release. |
| Source navigation | Display/copy file, function, line; safe workspace classification | Brief promised editor navigation; direct IDE opening is deferred. |
| Comparison | Optional pairwise comparison; robust regressions later | Not a P0 release blocker despite the brief's acceptance list. |
| Request inspector | Inspect HTTP/DB/outbound-call attributes, events, errors already present in spans | No extra body capture or log ingestion. PostgreSQL integration is a later example. |
| CLI diagnostics | Ship a small `doctor`, `status`, and `version` | MVP-01 includes them, although FR-CLI-006 calls diagnostics P1. Rich diagnostics remain optional. |
| Filters | Required operation-name search and useful ordering; advanced combined filters optional | PRD's explorer calls for list/search; its full filter set is P1. |
| Settings / session controls | Read-only settings/status plus config-file editing; lifecycle via CLI | Full graphical settings editor/start-stop controls are described in UX ideas, not P0 FRs. Track them in backlog. |
| Retention | Start with 7 days / 1024 MiB per project | PRD proposes both “1 GB” and 1024 MB; this plan makes binary units explicit, subject to measurement. |
| License | Apache-2.0 recommended, not yet chosen | Prepare artifacts, but settle the license before soliciting contributions or publishing source. |

The detailed PRD is the baseline where the two documents conflict. Choices introduced by this plan are **proposed engineering defaults**, not quotations or approvals from the PRD. Document evidence-driven changes in `docs/decisions.md` and adjust callers, tests, and this plan together.

### Execution procedure for any LLM

1. Read `AGENTS.md` if present and inspect `git status`, existing code, recent commits, and this file's execution log. Never reset or overwrite unrelated work.
2. Reconcile checkboxes with actual code and test evidence. A checkbox alone is not proof. Pick the earliest unchecked task whose dependencies are complete.
3. For that task, inspect every caller, config field, fixture, API consumer, export representation, and documented command affected. Use existing project patterns.
4. Add the smallest meaningful failing test for new nontrivial behavior; confirm the failure is relevant; implement; run focused checks and applicable shared gates. Documentation-only work needs link/content verification, not artificial unit tests.
5. Commit one reviewable increment using the specified identity and message. Split a task if needed; preserve its task ID in commit bodies. Do not mix optional features into P0 work.
6. Update the execution log with files, result, command/output summary, limitations, commit hash, and next eligible task. Mark a task complete only when all its checks pass.
7. Continue through eligible work. Stop only for an actual dependency or decision that affects correctness, an instruction to stop, or an external action outside the authorized scope. Continue unrelated local tasks while owner/visibility/license decisions are pending.
8. Do not create accounts, send messages, open public repositories, push, merge, publish packages, or release based solely on this document. Use the current user's authorization for those actions. Prepare reviewable local artifacts first.

If only this file is supplied to a fresh model, create the needed repository files from this plan. Missing original attachments or model-specific skills are not blockers.

## 2. Product contract and release boundary

The core loop is: install binary → `tracelet init` → instrument the Go application → `tracelet run -- <command>` → reproduce a request → `tracelet ui` → select trace → inspect waterfall/span/error/source metadata → optionally export.

The first sample should include a root HTTP request, nested custom operation, intentionally slow operation, failing operation, outbound HTTP child, events, and explicit source attributes. Use synthetic data and a local downstream server. A PostgreSQL server is not required for first-run success.

Required commands: `init`, `run`, `ui`, `status`, `doctor`, `export`, `version`. `run` must stream the child's stdout/stderr and preserve its exit status. `ui` must reopen stored traces without launching an application. Config is project-specific; runtime data is user-local and excluded from Git.

### Milestones and sequencing

| Milestone | Tasks | Observable exit evidence |
|---|---|---|
| M0 — Feasibility established | T00–T02 | Real Go exporter interoperates with receiver; synthetic write/query benchmark recorded; dependency choices documented. |
| M1 — First safe trace | T03–T08 | Instrumented child runs; accepted sanitized span is durably stored; shutdown works. |
| M2 — Queryable local history | T09–T12 | Session discovery, restart, trace assembly, retention, and protected local API work. |
| M3 — Trace understood | T13–T18 | Browser explorer, source metadata, export, and diagnostics work with actionable empty/error states. |
| M4 — Safe and portable alpha | T19–T21 | End-to-end, security, crash recovery, and measured performance evidence exist. |
| M5 — Public v0.1 candidate | T22–T24 | Tested binaries, documentation, source/legal decisions, release evidence, then authorized publication. |

Critical path: M0 → M1 → M2 → M3 → M4 → M5. UI work may use agreed fixtures after T04/T12 contracts are stable; it must integrate with real ingestion before M3 closes. Documentation, product validation, and platform research can proceed in parallel. Do not assign two agents ownership of the same file at the same time.

## 3. Proposed engineering decisions and defaults

These choices let implementation start. Validate the marked spike items before committing to the public contract.

| Decision | Starting choice | Validation / change gate |
|---|---|---|
| Process topology | Foreground owner, no background daemon | `run` owns capture; `ui` connects to active owner or serves stored history itself. See lifecycle contract. |
| HTTP topology | Separate receiver and UI/API listeners, both `127.0.0.1:0` by default | Hold both listeners from bind to shutdown; never discover then reopen a free port. |
| Storage | One SQLite DB per project; one bounded writer | Benchmark `modernc.org/sqlite` and WAL/durability settings in T02/T06. |
| Model | Generated OTLP types at ingestion; compact query DTOs | Preserve typed attributes and raw identity; use decimal strings for nanoseconds in JSON. |
| Delivery | Acknowledge only after transaction commit | Document timeout-after-commit ambiguity and retry idempotency; no success for queue-only acceptance. |
| Duplicate spans | First committed sanitized record wins for `(trace_id, span_id)` within a project DB | Count conflicting duplicates without logging values; late new span IDs are accepted. No invented merging of final spans. |
| Source | Display/copy only, no file-content endpoint | Canonical containment and symlink checks; inaccessible/foreign paths are unmapped. |
| UI | React/TS + Vite; local assets/fonts; virtualization when benchmark requires it | Use accessible native elements or established small components; no new design framework solely for decoration. |
| Distribution | Go executable with embedded production web build | No native desktop shell or mandatory Docker. |
| Initial module path | `tracelet.local/tracelet` for private local work if owner remains unknown | Replace with verified `github.com/<owner>/tracelet` and update imports before publishing. Placeholder owner is never a command to run literally. |

### Configuration v1 proposal

Finalize the schema in T03 and document it in `docs/configuration.md`. Resolve relative paths against the config directory/workspace definition, not whichever directory the process later enters. Unknown keys, unsupported versions, nonpositive limits, and invalid paths must yield actionable errors. Proposed values:

| Setting | Initial default / rule |
|---|---|
| Config path and version | `.tracelet/config.yaml`, `version: 1`; `project.id` generated once and stored; `project.name` from directory name |
| Data directory | OS user-data directory under `tracelet/projects/<project-id>/`; explicit `storage.path` may override |
| Collector | `bind: 127.0.0.1`, `port: 0`, `protocol: otlp-http-protobuf` |
| UI | `bind: 127.0.0.1`, `port: 0`, `open_browser: true`; `ui --no-open` prints URL |
| Storage | `retention_days: 7`, `max_size_mb: 1024` (MiB, document the historical field spelling) |
| Shutdown | `shutdown_grace_seconds: 5`; receiver stays alive during child grace, then bounded request/write drain |
| Request bounds | 4 MiB compressed/wire body; 16 MiB decoded body; 5,000 spans per batch; 4 concurrent decoded requests |
| Attribute bounds | 128 entries per container; 256-byte keys; 4 KiB string/byte values; nesting depth 8; 128 events and 128 links per span |
| Queue bounds | At most 8 pending batches and 32 MiB of estimated normalized data, whichever limit is reached first |
| HTTP timeouts | 5 seconds headers; 15 seconds whole read; 15 seconds response; 60 seconds idle; make shutdown/write deadlines consistent |
| Source | `workspace_root: .` resolved to workspace; no mappings in required MVP |
| Privacy | `capture_http_bodies: false` only; reject enabling it in v0.1; configurable key globs added to safe defaults |

Limits are conservative project choices, not OTLP-mandated defaults. Bound decoded allocations/concurrency before fully materializing arbitrary payloads; a channel length alone is not a memory bound. Reject out-of-policy spans rather than silently truncating telemetry facts; UI display truncation does not change stored values. A malformed/oversized envelope rejects the request; a structurally valid mixed batch may use OTLP partial success with accurate rejected-span counts after accepted spans commit. Sanitize diagnostics.

### Ingestion and normalization contract

- Route: `POST /v1/traces`, `application/x-protobuf`; accept uncompressed and gzip with independent wire/decoded limits. Return generated Protobuf responses/errors, distinguish permanent input rejection from retryable overload/storage failure, and test with a real SDK. HTTP success is 200 with `ExportTraceServiceResponse`; empty batches succeed; partial success includes rejection counts. Verify the precise failure codes against the pinned protocol before implementation. [OTLP specification](https://opentelemetry.io/docs/specs/otlp/)
- Preserve resource/scope metadata and schema URLs, trace/span/parent IDs, timestamps, span kind/status, trace state/flags where present, attributes with their types, events, links, and OTLP dropped counts. This project's retention, redaction, and explicit limits are the only intentional reductions.
- Validate 16-byte trace IDs and 8-byte span IDs, nonzero required IDs, legal optional parent ID, timestamp representability and end ≥ start. Treat missing/zero end as invalid for the initial final-span model; allow valid zero duration. Never silently clamp invalid duration.
- All attribute containers go through one sanitizer before persistence: span, resource, scope, event, link, nested arrays/key-values. Redacted keys stay visible with `[REDACTED]` and a marker; do not retain their original values in auxiliary summaries.
- Initial matching: case-insensitive full-key glob, `*` matches any sequence and `?` one character; match each nested key and its dotted path. Defaults cover authorization/proxy authorization, cookies/set-cookie, password/passwd, token, secret, API key, email, body/payload, `url.full`, `url.query`, `http.url`, and raw SQL/query text (`db.statement`, `db.query.text`). Write exact patterns in config/docs and fixtures; favor privacy over raw query visibility initially.
- Key redaction cannot guarantee removal of secrets in arbitrary span names, exception messages, stack traces, free-text values, or the child program's own stdout/stderr. Do not claim comprehensive PII removal. Tracelet diagnostics contain counts/reasons, never raw payloads. Export docs explain residual risk; there is no unredacted mode in v0.1.
- No automatic SDK injection or code rewriting. Respect explicit SDK/exporter configuration. Environment variables cannot override programmatic SDK options reliably; explain that limitation.

### Child environment contract

Resolve existing trace-specific exporter values before generic values. If either explicitly provides an endpoint/protocol, preserve it and report whether capture will reach Tracelet. When neither provides a value, add trace-specific `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://127.0.0.1:<port>/v1/traces` and `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL=http/protobuf`. Do not set generic defaults that accidentally redirect metrics/logs. A trace-specific HTTP endpoint includes the route; a generic endpoint is a base URL. [Exporter configuration](https://opentelemetry.io/docs/languages/sdk-configuration/otlp-exporter/)

`run --override-otel -- <command>` explicitly replaces the child's trace endpoint/protocol. It does not change the parent's environment, inject instrumentation, enable disabled exporters, override sampling, or rewrite SDK code. Preserve `OTEL_SERVICE_NAME` or `service.name` in `OTEL_RESOURCE_ATTRIBUTES`; set a project-based service name only if neither supplies one. Do not print headers, URL credentials, or secrets while reporting effective configuration. Test endpoint-only, protocol-only, generic, trace-specific, disabled SDK, explicit service name, and programmatic override cases.

### Lifecycle and session ownership contract

1. Acquire an OS-backed project lock before writable DB ownership. Store a private session descriptor atomically with project ID, owner PID, random instance nonce, UI/receiver endpoints, version, and role. PID alone is insufficient because PIDs are reused.
2. `run` validates config, opens DB, binds/retains listeners, passes readiness checks, publishes discovery state, then starts the child directly with argument boundaries intact; do not invoke a shell unnecessarily. Publish no ready descriptor on failure.
3. `ui` connects to a healthy matching active instance and opens/prints its URL. With no owner, it starts a foreground history UI with DB ownership and retention, no child and no receiver. It prints how to stop it and stays running. A subsequent `run` reports that the foreground history UI must be stopped; seamless ownership transfer is deferred.
4. `status` and `doctor` do not start services. Validate descriptor identity via a bounded health check; an unrelated process on a reused port is not Tracelet. Clear stale descriptors only while holding the project lock. Never kill a process merely because a descriptor names its PID.
5. First interruption forwards termination to the child process group/tree, leaves receiver available for SDK shutdown, waits up to the configured grace, then terminates remaining descendants with platform-specific logic. Drain already active requests/writes, close listeners/DB, remove only this instance's descriptor, release lock. Handle a second interrupt as bounded forced termination.
6. Normal child exit still drains active requests/writes. A clean successful child returns its exit code; signal termination gets documented platform mapping. If Tracelet fails after a successful child, return a Tracelet failure code, not false success. Preserve a nonzero child exit code and additionally report any internal cleanup failure. Accepted committed data survives abnormal exit.
7. `export` queries the active owner when present, otherwise takes the project lock and opens storage directly. It must not race another writer or migrate a live DB from a second process.

### Storage, time, and trace assembly contract

- Project-isolated DB. Model project/session, resources, instrumentation scopes, traces/summaries, spans, events, links, and typed attributes. Do not use span ID alone as a globally unique key.
- Begin with indexed scalar columns for identity, parents, time, operation, service, status, and a bounded Protobuf or typed JSON payload for details. Compare with normalized attribute rows in the spike; arbitrary attribute indexing is out of scope. Use canonical sanitized resource/scope content for deduplication, including schema identity.
- Define explicit migrations from schema 0 to 1. Newer unsupported schema means fail safely, never delete/recreate. Run upgrades transactionally where possible and test rollback/recovery. Use foreign keys and transactional summary maintenance.
- Use signed integer nanoseconds only within validated range. JSON/UI/export use decimal strings for epoch nanoseconds, durations, and int64 attribute values. Do not convert absolute nanoseconds into JS `Number`; calculate offsets with `BigInt`, then convert bounded relative values for drawing.
- Trace duration is `max(end) - min(start)` over observed valid spans; never sum overlapping children. Roots and orphans are distinct. Multiple services/roots are valid. Stable ordering: start time then span ID; root display choice is deterministic and documented.
- Missing parents, cycles, absent root, and time skew are warnings. Break cycles for display without altering stored identities. No “complete” claim based solely on having a root or a quiet period: unseen spans may still exist.
- Pagination is keyset based using the selected sort tuple plus trace ID (start time for newest; duration for slowest; error state then start time for errors). Bounded pages prevent loading the entire database. Stability is guaranteed for an unchanged dataset; expose a dataset revision and restart paging after changes to sort values. Summary counts reflect deduplicated rows. Newly accepted late spans update bounds, counts, last-seen time and error status; duplicate-only replay does not refresh retention age.
- Associate spans with the receiving session for provenance, not to rewrite their trace IDs. A trace may receive data across sessions; account for that explicitly in future comparison.
- Retention runs incrementally at startup and periodically (initially every minute). Prune whole oldest-eligible traces by last-seen time, retaining recently active traces for a short documented quiet window (initially 30 seconds). Count main DB, WAL and sidecars in reported disk use. Reuse free pages and checkpoint safely; measure physical reclamation rather than assuming `DELETE` shrinks disk.
- If no eligible data can be removed and the budget cannot safely admit another batch, reject new ingest visibly until space is available. Allow documented bounded transaction/WAL overhead rather than claiming a byte-exact hard cap. Do not wipe the DB on disk-full or corruption; preserve it and give recovery instructions.

### Local API, browser, and export contract

Proposed API prefix `/api/v1`, same origin as the bundled UI. Implement read-only routes first:

| Route | Result |
|---|---|
| `GET /healthz` | Minimal version/instance identity, no telemetry or secrets |
| `GET /api/v1/status` | Project/session, receipt/rejection counts, last receipt, storage/retention state, safe effective settings |
| `GET /api/v1/traces?limit=50&cursor=...&q=...&sort=newest` | Trace summaries, opaque next cursor; max page 200; operation-name search; newest/slowest/errors ordering |
| `GET /api/v1/traces/{trace_id}` | Summary, trace revision/count, bounded waterfall-node pages and hierarchy warnings |
| `GET /api/v1/traces/{trace_id}/spans/{span_id}` | Sanitized full span details including resource/scope/event/link/source fields |
| `GET /api/v1/traces/{trace_id}/export` | Versioned export with sanitized data and attachment headers |

Proposed Go seams, finalized by their owning tasks: `config.Load(path string) (Config, error)`, `privacy.Sanitize(req *collectortracepb.ExportTraceServiceRequest, policy Policy) (Report, error)`, `storage.Open(ctx context.Context, cfg Config) (*Store, error)`, `(*Store).Ingest(ctx context.Context, batch *collectortracepb.ExportTraceServiceRequest) (IngestResult, error)`, `(*Store).ListTraces(ctx context.Context, query TraceQuery) (TracePage, error)`, `(*Store).Trace(ctx context.Context, traceID string) (TraceDetail, error)`, `trace.BuildTree(spans []SpanSummary) Tree`, `sourcemap.Resolve(root string, location Location) Result`. Put transport DTOs in `internal/trace` and keep generated Protobuf types at boundaries; do not create broad public interfaces solely for mocks. Names can change with a decision-log entry and coordinated callers/tests.

Collect all timeline-node pages for one trace revision before final missing-parent classification; an unloaded parent is not an orphan. A revision change restarts paging, and the UI shows loading/recently-updated state rather than false incomplete-data warnings. Keep each page bounded; large-trace tests exercise this contract.

Validate all path/query inputs, restrict sort keys to an allowlist, parameterize SQL, and return a uniform safe error `{error:{code,message}}`. Never include SQL/payloads/secrets in errors. Non-OTLP API errors use suitable HTTP statuses; no dynamic filesystem route or command execution endpoint.

Loopback binding alone is insufficient. Enforce allowed Host/port and browser Origin; no wildcard CORS, reject hostile/null origins and cross-site browser fetches, serve no sensitive state through unauthenticated cross-origin scripts, and test DNS-rebinding-style Host values. Allow originless SDK traffic on the collector with appropriate content type. On the API, accept same-origin browser requests that omit Origin after Host/fetch-metadata checks, and native local CLI requests authenticated with the private instance nonce in a header. Reject a supplied hostile Origin or cross-site fetch metadata. Test these cases separately; do not reject every request merely because Origin is absent. Do not invent a mandatory OTLP auth extension. The threat model excludes a malicious same-user local process that already has filesystem access. Add restrictive CSP/frame policy, no third-party/CDN assets, no HTML rendering of telemetry, and clear caching rules for private responses. Non-loopback support is rejected in v0.1 rather than advertising unreviewed remote hosting.

Export proposal: `{format:"tracelet.trace", version:1, exported_at:<RFC3339>, trace:<summary>, resource_spans:<OTLP ProtoJSON data>, warnings:[]}`. The format document must show an example, type/encoding rules, schema version, redaction marker, and completeness limitations. Full trace structure, all retained typed values, scope/events/links, nanosecond precision and warnings must survive an export/reparse fixture test. Read summary, resources/scopes and spans from one SQLite read transaction/snapshot; concurrent retention yields a complete earlier snapshot or clear not-found, never a partial successful export. Generated ProtoJSON byte fields (including trace/span IDs) use base64; query-summary IDs use hexadecimal strings. This is a Tracelet envelope, not an OTLP/JSON wire request or a promise of direct import into any OTLP viewer; an import command is deferred. Reapply current privacy policy on export of older records. `--out` creates a file exclusively or requires explicit overwrite; use a private temp file and atomic replacement where supported, clean up on failure, and keep stdout JSON separate from stderr diagnostics.

### Source location and UI contract

Recognize `code.file.path`, `code.function.name`, `code.line.number`; missing metadata is normal. These are stable semantic-convention names in the checked registry. [Code attributes](https://opentelemetry.io/docs/specs/semconv/registry/attributes/code/)

Resolve relative paths against the configured workspace, canonicalize existing roots/targets and symlinks, compare path components, and reject traversal, sibling-prefix tricks, foreign Windows/UNC paths, and external symlink escapes. Missing files are `unavailable`; external/unresolvable paths are `unmapped`; no metadata is `not_provided`. Lines must be positive integers to be presented as valid. v0.1 can stat a safe path for classification but never returns source contents. Do not use browser `file://` or run editor shell commands.

UI screens: project/session overview, trace list, waterfall with span panel, and read-only effective settings. Required states: no config, no active session, no telemetry, rejected telemetry, no matching traces, loading, API error, trace removed by retention, incomplete trace, missing/unmapped source. Include keyboard selection, collapse/expand, fit/zoom, copyable IDs/location, event/error inspection, readable contrast, text status labels, long-value inspection, local time with explicit UTC details, and responsive narrow/wide layouts. Indicate redaction and instrumentation limitations honestly. Use manual refresh first; live updates are optional.

## 4. File and responsibility map

Create these files only when the owning task needs them. Names are concrete planning targets, not a requirement to split trivial code across empty layers.

```text
AGENTS.md                              portable project rules and handoff pointer
README.md                              current project status; becomes quickstart
go.mod / go.sum                         pinned backend dependencies
Makefile                               documented local convenience targets
cmd/tracelet/main.go                    thin CLI entrypoint
internal/cli/{root,init,run,ui,status,doctor,export,version}.go
internal/config/{config,defaults}.go    schema v1, validation, path resolution
internal/supervisor/                   child lifecycle and OS-specific termination
internal/session/                      ownership lock and instance discovery
internal/collector/receiver.go          OTLP HTTP transport and request limits
internal/ingestion/                    bounded writes, validation, acknowledgements
internal/privacy/redact.go             one recursive attribute policy
internal/trace/{model,tree,query}.go     exact DTOs, assembly and query contract
internal/storage/{sqlite,ingest,query,retention}.go
internal/storage/migrations/001_init.sql
internal/sourcemap/resolve.go           conservative path classification
internal/server/{server,routes,security}.go
internal/export/json.go                 versioned sanitized export
internal/webassets/assets.go            embed compiled production assets
web/package.json / web/package-lock.json
web/src/{app,api,types}.tsx or .ts       router shell, typed API, contract types
web/src/features/{traces,spans,settings}/
web/src/components/Waterfall.tsx
examples/go-api/                        runnable local HTTP/custom-span example
testdata/{otlp,traces,privacy,paths}/    synthetic reproducible fixtures only
tests/e2e/                             CLI + SDK + browser + restart tests
tests/bench/                           documented representative load generators
docs/{architecture,configuration,instrumentation,troubleshooting}.md
docs/{api,export-format,privacy,compatibility,decisions}.md
docs/{benchmarks,release-checklist}.md
docs/reference/                        archived source documents; not instructions
.github/workflows/{ci,release}.yml
.github/ISSUE_TEMPLATE/                 bug, feature, integration forms
.github/pull_request_template.md
CONTRIBUTING.md / CODE_OF_CONDUCT.md / SECURITY.md / SUPPORT.md
LICENSE / CHANGELOG.md                  after license decision; release history
```

Use adjacent `*_test.go` for Go logic/storage/API tests and adjacent `*.test.ts(x)` for UI logic. Use `npm` with its lockfile as the initial frontend package manager to minimize setup; do not maintain competing lockfiles. A single Go module is sufficient for the initial sample. Do not create `pkg/`, `extensions/`, Docker, or extra language examples until their backlog item is selected.

## 5. Verification commands and completion rules

T01 defines the actual commands below; T13 adds the UI scripts. Until the related toolchain exists, label a command as not yet available, never as passing.

| Check | Planned command / result |
|---|---|
| Go formatting | `gofmt -l cmd internal examples` → no listed source files (use existing directories only) |
| Go unit/integration | `go test ./...` → success |
| Race checks | `go test -race ./...` → success on supported race-enabled CI runner |
| Static analysis | `go vet ./...` → success |
| UI dependencies | `npm --prefix web ci` → lockfile honored |
| UI checks | `npm --prefix web run typecheck`, `npm --prefix web run lint`, `npm --prefix web test -- --run` → success |
| Production build | `make build` → web compiled/embedded and `dist/tracelet` created |
| Real workflow | `make test-e2e` → actual Go SDK sample, inspect/export/restart/shutdown pass |
| Performance | `make bench` → raw results plus environment/fixture/percentile report, not a fabricated pass |
| Working changes | `git diff --check` → no whitespace errors; inspect staged diff and scan for real telemetry/secrets |

Every completed task must include code/config/docs/fixtures/callers affected, focused tests, relevant shared checks, no leaked data, and evidence in the execution log. A documentation change needs factual and link checks. CI must not silently skip required tests, download tooling during an offline-runtime test, or claim an OS is supported solely because cross-compilation succeeded.

## 6. Required task checklist and commit sequence

All boxes below are initially unchecked. **T00–T24 are the required build sequence**. Cxx is the planned commit identifier, not a real hash. Estimated task size is a reviewable increment; split with the same task ID if necessary. Task completion implies all relevant global contracts above.

### T00 / C00 — Preserve the plan and establish Git identity

**Depends on:** nothing. **Files:** this plan, `AGENTS.md`, `README.md`, `docs/reference/`, `.gitignore`.

- [x] Read current files and verify the initial status; retain all user work. Initialize a local `main` repository only if none exists.
- [x] Set repository-local author name/email as specified in §7; verify both author and committer identity before the first commit. Do not edit global identity.
- [x] Ignore runtime data, `.tracelet/` local state, DB/WAL/SHM files, `.env*` except deliberate examples, trace exports, build products, and dependency directories. Commit only synthetic fixtures.
- [x] Review source provenance, plan scope, and links; record unresolved GitHub owner/visibility/license without blocking local work.
- [x] Commit: `docs(plan): add Tracelet implementation handoff and delivery checklist`.

**Done when:** planning documents are tracked with correct identity, no runtime data is tracked, and the repository is ready for T01.

### T01 / C01 — Bootstrap a reproducible executable and basic CI

**Depends on:** T00. **Files:** `go.mod`, `go.sum`, `cmd/tracelet/main.go`, `internal/cli/root.go`, `internal/cli/version.go`, `Makefile`, `.github/workflows/ci.yml`, `docs/decisions.md`.

- [x] Select compatible maintained Go/dependency versions from official documentation; record exact pins and licenses. Use verified GitHub module path or the documented temporary local path.
- [x] Implement Cobra root and version commands with build version/commit/protocol fields; keep `main` thin and failures nonzero.
- [x] Add `TestVersionOutput` and `TestUnknownCommand`; assert deterministic version output and actionable invalid-command failure.
- [x] Make build/test/vet targets runnable; start CI with Go build/tests/vet and minimal permissions. Add frontend gates when T13 creates the frontend.
- [x] Run `go test ./internal/cli ./cmd/tracelet`, `go vet ./...`, and the initial build. Commit: `chore(build): bootstrap Go CLI and continuous integration`.

**Done when:** a clean checkout builds a version-reporting CLI and CI can verify it.

### T02 / C02 — Prove ingestion and SQLite feasibility

**Depends on:** T01. **Files:** `experiments/otlp-spike/`, `docs/feasibility.md`, `docs/benchmarks.md`, `docs/decisions.md`.

- [x] Build a deliberately isolated synthetic-data spike using a real Go SDK HTTP/Protobuf exporter, generated receiver types, and SQLite. No production CLI ingestion yet.
- [x] Assert one known trace ID survives send → decode → committed write → process close/reopen → query. Assert gzip interoperability and exporter shutdown flush.
- [x] Compare bounded JSON/Protobuf detail storage vs normalized attributes; measure batched ingest/query on representative synthetic fixtures and record hardware, versions, row counts, latency, throughput, memory, and DB/WAL bytes.
- [x] Record dependency/receiver/driver/WAL choices, observed code attributes, failures, and go/no-go findings. Missing targets trigger optimization or documented scope decisions, not invented numbers.
- [x] Prepare interview script for 5–10 developers, competitive-workflow comparison, and a fixture-based UI prototype brief. Record real observations only; interviews and outreach require actual participants/authorization and are not a blocker for the technical spike.
- [x] Run spike assert/check command documented in its README; commit: `test(spike): validate Go OTLP ingestion and SQLite feasibility`.

**Done when:** a real exporter and durable storage path work, and architecture choices have measured evidence. Promote useful spike code into production tasks and remove obsolete experiment code by T19.

### T03 / C03 — Version project configuration and implement init

**Depends on:** T01, T02. **Files:** `internal/config/`, `internal/cli/init.go`, config tests, `docs/configuration.md`.

- [x] Implement `config.Load` and validated defaults from §3; assign project ID; choose OS data paths; define flags > file > defaults precedence. OTEL environment resolution belongs to T08.
- [x] Implement `init` as exclusive creation with safe permissions, no source modification, and refusal to overwrite unless explicitly requested. Print path and next step.
- [x] Add tests for duplicate init, malformed/unknown YAML, unsupported schema version, relative paths, permissions, invalid limits/ports, and unsupported body capture/non-loopback binding.
- [x] Run `go test ./internal/config ./internal/cli`; inspect a generated sample config. Commit: `feat(config): add versioned project setup and safe defaults`.

**Done when:** configuration is deterministic, documented, and never destructively overwritten by default.

### T04 / C04 — Establish lossless telemetry models and fixtures

**Depends on:** T02. **Files:** `internal/trace/model.go`, `internal/ingestion/validate.go`, `testdata/`, model/validation tests.

- [x] Define TraceSummary, SpanSummary, TraceDetail, TreeWarning, typed detail/ID/time encoding and schema version. Preserve OTLP fields listed in §3.
- [x] Build synthetic simple, four-level nested, error/event, partial/late, multi-service, cyclic-parent, 2,000+ span, sensitive, path, duplicate/conflict, gzip/malformed/oversized fixtures with expected outcomes.
- [x] Add `TestValidateSpanIdentityAndTime` and `TestTelemetryRoundTripPrecision`: invalid/zero IDs and invalid intervals reject; valid zero duration and nanoseconds/int64 above JS safe range survive unchanged.
- [x] Run `go test ./internal/trace ./internal/ingestion`; ensure fixtures contain no real personal/production data. Commit: `feat(trace): define precise telemetry models and synthetic fixtures`.

**Done when:** later tasks can share stable IDs/types and fixtures without flattening or losing telemetry data.

### T05 / C05 — Redact telemetry before it can be stored

**Depends on:** T03, T04. **Files:** `internal/privacy/redact.go`, adjacent tests, `testdata/privacy/`, `docs/privacy.md`.

- [x] Implement the single recursive sanitizer and exact configurable glob semantics from §3; preserve non-sensitive value types and record redaction markers.
- [x] Add `TestRedactionAllContainers` for span/resource/scope/event/link/nested keys, key casing, bare and namespaced secrets, body/query fields, and invalid patterns.
- [x] Assert synthetic secret sentinels do not survive the sanitized serialized payload; assert an unrelated typed attribute remains unchanged. Document free-text/child-output limitations.
- [x] Run `go test ./internal/privacy`; commit: `feat(privacy): redact sensitive telemetry before persistence`.

**Done when:** production persistence has a reusable, tested sanitizer and a clear privacy contract.

### T06 / C06 — Persist spans transactionally and migrate safely

**Depends on:** T03–T05. **Files:** `internal/storage/sqlite.go`, `ingest.go`, `migrations/001_init.sql`, `internal/session/lock.go` and OS-specific lock files, storage/lock tests, `docs/architecture.md`.

- [x] Implement the minimal OS-backed project ownership lock before any writable open; callers acquire it before `storage.Open`. Keep the lock package independent of storage to avoid import cycles. Test competing owners and release-after-crash. Implement schema v1, transactional `Store.Ingest`, constrained resource/scope reuse, and indexes measured in the spike. Apply sanitizer at the ingestion boundary; never persist unsanitized request buffers.
- [x] Define first-committed-wins duplicate behavior and atomically update trace summaries; accepted data means commit completed. Use bounded batches and document durability configuration.
- [x] Add tests for close/reopen, duplicate retry, conflicting duplicate, typed metadata, transaction rollback, permissions, older-to-current migration, failed migration, unknown newer schema, and disk/write failure.
- [x] Run `go test ./internal/storage` and integrity checks on a reopened fixture DB. Commit: `feat(storage): persist sanitized traces with versioned SQLite migrations`.

**Done when:** accepted sanitized telemetry is durable and retries cannot create duplicate rows or inconsistent counts.

### T07 / C07 — Ship the bounded OTLP/HTTP receiver

**Depends on:** T05, T06. **Files:** `internal/collector/receiver.go`, `internal/ingestion/pipeline.go`, protocol tests, `testdata/otlp/`.

- [ ] Implement retained loopback listener, generated Protobuf decoding/response, gzip, request/decoded/count limits, timeouts, bounded concurrency/queue, backpressure, and commit-before-acknowledgement.
- [ ] Test valid/empty/mixed-invalid batches, malformed Protobuf/gzip, wrong method/media type/encoding, wire and decoded oversize, zero IDs, queue saturation, writer failure, cancellation, and a valid request immediately after rejection.
- [ ] Verify real pinned Go SDK interoperability and accurate OTLP partial-success/retry semantics; do not return an HTML or private JSON error to a Protobuf exporter.
- [ ] Run `go test ./internal/collector ./internal/ingestion ./internal/storage`; commit: `feat(collector): receive bounded OTLP HTTP trace exports`.

**Done when:** M1 ingestion accepts valid input durably and fails safely under hostile or overloaded input.

### T08 / C08 — Run and shut down instrumented child processes

**Depends on:** T03, T07. **Files:** `internal/cli/run.go`, `internal/supervisor/`, `examples/go-api/`, environment/lifecycle tests.

- [ ] Acquire the T06 project lock and start storage/receiver before child; implement the exact environment precedence and explicit `--override-otel` behavior in §3. Respect argument boundaries and stream stdout/stderr.
- [ ] Implement platform-specific process-tree termination, grace period, final ingest drain, exit-code mapping, and cleanup on partial startup failure.
- [ ] Add local instrumented Go API and downstream server: HTTP root, custom nested slow/error spans, event, explicit source metadata, and bounded SDK shutdown.
- [ ] Test endpoint/protocol precedence matrix, service-name preservation, executable-not-found, child exit 0/nonzero, startup failure, interrupt, descendants, repeat interrupt, and final span emitted during shutdown.
- [ ] Run `go test ./internal/supervisor ./internal/cli` and sample-through-receiver smoke. Commit: `feat(cli): supervise instrumented applications and flush on shutdown`.

**Done when:** an actual application can be run through Tracelet without silent exporter redirection or orphaned children.

### T09 / C09 — Own and discover local sessions

**Depends on:** T08. **Files:** `internal/session/`, `internal/cli/ui.go`, `internal/cli/status.go`, session tests.

- [ ] Reuse the T06 ownership lock; add atomic private descriptor, nonce-verified discovery, stale cleanup, and foreground history mode from §3.
- [ ] Make `ui` connect/open/print or start history UI; make `status` read-only and safe when no process is active. A minimal history HTTP page is sufficient until T13.
- [ ] Test two simultaneous owners, stale PID/reused port, crash leftovers, browser-opening failure, `--no-open`, history-only mode, and run-during-history conflict; never kill an unrelated process.
- [ ] Run `go test ./internal/session ./internal/cli`; commit: `feat(session): add local ownership discovery and history browsing lifecycle`.

**Done when:** separate CLI invocations reliably identify the correct project instance and stored data can reopen.

### T10 / C10 — Query and assemble honest trace hierarchies

**Depends on:** T04, T06. **Files:** `internal/storage/query.go`, `internal/trace/tree.go`, `query.go`, adjacent tests.

- [ ] Implement `Store.ListTraces`, `Store.Trace`, exact span lookup, stable pagination, operation search and newest/slowest/errors ordering.
- [ ] Implement iterative/bounded tree assembly for orphan/multi-root/cycle/late spans, exact durations, service summaries and error summaries. Preserve all retained spans.
- [ ] Assert child-before-parent later resolves; missing parent stays visible; cycles terminate; concurrent children are not added together; tie pagination has no repeats; late data updates summaries.
- [ ] Run `go test ./internal/trace ./internal/storage`; commit: `feat(trace): query traces and reconstruct partial hierarchies`.

**Done when:** the explorer has deterministic, bounded, accurate query data including incomplete traces.

### T11 / C11 — Enforce retention and storage budgets

**Depends on:** T06, T09, T10. **Files:** `internal/storage/retention.go`, retention tests, config/status callers, `docs/privacy.md`.

- [ ] Implement incremental oldest-eligible whole-trace deletion, age/budget policies, active-trace quiet window, usage reporting, and writer-visible admission/backpressure on full storage.
- [ ] Account for DB/WAL/sidecars and checkpoint/free-page behavior; expose when pruning happens. Document bounded budget overshoot and manual data deletion after shutdown.
- [ ] Use a fake clock and temporary DB to assert age boundaries, oldest-before-newest, protected active traces, orphan metadata cleanup, byte accounting, retained newer data, full disk and no-eligible-data behavior.
- [ ] Run `go test ./internal/storage ./internal/session`; commit: `feat(storage): enforce retention and report disk pressure`.

**Done when:** old telemetry cannot consume disk indefinitely and storage pressure never silently loses acknowledged data.

### T12 / C12 — Expose the protected local read API

**Depends on:** T09–T11. **Files:** `internal/server/`, API DTO definitions/tests, `docs/api.md`.

- [ ] Implement §3 routes, exact time encoding, bounded pages/detail payloads, safe errors and status/settings snapshots. API export route may be connected in T17.
- [ ] Add Host/Origin/fetch-metadata protections, safe content types/cache headers/CSP, request timeouts and SQL/query validation. Keep receiver traffic on its dedicated listener.
- [ ] Test pagination/filter input, revision changes between node pages, missing trace/span, retention racing reads, wrong method, unsafe origin/Host, valid originless same-origin browser/nonce-authenticated CLI requests, oversized query input, SQL-like search text and no secret fields in status/errors.
- [ ] Run `go test ./internal/server ./internal/storage`; publish representative response fixtures for frontend work. Commit: `feat(api): serve protected local trace queries and session status`.

**Done when:** a browser can read real traces from the correct local instance without cross-origin data exposure.

### T13 / C13 — Embed an offline trace-list UI

**Depends on:** T12. **Files:** `web/`, `internal/webassets/assets.go`, Go server asset routes, `Makefile`, CI frontend jobs.

- [ ] Pin React/TypeScript/Vite and minimal accessible UI/testing dependencies; add one npm lockfile, typecheck/lint/test/build scripts and embed the production bundle into the Go binary.
- [ ] Implement project/status overview, recent trace list, operation search, sort, bounded paging, manual refresh and read-only effective settings. Show service/operation/start/duration/error/span count.
- [ ] Test loading/no-project/no-session/no-telemetry/rejected/no-results/API-error states, list keyboard use and safe rendering of malicious span names. Use fixtures only for isolated tests; main UI calls the real API.
- [ ] Run UI scripts, `make build`, and an offline packaged-binary smoke with no dev server/CDN. Commit: `feat(web): embed offline trace list and session overview`.

**Done when:** installed binary serves a functional accessible list of real persisted traces.

### T14 / C14 — Render the nested waterfall

**Depends on:** T10, T13. **Files:** `web/src/components/Waterfall.tsx`, trace feature files, layout tests, large fixture.

- [ ] Implement hierarchical rows, timeline scale, exact relative offsets, collapse/expand, fit/zoom, selected row and keyboard navigation. Use virtualization/progressive rendering for large traces.
- [ ] Label roots/orphans/cycles/time-skew warnings; show errors using text/icons as well as color. Zero-duration spans remain selectable; no negative widths.
- [ ] Assert deterministic layout for overlapping children, multi-root traces, orphan/late/cyclic inputs and very large absolute timestamps. Test collapsed-row focus and 2,000+ spans without losing selection.
- [ ] Run UI tests/typecheck/build and inspect narrow/wide browser layouts. Commit: `feat(web): render accessible hierarchical trace waterfalls`.

**Done when:** a developer can identify the deliberately slow operation without timing or hierarchy distortion.

### T15 / C15 — Inspect spans, requests, dependencies and errors

**Depends on:** T12–T14. **Files:** `web/src/features/spans/`, detail API caller/tests, API field documentation.

- [ ] Show typed attributes, resource/scope metadata, events, links, kind/status, exception details and redaction markers. Present HTTP/DB/outbound metadata when provided, with no inferred spans or body capture.
- [ ] Add copyable trace/span IDs, full-value expansion, local timestamp plus explicit UTC, readable event order and navigable links within retained data.
- [ ] Test malicious strings/HTML, very long values, absent service/status fields, redacted keys, int64 precision, keyboard focus and a trace deleted by retention while open.
- [ ] Run UI checks plus API detail tests. Commit: `feat(web): inspect span metadata events and dependency errors`.

**Done when:** all retained trace details needed to understand a request are inspectable and safe to render.

### T16 / C16 — Classify and display source locations safely

**Depends on:** T03, T15. **Files:** `internal/sourcemap/resolve.go`, tests, span DTO/API/UI callers, `docs/instrumentation.md`.

- [ ] Implement stable code-attribute extraction and mapped/unmapped/unavailable/not-provided states with positive line-number validation.
- [ ] Enforce component-aware canonical containment; test traversal, sibling prefixes, symlink escape, relative/absolute paths, nonexisting files, Windows/UNC foreign paths and unavailable metadata.
- [ ] Show/copy file/function/line and explain why a location cannot map; no source-content API, shell invocation or browser file navigation.
- [ ] Run `go test ./internal/sourcemap ./internal/server` and UI source-panel tests. Commit: `feat(source): display workspace-safe source metadata`.

**Done when:** AC-07/08 pass and source metadata makes no false claim about automatic function tracing.

### T17 / C17 — Export a complete, redacted trace

**Depends on:** T05, T09–T12, T16. **Files:** `internal/export/json.go`, `internal/cli/export.go`, export API/UI callers, `docs/export-format.md`, fixtures/tests.

- [ ] Implement versioned export from §3, active-instance/direct-storage selection, exact nanoseconds/typed values, current-policy redaction, stdout and safe `--out` behavior.
- [ ] Add roundtrip test asserting IDs, parents, timings, events, links, resources/scopes and warning/redaction states survive. Assert synthetic secret sentinels are absent in DB and export.
- [ ] Test nonexistent trace, invalid ID, existing output file, interrupted write, permission failure, disk-full, no-session export, concurrent late ingestion/retention with snapshot consistency, and stdout/stderr separation.
- [ ] Run `go test ./internal/export ./internal/cli ./internal/server` and inspect a portable example. Commit: `feat(export): write versioned redacted trace JSON`.

**Done when:** an exported trace can reproduce inspection via its documented schema without losing precision or bypassing privacy policy.

### T18 / C18 — Complete diagnostics and onboarding behavior

**Depends on:** T08–T17. **Files:** `internal/cli/doctor.go`, status/version callers, UI status states, `README.md`, `docs/troubleshooting.md`.

- [ ] Diagnose config/data permissions, loopback address, bind conflicts, active receiver reachability, last valid ingestion, rejected batches, storage pressure and exporter conflicts. No hanging connection checks or payload logging.
- [ ] Make help/error text explain that instrumentation is required, SDK code can override env, and received spans may lack source metadata.
- [ ] Test no telemetry vs malformed telemetry vs unavailable receiver vs healthy capture, clean/no-config startup and browser-opening failure; verify status/version are useful offline.
- [ ] Run CLI/UI focused checks and follow the quickstart from a clean temporary project. Commit: `feat(cli): add actionable status and setup diagnostics`.

**Done when:** all seven required commands exist and common first-run failures are diagnosable.

### T19 / C19 — Exercise the complete user workflow

**Depends on:** T18. **Files:** `tests/e2e/`, workflow fixtures, Makefile/CI, `docs/release-checklist.md`.

- [ ] Add bounded real-SDK tests for init → run sample → HTTP request → query → waterfall/detail → source → export → stop → reopen. Use deterministic IDs/data and condition-based waits rather than fragile sleeps.
- [ ] Cover AC-01 through AC-15 with named automated cases or explicit manual evidence owners; test no external network dependency after build/dependencies are cached.
- [ ] Run controlled 100-request capture experiment and compare emitted vs persisted IDs; target at least 95% successful capture and record the actual rate. Investigate every loss; the metric does not relax accepted-span durability. Stress shutdown/final batch and nonzero child exits.
- [ ] Remove obsolete spike-only production duplicates; retain reproducible benchmark evidence. Run `make test-e2e` plus core checks. Commit: `test(e2e): verify capture inspection export and restart workflows`.

**Done when:** the integrated application, not mocks alone, demonstrates the P0 acceptance scenarios.

### T20 / C20 — Harden security and crash recovery

**Depends on:** T19. **Files:** collector/privacy/server/source/storage/supervisor tests and fixes, `docs/privacy.md`, security review record.

- [ ] Test hostile origins/Host/rebinding, gzip bombs/count bounds, invalid IDs, path escapes, XSS strings, private data-file permissions, secret-safe diagnostics and no external transfer. Fuzz parsing/normalization/path handling with bounded corpus where useful.
- [ ] Force terminate during writes, retention and migration; assert integrity check and previously committed traces survive. Test disk-full/read-only directory and graceful actionable errors without data reset.
- [ ] Run race detector and relevant vulnerability/secret/dependency scans; fix findings or document assessed exceptions without treating scanner output as proof of safety.
- [ ] Run focused regression tests, core suite and network inspection. Commit: `fix(security): harden local exposure input bounds and crash recovery` (use `test(security)` if no code fix is needed).

**Done when:** no unresolved release-blocking privacy/data-loss/security defect remains; residual limitations are documented.

### T21 / C21 — Measure and meet the stated quality targets

**Depends on:** T19, T20. **Files:** `tests/bench/`, storage/ingestion/UI hot paths only if measured, `docs/benchmarks.md`.

- [ ] Reproduce 1,000 spans/sec for 5 minutes; p95 acknowledgement <100 ms for batches of 100; p95 list query <300 ms on 100,000 spans; first render <2 seconds for 2,000-span trace.
- [ ] Record 8-core/16-GB reference hardware (or explicit differences), OS/browser/dependency versions, fixture sizes, concurrency, warmup, raw measurements, RSS/queue/disk growth, sample count and calculation method.
- [ ] Fix measured bottlenecks with smallest changes and regression tests. Distinguish ingestion-only throughput from realistic UI/query contention. Do not count dropped/rejected spans as successful throughput.
- [ ] Run `make bench` and relevant correctness tests. Commit: `perf(trace): validate bounded ingestion queries and waterfall rendering` (use `test(perf)` when no optimization is needed).

**Done when:** targets are met or measured gaps are explicitly accepted/documented for release; hard correctness/resource/safety requirements cannot be waived as performance exceptions.

### T22 / C22 — Package and smoke-test supported platforms

**Depends on:** T19–T21. **Files:** build/release scripts, `.github/workflows/release.yml`, `docs/compatibility.md`, packaging tests.

- [ ] Build native assets for proposed macOS amd64/arm64, Linux amd64/arm64 and Windows amd64; only advertise platform/architecture pairs actually smoke-tested. Record untested artifacts as experimental or omit them.
- [ ] Verify embedded UI, version/commit fields, archive layout, licenses, checksums, clean install/uninstall, no mandatory runtime dependencies, and spaces/non-ASCII paths. Run process-tree/loopback tests on each claimed OS.
- [ ] Add least-privilege tag-triggered draft-release workflow, pinned actions, dependency/secret scanning, checksum verification and provenance/signing where feasible. Keep release publishing gated on authorized release action.
- [ ] Prepare Homebrew recipe only after owner/release URL/checksum exist; do not invent published URLs. Docker is deferred.
- [ ] Run cross-platform smoke jobs on real runners and archive their evidence. Commit: `build(release): package tested standalone binaries and checksums`.

**Done when:** release candidates install and run without a separate Go/Node/SQLite/Docker installation on every claimed target.

### T23 / C23 — Finish documentation and GitHub project hygiene

**Depends on:** T18, T22; publication choices may proceed in parallel earlier. **Files:** README, docs, LICENSE when chosen, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, SUPPORT, CHANGELOG, issue/PR templates, dependency-update config.

- [ ] Finish installation, under-five-minute sample, instrumentation, config/defaults, architecture, API/export, privacy/data deletion, troubleshooting, compatibility, development, and known-limitations docs.
- [ ] Resolve owner/repository visibility/product namespace/license with maintainer; check public-name/package availability before announcement. Apache-2.0 is recommended; record actual choice and attribution/NOTICE obligations accurately.
- [ ] Prepare bug/feature/integration templates with redacted reproducible fixtures, contribution checks, conduct policy, supported versions, private security reporting and support routes. Do not invent contact addresses or claim unconfigured GitHub features exist.
- [ ] Run documentation commands from a clean checkout and timed onboarding with a new tester when available; record time and excluded downloads. Prepare usability test for slowest-span identification (target ≥80% success).
- [ ] Commit: `docs(project): publish setup architecture contribution and support guides` (the subject denotes repository docs; remote publication still needs authorization).

**Done when:** another developer can build, use, diagnose, report a bug and contribute without this conversation.

### T24 / C24 — Review the release and publish when authorized

**Depends on:** T00–T23 complete; unresolved publication choices settled. **Files:** `CHANGELOG.md`, `docs/release-checklist.md`, this plan's execution log.

- [ ] Audit all P0 FRs, all ACs, all mandatory NFRs, known performance gaps, dependency/legal metadata, secret scans and platform evidence. No open P0 acceptance criterion.
- [ ] Run final clean build/core/UI/e2e gates once on the candidate; repeat only for changed code or a new failure. Verify artifacts match the reviewed commit and checksums.
- [ ] Prepare release notes with actual features, limits, install commands, supported targets, migrations/recovery instructions and no unearned performance claims.
- [ ] Commit: `chore(release): prepare v0.1.0 release evidence and notes`.
- [ ] With user authorization, push/merge reviewed work, tag `v0.1.0` at the tested commit, publish GitHub Release/artifacts, then verify downloads/install. Never move an existing public tag to hide a bad release; ship a corrective version.
- [ ] Record release URL/tag/hash, remaining issues, rollback/withdrawal instructions and next post-release priorities. Close only issues whose acceptance evidence exists.

**Done when:** the tested release is actually available on GitHub and verified, or explicitly recorded as a fully prepared local candidate awaiting the specific publication decision. Candidate-ready is not published.

## 7. Git, GitHub, issues, pull requests, and commit identity

### Repository-local identity: required before committing

The email is **commit metadata**, not a suffix to add to commit messages. Apply it in this repository only. First inspect whether a repository already exists; run initialization only when needed:

```bash
git init -b main
git config --local user.name 'Arjav Patel'
git config --local user.email 'arjav1528@gmail.com'
git config --local --get user.name
git config --local --get user.email
git var GIT_AUTHOR_IDENT
git var GIT_COMMITTER_IDENT
```

Expected identity for both roles: `Arjav Patel <arjav1528@gmail.com>`. Environment variables and command flags can override Git settings, so diagnose any mismatch before committing. Use this after each new authored commit:

```bash
git log -1 --format='%h %s%nAuthor: %an <%ae>%nCommitter: %cn <%ce>'
```

Do not change global Git configuration, rewrite published history, claim authorship of someone else's imported work, or replace the selected email with an agent/bot/work address. GitHub attribution depends on the email being associated with the intended account; account settings and web-based merge choices can affect attribution. Verify the resulting identity when using GitHub squash/rebase merges. [GitHub commit-email documentation](https://docs.github.com/en/account-and-profile/how-tos/email-preferences/setting-your-commit-email-address)

### GitHub setup checklist

- [ ] Confirm owner/organization, final repository name, visibility and license. Record URL and canonical Go module path in `docs/decisions.md`.
- [ ] Check an existing configured remote before adding/changing one. Do not create a duplicate repository or assume `arjav1528` is the owner.
- [ ] When authorized, create or connect the chosen GitHub repository; prefer an empty remote when local history already exists. Add `origin`, inspect `git remote -v`, and push the reviewed branch. Do not overwrite unrelated remote history.
- [ ] Create milestones M0–M5, with v0.1 optional work clearly separated. Create labels for P0/P1/P2, bug/feature/docs, blocked, security, and relevant subsystem only as needed.
- [ ] Convert each required Txx task into one issue with its dependency list, acceptance criteria, tests and commit subject. Use stable Txx IDs even when GitHub assigns different issue numbers.
- [ ] Configure `main` checks for the actual CI job names; prohibit accidental force-push/deletion where available. Require another reviewer when one is available; do not fabricate a reviewer requirement that makes a solo project unusable.
- [ ] Set up dependency updates and available secret/dependency scanning; verify configuration rather than claiming a platform feature is enabled. Do not supply release credentials to untrusted pull-request code.
- [ ] Add project description/topics and a website URL only if a real site exists. Add Homebrew/package links only once published and verified.
- [ ] Confirm the selected author email is attached to the intended GitHub account before first public push; account verification is a user-side action if needed.

Use short-lived branches: `chore/t01-bootstrap`, `feat/t07-otlp-http`, `feat/t14-waterfall`, `fix/t20-local-security`. Keep `main` buildable. Avoid a permanent `develop` branch unless a real workflow needs it. Use ordinary merges that preserve reviewed local commit identity; review any squash metadata before accepting it.

### Issue body template

```markdown
Task: Txx — <concrete deliverable>
Milestone / priority: <M#> / <P0 or P1>
Depends on: <task IDs and real issue links when available>
Requirements: <FR/MVP/NFR/AC IDs>

Problem and intended behavior: <short description>
Files/contracts: <the relevant plan subsection>

- [ ] <observable acceptance check>
- [ ] <failure or edge-case check>
- [ ] <documentation/fixture update>
- [ ] Record verification command, result, and commit/PR link

Planned commit: <subject from this plan>
Known decisions/limits: <only relevant ones>
```

### Pull request body template

```markdown
<Concrete problem and resulting behavior, including a before/after example if useful.>

Task: Txx. Requirements: <IDs>. Closes #<real issue number>.

Validation: <commands actually run and results; include manual UI/platform evidence when relevant>.
Limitations / data or compatibility effects: <only material ones>.
```

Do not put placeholder issue numbers into a real PR. Use a file with actual newlines for multiline CLI-created descriptions. Stage specific intended files after reviewing the diff; do not blindly commit all local state. Optional features receive their own branches/issues. A task may have several focused commits, but a test-only or documentation-only commit must describe what it actually changes.

### Commit ledger (planned, none created yet)

| Commit | Task | Subject |
|---|---|---|
| C00 | T00 | `docs(plan): add Tracelet implementation handoff and delivery checklist` |
| C01 | T01 | `chore(build): bootstrap Go CLI and continuous integration` |
| C02 | T02 | `test(spike): validate Go OTLP ingestion and SQLite feasibility` |
| C03 | T03 | `feat(config): add versioned project setup and safe defaults` |
| C04 | T04 | `feat(trace): define precise telemetry models and synthetic fixtures` |
| C05 | T05 | `feat(privacy): redact sensitive telemetry before persistence` |
| C06 | T06 | `feat(storage): persist sanitized traces with versioned SQLite migrations` |
| C07 | T07 | `feat(collector): receive bounded OTLP HTTP trace exports` |
| C08 | T08 | `feat(cli): supervise instrumented applications and flush on shutdown` |
| C09 | T09 | `feat(session): add local ownership discovery and history browsing lifecycle` |
| C10 | T10 | `feat(trace): query traces and reconstruct partial hierarchies` |
| C11 | T11 | `feat(storage): enforce retention and report disk pressure` |
| C12 | T12 | `feat(api): serve protected local trace queries and session status` |
| C13 | T13 | `feat(web): embed offline trace list and session overview` |
| C14 | T14 | `feat(web): render accessible hierarchical trace waterfalls` |
| C15 | T15 | `feat(web): inspect span metadata events and dependency errors` |
| C16 | T16 | `feat(source): display workspace-safe source metadata` |
| C17 | T17 | `feat(export): write versioned redacted trace JSON` |
| C18 | T18 | `feat(cli): add actionable status and setup diagnostics` |
| C19 | T19 | `test(e2e): verify capture inspection export and restart workflows` |
| C20 | T20 | `fix(security): harden local exposure input bounds and crash recovery` |
| C21 | T21 | `perf(trace): validate bounded ingestion queries and waterfall rendering` |
| C22 | T22 | `build(release): package tested standalone binaries and checksums` |
| C23 | T23 | `docs(project): publish setup architecture contribution and support guides` |
| C24 | T24 | `chore(release): prepare v0.1.0 release evidence and notes` |

Use `test(security)` / `test(perf)` for C20/C21 if the result only adds verification, as noted in those tasks. Commit bodies should reference Txx, requirement IDs, checks and any material decision. Do not use empty commits to manufacture the expected count.

## 8. Optional v0.1 work and complete future backlog

These tasks preserve the broader vision without turning every idea into a v0.1 requirement. **Do not start them automatically after finishing a P0 task.** Select them only after M3 works and the release budget permits, or when the user explicitly chooses one. They do not block required v0.1 acceptance.

### Optional tasks with execution-ready boundaries

| Status / ID | Deliverable and dependencies | Files / acceptance evidence | Planned commit |
|---|---|---|---|
| [ ] O01 | Combined service/time/operation/status/min-duration filters; after T10/T13 | Extend `internal/trace/query.go`, storage/API/UI filters; use inclusive documented boundaries and stable pagination; test combined filters, empty results and indexed query costs. Covers FR-TRACE-005. | `feat(search): add combined trace filters` |
| [ ] O02 | Live trace updates; after T12–T14 | Use one server-sent-event stream or bounded polling, not both; notify IDs/summaries after commit; refresh via existing API; test reconnect, retention, slow clients and selection preservation. Never block ingestion on a browser. | `feat(web): refresh committed traces during capture` |
| [ ] O03 | Source prefix mappings; after T16 | Extend config/sourcemap/UI; longest component-prefix match, then canonical workspace containment; test container→host, collisions, traversal and symlinks. Covers FR-SRC-003. | `feat(source): map build paths into local workspaces` |
| [ ] O04 | Pairwise trace comparison; after T10/T14/T17 | Add trace comparison logic/API/UI; match normalized service + operation + ancestor path + occurrence, label ambiguity/unmatched spans, show absolute and percent deltas with zero-baseline handling; test reordered/repeated/added/removed/concurrent spans. Describe observations, not statistically proven regressions. | `feat(compare): compare two trace execution paths` |
| [ ] O05 | Clickable prototype and project validation report; prototype can follow T02 and usability validation follows M3 | Build a clickable trace-explorer prototype from representative synthetic OTLP fixtures before UI polish; record 5–10 authorized interviews, observed alternative workflows, source-metadata availability, time-to-first-trace and slowest-span task success. No fabricated users/results; update priorities from evidence. | `docs(validation): record developer workflow and usability findings` |

### Post-v0.1 backlog (requires a scoped design when selected)

Each row is a separate potential issue. Its exit gate defines what the eventual scoped plan must prove; none is already implemented or authorized for immediate execution.

| Status / ID | Scope and prerequisite | Exit gate / suggested commit |
|---|---|---|
| [ ] F01 | Node.js and Python examples after Go onboarding is stable | Real SDK examples prove propagation, exporter config, privacy and source metadata; support matrix updated. `feat(examples): add Node.js and Python tracing samples` (split per language). |
| [ ] F02 | PostgreSQL-first database example, then Redis/MongoDB/gRPC/messaging as separately selected integrations | Instrument real supported client libraries, show safe query/operation metadata and error spans, avoid double instrumentation; optional dependencies do not affect core startup. `feat(examples): add PostgreSQL request tracing` first. |
| [ ] F03 | VS Code extension after stable local API/source rules | Native commands/tree views where useful, explicit workspace trust, safe source opening, real extension lifecycle tests. `feat(vscode): navigate from retained spans to workspace code`. |
| [ ] F04 | Function performance summaries/annotations after source identity quality is measured | Group by qualified function/file/version context; label sampling/coverage; don't imply all functions were observed. `feat(analysis): summarize instrumented function timings`. |
| [ ] F05 | Statistical run comparison, p50/p95/p99 and regression insights after O04 | Define workload identity, sample sizes, aggregation/quantile method, noise controls, baseline/version metadata and missing-data handling. `feat(analysis): compare repeatable latency distributions`. |
| [ ] F06 | CI regression checks after F05 validates reproducibility | Opt-in thresholds and deterministic fixtures; warn/fail semantics; noisy-run safeguards; no single-trace “regression proof.” `feat(ci): check reproducible performance baselines`. |
| [ ] F07 | Log correlation after trace-first retention/UX is stable | Define log transport/storage/redaction/resource budget; correlate via trace/span IDs; test absent IDs and high volume. Metrics analytics is a separate design, not silently bundled. `feat(logs): correlate local logs with trace spans`. |
| [ ] F08 | Saved searches and richer settings/session controls after API and config migration policy stabilize | Validated settings edits, restart semantics, safe concurrent updates, search migration tests. `feat(settings): manage validated local project preferences`. |
| [ ] F09 | Explicit data purge and import after retention/export stability | Preview scope, deliberate deletion, atomic import/schema validation, redaction on import and duplicate policy. Split commands into separate issues. `feat(maintenance): purge selected local trace history`. |
| [ ] F10 | OTLP/gRPC and broader exporter compatibility after actual demand | Same safety/durability/limits as HTTP; real cross-SDK contract tests; clearly advertised protocols. `feat(collector): receive OTLP gRPC traces`. |
| [ ] F11 | Continuous CPU/memory/allocation profiling | Separate signal model and feasibility spike; sample-rate/overhead evidence, runtime compatibility, readable profiles. `feat(profiling): capture and inspect supported runtime profiles`. |
| [ ] F12 | eBPF observation | Linux/kernel/privilege compatibility, explicit permissions, measured overhead and clear instrumentation limits. Requires separate security design. `feat(ebpf): observe a documented Linux workload`. |
| [ ] F13 | Remote/team hosting or remote storage | Authn/authz, tenant isolation, TLS, retention operations, audit/privacy review and support model before any non-loopback exposure. `feat(server): add authenticated shared trace access` only after design. |
| [ ] F14 | Plugin/instrumentation/storage adapter framework | Demonstrated multiple real extensions, versioned compatibility/security boundary; avoid framework before concrete need. `feat(extensions): define the validated adapter contract`. |
| [ ] F15 | Optional AI explanation | Explicit opt-in, provider/data disclosure, export redaction review, offline deterministic features remain complete. `feat(assistant): add opt-in trace explanations`. |
| [ ] F16 | Additional packaging: Homebrew distribution, Docker and other package managers | Verified artifact sources/checksums and real install/update/uninstall tests; Docker networking/data mounts documented without changing local defaults. Split each package format. `build(packaging): add verified Homebrew distribution`. |
| [ ] F17 | Engineering articles and demo material | Publish accurate evidence-based writeups about OTLP collection, SQLite, waterfall, source mapping and local-first design; use synthetic screenshots/traces. Publishing requires authorization. `docs(story): explain Tracelet engineering decisions`. |

## 9. Decisions, risks, and release review sheet

### Decision queue

| ID | Choice | Default / next action | Blocks |
|---|---|---|---|
| D01 | GitHub owner/repo/visibility | Ask only when needed for remote creation; local private work can start | Remote creation/push; canonical published module path |
| D02 | License | Apache-2.0 proposal; maintainer chooses; inspect dependency obligations | Public publication/contributions |
| D03 | Name/namespace | Tracelet working name; check repository/package/domain and relevant naming conflicts before announcement | Branding/package publication; no legal availability claim is made here |
| D04 | Toolchain/receiver/SQLite/schema/WAL | Validate and pin in T01/T02/T06 | Freezing implementation/storage contract |
| D05 | Configuration/limits/retention | Proposed §3 values; validate in spike/benchmarks and version changes | Config v1/alpha promise |
| D06 | Platform support | Target macOS/Linux/Windows; publish only tested pairs | Compatibility statement and artifacts |
| D07 | P1 inclusion | Required tasks first; choose optional work based on M3 and available time | Optional task selection only |
| D08 | Performance gaps | Publish measured evidence; explicit release decision for missed targets | Quality claims/release readiness |
| D09 | Signing/notarization/package credentials | Prepare checksums/provenance locally; enable signing only with authorized credentials | Claims of signing/notarization; not local development |

Architecture changes are recorded with date, evidence, chosen alternative, affected tasks/contracts, migrations and tests. Do not leave “TBD” as a hidden implementation requirement: either use the proposed default or name the exact blocking decision.

### Principal risks and mitigations

| Risk | Practical mitigation / evidence owner |
|---|---|
| Users expect tracing without instrumentation | Repeat prerequisite in quickstart/help; working example and no-data doctor checks (T08/T18/T23) |
| Source attributes absent or build paths differ | Optional metadata states and safe unmapped behavior; measure sample coverage (T02/T16/O03) |
| SQLite stalls, disk growth, corruption | Bounded transactions/queue, retention accounting, forced-kill/disk-full tests and measured schema (T06/T11/T20/T21) |
| Sensitive or internal data leaks | Recursive pre-store redaction, local binding/origin protection, no uploads/source reads, export warning and synthetic fixtures (T05/T12/T17/T20) |
| Cross-platform child/process or path differences | Native OS tests; target-only labels until verified (T08/T16/T22) |
| Large traces freeze the UI | Bounded API metadata, on-demand details, virtualization and 2,000-span benchmark (T12/T14/T21) |
| Wrong commit identity or repository | Repository-local identity/verification; never infer GitHub owner; inspect remote before push (T00/T23) |
| Feature breadth destroys schedule | Separate required/optional/future lists; no automatic scope promotion (all tasks) |
| Existing tools already solve target workflow | Conduct concrete workflow/usability comparison and adjust positioning (T02/O05) |

### Candidate release checklist

- [ ] All required tasks have acceptance evidence; all 22 P0 functional requirements and AC-01…AC-15 are satisfied.
- [ ] Minimal doctor/status/version exist; unshipped P1/future features are clearly labeled.
- [ ] Required protocol/privacy/storage/lifecycle/accessibility behaviors are tested; no unresolved P0 crash, data-loss or exposure issue.
- [ ] Benchmarks and observed capture rate are published with environment and gaps; no target is presented as a result.
- [ ] Actual distributed binaries work offline and pass smoke tests on every advertised platform/architecture.
- [ ] New-user first-trace time is measured where a tester is available; exclusions are documented. Usability target is ≥80% of participants finding the slowest span unassisted.
- [ ] No credentials, real traces, local DBs or machine-private data in the repository/artifacts; raw source transcriptions reviewed for intended public inclusion.
- [ ] Author metadata is Arjav Patel <arjav1528@gmail.com> for authored local commits; real remote/owner/license decisions recorded.
- [ ] Release notes, checksums, compatibility, migration/recovery/deletion docs and vulnerability-reporting route are usable.
- [ ] Publication has authorization; release/tag/download/install verified; next known issues recorded.

## 10. Resume state and handoff protocol

This state is a template to update during implementation. Do not replace evidence with optimistic status.

```markdown
Last updated: 2026-11-26
Current milestone: M1 (in progress)
Current task: T07 (in progress)
Last completed task: T06
Repository/remote: local Git initialized; no remote
Branch / HEAD: feat/tracelet-core / C06 (see git log)
Git identity required: Arjav Patel <arjav1528@gmail.com>
Verified implementation: version/help/init CLI, strict configuration v1, isolated SDK/SQLite feasibility
Verification run: committed tasks T00–T06 have focused passing checks; T07 receiver is in progress, so full workspace may include unfinished tests
Next action: finish T07 receiver, then connect T08 supervisor and sample
External decisions pending: GitHub owner/name/visibility, license, account email association
Scope decisions: PRD baseline; proposed engineering defaults in §3
Do not lose: source requirements and precision/privacy/lifecycle contracts; completed tasks require evidence
```

Execution log (append one row per completed or materially blocked increment):

| Date | Task/status | Files/behavior changed | Verification and result | Commit / PR | Remaining issue / next task |
|---|---|---|---|---|---|
| 2026-11-26 | T06 complete | Private schema-v1 SQLite storage, durable sanitized transactions, duplicate rules and OS ownership locks | Storage/session tests+race+vet pass; crash releases lock; disk/write failure rollback; unsupported DB preservation; Windows/Linux lock crosscompile | C06 (this commit) | T07; native non-macOS tests and full crash/load gates remain |
| 2026-11-26 | T05 complete | Recursive sanitizer, shared safe defaults, bounded marker reports and privacy docs | Sensitive sentinel absent in all serialized containers; typed values retained; privacy/config/CLI race+vet pass; count mismatch RED→GREEN | 04ba236 | T06; free-text values are not guaranteed scrubbed |
| 2026-11-26 | T04 complete | Typed summaries, exact ProtoJSON wrapper, bounded identity/time/attribute validation and synthetic fixture generators | Focused tests/race pass; exact nanosecond/int64/bytes roundtrips; all fixture shapes validated | 0f3ef8e | T05; envelope byte IDs are base64, query IDs hex |
| 2026-11-26 | T03 complete | Versioned YAML config, init/force, data paths, validation and docs | Missing Init/Load and init command RED→GREEN; invalid NUL path RED→GREEN; config/CLI tests, full check/build pass | e5868ee | T04; no production capture yet |
| 2026-11-26 | T02 complete | Isolated gzip SDK/SQLite spike, 100k-span layout comparison, feasibility and benchmark docs | SDK→commit→fresh process preserves exact metadata; focused race/vet and full make check/build pass with loopback permission | 87587a9 | T03; measurements storage-only, not release NFR proof |
| 2026-11-26 | T01 complete | Cobra CLI, pinned Go toolchain/dependencies, Makefile and read-only CI | RED missing NewRoot; GREEN make check/build and version/invalid-command smoke | 6829ac3 | T02; GitHub CI not run remotely |
| 2026-11-26 | T00 complete | Local Git identity, ignore rules, preserved planning/reference files | Author and committer verified; source IDs, links and ignore rules checked | 695c49a | T01; GitHub choices remain open |
| 2026-11-26 | Planning handoff created | Self-contained scope, tasks, contracts, commit plan and requirement map | Document review only; no application built/tested | None | Begin T00 on implementation request |

Reusable prompt for another LLM:

```text
Implement Tracelet using the attached implementation plan as project context.
Read it fully, inspect the actual workspace and AGENTS.md, and reconcile its
resume state with files, Git history and real test results. Start the earliest
eligible unchecked required task and continue task by task. Use Arjav Patel
<arjav1528@gmail.com> for new authored commits, configuring only this repository.
Preserve unrelated work. Keep GitHub owner/visibility/license decisions explicit;
do not guess them or publish without the current user's authorization.
Follow the plan's contracts, prove meaningful behavior with focused tests, keep
commits reviewable, and update checkboxes plus the execution log with evidence.
Do not implement optional/backlog items unless selected. Missing source
attachments or model-specific skills do not block using this self-contained file.
```

## 11. References and evidence status

The attached PRD and brief are the primary product inputs. Official protocol, exporter configuration, code attributes, and GitHub email guidance were consulted while preparing this plan on 26 November 2026. The linked contracts in §3/§7 are the verified reference points; library/toolchain versions, package availability, performance, GitHub account state and platform behavior still require their designated implementation tasks.

- [OpenTelemetry OTLP specification](https://opentelemetry.io/docs/specs/otlp/) — transport and response interoperability.
- [OpenTelemetry OTLP exporter configuration](https://opentelemetry.io/docs/languages/sdk-configuration/otlp-exporter/) — generic/signal-specific endpoint behavior.
- [OpenTelemetry code attribute registry](https://opentelemetry.io/docs/specs/semconv/registry/attributes/code/) — current source metadata names.
- [GitHub commit-email configuration](https://docs.github.com/en/account-and-profile/how-tos/email-preferences/setting-your-commit-email-address) — local identity and account attribution.

The PRD also points to Go instrumentation, HTTP semantic conventions, semantic-convention overview and VS Code webview guidance. Recheck their official documentation and pinned dependencies when the relevant task starts; this plan does not claim those implementation surfaces were validated.



## 12. Requirements and acceptance traceability

This inventory preserves the PRD's IDs and priorities. It is a coverage map, not evidence of completion. All requirements start unverified. The required task sequence may include a minimal implementation of a source P1 item when needed to satisfy a must-ship MVP capability; the source priority itself is not rewritten. Optional task IDs O01 and O03 refer to the optional backlog.

**Inventory:** 9 MVP capabilities; 26 functional requirements (22 P0, 4 P1); 14 non-functional requirements; 15 acceptance scenarios.

### MVP capabilities

| ID | PRD capability and acceptance boundary | Implementing / verifying tasks |
|---|---|---|
| MVP-01 | Installable CLI with `init`, `run`, `ui`, `status`, `doctor`, `export`, `version`. | T01, T03, T08, T09, T17, T18, T22 |
| MVP-02 | Receive OTLP/HTTP Protobuf traces on loopback; safely reject malformed and oversized input. | T04, T07, T19, T20 |
| MVP-03 | Launch child process, preserve explicit endpoint/service settings, forward termination and allow exporter flush. | T08, T19, T22 |
| MVP-04 | Persist resources, traces, spans, events, attributes and status in local SQLite. | T04, T06, T10, T19 |
| MVP-05 | List/search traces; show parent-child waterfall, duration, status and span details. | T10, T12–T15, T19 |
| MVP-06 | Read stable `code.*` metadata; show file/function/line and clearly identify unavailable or unmapped paths. | T16, T19, T20 |
| MVP-07 | Export selected trace to documented JSON suitable for issue reports and fixtures. | T17, T19, T23 |
| MVP-08 | Loopback defaults, bounded requests, redaction, retention limits and no telemetry upload. | T03, T05, T07, T11, T12, T19, T20 |
| MVP-09 | Working Go sample plus setup, instrumentation, troubleshooting and architecture guides. | T08, T18, T19, T23 |

### Functional requirements

P0 means release-blocking MVP requirement. P1 means desirable for v0.1 if scope permits. No FR in the source is assigned P2.

| ID | Source priority | Acceptance requirement | Implementing / verifying tasks |
|---|---|---|---|
| FR-CLI-001 | P0 | `init` creates workspace config, refuses destructive overwrite without explicit confirmation and prints the path. | T03, T19 |
| FR-CLI-002 | P0 | `run` starts the receiver before the child, supplies configured OTEL environment, streams stdout/stderr, returns child exit code and shuts down cleanly. | T08, T19, T22 |
| FR-CLI-003 | P0 | Existing OTEL endpoint/protocol values are not silently overwritten; report effective values and provide an explicit override. | T08, T18, T19 |
| FR-CLI-004 | P0 | Ctrl+C and normal child exit forward shutdown, allow configurable flush grace, close SQLite and leave no orphaned child. | T08, T19, T20, T22 |
| FR-CLI-005 | P0 | `ui` opens the selected project's local UI or prints its URL when automatic opening is disabled. | T09, T13, T18, T19 |
| FR-CLI-006 | P1 | `doctor` checks config validity, writable data directory, bind address, receiver reachability and telemetry receipt. | T18; minimal command required by MVP-01 |
| FR-ING-001 | P0 | Accept OTLP/HTTP Protobuf at the standard traces route and return protocol-compatible success/error responses. | T02, T07, T19 |
| FR-ING-002 | P0 | Reject malformed payloads, unsupported content types and requests over configured limits without crashing or unbounded allocation. | T04, T07, T20 |
| FR-ING-003 | P0 | Preserve trace/span/parent IDs, timestamps, kind/status, attributes, events, links, resource attributes and instrumentation scope as represented by OTLP. | T04–T07, T10, T15, T17 |
| FR-ING-004 | P1 | CLI/UI distinguish no telemetry, unavailable receiver, malformed input and valid ingestion. | T07, T09, T13, T18 |
| FR-DATA-001 | P0 | Accepted spans persist in configured SQLite and remain available after restart. | T06, T09, T19, T20 |
| FR-DATA-002 | P0 | Repeated trace/span identities do not create confusing duplicate rows; document idempotency or update behavior. | T06, T07, T10 |
| FR-DATA-003 | P0 | Prune expired data and enforce storage limits without deleting newer data before older eligible data. | T11, T19, T20 |
| FR-TRACE-001 | P0 | Trace list shows service, operation, start time, duration and error status. | T10, T12, T13, T19 |
| FR-TRACE-002 | P0 | Show hierarchical timeline with correct parent nesting and durations. | T10, T14, T19 |
| FR-TRACE-003 | P0 | Selected span shows attributes, events, status, exceptions, resource metadata and instrumentation scope. | T04, T12, T15, T19 |
| FR-TRACE-004 | P0 | Missing parents and late spans do not crash; indicate incomplete or orphaned relationships where appropriate. | T04, T10, T14, T19 |
| FR-TRACE-005 | P1 | Filter by service, time range, operation, status and minimum duration. | O01; baseline operation search is T10/T13 |
| FR-SRC-001 | P0 | Recognize `code.file.path`, `code.function.name`, `code.line.number` when present. | T04, T16, T19 |
| FR-SRC-002 | P0 | Display source location and workspace containment; do not silently open arbitrary filesystem paths. | T16, T19, T20 |
| FR-SRC-003 | P1 | Permit configured path-prefix mappings from container/build paths to local workspace paths. | O03 |
| FR-EXP-001 | P0 | Export selected trace as documented JSON with IDs, timing, status, attributes and parent relationships sufficient to reproduce inspection. | T17, T19, T23 |
| FR-SEC-001 | P0 | Receiver/UI default to loopback; no external telemetry upload or hosted dependency enabled by default. | T03, T07, T09, T12, T13, T19, T20, T22 |
| FR-SEC-002 | P0 | Configurable sensitive-key patterns redact before persistence/export; HTTP bodies are not captured by default. | T03, T05–T07, T17, T20 |
| FR-SEC-003 | P0 | Configurable payload, attribute-count/size, retention and storage limits protect local resources. | T03–T07, T11, T20, T21 |
| FR-DOC-001 | P0 | Guide installation through viewing a sample trace, including instrumentation prerequisite and common failures. | T08, T18, T19, T23 |

### Non-functional requirements

The four performance figures are proposed reference-machine targets, not current measurements. Document fixtures, hardware/software and gaps. Resource bounds, safety and correctness are mandatory regardless of throughput results.

| ID | Quality attribute | PRD target / requirement | Implementing / verifying tasks |
|---|---|---|---|
| NFR-PERF-01 | Throughput | At least 1,000 spans/second for 5 minutes on a documented 8-core/16-GB reference machine with representative bounded spans. | T02, T07, T21 |
| NFR-PERF-02 | Acknowledgement latency | p95 local OTLP acknowledgement below 100 ms for 100-span batches under documented benchmark load. | T07, T21 |
| NFR-PERF-03 | Query latency | p95 trace-list query below 300 ms with a 100,000-span fixture on the reference machine. | T06, T10, T21 |
| NFR-PERF-04 | UI responsiveness | First render below 2 seconds for a trace of up to 2,000 spans on the reference machine/browser. | T14, T21 |
| NFR-RES-01 | Bounded memory | Sustained ingestion never causes unbounded queue growth; overload applies backpressure or visible rejection. | T07, T20, T21 |
| NFR-RES-02 | Disk control | Enforce retention and size limits and show their effects/usage to the user. | T11, T13, T18, T20 |
| NFR-REL-01 | Crash recovery | Forced termination preserves DB integrity and previously committed traces remain queryable. | T06, T19, T20 |
| NFR-REL-02 | Incomplete data | Partial traces and late spans do not fail the UI or process. | T04, T10, T14, T19 |
| NFR-PORT-01 | Platforms | Target macOS, Linux and Windows; validate each before claiming support. | T08, T20, T22 |
| NFR-PORT-02 | Runtime dependencies | End users require no separately installed Go, Node.js, SQLite server or Docker runtime. | T13, T19, T22 |
| NFR-SEC-01 | Local exposure | Default sockets are loopback-only; test every supported platform. | T03, T07, T12, T20, T22 |
| NFR-MAINT-01 | Schema evolution | Version DB migrations and test them against previous released schemas. | T06, T20, T24 |
| NFR-ACCESS-01 | Accessibility | Core workflows support keyboard navigation, readable contrast, status text and non-color-only signaling. | T13–T16, T19 |
| NFR-DOC-01 | Onboarding | New user reaches visible trace from installed CLI in under 5 minutes, excluding dependency downloads. | T18, T19, T23 |

### Acceptance scenarios

All scenarios are release acceptance checks. Record actual results in the execution/release log; a task mapping alone does not establish a pass.

| ID | Scenario | Required acceptance evidence | Implementing / verifying tasks |
|---|---|---|---|
| AC-01 | First run | On a clean supported workstation, documented install/init/Go instrumentation/sample launch produces a visible trace without a hosted account. | T03, T08, T13, T18, T19, T22, T23 |
| AC-02 | Valid ingestion | A valid OTLP/HTTP Protobuf trace export is acknowledged and queryable. | T02, T06, T07, T10, T19 |
| AC-03 | Invalid ingestion | Malformed/oversized telemetry receives a controlled error and safe diagnostic; subsequent valid requests still succeed. | T07, T18, T19, T20 |
| AC-04 | Lifecycle | Ctrl+C or child exit forwards shutdown, waits configured grace, closes storage and leaves no orphan. | T08, T19, T22 |
| AC-05 | Hierarchy | Root, nested children, events and error span show correct relationships, timing, status and events. | T04, T10, T14, T15, T19 |
| AC-06 | Partial trace | Child-before-parent and missing-parent spans remain visible with explicit incomplete/missing-parent indication. | T04, T10, T14, T19 |
| AC-07 | Source metadata | Stable code attributes inside workspace show correct file/function/line; missing attributes explicitly report metadata not provided. | T16, T19 |
| AC-08 | Safe paths | Outside-workspace source paths are external/unmapped and cause no file-content read. | T16, T19, T20 |
| AC-09 | Redaction | Attributes matching configured sensitive patterns are redacted in persisted data and exported output. | T05–T07, T17, T19, T20 |
| AC-10 | Retention | Exceeding age/size threshold prunes older eligible traces, preserves newer data where possible and updates usage. | T11, T13, T18, T19, T20 |
| AC-11 | Offline | With external network unavailable, local receiver/storage/UI workflow continues to work. | T13, T19, T22 |
| AC-12 | Restart | Stored previous-session traces remain available after stop/reopen until retention or user deletion removes them. | T06, T09, T19, T20 |
| AC-13 | Export | Selected trace produces valid documented JSON retaining trace structure and redaction. | T17, T19, T23 |
| AC-14 | Loopback | Default receiver and UI cannot be reached from another device on the LAN. | T07, T12, T19, T20, T22 |
| AC-15 | Explicit configuration | Existing custom OTLP endpoint remains unchanged without explicit override and the conflict is clearly reported. | T08, T18, T19 |

### Source conflicts and their interpretation

- The early brief's checked ten-item list is illustrative; the workspace has no completed implementation. Every feature needs evidence.
- The detailed PRD narrows the early vision: HTTP/Protobuf first; Go first; source display/copy; trace-first storage; no required gRPC, IDE extension, direct editor opening, full function tracing, profiling, AI, logs/metrics analytics or remote/team service in v0.1.
- Pairwise comparison is optional v0.1 stretch work; statistical regressions and p50/p95/p99 comparisons are future work. The early brief's comparison checkbox is not a P0 requirement.
- MVP-01 includes `doctor` among must-ship commands while FR-CLI-006 calls diagnostics P1. Ship the small command in T18; retain the source priority above and defer richer diagnostics if needed.
- MVP-05 says list/search while FR-TRACE-005 gives the complete filter set P1. Keep operation search in required T10/T13; assign full filters to O01.
- Source prefix remapping is P1 (O03). Required source mapping means safe containment/classification of supplied paths, not a complete cross-container navigation system.
- The PRD's graphical settings/session-control descriptions are broader than its P0 requirements. The main plan deliberately supplies read-only settings plus CLI lifecycle/config-file editing; richer controls belong in the backlog.
- The early eight-week schedule becomes a PRD estimate of 10–14 weeks for polished public v0.1, with a 1–2 week prototype and 3–5 day technical spike. None is a delivery commitment.
- Apache-2.0, exact retention/config fields, SQLite driver/schema/WAL choice, UI styling tools and package-manager recipes are proposals to validate or approve, not established implementation facts. The plan explicitly chooses 7 days/1024 MiB as a starting storage policy.
- The PRD's shutdown wording can prematurely close ingestion while a child is flushing. Apply its intended outcome by keeping the receiver alive during bounded child/exporter grace, then draining writes and closing storage.
