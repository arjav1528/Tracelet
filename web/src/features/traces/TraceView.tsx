import { useEffect, useMemo, useRef, useState } from "react";
import { APIError } from "../../api";
import { duration, exactUTC } from "../../format";
import type { TraceSummary } from "../../types";
import { Waterfall } from "../../components/Waterfall";
import { buildTree } from "./tree";
import { loadTrace, maxTimelineNodes, TimelineError } from "./load";

export function TraceView({
  selected,
  token,
  close,
  onUnauthorized,
}: {
  selected: TraceSummary;
  token: string;
  close: () => void;
  onUnauthorized: () => void;
}) {
  const [data, setData] = useState<Awaited<
    ReturnType<typeof loadTrace>
  > | null>(null);
  const [error, setError] = useState("");
  const [loaded, setLoaded] = useState(0);
  const [total, setTotal] = useState(selected.span_count);
  const [refresh, setRefresh] = useState(0);
  const [copied, setCopied] = useState("");
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    heading.current?.scrollIntoView?.({ block: "start" });
    heading.current?.focus({ preventScroll: true });
  }, [selected.trace_id]);
  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    setData(null);
    setError("");
    setLoaded(0);
    setTotal(selected.span_count);
    loadTrace(selected.trace_id, token, controller.signal, (count, size) => {
      if (active) {
        setLoaded(count);
        setTotal(size);
      }
    })
      .then((result) => {
        if (active) setData(result);
      })
      .catch((failure: unknown) => {
        if (!active) return;
        if (failure instanceof APIError && failure.status === 401)
          onUnauthorized();
        else if (failure instanceof APIError && failure.status === 404)
          setError(
            "This trace is no longer available. It may have been removed by retention. Refresh the trace list.",
          );
        else if (failure instanceof APIError && failure.status === 409)
          setError(
            "This trace keeps changing. Refresh the timeline after new spans settle.",
          );
        else if (
          failure instanceof TimelineError &&
          failure.reason === "too_large"
        )
          setError(
            `This trace exceeds the ${maxTimelineNodes.toLocaleString()}-span timeline limit. The stored trace is preserved; its timeline has not been partially displayed.`,
          );
        else
          setError(
            "The timeline could not be loaded consistently. Refresh to try again.",
          );
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [selected.trace_id, selected.span_count, token, refresh, onUnauthorized]);
  const tree = useMemo(() => (data ? buildTree(data.spans) : null), [data]);
  const summary = data?.summary ?? selected;
  async function copy() {
    try {
      await navigator.clipboard.writeText(summary.trace_id);
      setCopied("Trace ID copied.");
    } catch {
      setCopied("Copy unavailable. Select the trace ID below to copy it.");
    }
  }
  return (
    <section className="selection" aria-labelledby="selection-heading">
      <div className="section-heading">
        <h2 id="selection-heading" ref={heading} tabIndex={-1}>
          Trace summary
        </h2>
        <div className="trace-actions">
          <button
            className="quiet-button"
            onClick={() => setRefresh((v) => v + 1)}
          >
            Refresh timeline
          </button>
          <button className="quiet-button" onClick={close}>
            Close summary
          </button>
        </div>
      </div>
      <h3>{summary.operation || "Unnamed operation"}</h3>
      <dl className="selection-grid">
        <div className="wide">
          <dt>Trace ID</dt>
          <dd className="trace-id">
            {summary.trace_id}
            <button className="quiet-button" onClick={copy}>
              Copy ID
            </button>
          </dd>
        </div>
        <div>
          <dt>Service</dt>
          <dd>{summary.service_name || "Not provided"}</dd>
        </div>
        <div>
          <dt>Duration</dt>
          <dd>{duration(summary.duration_nano)}</dd>
        </div>
        <div>
          <dt>Start in UTC</dt>
          <dd>{exactUTC(summary.start_time_unix_nano)}</dd>
        </div>
        <div>
          <dt>Observed spans</dt>
          <dd>{summary.span_count.toLocaleString()}</dd>
        </div>
      </dl>
      <p className="quiet">
        Trace bounds reflect received spans; additional spans may arrive later.
      </p>
      <p role="status" className="quiet">
        {copied}
      </p>
      {data?.restarted && (
        <p className="notice" role="status">
          Trace data changed while loading. The timeline restarted with the
          latest revision.
        </p>
      )}
      {error ? (
        <p className="notice warning" role="alert">
          {error}
        </p>
      ) : tree ? (
        <Waterfall
          key={summary.trace_id + "/" + summary.revision}
          tree={tree}
        />
      ) : (
        <p className="loading" role="status">
          Loading timeline: {loaded.toLocaleString()} of{" "}
          {total.toLocaleString()} spans…
        </p>
      )}
    </section>
  );
}
