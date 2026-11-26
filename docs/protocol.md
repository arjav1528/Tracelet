# Trace ingestion protocol

Tracelet accepts `POST /v1/traces` on its retained IPv4 loopback listener. Content type is `application/x-protobuf`; identity and gzip encoding are supported. JSON, gRPC, logs and metrics are outside v0.1. The receiver rejects browser Origin headers and nonmatching Host headers.

Success uses the generated OTLP response. A successful acknowledgement follows SQLite commit. Valid spans from a mixed batch commit before a partial-success response reports the exact number rejected by identity, time or metadata limits. Exact duplicates are idempotent; conflicting repeated span IDs retain the first stored value. Empty requests succeed without a write. Invalid shared resource/scope metadata rejects the affected spans.

Failures use generated `google.rpc.Status` Protobuf: malformed input 400, wrong method 405, unsupported media/encoding 415, oversized payload/count/structural memory budget 413, unsafe origin/Host 403, and temporary capacity or storage failures 503 with `Retry-After: 1`. Error text never echoes telemetry or private database errors. A timeout or lost response can follow a committed batch; retrying is safe under the duplicate rule.

Wire size, decompressed size, recursive depth, object allocation, span count and attribute limits are separate checks. A descriptor-guided wire scan limits object amplification before generated decoding. Concurrency bounds decoding buffers. A second conservative reservation estimates pending message objects and the storage clone; it is not a whole-process RSS limit. Pending reservations remain held through the write and are released on failure/cancellation. There is no unacknowledged background write queue. Configured defaults are documented in [configuration](configuration.md).

Tests cover a pinned Go SDK gzip export through the production receiver and SQLite, exact metadata, persisted redaction, partial success, malformed/gzip/oversized traffic, pressure, cancellation and recovery. Sustained performance and all target operating systems remain release gates.
