# Benchmark evidence

## T02: isolated storage feasibility, 26 November 2026

Host: Apple M5 Max (Mac17,6), 18 logical CPUs, 36 GiB memory, macOS 26.7.3 (25G342), arm64. Go 1.27.4; modernc SQLite v1.63.0/libc v1.80.1. This is **not** the proposed 8-core/16-GB reference machine.

The synthetic fixture contains 100,000 spans in 1,000 traces, eight typed attributes and one event per span. Both candidates use one connection, WAL, synchronous FULL, scalar lookup indexes and 100-span transactions. The list query returns 50 trace summaries; one warmup precedes 100 timed samples. p95 is nearest-rank. The candidates were measured sequentially once, so these results guide the schema choice without establishing a robust speedup claim.

| Layout | Spans/s | Batch commit p95 | List query p95 | Closed database bytes |
|---|---:|---:|---:|---:|
| Indexed scalar fields + Protobuf span details | 153,676 | 0.774 ms | 23.685 ms | 63,651,840 |
| Indexed scalar fields + normalized attributes | 44,742 | 5.650 ms | 20.521 ms | 173,367,296 |

Raw output, heap/allocation snapshots and DB/WAL/SHM byte counts are in [`storage-results.json`](../experiments/otlp-spike/storage-results.json). These are storage-only results; they omit production HTTP processing, redaction, summaries, queue contention, retention and UI load. Heap snapshots are not peak process RSS. No five-minute sustained run, release reference-machine run or cross-platform performance claim has been completed.

Reproduce with:

```sh
TRACELET_SPIKE_BENCH=1 go test ./experiments/otlp-spike -run TestStorageExperiment -count=1 -v
```

The experiment asserts row counts, exact retained values, integrity and reopening. T21 will measure all release targets against the production implementation and record rejected/dropped input separately from successful throughput.
