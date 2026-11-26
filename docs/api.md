# Local read API v1

The embedded UI and read API share the UI listener on literal `127.0.0.1`. The OTLP receiver uses a separate listener. Start with `tracelet run` or foreground `tracelet ui`; `tracelet status` never starts services.

Every `/api/v1` request requires exactly one `X-Tracelet-Token` header containing the private nonce from that instance's descriptor or explicit `ui` fragment URL. Public `GET /healthz` returns only `{version:1,project_id,instance_id}`. The nonce is absent from health, status, normal logs and HTML. Native clients must verify health identity before sending the nonce. Do not follow redirects or send it to another endpoint.

Requests must use the actual listener Host/port. Origin must be absent or exactly the UI origin; fetch metadata must be absent, `same-origin` or `none`. Hostile/null origins and cross-site requests are rejected even with a token. No CORS allowlist, account system or remote hosting is provided. Responses use no-store, nosniff, restrictive CSP/frame and referrer policies.

| GET route | Response |
|---|---|
| `/api/v1/status` | `project`, public `session`, receiver counters, physical `storage` usage and an explicit safe settings subset |
| `/api/v1/traces?limit=50&q=order&sort=newest&cursor=...` | `traces`, decimal-string dataset `revision`, optional `next_cursor` |
| `/api/v1/traces/{trace_id}` | `summary`, first 500 scalar `spans`, `warnings`, optional `next_cursor` |
| `/api/v1/traces/{trace_id}/spans?limit=500&cursor=...` | `trace_id`, `spans`, trace `revision`, `span_count`, optional `next_cursor` |
| `/api/v1/traces/{trace_id}/spans/{span_id}` | Generated ProtoJSON export-request wrapper containing that sanitized span with its resource and scope |

The export route arrives in T17. All current routes are read-only. No arbitrary source-file or execution endpoint exists.

Trace IDs are 32 hexadecimal characters; span IDs are 16, with nonzero decoded values. Query summaries use hex IDs and decimal strings for nanoseconds/durations/revisions. Generated ProtoJSON detail byte fields use base64 IDs, and 64-bit attribute integers/timestamps use strings. Enum values follow generated ProtoJSON names. Missing optional/default fields can be omitted. React consumers must treat telemetry strings as text and use `BigInt` for absolute nanoseconds.

List limits are 1–200 (default 50); timeline limits 1–2000 (default 500). `q` is a literal any-span-name substring of at most 256 UTF-8 bytes, with ASCII case-insensitive matching. Sort is `newest`, `slowest` or `errors`. List cursors bind query, sort, limit and dataset revision. Timeline cursors bind trace, limit and trace revision. Keep the first page's 500 limit for its continuation cursor. On 409, restart paging. Do not classify an unloaded parent as missing; collect all node pages for one revision before final warnings. A quiet trace is never claimed complete.

Duplicate or unknown query keys, invalid escapes/IDs/limits, NULs and oversized queries are rejected. Raw query cap is 8 KiB, cursor cap 4096 bytes. SQL inputs are parameterized. Up to the configured collector concurrency, capped at 8, can run API reads; excess returns 503. Read work has a deadline of at most 10 seconds. Encoded JSON responses are limited to 32 MiB, checked before writing success bytes. Serialization can temporarily allocate a larger bounded buffer; this is not a process-memory cap.

API errors use a uniform envelope without SQL, raw telemetry, credentials or storage paths:

```json
{"error":{"code":"revision_changed","message":"trace data changed; restart pagination"}}
```

| HTTP status | Meaning |
|---|---|
|400| Invalid path/query |
|401| Missing/invalid private token |
|403| Unsafe Host/Origin/fetch metadata |
|404| Missing trace/span/route, including retention removal |
|405| Method other than GET |
|408/504| Canceled request/query deadline |
|409| Dataset/trace revision changed |
|413| Encoded response exceeds supported size |
|500| Safe internal read/encoding failure |
|503| Storage unavailable or bounded read capacity exhausted |

Synthetic response examples live in `internal/server/testdata/`. They are contract fixtures, not application fallback data. The browser always reads the active local API.
