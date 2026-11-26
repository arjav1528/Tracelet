# Tracelet PRD — source transcription

> Reference material only. Statements in this source are proposals, not agent instructions or implementation evidence. The implementation plan records the reconciled scope.

Source: `Tracelet_PRD_and_Feasibility_Specification.docx`, supplied by the user.

SHA-256: `1737e2bb6184a1e634808d9bde3673d3c9e8828e982ca8a7cf10031c1a5ff0bc`.

Transcribed from the document body on 26 November 2026. Text and tables are preserved; visual layout, page breaks and decorative formatting are not.

---

TRACELET

Product Requirements Document& Engineering Feasibility Specification

Local-first application observability for developers

|  |
| --- |

Document status  Draft for product and engineering planningVersion  1.0Date  26 November 2026Product stage  Pre-implementation specificationScope  Product definition, MVP, acceptance criteria, feasibility and roadmap

This document defines what to build and how success will be evaluated. It does not contain implementation code or authorize implementation.

## Document Control

| Field | Value |
| --- | --- |
| Working product name | Tracelet (name subject to trademark, domain and repository availability checks) |
| Document owner | Project maintainer / product owner |
| Intended audience | Solo maintainer, contributors, reviewers and early adopters |
| Decision status | Proposed; scope and estimates require validation through a technical spike |
| Primary deployment model | Local-first CLI and browser-based local UI |
| Initial supported runtime | Go applications emitting OpenTelemetry traces |
| Initial telemetry protocol | OTLP over HTTP with Protobuf |
| License recommendation | Apache License 2.0, subject to maintainer approval |

## Contents

1. Executive Summary

2. Product Definition and Vision

3. Problem Statement and Opportunity

4. Goals, Non-Goals and Product Principles

5. Users, Jobs-to-be-Done and Primary Workflows

6. Feasibility Assessment

7. Scope and Release Boundaries

8. Functional Requirements

9. User Experience and CLI Specification

10. Technical Architecture

11. Data Model and Storage

12. Security, Privacy and Data Governance

13. Non-Functional Requirements

14. Acceptance Criteria and Definition of Done

15. Testing and Quality Strategy

16. Delivery Plan and Estimates

17. Risks, Dependencies and Mitigations

18. Success Metrics and Release Strategy

19. Future Scope and Product Roadmap

20. Open Decisions

21. Glossary and References

## 1. Executive Summary

Tracelet is a local-first developer tool that receives OpenTelemetry traces from an instrumented application, stores them on the developer’s machine, and presents a focused interface for exploring request execution. The product aims to connect high-level request behaviour to the spans, attributes, errors and source locations that explain it.

The central user experience is a trace waterfall: a developer selects a request, sees its nested operations in time order, inspects an individual span, and—when usable source metadata exists—identifies the associated source file, function and line. Tracelet will also make it easy to compare two executions and understand where latency changed.

| Product positioning<br>Tracelet is not intended to replace production observability platforms. It is a developer-workflow tool for local debugging, investigation and regression analysis, with a low-friction local setup and no mandatory hosted account. |
| --- |

The recommended MVP deliberately starts with Go applications that already emit OpenTelemetry traces. Tracelet will provide an OTLP receiver, local trace storage, a CLI workflow, a local browser UI, trace inspection and basic source metadata display. It will not automatically instrument arbitrary application code, capture every function call, profile CPU or memory, or provide a team-hosted observability service in its first release.

A realistic estimate for a solo experienced engineer is approximately 10–14 weeks to reach a polished public v0.1, including hardening, documentation and release work. A narrow end-to-end prototype should be achievable in 1–2 weeks. These are planning estimates, not commitments; the first technical spike should validate ingestion, storage and query performance.

## 2. Product Definition and Vision

### 2.1 Product statement

For developers debugging an application locally, Tracelet provides a quick way to inspect the execution of a request from a single local interface. Unlike infrastructure-first observability products, Tracelet focuses on the developer’s immediate loop: run the app, reproduce a request, inspect the trace, understand the slow or failing operation, and return to the code.

### 2.2 Product vision

Make application execution understandable during development. A developer should be able to answer “what happened during this request, where did the time go, and which part of my code should I inspect?” without assembling several separate tools.

### 2.3 Product category

Developer tooling / local application observability / trace exploration.

### 2.4 Product boundaries

| Dimension | Decision |
| --- | --- |
| Primary user | Individual software developer working on a local project |
| Primary environment | Developer workstation; local process and local browser UI |
| Primary input | OpenTelemetry trace data exported by an instrumented application |
| Primary output | Searchable traces, span timelines, span details and source metadata |
| Default data location | User-owned local application data directory or configured project path |
| Network requirement | No external network connection required for core functionality |
| Initial language focus | Go; telemetry protocol remains language-neutral |
| User interface | CLI for workflow and diagnostics; browser UI for trace exploration |

## 3. Problem Statement and Opportunity

### 3.1 Current developer pain

A slow endpoint often involves multiple layers: handlers, middleware, database drivers, outbound HTTP calls and application code.

Logs are often fragmented and may not preserve causal relationships between parent operations and their child operations.

Developers can spend time correlating timestamps and request identifiers across terminal output, database consoles and hosted observability products.

Production observability products may be too heavy for a small local debugging task or may require hosted services, accounts and configuration.

The link between a trace and the source code that produced it is often not part of the default debugging workflow.

### 3.2 Opportunity

Tracelet can reduce tool switching by presenting trace structure, timing, errors and available code metadata in a single local workflow. The product should win through speed of setup, clarity of its trace explorer, safe local defaults and useful developer ergonomics—not by matching the breadth of mature observability platforms.

### 3.3 Product hypothesis

| Hypothesis to validate<br>Developers working on instrumented services will adopt a local trace explorer when it can be installed quickly, receive traces with minimal configuration, and make it noticeably easier to diagnose a slow or failing request. |
| --- |

### 3.4 Validation before broad investment

Interview 5–10 developers who regularly debug Go or backend services.

Ask them to reproduce one slow or failing endpoint using their current workflow and record the steps/tool switches.

Build a clickable trace-explorer prototype using representative OTLP data before polishing the full application.

Compare Tracelet against Jaeger, Grafana Tempo, SigNoz and existing IDE/debugger workflows; document the unmet developer-specific need.

Validate that source-location attributes are actually available in the target instrumentation libraries and custom spans.

## 4. Goals, Non-Goals and Product Principles

### 4.1 Goals for v0.1

Provide a reliable local OTLP/HTTP Protobuf receiver for trace data.

Launch an instrumented child application through a Tracelet CLI command and direct its OpenTelemetry exporter to the local receiver when configuration permits.

Persist and query traces locally without requiring cloud services, Redis or a separately managed database.

Visualize trace hierarchy and span timing in a readable waterfall.

Allow developers to inspect span attributes, events, status, errors and resource metadata.

Show source file, function and line metadata when those attributes are present and map safely to the local workspace.

Provide safe defaults for retention, data capture and local network binding.

Offer documentation and sample projects that make the first successful trace achievable in a few minutes.

### 4.2 Non-goals for v0.1

Automatic dynamic instrumentation of arbitrary binaries or source code.

Capturing every function call or generating complete call graphs without explicit instrumentation or a profiler.

Continuous CPU, memory, allocation or wall-clock profiling.

Production alerting, SLO management, uptime checks or hosted dashboards.

Multi-user access control, centralized team workspaces or cloud synchronization.

Full logs and metrics analytics. The MVP is trace-first; log and metric correlation can be considered later.

Capturing HTTP request/response bodies by default.

Automatic code fixes or AI-generated diagnoses as a required product capability.

### 4.3 Product principles

| Principle | Implication |
| --- | --- |
| Local-first | Core workflows work offline and do not upload telemetry. |
| Open standards first | Consume OTLP and OpenTelemetry semantic conventions wherever practical. |
| Explicit over magical | Tell the user what is instrumented; never imply that uninstrumented code is fully traced. |
| Safe by default | Bind to loopback, limit retention, redact sensitive attributes and avoid payload capture. |
| Fast feedback | A trace should be easy to find and inspect immediately after a request is made. |
| Composable | Keep ingestion, processing, storage and UI behind clear interfaces for future extension. |
| Useful before clever | Prioritize trace correctness, navigation and performance over AI or advanced automation. |

## 5. Users, Jobs-to-be-Done and Primary Workflows

### 5.1 Primary persona: backend developer

Builds and debugs a local API or service, uses Go for the first supported workflow, and wants to understand request latency, errors and external dependencies without configuring a full observability stack.

### 5.2 Secondary personas

Open-source contributor: wants to inspect a sample project and help improve the collector, storage engine, UI or integrations.Technical lead: wants a reproducible local debugging workflow for a team, while retaining each developer’s local data and setup.

### 5.3 Jobs-to-be-done

| When… | I want to… | So that… |
| --- | --- | --- |
| An endpoint is slow | See the time spent in each nested operation | I can find the dominant latency contributor. |
| A request fails | Inspect failed spans, events and exception metadata | I can identify the failing dependency or operation. |
| A span looks suspicious | See its attributes and source metadata | I can understand the code path that produced it. |
| A change may have caused a regression | Compare two traces of similar requests | I can see which spans became slower or appeared/disappeared. |
| I am onboarding to a project | Run a sample and view a trace locally | I can verify the tool quickly without a hosted account. |

### 5.4 Primary workflow

1.  Install Tracelet and initialize a project configuration.

2.  Add or enable OpenTelemetry instrumentation in the application using the language SDK and relevant instrumentation libraries.

3.  Run the application through `tracelet run -- <command> [args...]`.

4.  Make a request to the application using curl, a browser or an API client.

5.  Open the local Tracelet UI and select the new trace.

6.  Inspect the waterfall, errors, span attributes and available code metadata.

7.  Optionally export the trace or compare it with another captured execution.

| Critical prerequisite<br>Tracelet receives telemetry; it does not create spans for uninstrumented application code. The sample project and onboarding guide must make the instrumentation prerequisite clear and straightforward. |
| --- |

## 6. Feasibility Assessment

### 6.1 Overall assessment

| Area | Assessment | Reasoning |
| --- | --- | --- |
| OTLP ingestion | High feasibility | A documented protocol and existing Go libraries provide a defined input contract. |
| Local CLI and process wrapper | High feasibility | A Go CLI can start the local receiver, set child-process environment variables and manage shutdown. |
| Trace storage with SQLite | High feasibility | Trace/span records and indexed lookups are a suitable initial local workload; benchmark write patterns early. |
| Trace waterfall UI | High feasibility | Span parent IDs and timestamps are sufficient to build a hierarchical timeline. |
| Source navigation | Medium feasibility | Standard code attributes exist, but instrumentation may omit them and absolute paths may not match the current workspace. |
| Automatic function tracing | Low feasibility for MVP | Requires language-specific compiler/runtime hooks, profiler support or explicit instrumentation; explicitly excluded. |
| Cross-language support | Medium feasibility | OTLP is language-neutral, but examples, setup and source mapping vary by SDK and runtime. |
| Performance regression detection | Medium feasibility | Basic pairwise comparison is achievable; statistically robust baselines require more data and careful normalization. |
| IDE integration | Medium feasibility | VS Code APIs support custom views and commands, but extension lifecycle and security need separate design. |
| Team / hosted product | Medium-to-low feasibility for early stage | Introduces authentication, multi-tenancy, remote retention, access control, deployment and operational burden. |

### 6.2 Technical foundation

OpenTelemetry is the recommended foundation. The OTLP specification defines telemetry transport over gRPC and HTTP; this plan chooses OTLP/HTTP Protobuf for the initial receiver to keep the first transport surface narrow. The current OTLP specification marks traces, metrics and logs stable, and documents default OTLP ports and request behaviour [R1].

OpenTelemetry Go documentation describes stable trace support and the use of an SDK to initialize telemetry and manual instrumentation [R2]. This supports a Go-first sample application and means Tracelet can avoid building its own instrumentation API.

For source metadata, the current semantic convention registry defines stable code attributes such as `code.file.path`, `code.function.name` and `code.line.number` [R3]. Tracelet should read these attributes when present, not assume every span contains them.

### 6.3 Key feasibility constraints

Instrumentation coverage is external to Tracelet. Missing spans cannot be reconstructed from OTLP data.

Function-level source mapping is only as good as the attributes emitted by the SDK or instrumentation library. The tool must not claim complete function tracing.

Absolute source paths can differ between build environments, containers and the host workspace. A configurable path mapping mechanism may be required.

Trace cardinality and attribute volume can grow quickly. Ingestion limits, retention pruning and bounded payload sizes are required.

High-fidelity capture of request/response bodies creates substantial privacy and storage risks and is excluded from default scope.

Performance claims must be backed by repeatable benchmarks on documented hardware and representative trace fixtures.

### 6.4 Planning estimate and confidence

| Delivery target | Estimate | Confidence / conditions |
| --- | --- | --- |
| Technical spike | 3–5 engineering days | High if limited to one instrumented Go sample, OTLP/HTTP receiver and trace persistence. |
| Usable internal alpha | 6–8 weeks | Medium; requires ingestion, query model, basic UI and reliable local lifecycle. |
| Public v0.1 release | 10–14 weeks | Medium-low; includes edge cases, privacy, packaging, docs, cross-platform smoke tests and release readiness. |
| Polished v1.0 | 4–6 months total | Low until v0.1 feedback validates the product direction and feature priorities. |

Estimate assumptions: one experienced engineer working substantially full time, limited scope, no requirement for a native desktop shell, and use of mature OpenTelemetry libraries rather than custom protocol or instrumentation implementations.

## 7. Scope and Release Boundaries

### 7.1 MVP / v0.1 — must ship

| ID | Capability | MVP definition |
| --- | --- | --- |
| MVP-01 | CLI bootstrap | Installable CLI; `init`, `run`, `ui`, `status`, `doctor`, `export`, `version` commands. |
| MVP-02 | Local OTLP receiver | Receive OTLP/HTTP Protobuf trace exports on loopback; validate and reject malformed/oversized input safely. |
| MVP-03 | Process wrapper | Launch a child process, set Tracelet OTLP endpoint and service name only when not explicitly overridden, forward termination signals and allow telemetry flush. |
| MVP-04 | Trace storage | Persist resources, traces, spans, events, attributes and status locally in SQLite. |
| MVP-05 | Trace explorer | List/search traces and display a parent-child waterfall with duration, status and span details. |
| MVP-06 | Source metadata | Read stable `code.*` attributes; show file/function/line when present; identify unmapped or unavailable source paths clearly. |
| MVP-07 | Export | Export a selected trace to a documented JSON representation suitable for issue reports and fixtures. |
| MVP-08 | Safety controls | Loopback-only binding by default, bounded request sizes, redaction controls, retention limits and no telemetry upload. |
| MVP-09 | Sample and docs | One working Go sample, setup guide, instrumentation guide, troubleshooting and architecture overview. |

### 7.2 Should ship if time allows

Basic trace comparison between two selected traces with span duration deltas.

Configurable source-root mappings for host/container path differences.

Live updates in the UI while a run is active.

Basic trace filters by service, operation name, status and time window.

### 7.3 Explicitly deferred

| Deferred capability | Reason for deferral / entry condition |
| --- | --- |
| Node.js and Python examples | Add after Go workflow, data model and onboarding are stable. |
| VS Code extension | Build after source metadata UX and stable local API are validated. |
| CPU and memory profiling | Separate profiling signal and runtime support; requires distinct ingestion and visualization design. |
| eBPF-based automatic tracing | Platform-specific, permissions-sensitive and substantially larger scope. |
| Log and metric analytics | Expand only after trace-first product value is demonstrated. |
| Continuous performance baselines / CI gate | Requires representative repeated workloads, normalization and noise controls. |
| Remote/team deployment | Requires authn/authz, tenancy, retention operations and security review. |
| AI diagnosis | Optional future layer; deterministic trace inspection must remain complete without AI. |

## 8. Functional Requirements

Priority definitions: P0 = release-blocking MVP requirement; P1 = desirable for v0.1 if scope permits; P2 = post-MVP. Acceptance criteria are testable statements, not implementation prescriptions.

| Requirement ID | Priority | Requirement | Acceptance statement |
| --- | --- | --- | --- |
| FR-CLI-001 | P0 | Initialize project | `tracelet init` creates a project config in the current workspace, refuses destructive overwrite without explicit confirmation, and prints the config path. |
| FR-CLI-002 | P0 | Run child process | `tracelet run -- <command>` starts the local receiver before launching the child, passes configured OTLP environment variables, streams child stdout/stderr, returns the child exit code and shuts down cleanly. |
| FR-CLI-003 | P0 | Respect explicit configuration | Existing OTEL exporter endpoint/protocol values are not silently overwritten. The CLI reports the effective values and provides an explicit override option. |
| FR-CLI-004 | P0 | Manage lifecycle | Ctrl+C and normal child exit stop the receiver, allow a configurable flush grace period, close SQLite cleanly and leave no orphaned Tracelet child process. |
| FR-CLI-005 | P0 | Open UI | `tracelet ui` opens the local UI for the selected project or prints its URL if automatic browser opening is disabled. |
| FR-CLI-006 | P1 | Diagnostics | `tracelet doctor` checks config validity, data directory write access, selected bind address, receiver reachability and whether telemetry has been received. |
| FR-ING-001 | P0 | Accept OTLP traces | The receiver accepts OTLP/HTTP Protobuf trace exports at the standard traces route and returns protocol-compatible success/error responses. |
| FR-ING-002 | P0 | Validate and bound input | Malformed payloads, unsupported content types and requests exceeding configured limits are rejected without crashing the process or unbounded memory allocation. |
| FR-ING-003 | P0 | Preserve trace structure | Trace ID, span ID, parent span ID, start/end timestamps, kind, status, attributes, events, links, resource attributes and instrumentation scope are preserved to the extent represented by the OTLP payload. |
| FR-ING-004 | P1 | Report receiver health | The UI and CLI can distinguish no telemetry received, receiver unavailable, malformed input and valid ingestion. |
| FR-DATA-001 | P0 | Persist locally | Accepted spans are stored in the configured local SQLite database and remain available after Tracelet restarts. |
| FR-DATA-002 | P0 | Deduplicate/retry safely | Repeated delivery of the same trace/span identity does not create confusing duplicate span rows; update or idempotency behaviour is documented. |
| FR-DATA-003 | P0 | Retention and size limits | The application prunes expired data and enforces configured storage limits without deleting newer data before older eligible data. |
| FR-TRACE-001 | P0 | List traces | The user can view traces with service, operation, start time, duration and error status. |
| FR-TRACE-002 | P0 | Open waterfall | The user can view a hierarchical timeline built from trace ID and parent span IDs, with correct nesting and duration. |
| FR-TRACE-003 | P0 | Inspect span details | Selecting a span displays attributes, events, status, exception details, resource metadata and instrumentation scope. |
| FR-TRACE-004 | P0 | Handle incomplete traces | A missing parent or late-arriving span is represented without crashing; the UI indicates incomplete or orphaned relationships where appropriate. |
| FR-TRACE-005 | P1 | Filter traces | The user can filter by service, time range, operation, status and minimum duration. |
| FR-SRC-001 | P0 | Read code metadata | The system recognizes `code.file.path`, `code.function.name` and `code.line.number` attributes when present. |
| FR-SRC-002 | P0 | Map source paths safely | The UI displays the source location and indicates whether the path is inside a configured workspace root; it does not silently open arbitrary filesystem paths. |
| FR-SRC-003 | P1 | Support path remapping | A project can define path-prefix mappings for container or build paths to local workspace paths. |
| FR-EXP-001 | P0 | Export trace | A selected trace can be exported to a documented JSON file including enough identifiers, timing, status, attributes and parent relationships to reproduce UI inspection. |
| FR-SEC-001 | P0 | Local-only default | Collector and UI bind to loopback by default; no external telemetry upload or hosted dependency is enabled by default. |
| FR-SEC-002 | P0 | Redact sensitive values | Configurable attribute-key patterns redact values before persistence and export; HTTP bodies are not captured by default. |
| FR-SEC-003 | P0 | Limit resource use | Configurable maximum payload size, attribute count/size, retention window and storage budget protect local resources. |
| FR-DOC-001 | P0 | First-run documentation | A developer can follow the sample guide from installation through viewing a trace, including the instrumentation prerequisite and common failure states. |

## 9. User Experience and CLI Specification

### 9.1 CLI command surface

| Command | Purpose | Expected behaviour |
| --- | --- | --- |
| `tracelet init` | Initialize the current project | Writes `.tracelet/config.yaml` or the documented config location; does not modify application source. |
| `tracelet run -- CMD` | Run an instrumented application | Starts local receiver, sets supported OTEL environment values, launches CMD and streams its output. |
| `tracelet ui` | Open Tracelet UI | Starts or connects to the local UI service and opens the default browser when configured. |
| `tracelet status` | Show current state | Reports receiver/UI state, project, database path, received trace count and last received time. |
| `tracelet doctor` | Diagnose setup | Checks port availability, configuration, database access and telemetry receipt. |
| `tracelet export TRACE_ID` | Export a trace | Writes trace JSON to a named file or stdout; sensitive attributes are redacted according to project policy. |
| `tracelet version` | Show build information | Prints version, commit and supported OTLP protocol information. |

### 9.2 Example workflow

# Create Tracelet project configuration

tracelet init

# Run an already-instrumented Go service

tracelet run -- go run ./cmd/api

# In another terminal, make a request

curl http://localhost:8080/orders/123

# Open the local trace explorer

tracelet ui

# Diagnose a missing trace

tracelet doctor

# Export a selected trace

tracelet export 4bf92f3577b34da6a3ce929d0e0e4736 --out ./trace.json

### 9.3 Configuration proposal

project:

  name: orders-api

collector:

  bind: 127.0.0.1

  port: 0                  # 0 means choose an available local port

  protocol: otlp-http-protobuf

storage:

  path: ""                 # default under the user data directory

  max_size_mb: 1024

  retention_days: 7

ui:

  open_browser: true

privacy:

  capture_http_bodies: false

  redact_attribute_keys:

    - "*.authorization"

    - "*.cookie"

    - "*.password"

    - "*.token"

source:

  workspace_root: "."

  path_mappings: []

This YAML is a proposed product configuration contract. Exact field names, defaults and migration rules must be finalized before implementation and then versioned.

### 9.4 Key UI screens

| Screen | Core content | Important interaction |
| --- | --- | --- |
| Project / session overview | Active session, receiver state, recent traces, storage usage and last event time. | Start/stop or reconnect to a local session; open a recent trace. |
| Trace list | Service, operation, start time, duration, status and span count. | Search and filter; sort newest, slowest or errors first. |
| Trace detail | Waterfall, timeline scale, nested spans and total duration. | Expand/collapse spans; select a span; zoom or fit timeline. |
| Span detail | Attributes, events, status, exceptions, resource and code metadata. | Copy trace/span IDs; copy source location; reveal redaction state. |
| Settings | Storage, retention, receiver, privacy and source path configuration. | Change configuration with validation and clear restart requirements. |

### 9.5 UX requirements

Use consistent visual indicators for error, warning, success and incomplete data; do not rely on color alone.

The waterfall must remain usable for traces with hundreds or thousands of spans through virtualization or progressive rendering.

Long attribute values must be truncated in the list view while remaining inspectable in a detail panel.

Provide clear empty states: no project, no session, no spans received, no matching traces and telemetry rejected.

All important values such as trace ID, span ID and source location must be copyable.

Timestamps must display local time and provide an unambiguous UTC value on hover or in details.

## 10. Technical Architecture

### 10.1 Architectural style

Build a modular monolith distributed as one local CLI binary plus a bundled web UI. Internally separate the receiver, processor, storage, query API and session supervisor behind interfaces. Do not introduce networked microservices for the local MVP.

### 10.2 High-level components

| Component | Responsibility | MVP implementation direction |
| --- | --- | --- |
| CLI / Supervisor | Parse commands, load config, manage child process, start/stop local services and report status. | Go CLI with a mature command framework; subprocess management in Go. |
| OTLP Receiver | Accept trace batches over OTLP/HTTP Protobuf; validate sizes and payloads. | Use established OpenTelemetry protocol libraries or Collector components where suitable; avoid hand-rolling Protobuf parsing. |
| Ingestion Pipeline | Normalize resources/spans, validate identifiers/timestamps, apply redaction, enqueue writes. | In-process bounded worker pipeline with explicit backpressure. |
| Trace Processor | Resolve span-parent relationships and trace summaries; handle partial/incomplete traces. | Derived views or query-time assembly initially; avoid premature complex stateful aggregation. |
| Storage Adapter | Persist and query telemetry. | SQLite; schema migrations; transaction batches and indexes. |
| Local API | Serve trace queries, project status and UI assets; stream live updates if feasible. | Loopback HTTP API; WebSocket or Server-Sent Events for optional live updates. |
| Web UI | Render trace list, waterfall and span inspection. | React + TypeScript; UI can be served by the local binary. |

### 10.3 Data flow

1.  The developer runs `tracelet run -- <command>`. Tracelet selects a loopback port and starts the OTLP receiver and local API.

2.  Tracelet starts the child process with OTLP environment variables pointing to the local receiver. Existing explicit exporter configuration is preserved unless the user opts into override.

3.  The application emits spans through its OpenTelemetry SDK and sends OTLP batches to Tracelet.

4.  The receiver validates the payload, applies configured redaction and submits records to the bounded ingestion pipeline.

5.  The storage adapter writes resource, trace, span, event and attribute data to SQLite in bounded transactions.

6.  The local API queries stored data and returns trace summaries and span details to the web UI.

7.  The UI builds the waterfall from start/end timestamps and parent span IDs, then displays attributes and available source metadata.

### 10.4 Proposed technology choices

| Area | Recommendation | Rationale / caveat |
| --- | --- | --- |
| Implementation language | Go | Good fit for a single distributable CLI, concurrency and HTTP services; initial sample is Go. |
| CLI | Cobra or equivalent | Mature command hierarchy and flag support; avoid building an ad hoc parser. |
| Telemetry | OpenTelemetry / OTLP | Established protocol and semantic conventions; remain compatible rather than defining a private wire protocol. |
| Initial transport | OTLP/HTTP Protobuf | One transport reduces MVP surface area; add gRPC after usage and compatibility feedback. |
| Storage | SQLite | Zero-admin local deployment; transaction and index design must be benchmarked. |
| Frontend | React + TypeScript | Broad ecosystem and suitable for interactive timeline UI. |
| Visualization | Custom virtualized waterfall | A trace-specific layout offers control over hierarchy, zoom and span selection. |
| Packaging | Native binaries + package-manager recipes | No mandatory Docker, Node runtime or database service for end users. |
| IDE integration | VS Code API later | Use native commands/tree views where possible and webviews only for complex custom visualization [R5]. |

### 10.5 Process and port lifecycle

The CLI should allocate an available loopback port by binding to port 0, retain the listener, and publish the chosen endpoint to the child process. It should not reserve a port, close it and then reopen it, because that creates a race. The receiver must bind only to 127.0.0.1 by default. The UI should use a separate local listener or a documented shared HTTP server route.

On child exit or interruption, Tracelet should stop accepting new data, wait for a bounded flush grace period, finish pending writes, close the database and exit with the child process status. If the child exits abnormally, the tool should still preserve already accepted spans.

### 10.6 Source mapping design

Tracelet should support the stable OpenTelemetry code attributes `code.file.path`, `code.function.name` and `code.line.number` [R3]. Source metadata is optional; a span can be valid without it. Mapping should be conservative: normalize paths, compare them against explicitly configured workspace roots, and show an unmapped state when a path cannot be safely resolved. Do not allow the browser UI to open arbitrary local file paths.

The initial UI can display and copy file/function/line metadata. Direct editor navigation should be added through a CLI command or the later VS Code extension, not by relying on browser `file://` navigation.

### 10.7 Storage and query design notes

Use batched inserts and bounded transactions to avoid one transaction per span.

Index trace_id, span_id, parent_span_id, start_time, service_name and operation_name according to measured query patterns.

Keep high-cardinality or large attributes in a bounded representation; do not create one index per attribute key.

Use schema versioning and migration tests from the first persisted release.

Run retention pruning incrementally so startup does not block on a large database cleanup.

Use WAL mode only after validating its lifecycle and backup/export implications; document how to safely close the database.

## 11. Data Model and Storage

### 11.1 Logical entities

| Entity | Important fields | Notes |
| --- | --- | --- |
| Project | project_id, name, workspace_root, config_version | Local workspace identity; project data remains on the machine. |
| Session | session_id, project_id, started_at, ended_at, command, exit_code | One Tracelet run or local collection session. |
| Resource | resource_id, service_name, resource_attributes | Represents an emitting service/process resource. |
| Trace | trace_id, project_id, first_seen_at, last_seen_at, root_span_id, span_count, status_summary | Derived summary; may be incomplete while spans are arriving. |
| Span | trace_id, span_id, parent_span_id, name, kind, start_time, end_time, status, resource_id, scope_id | Primary trace operation record. |
| Span attribute | span_id, key, typed_value | May be stored as bounded JSON or normalized key-value rows; benchmark before finalizing. |
| Span event | span_id, timestamp, name, attributes | Includes exception events and other point-in-time events. |
| Span link | span_id, linked_trace_id, linked_span_id, attributes | Preserves cross-trace relationships where available. |

### 11.2 Identifier and time handling

Preserve OTLP trace IDs and span IDs exactly; validate expected binary lengths before persistence.

Store timestamps in a consistent integer representation (UTC epoch nanoseconds or an equally precise documented representation).

Calculate span duration from end minus start; do not assume the trace root span duration equals the sum of child spans.

Handle absent end timestamps or invalid intervals as invalid/incomplete input rather than silently inventing values.

Use stable identifiers for resource and instrumentation scope records to avoid duplicating repeated metadata unnecessarily.

### 11.3 Retention defaults

Proposed defaults: retain traces for 7 days and limit the database to 1 GB per project. Both values are configurable. When a size limit is reached, prune the oldest eligible trace data, expose the current storage usage and report when data has been removed. Defaults should be tested against actual trace volume and may be adjusted before release.

## 12. Security, Privacy and Data Governance

### 12.1 Threat model summary

Tracelet runs locally but receives developer application data that may contain secrets, customer identifiers or internal URLs. A local process is not automatically risk-free: telemetry may include credentials in attributes, a malicious local webpage could attempt to interact with a local service, or a misconfigured receiver could expose data on the network.

### 12.2 Mandatory safeguards

| Control | Requirement |
| --- | --- |
| Network binding | Bind receiver and UI to loopback only by default. Remote binding requires an explicit opt-in setting and prominent warning. |
| CORS / origin checks | Restrict browser-origin access to the Tracelet UI origin; do not allow broad wildcard origins for local APIs. |
| Payload limits | Set maximum compressed and decompressed payload sizes, request timeouts, span/attribute counts and string lengths. |
| Redaction | Redact sensitive attribute keys before persistence and export. Ship a conservative default key list. |
| Body capture | HTTP request and response bodies are off by default and excluded from v0.1 unless a future opt-in design includes size limits and explicit warnings. |
| Filesystem access | Do not expose arbitrary file read APIs. Source mapping is display/copy only in MVP and constrained to configured workspace roots. |
| Database permissions | Create local data files with user-only permissions where supported by the operating system. |
| No telemetry upload | No analytics or trace data leaves the machine by default. Any future usage telemetry must be explicit opt-in and separate from trace payloads. |
| Logging | Tracelet’s own logs must not print sensitive attribute values or full payloads by default. |
| Export | Exports inherit the same redaction policy; include a warning before exporting unredacted data if such a mode is ever introduced. |

### 12.3 Privacy defaults

No user account and no hosted dependency.

No background upload of traces, logs, source code or configuration.

No body capture in the MVP.

Redaction before data reaches persistent storage.

Configurable retention and an explicit purge command in a later maintenance release if not ready for v0.1.

Document exactly what is stored, where it is stored and how to delete it.

### 12.4 Security review gates

Before public v0.1, review local server binding, request parsing, memory bounds, CORS/origin behaviour, path handling, database file permissions, export redaction, dependency vulnerabilities and cross-platform process shutdown. Publish a vulnerability reporting policy and supported release policy.

## 13. Non-Functional Requirements

The following are proposed initial quality targets for a defined reference-machine benchmark. They are not claims about current performance. Measure them with reproducible fixtures and document the hardware/software environment before treating them as release commitments.

| ID | Quality attribute | Initial target / requirement |
| --- | --- | --- |
| NFR-PERF-01 | Ingestion throughput | Target at least 1,000 spans/second for a 5-minute sustained test using a documented 8-core / 16-GB reference machine and representative bounded span sizes. |
| NFR-PERF-02 | Ingestion latency | Target p95 local OTLP acknowledgement below 100 ms for a batch of 100 spans under the documented benchmark load. |
| NFR-PERF-03 | Trace query | Target p95 trace-list query below 300 ms with a 100,000-span fixture on the reference machine. |
| NFR-PERF-04 | UI responsiveness | Target first render below 2 seconds for a trace containing up to 2,000 spans on the reference machine/browser. |
| NFR-RES-01 | Bounded memory | Sustained ingest must not cause unbounded queue growth; overload behaviour must apply backpressure or reject with a visible error. |
| NFR-RES-02 | Disk control | Retention and size limits must be enforced and visible to the user. |
| NFR-REL-01 | Crash recovery | After forced termination, database integrity checks must pass and previously committed traces must remain queryable. |
| NFR-REL-02 | Incomplete data | Partial traces and late spans must be handled without UI or process failure. |
| NFR-PORT-01 | Platforms | Target macOS, Linux and Windows CLI builds; validate each platform before claiming support. |
| NFR-PORT-02 | Runtime dependencies | End users should not need a separately installed Go, Node.js, SQLite server or Docker runtime. |
| NFR-SEC-01 | Local exposure | Default listening sockets must be loopback-only and tested on all supported platforms. |
| NFR-MAINT-01 | Schema evolution | Database migrations must be versioned and tested against previous released schemas. |
| NFR-ACCESS-01 | UI accessibility | Keyboard navigation, readable contrast, status text and non-color-only signalling for core trace workflows. |
| NFR-DOC-01 | Onboarding | A new user with an instrumented sample should reach a visible trace in under 5 minutes after installation, excluding dependency downloads. |

## 14. Acceptance Criteria and Definition of Done

### 14.1 MVP acceptance scenarios

| Scenario | Acceptance criteria |
| --- | --- |
| AC-01: First-run setup | Given a clean supported workstation and the documented sample repository, when a developer installs Tracelet, initializes the project and follows the Go instrumentation guide, then the sample can be launched through Tracelet and a trace can be viewed without a hosted account. |
| AC-02: OTLP ingestion | Given a valid OTLP/HTTP Protobuf trace export, when it is sent to the configured receiver, then the receiver acknowledges it and the accepted trace is queryable. |
| AC-03: Invalid input | Given malformed or oversized telemetry, when the receiver processes it, then it returns a controlled error, records a safe diagnostic and remains available for later valid requests. |
| AC-04: Process lifecycle | Given a long-running instrumented application launched with `tracelet run`, when the user sends Ctrl+C or the child exits, then Tracelet forwards shutdown, waits for the configured grace period, closes storage and exits without leaving an orphan process. |
| AC-05: Trace hierarchy | Given a trace with a root span, nested children, events and an error span, when the trace is opened, then the waterfall reflects parent-child relationships and timing and the details panel shows status and events. |
| AC-06: Partial trace | Given a child span arrives before its parent or the parent is absent, when the trace is shown, then the span remains visible with an explicit missing-parent/incomplete indication. |
| AC-07: Source metadata | Given a span with stable `code.*` attributes pointing inside the configured workspace, when the span is selected, then the file, function and line are shown accurately. Given missing metadata, the UI clearly reports that source metadata was not provided. |
| AC-08: Safe paths | Given a span source path outside configured workspace roots, when the span is selected, then the path is treated as external/unmapped and no file content is read. |
| AC-09: Redaction | Given a span attribute whose key matches a configured sensitive pattern, when the span is persisted and exported, then the sensitive value is redacted according to the policy. |
| AC-10: Retention | Given a database exceeding its configured age or size threshold, when retention runs, then older eligible trace data is pruned, newer data is preserved where possible, and storage usage is updated. |
| AC-11: Offline operation | Given a workstation without external network access, when the user runs Tracelet with a local instrumented application, then receiver, storage and UI continue to work. |
| AC-12: Restart persistence | Given traces stored from a previous session, when Tracelet is stopped and reopened, then those traces remain available until removed by configured retention or user deletion. |
| AC-13: Export | Given a selected trace, when the user exports it, then the file is valid documented JSON, retains trace structure and follows the configured redaction policy. |
| AC-14: Loopback security | Given a default configuration, when Tracelet starts, then its receiver and UI are not reachable from another device on the LAN. |
| AC-15: No silent override | Given an application already configured with a custom OTLP endpoint, when it is launched through Tracelet without explicit override, then Tracelet does not silently redirect the application telemetry and clearly reports the configuration conflict. |

### 14.2 Definition of Done for a release feature

Functional behaviour is implemented against an approved requirement and has automated tests where practical.

Failure modes and edge cases are covered; errors are actionable and do not leak telemetry secrets.

Configuration and CLI behaviour are documented, including defaults and backwards-compatibility implications.

Performance impact and storage behaviour are measured for telemetry-heavy paths.

UI behaviour is keyboard-accessible and reviewed at narrow and wide viewport sizes.

No external network transfer occurs unless explicitly documented and user-enabled.

Feature is covered by a sample or fixture where appropriate.

Release notes identify changes, known limitations and migration requirements.

## 15. Testing and Quality Strategy

### 15.1 Test layers

| Layer | What to test | Examples |
| --- | --- | --- |
| Unit | Pure logic and data transformations. | OTLP normalization, span duration, parent resolution, redaction matcher, path normalization, config parsing. |
| Protocol / contract | Interoperability with OTLP exporters and payloads. | Valid/invalid Protobuf, partial success, content type, compressed payload limits. |
| Storage integration | SQLite transactions, indexes and migrations. | Batch writes, duplicate spans, crash recovery, retention pruning, export consistency. |
| API integration | Local HTTP API and UI data contract. | Query filters, pagination, errors, CORS/origin controls, large trace responses. |
| End-to-end | A real instrumented app through the CLI. | Run sample, make request, inspect trace, export, stop and reopen. |
| Performance | Ingestion and rendering under realistic load. | 1k spans/sec target, 100k-span dataset, 2k-span waterfall rendering. |
| Security / privacy | Data exposure and input hardening. | Secret redaction, loopback binding, malicious paths, oversized payloads, export inspection. |
| Cross-platform | CLI lifecycle and filesystem behaviour. | macOS, Linux and Windows start/stop, signals, paths, ports and permissions. |

### 15.2 Required test fixtures

Simple trace: root span and two children.

Nested trace: at least four levels of parent-child spans.

Error trace: failed span, exception event and stacktrace.

Partial trace: missing parent, late-arriving span and incomplete batch.

Large trace: 2,000+ spans with many attributes and events.

Sensitive trace: authorization, token, cookie, email and query-like attribute values.

Path mapping: absolute host path, container path, relative path and path traversal attempt.

Protocol fixtures: valid OTLP export, malformed Protobuf, unsupported content type, gzip and oversize payload.

### 15.3 Release quality gates

A v0.1 candidate must pass automated unit/integration tests, cross-platform smoke tests for claimed platforms, storage migration tests, security/privacy checks, reproducible performance benchmarks and a documented first-run manual test. No P0 acceptance criterion may remain open.

## 16. Delivery Plan and Estimates

The schedule below is a sequencing proposal. It is intended to prevent premature investment in the UI before ingestion and storage are proven. The order may change based on the technical spike.

| Phase | Indicative duration | Deliverables / exit gate |
| --- | --- | --- |
| 0. Product validation | 3–5 days | Developer interviews, competitor review, instrumented Go sample, data fixture and a tested OTLP/HTTP ingestion spike. |
| 1. CLI and local receiver | 1–2 weeks | `init` and `run`; loopback receiver; child process environment; valid traces accepted. |
| 2. Storage and query model | 1–2 weeks | SQLite schema, batch writes, trace list/detail queries, retention prototype and migration tests. |
| 3. Trace explorer UI | 1–2 weeks | Trace list, hierarchical waterfall, span detail panel, empty/error states. |
| 4. Source metadata and export | 1 week | Stable `code.*` attributes displayed, safe workspace mapping, JSON export and redaction. |
| 5. Hardening and portability | 2–3 weeks | Load tests, storage recovery, privacy/security review, macOS/Linux/Windows smoke tests, diagnostics. |
| 6. Documentation and release | 1–2 weeks | Quickstart, sample app, troubleshooting, contribution guide, release packaging and v0.1 release. |

### 16.1 Suggested milestones

| Milestone | Evidence of completion |
| --- | --- |
| M1 — First trace | Tracelet receives a real span from the Go sample and acknowledges it. |
| M2 — Trace persisted | Trace remains queryable after stopping and restarting Tracelet. |
| M3 — Trace understood | Developer can inspect a nested waterfall and identify an intentionally slow span. |
| M4 — Safe local workflow | Redaction, retention, loopback binding and child process shutdown tests pass. |
| M5 — Public v0.1 | Installable binary, sample project, documentation, release notes and issue templates are published. |

### 16.2 Team and effort assumptions

One experienced Go/backend engineer can own the initial implementation if the scope remains trace-first and local-only. A second contributor with frontend experience can reduce UI schedule risk. A reviewer with OpenTelemetry and security experience is valuable for protocol correctness, redaction and local API exposure. The estimate excludes extended community support, packaging for niche distributions and a native desktop application.

## 17. Risks, Dependencies and Mitigations

| Risk | Likelihood / impact | Mitigation |
| --- | --- | --- |
| Developers expect automatic instrumentation | High / High | State the instrumentation prerequisite on the homepage, quickstart and CLI output. Ship working sample instrumentation and explain the boundary. |
| Source attributes are absent or inconsistent | High / Medium | Treat source metadata as optional; rely on stable `code.*` attributes; test the target Go instrumentation path early. |
| Product overlaps with mature tracing tools | High / High | Differentiate on local-first workflow, trace-to-source experience and frictionless local session. Validate with interviews and competitor testing. |
| SQLite write contention or large trace latency | Medium / High | Prototype batched writes and indexes in the first two weeks; use fixtures and benchmarks before finalizing schema. |
| Telemetry contains secrets or personal data | Medium / High | Redact before persistence, disable body capture, constrain exports and document local storage. |
| Local API exposed to LAN or browser-origin abuse | Medium / High | Loopback-only binding, strict CORS/origin policy, request limits and security tests. |
| Port/process lifecycle differs by OS | Medium / Medium | Build cross-platform smoke tests early; avoid race-prone port reservation; define signal/termination behaviour per OS. |
| Large trace overwhelms UI | Medium / High | Virtualize rows, bound initial response payloads and benchmark 2,000+ span traces. |
| Scope creep into profiler or hosted platform | High / High | Use explicit non-goals and require product-owner approval for new P0 features. |
| Working name is unavailable | Medium / Low | Check trademarks, domain, GitHub organization/repository and package-manager namespace before public release. |

### 17.1 External dependencies

OpenTelemetry specifications, semantic conventions, Go SDK and instrumentation libraries.

Go toolchain and cross-compilation/release workflows.

SQLite library/driver compatibility and supported operating systems.

React/TypeScript toolchain and browser compatibility.

GitHub Actions or equivalent CI, code signing/notarization requirements where applicable and package distribution channels.

## 18. Success Metrics and Release Strategy

### 18.1 Product success metrics

| Metric | Initial target / signal | How to measure |
| --- | --- | --- |
| Time to first trace | Under 5 minutes from installed CLI to first trace for the documented instrumented sample. | Timed onboarding test with new users. |
| Trace capture success | At least 95% successful capture in a controlled 100-run sample test. | Automated end-to-end test; compare emitted vs persisted trace IDs. |
| Core task completion | At least 80% of test users can locate the slowest span and inspect its details without assistance. | Moderated usability test. |
| Reliability | No P0 data-loss or crash defects in release-candidate test suite. | Issue triage and integration suite. |
| Performance | Meet documented NFR benchmark targets or publish measured gaps. | Repeatable benchmark CI/manual benchmark. |
| Open-source health | Meaningful external issues, feedback, documentation fixes and contributions. | Repository activity; evaluate quality, not raw star count. |
| Privacy trust | No telemetry upload by default; no unresolved high-severity local exposure findings. | Code review, network inspection and security checklist. |

### 18.2 Launch plan

1.  Private alpha: use the sample Go API and one real developer project; capture usability feedback.

2.  Public preview: publish source, architecture, quickstart and known limitations; invite issue reports.

3.  v0.1: publish signed/checksummed binaries where feasible, release notes, documentation and reproducible examples.

4.  Post-release: prioritize bug reports around ingestion correctness, storage safety, onboarding friction and UI usefulness before adding integrations.

### 18.3 Open-source project hygiene

Choose an OSI-approved license; Apache 2.0 is the current recommendation.

Include CONTRIBUTING, CODE_OF_CONDUCT, SECURITY and SUPPORT documentation.

Use issue templates for bugs, feature requests and integrations.

Publish a compatibility matrix and clearly distinguish supported from experimental features.

Use dependency scanning, secret scanning, reproducible release builds and checksums.

Do not collect anonymous usage telemetry by default. If later considered, make it opt-in and disclose the exact fields.

## 19. Future Scope and Product Roadmap

Future scope is intentionally separated from the MVP. Each expansion should be justified by user demand and must preserve the local-first core experience.

| Horizon | Capability | Value | Prerequisite / caution |
| --- | --- | --- | --- |
| v0.2 | Node.js and Python examples | Broader adoption while retaining OTLP compatibility. | Validate SDK setup, resource identity and language-specific source metadata. |
| v0.2 | Advanced trace search and saved filters | Faster repeated investigations. | Stable query API and indexed storage. |
| v0.3 | Trace comparison and regression insights | Identify changed spans and latency deltas between runs. | Comparable request workloads; explain noisy or non-deterministic traces. |
| v0.3 | VS Code extension | Open mapped source locations and inspect traces inside the editor. | Stable local API and safe source mapping; use native VS Code UI where possible. |
| v0.4 | Log correlation | Connect logs to trace and span IDs. | Define log ingestion/storage scope and retention implications. |
| v0.4 | More instrumentation examples | PostgreSQL, Redis, MongoDB, gRPC, messaging. | Maintain compatibility and avoid duplicate nested spans. |
| v0.5+ | Continuous profiling | CPU, memory, allocation and flame-graph workflows. | Separate profiling architecture and platform-specific feasibility validation. |
| v0.5+ | eBPF-based observation | Lower-friction visibility for selected runtimes and systems. | Linux-specific permissions, kernel compatibility and elevated-risk scope. |
| v1.x | CI performance checks | Fail or warn on reproducible latency regressions. | Baseline stability, deterministic workloads and opt-in CI integration. |
| v1.x | Remote/team deployment | Shared traces and team-level debugging. | Authentication, authorization, tenancy, transport security, operations and support model. |
| v1.x | Plugin/adapter system | Community integrations for languages, databases and frameworks. | Stable extension API, compatibility policy and sandbox/security boundaries. |
| Optional | AI-assisted trace explanation | Summarize complex traces and propose investigation paths. | Must be optional, disclose data sent to providers, and never replace deterministic trace facts. |

### 19.1 Long-term product direction

If the local trace explorer proves useful, Tracelet can grow into a developer observability workbench: traces first, then correlated logs, profiling and performance regression workflows. The product should not become a generic all-in-one monitoring suite without evidence that developers need that broader scope.

## 20. Open Decisions Before Implementation

| Decision | Recommended starting point | When to finalize |
| --- | --- | --- |
| Product name / namespace | Keep Tracelet as a working title only. | Before public repository/package announcement. |
| OTLP receiver implementation | Prefer established OpenTelemetry libraries/components over custom protocol code. | During technical spike. |
| HTTP Protobuf only vs add gRPC | HTTP Protobuf only in v0.1. | Before receiver API is frozen. |
| Embedded receiver vs bundled collector process | Start as in-process receiver in the CLI binary unless reuse strongly favours a separate component. | During architecture spike. |
| Storage schema for attributes | Compare normalized key-value rows with bounded JSON payloads using actual filters and trace fixtures. | Before schema v1. |
| Default retention / storage budget | Propose 7 days / 1 GB; validate through sample usage. | Before alpha. |
| Live updates | Defer if they risk waterfall correctness or storage reliability. | After static trace explorer works. |
| Source navigation UX | MVP displays and copies location; IDE navigation in extension milestone. | Before UI design freeze. |
| License | Apache-2.0 recommendation. | Before first public commit if contributors are expected. |
| Supported OS matrix | Target macOS, Linux and Windows; release only platforms that pass smoke tests. | Before public preview. |

## 21. Glossary and References

### 21.1 Glossary

| Term | Definition |
| --- | --- |
| Trace | A collection of spans representing a request or operation across one or more components. |
| Span | A timed operation within a trace, with a unique span ID, optional parent span ID, attributes, events and status. |
| OTLP | OpenTelemetry Protocol, used to transport telemetry between instrumentation, collectors and backends. |
| Instrumentation | Code or libraries that emit telemetry from an application. |
| Resource | Attributes describing the entity producing telemetry, such as service name and process metadata. |
| Semantic conventions | Standardized attribute and operation names that improve compatibility across instrumentation and backends. |
| Waterfall | A timeline view of spans, usually nested by parent-child relationships. |
| Source metadata | Code attributes such as file path, function name and line number associated with a span. |
| Local-first | A design approach where data and core workflows remain on the user’s device by default. |
| Retention | Policy controlling how long or how much telemetry is stored before older data is deleted. |

### 21.2 Technical references

[R1] OpenTelemetry, OTLP Specification. Defines OTLP transports and trace export behaviour. https://opentelemetry.io/docs/specs/otlp/

[R2] OpenTelemetry, Go Instrumentation. Describes SDK setup and manual instrumentation for Go. https://opentelemetry.io/docs/languages/go/instrumentation/

[R3] OpenTelemetry, Code Attributes Semantic Convention. Defines stable code source attributes including code.file.path, code.function.name and code.line.number. https://opentelemetry.io/docs/specs/semconv/registry/attributes/code/

[R4] OpenTelemetry, HTTP Semantic Conventions. Defines common HTTP span, metric and log conventions. https://opentelemetry.io/docs/specs/semconv/http/

[R5] Visual Studio Code Extension API, Webviews. Guidance for custom extension views and when to use them. https://code.visualstudio.com/api/ux-guidelines/webviews

[R6] OpenTelemetry, Semantic Conventions overview. Explains common naming and semantic attribute conventions. https://opentelemetry.io/docs/concepts/semantic-conventions/

Reference review date: 26 November 2026. External specifications and SDK behaviour can change; verify versions and stability status again before implementation.

| Specification status<br>This is a planning baseline. It is detailed enough to estimate, review and break into implementation issues, but it is not an implementation. The first engineering activity should be a short feasibility spike that validates one real Go SDK exporter against the proposed local receiver and measures a representative trace write/query workload. |
| --- |
