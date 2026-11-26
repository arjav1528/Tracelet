import type { SpanSummary } from "../src/types";

export function span(
  id: string,
  parent: string,
  start: bigint,
  end: bigint,
): SpanSummary {
  return {
    trace_id: "1".repeat(32),
    span_id: id,
    parent_span_id: parent,
    name: id,
    service_name: "synthetic",
    start_time_unix_nano: String(start),
    end_time_unix_nano: String(end),
    duration_nano: String(end - start),
    kind: 1,
    status: 0,
  };
}
