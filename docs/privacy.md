# Local telemetry and privacy

Tracelet's sanitizer replaces sensitive attribute values with `[REDACTED]` before persistence. It traverses span, resource, instrumentation-scope, event and link attributes, including nested key-value lists and arrays. Unrelated types, identifiers and timestamps are retained. Sanitization is idempotent.

Patterns are case-insensitive full-key matches: `*` matches any sequence and `?` one character, including Unicode characters. Other characters are literal. Both each key and its dotted nested path are checked. Default patterns cover authorization, cookies, password/passwd, token, secret, API key, email, body/payload, `url.full`, `url.query`, `http.url`, `db.statement` and `db.query.text`. Custom patterns add to defaults and cannot disable them. Configuration permits at most 128 effective pattern entries of 1–256 UTF-8 bytes each.

The sanitizer reports a redaction count and structural paths, never original values or key text. It records at most 1,024 paths and explicitly marks a truncated report while continuing to redact all values and count them. It rejects nesting over 64 or traversal over four million nodes as a defensive ceiling; configured ingestion limits are smaller and apply first. On a sanitization error the owned request is cleared and must be rejected, never persisted as the original payload.

Key matching is not comprehensive secret or personal-data removal. Arbitrary span names, exception messages, stack traces, unknown/free-text fields and the child application's own stdout/stderr can contain sensitive material. Tracelet does not promise exports are safe to publish without inspection. Full URLs, query text and HTTP bodies are redacted by default even when instrumented upstream; Tracelet does not add body capture. No unredacted export mode is planned for v0.1.

Production storage now reuses this sanitizer before committed writes. Retention and export follow in their owning tasks; export must reapply the current policy to older records and diagnostics must remain free of payloads. Config/data locations and permissions are described in [configuration](configuration.md). At this stage no capture service is exposed by the CLI.

## Retention and disk pressure

The owning run/history session prunes at startup and once a minute. Age is based on the last newly accepted span's receipt time, not an application timestamp; duplicate-only retries do not refresh it. Defaults are seven days and 1024 MiB. A 30-second quiet window protects recently active traces. Deletion removes whole traces, oldest eligible first, plus now-unreferenced resource/scope metadata. A bounded pass removes at most 64 traces and reclaims at most 4096 SQLite pages; a large expired backlog takes more than one pass.

Reported use includes the database, WAL, shared-memory and rollback-journal files. Checkpointing and incremental vacuum remeasure actual files; deletion alone does not imply free disk space. Held readers can delay reclamation. Status reports physical bytes, reusable pages, pressure and current-session prune counts. Maintenance messages report deletion and pressure without printing telemetry.

Admission reserves a conservative per-batch growth allowance before writing. If pruning cannot make space, new spans receive a retryable ingestion failure; already committed data remains intact and duplicate-only retries can succeed. The configured budget allows bounded overshoot from one serialized bounded transaction plus SQLite bookkeeping. It is not a filesystem quota. A single batch whose conservative reserve exceeds the whole budget is rejected without deleting history pointlessly; reduce batch size or raise the budget. Admission can prune eligible older traces before rejecting a batch if reclamation remains insufficient.

To delete all local trace data deliberately, first stop every run/history owner for the project, then remove its configured database and database sidecars. Do not delete a live lock file or replace a live database. Preserve config to retain the same project identity. No remote copy is created by Tracelet.
