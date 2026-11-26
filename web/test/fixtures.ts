export const status = {
  project: { id: "1".repeat(32), name: "checkout-api" },
  session: { role: "run", instance_id: "2".repeat(32) },
  receiver: {
    running: true,
    accepted_spans: 6,
    rejected_spans: 0,
    rejected_requests: 0,
    duplicates: 0,
    conflicts: 0,
    last_received_at: "2026-11-26T08:36:20.123456789Z",
  },
  storage: {
    trace_count: 2,
    span_count: 6,
    total_bytes: 1048576,
    budget_bytes: 1073741824,
    pressure: false,
    pruned_traces: 0,
  },
  settings: {
    default_service_name: "checkout-api",
    protocol: "otlp-http-protobuf",
    retention_days: 7,
    storage_max_size_mb: 1024,
    capture_http_bodies: false,
    redaction_enabled: true,
    max_wire_bytes: 4194304,
    max_decoded_bytes: 16777216,
    max_spans_per_batch: 5000,
  },
};

export const summary = {
  trace_id: "0".repeat(31) + "1",
  operation: "GET /checkout",
  service_name: "checkout-api",
  start_time_unix_nano: "1795679780123456789",
  end_time_unix_nano: "1795679780246913578",
  duration_nano: "123456789",
  span_count: 3,
  has_error: false,
  revision: "1",
};

export const page = { traces: [summary], revision: "1" };
