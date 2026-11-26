# OTLP / SQLite feasibility spike

This is isolated, test-only code using synthetic telemetry. It is not Tracelet's production receiver, schema, or privacy pipeline. It adds no CLI command.

Run the real exporter check:

```sh
go test ./experiments/otlp-spike -run TestGoExporterGzipPersistAndReopen -count=1 -v
go test -race ./experiments/otlp-spike -count=1
```

The test starts a `127.0.0.1` receiver, emits two parent/child spans with a known trace ID through the real Go SDK's gzip HTTP/Protobuf exporter, and commits the export before acknowledgement. A one-hour batch timer means the explicit SDK shutdown causes the final flush. It closes the server/database, verifies the persisted trace in a fresh process, reopens locally, and checks integrity plus complete Protobuf equality. Assertions cover resource/scope schema metadata, stable source attributes, event/link/array fields, parent identity, and integers/timestamps above JavaScript's exact integer range.

An explicit `WithEndpointURL` must include `/v1/traces`; supplying only the base URL sent `POST /` and failed the interoperability test. This differs from generic OTEL base-endpoint suffix construction. Source attributes here are deliberately supplied on custom spans; this does not establish automatic source coverage.

The child-only `TestReopenInFreshProcess` reports skipped in the parent test run; the main persistence test invokes it successfully in a fresh test process. Loopback network binding must be permitted by the test environment.

Run the opt-in storage experiment:

```sh
TRACELET_SPIKE_BENCH=1 go test ./experiments/otlp-spike -run TestStorageExperiment -count=1 -v
```

Both candidates use one connection, WAL, synchronous FULL, 100-span transactions, scalar ID/timestamp columns and the same indexes. The fixture has 100,000 spans in 1,000 traces, eight typed attributes and one event per span. One candidate stores a complete span Protobuf; the other stores the same non-attribute Protobuf fields and moves attributes into keyed rows with typed Protobuf values. Resource/scope storage is excluded from this comparison and covered by the interoperability check. Each candidate checks row counts, a reconstructed span, integrity, and reopening.

The list query groups spans by trace, orders by first timestamp, and returns 50 summaries. One query warmup precedes 100 timed samples. p95 uses the nearest-rank method. Throughput includes fixture construction, serialization, inserts, and transaction commits. It is storage-only throughput: no HTTP transport, privacy processing, summaries table, UI contention or production backpressure is represented. Go heap snapshots and cumulative allocation are measured, not process peak RSS. Before-close disk numbers include DB, WAL and SHM separately; closed DB size is also recorded.

The recorded run in `storage-results.json` used Go 1.27.4, modernc SQLite v1.63.0, macOS arm64, and 18 logical CPUs. The project benchmark report records the full measured host details. The candidates ran sequentially in the order shown; these single-run results are directional evidence, not a controlled reference-machine release benchmark. They support starting with indexed scalar columns plus Protobuf details; arbitrary attribute search is not required by v0.1.

Not covered: hostile input, redaction, process-kill/power-loss recovery, sustained five-minute load, concurrent clients, migrations, retention, other operating systems, or end-to-end release performance. Those remain in the production task checklist.
