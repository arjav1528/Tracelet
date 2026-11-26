# Synthetic telemetry fixtures

`fixtures.Request(name)` returns a fresh generated OTLP request with deterministic IDs, timestamps, schema URLs and source metadata. It contains no captured telemetry. Tests under `internal/trace` verify fixture structure, validation and protocol encoding; fixture generation is shared with later storage/API/UI integration tests.

| Name | Intended outcome |
|---|---|
| `simple` | HTTP root plus two children; 3 accepted spans. |
| `nested` | Four levels; hierarchy remains nested. |
| `error` | Error child and exception event/stacktrace; details remain inspectable. |
| `partial` | Child only; mark missing parent without hiding it. |
| `late-parent` | Parent for `partial`; accepting it resolves the missing-parent relationship. |
| `multi-service` | One trace across two resources/services; 3 spans preserved. |
| `cycle` | Two valid span records with cyclic parent references; tree code must break display cycles and warn. |
| `large` | Root plus 2,000 children; bounded paging/rendering remains usable. |
| `sensitive` | Synthetic sentinel secrets in resource, scope, span, nested, event and link attributes; redaction removes sensitive values before persistence. |
| `paths` | Absolute host/container paths, relative/traversal paths, Windows and UNC paths; classify safely on the actual host. |
| `duplicate` | Repeated identical span; first-committed identity gives 3 unique rows. |
| `conflicting-duplicate` | Same identity with different name; first-committed record wins and conflict is counted. |

`fixtures.Protocol(name)` returns `ContentType`, `ContentEncoding` and bytes for `valid`, `gzip`, `malformed`, `oversized` (16 MiB + 1 byte), and `unsupported-content-type`. Generate large payloads on demand instead of committing megabytes of opaque fixture data. Only the valid/gzip bodies should decode successfully; oversized input must be rejected before decoding and unsupported content type before accepting data.

Trace query summaries use lowercase hexadecimal IDs and decimal-string nanoseconds. Full telemetry JSON uses generated Protobuf JSON: int64/timestamps are decimal strings and byte fields, including IDs, are base64. This internal JSON representation is not the OTLP/JSON transport's special hexadecimal-ID encoding. The v0.1 receiver supports binary Protobuf only.
