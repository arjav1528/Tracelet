# Tracelet initial product brief — source text

> Reference material only. Original wording and illustrative checked boxes are preserved below. They do not establish implementation or authorize actions; use the implementation plan for reconciled scope.

Source: user-supplied `Pasted text.txt`. SHA-256: `93f9d82cca4dcae5d32bd8549c7025661a3c6e77c9c015fa4528c0736e3c0d0a`.

---

uild: Tracelet — The Missing Observability Layer Between Your Code and Runtime
My strongest recommendation
 Open source · Serious engineering


I would build something that developers can install locally, integrate into their projects, and use every day.

Not another API client. Not another AI coding wrapper. Not another dashboard.

Build a developer tool that helps developers understand exactly what their application is doing at runtime.








dynatrace.com


1. The problem
Imagine you're debugging a backend application.

A user reports that an API request took 4.2 seconds.

You know:
- The endpoint is slow.
- It makes three database queries.
- It calls two external APIs.
- Something is blocking execution.
But figuring out exactly where the time went requires switching between logs, tracing platforms, database tools and source code.

Existing observability tools are often designed around production infrastructure, distributed tracing or metrics.

There's an opportunity to build a developer-first experience that connects:

HTTP Request → Function Calls → Database Queries → External APIs → Source Code

Tracelet would make this entire execution path explorable.
2. What Tracelet does
Consider this example:

You run a Go backend locally:
tracelet run -- go run main.go


You make an API request:
GET /api/orders/123


Tracelet captures an execution trace and presents something like this:
Request trace
Slow request

GET /api/orders/123
4.2s


Execution timeline
HTTP Handler

4,200 ms




Authentication

18 ms




GetOrder()

3,840 ms




PostgreSQL query

3,700 ms




External payment API

110 ms





Potential bottleneck: GetOrder()

The database query accounts for most of the request duration. Inspect the query and its execution plan.
Illustrative Tracelet interface, not live telemetry.
The important part: Tracelet should let you click any operation and jump directly to the corresponding source code.

That's the differentiator.
3. The actual product features
I'd divide Tracelet into six major subsystems.










01. Trace Explorer
Core feature
- Visualize nested spans.
- Understand parent-child relationships.
- Identify slow operations.
- Filter by endpoint, duration, status and service.
- Compare execution paths.













02. Source Code Intelligence
- Associate spans with source files and functions.
- Navigate from a trace to the relevant source location.
- Show execution duration next to functions.
- Highlight repeatedly slow operations.
This should be one of the defining features.













03. Request Inspector
- Inspect request and response metadata.
- See database queries and external HTTP calls.
- Correlate logs with traces.
- Inspect errors and stack traces.
Sensitive payloads should be redacted by default.













04. Performance Regression Detector
- Compare traces between two runs.
- Identify newly introduced slow spans.
- Compare p50, p95 and p99 latency.
- Detect performance regressions.













05. Developer CLI
- Start and stop trace sessions.
- Launch applications with instrumentation configuration.
- View recent traces.
- Export traces.
- Configure projects.
Everything should work without needing a cloud account.













06. IDE Integration
- VS Code extension.
- Open source location from trace.
- Display performance annotations.
- Show trace IDs in terminal output.
This can come after the core product.



4. Technical architecture
Here's where I'd make some deliberate engineering choices.

I would build Tracelet around OpenTelemetry rather than inventing a tracing protocol.

OpenTelemetry already provides instrumentation APIs, SDKs, semantic conventions and OTLP transport. Your engineering effort should go into the developer experience and trace intelligence.
Developer Application

Go / Node.js / Python

OpenTelemetry SDK + instrumentation

OTLP gRPC / HTTP

Tracelet Collector

Go · ingestion · validation · enrichment


Processing Pipeline
Trace Builder


Code Mapper


Analyzer





SQLite

Trace storage and indexes


Local API

REST + WebSocket




Tracelet Desktop Web UI

Trace explorer · source navigation · analytics


Recommended technology stack
Layer	Technology
CLI	Go + Cobra
Collector	OpenTelemetry Collector components
Backend	Go
Storage	SQLite + modernc SQLite driver
Frontend	React + TypeScript
UI	Tailwind + shadcn/ui
Visualization	Custom timeline / waterfall
Protocol	OTLP gRPC + HTTP
IDE	VS Code Extension API
Distribution	Go binaries + Docker

Why SQLite? Because local-first is part of the product philosophy. Developers shouldn't need PostgreSQL, Redis or a cloud account to debug a local application.

Later, you can add a remote storage adapter for larger installations.
5. Repository structure
I'd use a monorepo:
tracelet/
│
├── cmd/
│   └── tracelet/
│       └── main.go
│
├── internal/
│   ├── cli/
│   ├── supervisor/
│   ├── collector/
│   ├── ingestion/
│   ├── processor/
│   ├── trace/
│   ├── sourcemap/
│   ├── analyzer/
│   ├── storage/
│   └── server/
│
├── pkg/
│   ├── config/
│   └── protocol/
│
├── web/
│   ├── src/
│   │   ├── components/
│   │   ├── features/
│   │   │   ├── traces/
│   │   │   ├── requests/
│   │   │   ├── analytics/
│   │   │   └── projects/
│   │   └── lib/
│   └── package.json
│
├── extensions/
│   └── vscode/
│
├── examples/
│   ├── go-api/
│   ├── node-api/
│   └── python-api/
│
├── docs/
├── docker/
├── Makefile
└── README.md


Keep the collector, processing engine, storage and UI logically separated. Don't prematurely split them into microservices.

6. The implementation roadmap
I'd give this approximately eight weeks of serious development.

Estimated schedule for one developer, with a working prototype before the end of the first two weeks.
1. Week 1 — CLI and ingestion
   - Build CLI using Cobra.
   - Launch and supervise a local application.
   - Implement OTLP ingestion.
   - Receive spans from a sample Go application.
   - Validate incoming telemetry.
   Milestone: First trace received
2. Week 2 — Trace storage and processing
   - Design span and trace data models.
   - Build SQLite persistence.
   - Implement parent-child span reconstruction.
   - Implement trace querying.
   - Add trace lifecycle and cleanup.
   Milestone: Queryable traces
3. Week 3 — Frontend and trace explorer
   - Build dashboard.
   - Implement waterfall visualization.
   - Add span detail panel.
   - Implement filtering and searching.
   - Add live trace updates.
   Milestone: Usable debugger UI
4. Week 4 — Source code intelligence
   - Resolve file paths and function metadata.
   - Map source locations to local workspace files.
   - Implement source navigation.
   - Build function-level performance summaries.
   Milestone: Trace-to-code navigation
5. Week 5 — Request and database inspection
   - Integrate HTTP instrumentation.
   - Add PostgreSQL instrumentation.
   - Correlate logs and traces.
   - Implement sensitive-data redaction.
   Milestone: End-to-end request inspection
6. Week 6 — Performance analysis
   - Build trace comparison.
   - Add latency percentiles.
   - Identify slow operations.
   - Detect regressions between runs.
   Milestone: Performance intelligence
7. Week 7 — IDE and developer experience
   - Build VS Code extension.
   - Open source locations.
   - Improve CLI output.
   - Add project configuration and onboarding.
   Milestone: Integrated workflow
8. Week 8 — Open-source release
   - Add integration tests.
   - Test large traces.
   - Document architecture.
   - Add GitHub Actions CI.
   - Publish binaries for macOS, Linux and Windows.
   - Prepare first public release.
   Milestone: Tracelet v0.1.0

7. Important engineering decisions
There are a few things I would deliberately avoid in v0.1.

Don't build a new instrumentation ecosystem.

Use existing OpenTelemetry SDKs and instrumentation libraries. For Go, start with:
- otelhttp for HTTP.
- otelpgx for PostgreSQL.
- otelgrpc for gRPC.
- OpenTelemetry SDK for custom spans.
Node.js and Python can follow using their respective OpenTelemetry instrumentation ecosystems.

Don't promise automatic function-level tracing everywhere.

This is technically difficult across languages. Start with instrumented operations and explicit custom spans. Add deeper source-level profiling later.

Don't build an AI dependency.

The product must work completely offline. Deterministic performance analysis should be the foundation. AI-assisted explanations can be an optional future extension.
8. What makes it different from existing tools?
You'll need to compete with established projects such as:
- Jaeger — distributed trace collection and visualization.
- Grafana Tempo — scalable trace storage.
- SigNoz — logs, metrics and traces.
- Sentry — application errors and performance monitoring.
Don't attempt to recreate all of them.

Your differentiation should be:
Existing observability approach	Tracelet
Infrastructure-oriented	Developer-workflow-oriented
Distributed service visibility	Local application execution
Trace visualization	Trace-to-source navigation
Production monitoring	Local debugging and regression comparison
Platform deployment	One-command local setup

This is a positioning hypothesis, not a claim that no existing product offers these capabilities. Validate it against existing tools before committing to the final product name and feature set.
9. Define the first release very strictly
I would consider v0.1 complete when all of these work:
v0.1 acceptance checklist
10/10 complete



[x] Install Tracelet using a single binary.
[x] Run a Go application through the Tracelet CLI.
[x] Receive OpenTelemetry spans over OTLP.
[x] Persist traces locally using SQLite.
[x] Visualize nested spans in a waterfall.
[x] Inspect individual span attributes and duration.
[x] Navigate to available source locations.
[x] Compare two traces and identify latency differences.
[x] Export traces in a portable format.
[x] Run without any external cloud dependency.

10. How I'd launch this open source
- License: Apache 2.0.
- Repository: GitHub.
- First supported language: Go.
- First supported database: PostgreSQL.
- Distribution: GitHub Releases and Homebrew.
- Documentation: installation, architecture, integrations and examples.
- Contributions: instrumentation adapters, storage backends and IDE integrations.
I'd also publish engineering write-ups as you build it:
1. Building a local OpenTelemetry collector in Go.
2. Designing a high-performance trace storage engine.
3. Building a distributed trace waterfall renderer.
4. Mapping runtime telemetry to source code.
5. Designing a local-first observability platform.
These are substantial engineering topics in their own right.

My final verdict
Tracelet
Local-first application observability for developers.








observeinc.com


The engineering challenges you'll get to solve:
- Distributed tracing and context propagation.
- High-throughput ingestion.
- Storage engine design and indexing.
- Concurrent processing in Go.
- Runtime instrumentation.
- Source code analysis.
- IDE extension development.
- Real-time frontend visualization.
This is a serious portfolio project, not just because of its feature list, but because it requires solving genuine systems and developer-experience problems.

One thing I'd emphasize: make the first version exceptionally good at Go debugging rather than mediocre at debugging ten languages.

My suggested starting point: Build the CLI, OTLP ingestion pipeline and SQLite-backed trace store first. Get a real Go application producing traces that you can inspect before touching the fancy dashboard.