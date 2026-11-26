# Project configuration v1

Run `tracelet init` in the project directory. It writes `.tracelet/config.yaml`, prints the path and creates no application source or telemetry database. The directory/file use owner-only permissions on supported systems. Existing configuration is refused unless `init --force` is explicit. **Force replaces configuration and creates a new project ID**; it does not delete old telemetry files. Keep the old ID/configuration when you need to reopen that history.

The loader rejects unknown fields, duplicate keys, multiple YAML documents, unsupported versions, malformed identifiers, invalid ports, unsafe bindings and nonpositive limits. The schema version must be present and equal to 1. A configuration file is limited to 1 MiB. HTTP body capture and non-loopback serving are unsupported in v0.1.

Values explicitly supplied by a command take precedence over file values; omitted file values use the documented defaults. At T03 only `init --force` is exposed as a command option; capture/service startup options arrive with their owning commands. There is no implied generic override mechanism.

Project identity is a generated 32-character lowercase hexadecimal value. It is stable when loading an existing config; initialization never silently replaces it. Paths inside the ordinary `.tracelet/config.yaml` resolve against the project directory. For an explicitly loaded config elsewhere, relative paths resolve against that config's directory. Workspace paths must identify an existing directory and are canonicalized; missing/inaccessible roots produce an error.

With `storage.path: ""`, data will live under:

- macOS: `~/Library/Application Support/tracelet/projects/<id>/traces.db`
- Linux: `$XDG_DATA_HOME/tracelet/projects/<id>/traces.db` when absolute, otherwise `~/.local/share/tracelet/projects/<id>/traces.db`
- Windows: `%LOCALAPPDATA%/tracelet/projects/<id>/traces.db`, falling back to the user's `AppData/Local`

The initial defaults are:

| Field | Default |
|---|---|
| `collector.bind`, `ui.bind` | `127.0.0.1` only |
| `collector.port`, `ui.port` | `0` (bind an available port) |
| `collector.protocol` | `otlp-http-protobuf` |
| `ui.open_browser` | `true` |
| `storage.retention_days` | `7` (valid 1–36,500) |
| `storage.max_size_mb` | `1024` MiB; the field name is historical |
| `shutdown_grace_seconds` | `5` (valid 1–3,600) |
| `source.workspace_root` | `.` |
| `privacy.capture_http_bodies` | `false`; `true` is rejected |
| `limits.max_wire_bytes`, `max_decoded_bytes` | 4 MiB / 16 MiB |
| `limits.max_spans_per_batch`, `max_concurrent_requests` | 5,000 / 4 |
| `limits.max_attributes`, `max_key_bytes`, `max_value_bytes` | 128 / 256 / 4,096 |
| `limits.max_depth`, `max_events`, `max_links` | 8 / 128 / 128 |
| `limits.max_pending_batches`, `max_pending_bytes` | 8 / 32 MiB |
| `limits.read_header_timeout_seconds`, `read_timeout_seconds` | 5 / 15 |
| `limits.write_timeout_seconds`, `idle_timeout_seconds` | 15 / 60 |

All limits must be positive and representable. HTTP timeouts cannot exceed 3,600 seconds; nesting depth cannot exceed 64. Effective payload, queue and retention enforcement is implemented in their subsequent tasks; merely creating this configuration does not start capture.

`privacy.redact_attribute_keys` adds patterns to conservative defaults covering authorization, cookies, passwords, tokens, secrets, API keys, email, bodies/payloads, full URLs/query strings and raw database queries. Users cannot accidentally disable defaults by supplying a custom list. Pattern semantics and the limitation of key-based sanitization are documented with the sanitizer in T05. No source remapping or body-capture feature is implied by this schema.
