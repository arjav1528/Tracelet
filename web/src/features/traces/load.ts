import { APIError, apiGet } from "../../api";
import type { SpanPage, SpanSummary, TraceDetail } from "../../types";

export const maxTimelineNodes = 100_000;
export class TimelineError extends Error {
  constructor(public reason: "too_large" | "incomplete") {
    super(
      reason === "too_large"
        ? "Trace exceeds the timeline node limit."
        : "Trace node pages are incomplete or inconsistent.",
    );
  }
}

export async function loadTrace(
  id: string,
  token: string,
  signal: AbortSignal,
  progress?: (loaded: number, total: number) => void,
) {
  if (!/^[a-f0-9]{32}$/.test(id)) throw new TimelineError("incomplete");
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      signal.throwIfAborted();
      const detail = await apiGet<TraceDetail>(
        `/api/v1/traces/${id}`,
        token,
        signal,
      );
      if (
        !detail.summary ||
        detail.summary.trace_id !== id ||
        !Number.isSafeInteger(detail.summary.span_count) ||
        detail.summary.span_count < 0
      )
        throw new TimelineError("incomplete");
      const total = detail.summary.span_count;
      if (total > maxTimelineNodes) throw new TimelineError("too_large");
      const spans: SpanSummary[] = [];
      const seen = new Set<string>();
      function append(page: SpanSummary[]) {
        if (!Array.isArray(page) || page.length > 500)
          throw new TimelineError("incomplete");
        for (const span of page) {
          if (span.trace_id !== id || seen.has(span.span_id) || !span.span_id)
            throw new TimelineError("incomplete");
          seen.add(span.span_id);
          spans.push(span);
        }
        if (spans.length > total) throw new TimelineError("incomplete");
        progress?.(spans.length, total);
      }
      append(detail.spans);
      const cursors = new Set<string>();
      let cursor = detail.next_cursor;
      while (cursor) {
        signal.throwIfAborted();
        if (cursors.has(cursor) || cursors.size >= 200 || spans.length >= total)
          throw new TimelineError("incomplete");
        cursors.add(cursor);
        const params = new URLSearchParams({ limit: "500", cursor });
        const page = await apiGet<SpanPage>(
          `/api/v1/traces/${id}/spans?${params}`,
          token,
          signal,
        );
        if (
          page.revision !== detail.summary.revision ||
          page.span_count !== total
        )
          throw new APIError(409);
        if (page.trace_id !== id || page.spans.length === 0)
          throw new TimelineError("incomplete");
        append(page.spans);
        cursor = page.next_cursor;
      }
      signal.throwIfAborted();
      if (spans.length !== total) throw new TimelineError("incomplete");
      return { summary: detail.summary, spans, restarted: attempt > 0 };
    } catch (error) {
      if (
        error instanceof APIError &&
        error.status === 409 &&
        attempt === 0 &&
        !signal.aborted
      )
        continue;
      throw error;
    }
  }
  throw new APIError(409);
}
