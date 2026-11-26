# Architecture and implementation boundaries

Tracelet is a Go modular monolith. The CLI owns project configuration, a local process, an OTLP/HTTP receiver, SQLite, and eventually an embedded browser UI/API. The current task checklist is the source of implementation status; the complete browser workflow is not yet available.

## Ownership and persistence

Every writable open must be preceded by `session.Acquire(config.Storage.Path + ".lock")`; hold it until storage closes. The OS-backed lock file stays on disk to prevent inode-replacement races. A leftover file is not ownership: the kernel releases the actual lock when the process exits or crashes. macOS ownership and crash-release tests run locally; Linux/Windows lock code has been cross-compiled, not claimed as natively validated.

Schema v1 uses one database per project, a project identity/global revision record, deduplicated resource and instrumentation-scope metadata, indexed span scalars with exact Protobuf details, and transactional trace summaries. Resource/scope hashes include schema URLs and canonical attribute-key ordering. Array order remains unchanged. The caller's request is not mutated during storage ingestion.

Storage validates, clones and sanitizes before writing. A transaction inserts spans and updates bounds, counts, error counts and revisions together. Success means the transaction committed. Duplicate identity is `(trace_id, span_id)`; the first committed sanitized version wins. A conflicting duplicate is counted without replacement, payload logging or refreshing retention age. New late spans update summaries. Resource/scope records are created only for new spans.

The initial connection policy is one SQLite connection, WAL, synchronous FULL, foreign keys and a bounded busy timeout, applied through connection initialization. The storage writer serializes ingestion before cloning to avoid multiple unbounded write preparations. Collector admission limits are a separate requirement. Database/file directories are private where the platform supports those permissions.

New databases migrate from an empty schema 0 to schema 1 transactionally. Unknown newer versions, project mismatches, malformed current schemas and unrelated nonempty databases fail safely. They are not deleted or reset. Initial databases enable incremental vacuum before table creation so retention can reclaim pages without rebuilding the entire database. [SQLite auto-vacuum documentation](https://sqlite.org/pragma.html#pragma_auto_vacuum)

## Data contracts

`internal/trace` contains query DTOs. IDs in summaries are hexadecimal strings; epoch nanoseconds, durations and revisions are decimal strings. Full telemetry retains generated Protobuf fields. Its generated ProtoJSON representation uses strings for 64-bit integers and base64 for byte fields, including byte IDs; it is not an OTLP/JSON wire encoder.

`internal/ingestion.Result` is the shared committed-ingestion result. Storage aliases it as `storage.IngestResult`, allowing the collector's narrow sink contract to depend on ingestion without a storage→ingestion→storage import cycle.

Sanitization is shared by ingestion/persistence and later export. See [privacy](privacy.md). Trace hierarchy, query pagination, retention, process discovery, local API and UI are implemented in their owning tasks rather than represented by empty frameworks here.


## Trace queries and hierarchy

Trace lists use snapshot reads and bounded keyset pages (default50/max200). Search is a literal substring of any retained span name, with SQLite ASCII case folding; `%` and `_` have no wildcard meaning. Newest sorts by start descending, slowest by observed duration descending then start, and errors by error presence then start; trace ID breaks ties. Opaque cursors bind the query, limit, ordering and dataset revision. Ingestion or retention changes invalidate list pagination; callers restart after a revision-change response.

A trace detail read returns the summary and first500 scalar timeline nodes, with an explicit next cursor. Node pages default500/max2000 and bind the trace revision. Complete span attributes/resource/scope/events/links are fetched only for the selected span in a separate snapshot. Timeline queries never decode full payloads. Exact times and revisions cross JSON as decimal strings.

Tree assembly is iterative and keeps all nodes visible, including missing parents, multiple roots and cycles. A deterministic display edge breaks each cycle without changing stored IDs. Warnings cover missing root/parent, cycles and child intervals outside their parents. Missing-parent classification waits until all metadata pages for one revision have loaded. Trace duration is max(end) minus min(start), not a sum of child durations. Operation and primary service come from the earliest explicit root by start/ID, falling back to the earliest observed span; individual nodes retain their own service.
