export type TraceSummary = {
  trace_id: string;
  operation: string;
  service_name: string;
  start_time_unix_nano: string;
  end_time_unix_nano: string;
  duration_nano: string;
  span_count: number;
  has_error: boolean;
  revision: string;
};

export type TracePage = {
  traces: TraceSummary[];
  revision: string;
  next_cursor?: string;
};

export type Status = {
  project: { id: string; name: string };
  session: { role: "run" | "history"; instance_id: string };
  receiver: {
    running: boolean;
    accepted_spans: number;
    rejected_spans: number;
    rejected_requests: number;
    duplicates: number;
    conflicts: number;
    last_received_at?: string;
  };
  storage: {
    trace_count: number;
    span_count: number;
    total_bytes: number;
    budget_bytes: number;
    pressure: boolean;
    pruned_traces: number;
  };
  settings: {
    default_service_name: string;
    protocol: string;
    retention_days: number;
    storage_max_size_mb: number;
    capture_http_bodies: boolean;
    redaction_enabled: boolean;
    max_wire_bytes: number;
    max_decoded_bytes: number;
    max_spans_per_batch: number;
  };
};
