import { useCallback, useEffect, useRef, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { APIError, apiGet } from "./api";
import { TraceView } from "./features/traces/TraceView";
import { bytes, duration, exactUTC, localTime } from "./format";
import type { Status, TracePage, TraceSummary } from "./types";

function Brand() {
  return (
    <a className="brand" href="#traces" aria-label="Tracelet home">
      <svg
        width="28"
        height="32"
        viewBox="0 0 28 32"
        fill="none"
        aria-hidden="true"
      >
        <path
          d="M6 4v16a6 6 0 006 6h10M6 10h16"
          stroke="currentColor"
          strokeWidth="2"
        />
        <circle cx="6" cy="4" r="3" fill="currentColor" />
        <circle cx="22" cy="10" r="3" fill="currentColor" />
        <circle cx="22" cy="26" r="3" fill="currentColor" />
      </svg>
      <span>tracelet</span>
    </a>
  );
}

function Empty({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="empty">
      <span className="empty-glyph" aria-hidden="true">
        ⌁
      </span>
      <h2>{title}</h2>
      <div>{children}</div>
    </div>
  );
}

function Access({ expired }: { expired: boolean }) {
  return (
    <main className="access">
      <Brand />
      <div className="access-copy">
        <p className="quiet">Local traces. Your workstation.</p>
        <h1>
          {expired ? "Reconnect to this project" : "Open your local project"}
        </h1>
        <p>
          {expired
            ? "This tab no longer has access to the running session. Open a fresh link from your project directory."
            : "Use the private link printed by Tracelet to connect this tab to your project."}
        </p>
        <code className="command">tracelet ui</code>
        <p className="quiet">
          Starting a new project? Run <code>tracelet init</code> first. Your
          application needs OpenTelemetry instrumentation to produce traces.
        </p>
        <p className="privacy-note">
          Telemetry stays on your workstation. This page does not contact
          external services.
        </p>
      </div>
    </main>
  );
}

function Settings({ status }: { status: Status }) {
  const settings = status.settings;
  return (
    <details id="settings" className="settings">
      <summary>
        Effective settings<span className="quiet">Read only</span>
      </summary>
      <p className="quiet">
        Change your project’s configuration file, then restart its Tracelet
        session to apply changes.
      </p>
      <dl className="settings-grid">
        <div>
          <dt>Default service</dt>
          <dd>{settings.default_service_name || "Not provided"}</dd>
        </div>
        <div>
          <dt>Trace receiver</dt>
          <dd>OTLP / HTTP Protobuf</dd>
        </div>
        <div>
          <dt>Retention</dt>
          <dd>{settings.retention_days} days</dd>
        </div>
        <div>
          <dt>Storage budget</dt>
          <dd>{settings.storage_max_size_mb} MiB</dd>
        </div>
        <div>
          <dt>Attribute redaction</dt>
          <dd>{settings.redaction_enabled ? "Enabled" : "Disabled"}</dd>
        </div>
        <div>
          <dt>Request and response bodies</dt>
          <dd>{settings.capture_http_bodies ? "Captured" : "Not captured"}</dd>
        </div>
        <div>
          <dt>Maximum request</dt>
          <dd>
            {bytes(settings.max_wire_bytes)} compressed /{" "}
            {bytes(settings.max_decoded_bytes)} decoded
          </dd>
        </div>
        <div>
          <dt>Maximum batch</dt>
          <dd>{settings.max_spans_per_batch.toLocaleString()} spans</dd>
        </div>
      </dl>
      <p className="privacy-note">
        Key-based redaction reduces exposure. Free-text attributes can still
        contain sensitive values.
      </p>
    </details>
  );
}

export function App({ token }: { token: string }) {
  const [status, setStatus] = useState<Status | null>(null);
  const [statusError, setStatusError] = useState(false);
  const [expired, setExpired] = useState(false);
  const [page, setPage] = useState<TracePage | null>(null);
  const [loading, setLoading] = useState(true);
  const [listError, setListError] = useState(false);
  const [query, setQuery] = useState("");
  const [draft, setDraft] = useState("");
  const [sort, setSort] = useState("newest");
  const [cursor, setCursor] = useState("");
  const [previous, setPrevious] = useState<string[]>([]);
  const [refresh, setRefresh] = useState(0);
  const [notice, setNotice] = useState("");
  const [selected, setSelected] = useState<TraceSummary | null>(null);
  const [searchError, setSearchError] = useState("");
  const onUnauthorized = useCallback(() => setExpired(true), []);
  const traceButtons = useRef(new Map<string, HTMLButtonElement>());

  useEffect(() => {
    if (!token || expired) return;
    const controller = new AbortController();
    let active = true;
    setStatusError(false);
    apiGet<Status>("/api/v1/status", token, controller.signal)
      .then((result) => {
        if (
          !result.project?.id ||
          !result.project.name ||
          !result.storage ||
          !result.receiver ||
          !result.settings
        )
          throw new Error("Invalid status");
        if (active) setStatus(result);
      })
      .catch((error: unknown) => {
        if (!active) return;
        if (error instanceof APIError && error.status === 401) setExpired(true);
        else setStatusError(true);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [token, refresh, expired]);

  useEffect(() => {
    if (!token || expired) return;
    const controller = new AbortController();
    let active = true;
    setLoading(true);
    setListError(false);
    setPage(null);
    setSelected(null);
    const params = new URLSearchParams({ limit: "50", sort });
    if (query) params.set("q", query);
    if (cursor) params.set("cursor", cursor);
    apiGet<TracePage>(
      "/api/v1/traces?" + params.toString(),
      token,
      controller.signal,
    )
      .then((result) => {
        if (!Array.isArray(result.traces) || result.traces.length > 50)
          throw new Error("Invalid trace page");
        if (active) setPage(result);
      })
      .catch((error: unknown) => {
        if (!active) return;
        if (error instanceof APIError && error.status === 401) setExpired(true);
        else if (error instanceof APIError && error.status === 409 && cursor) {
          setPrevious([]);
          setCursor("");
          setNotice("Trace data changed. Showing the latest first page.");
        } else setListError(true);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [token, expired, query, sort, cursor, refresh]);

  function resetPage() {
    setCursor("");
    setPrevious([]);
    setNotice("");
  }
  function refreshAll() {
    resetPage();
    setRefresh((value) => value + 1);
  }
  function search(event: FormEvent) {
    event.preventDefault();
    if (new TextEncoder().encode(draft).length > 256 || draft.includes("\0")) {
      setSearchError("Use an operation search of at most 256 bytes.");
      return;
    }
    setSearchError("");
    resetPage();
    setQuery(draft);
  }

  if (!token || expired) return <Access expired={expired} />;
  const rows = page?.traces ?? [];
  return (
    <div className="shell">
      <a className="skip-link" href="#traces">
        Skip to traces
      </a>
      <aside className="rail">
        <Brand />
        <div className="project-marker">
          <span className="project-symbol" aria-hidden="true">
            {status?.project.name.slice(0, 1).toUpperCase() || "·"}
          </span>
          <div>
            <span className="quiet">Project</span>
            <p>{status?.project.name || "Connecting…"}</p>
          </div>
        </div>
        <nav aria-label="Project">
          <a className="nav-active" href="#traces">
            Traces<span aria-hidden="true">⌁</span>
          </a>
          <a href="#settings">Settings</a>
        </nav>
        <div className="local-note">
          <span className="local-dot" aria-hidden="true" />
          <span>Stored locally</span>
          <p>
            No account. No cloud.
            <br />
            Your runtime, on your machine.
          </p>
        </div>
      </aside>
      <main className="workspace" id="traces">
        <header className="page-heading">
          <div>
            <p className="quiet">Project overview</p>
            <h1>{status?.project.name || "Your local traces"}</h1>
            <p className="session-label">
              <span
                className={
                  status?.receiver.running ? "status-dot live" : "status-dot"
                }
                aria-hidden="true"
              />
              {status
                ? status.receiver.running
                  ? "Capturing traces"
                  : "History session"
                : "Connecting to session…"}
            </p>
          </div>
          <button className="refresh-button" onClick={refreshAll}>
            <span aria-hidden="true">↻</span> Refresh
          </button>
        </header>
        {status && (
          <section className="receipt" aria-label="Session overview">
            <p>
              <strong>{status.storage.trace_count.toLocaleString()}</strong>{" "}
              traces <span className="quiet">/</span>{" "}
              <strong>{status.storage.span_count.toLocaleString()}</strong>{" "}
              spans stored
            </p>
            <p>
              {bytes(status.storage.total_bytes)}{" "}
              <span className="quiet">
                of {bytes(status.storage.budget_bytes)}
              </span>
            </p>
            <p className="receipt-last">
              {status.receiver.last_received_at ? (
                <>
                  Last receipt{" "}
                  <time title={status.receiver.last_received_at}>
                    {new Date(
                      status.receiver.last_received_at,
                    ).toLocaleTimeString()}
                  </time>
                </>
              ) : (
                "No telemetry received this session"
              )}
            </p>
            {status.receiver.running && (
              <p className="receipt-session">
                {status.receiver.accepted_spans.toLocaleString()} spans accepted
                this session
                {status.receiver.duplicates > 0
                  ? `, ${status.receiver.duplicates.toLocaleString()} duplicate deliveries ignored`
                  : ""}
                {status.receiver.conflicts > 0
                  ? `, ${status.receiver.conflicts.toLocaleString()} conflicting duplicates kept at their first accepted value`
                  : ""}
                .
              </p>
            )}
          </section>
        )}
        {status?.session.role === "history" && (
          <p className="history-note">
            Browsing stored traces. Stop the foreground history session before
            using <code>tracelet run</code> to capture more.
          </p>
        )}
        {status &&
          (status.receiver.rejected_spans > 0 ||
            status.receiver.rejected_requests > 0) && (
            <p className="notice warning" role="status">
              {status.receiver.rejected_spans} spans and{" "}
              {status.receiver.rejected_requests} requests rejected. Check your
              application’s export limits and Tracelet diagnostics.
            </p>
          )}
        {status?.storage.pressure && (
          <p className="notice warning" role="status">
            Storage is under pressure. New telemetry may be rejected until space
            is available. Review retention and the storage budget.
          </p>
        )}
        {statusError && (
          <div className="notice warning" role="status">
            Session status could not be refreshed. Stored counts may be out of
            date.
          </div>
        )}
        <section className="trace-section" aria-labelledby="trace-heading">
          <div className="section-heading">
            <h2 id="trace-heading">Traces</h2>
            <span className="quiet">Manual refresh</span>
          </div>
          <form role="search" className="filters" onSubmit={search}>
            <div className="search-field">
              <label htmlFor="search">Search operations</label>
              <div className="search-control">
                <input
                  id="search"
                  type="search"
                  value={draft}
                  placeholder="Find an operation…"
                  onChange={(event) => setDraft(event.target.value)}
                  aria-describedby={
                    searchError ? "search-error" : "search-hint"
                  }
                />
                <button type="submit">Search</button>
              </div>
              <span id="search-hint" className="quiet small">
                Matches any span’s operation name.
              </span>
            </div>
            <div className="sort-field">
              <label htmlFor="sort">Sort</label>
              <select
                id="sort"
                value={sort}
                onChange={(event) => {
                  resetPage();
                  setSort(event.target.value);
                }}
              >
                <option value="newest">Newest first</option>
                <option value="slowest">Slowest first</option>
                <option value="errors">Errors first</option>
              </select>
            </div>
          </form>
          {searchError && (
            <p id="search-error" className="notice warning" role="alert">
              {searchError}
            </p>
          )}
          {notice && (
            <p className="notice" role="status">
              {notice}
            </p>
          )}
          <div aria-busy={loading}>
            {loading ? (
              <div className="loading" role="status">
                <span className="loading-line" />
                Loading traces…
              </div>
            ) : listError ? (
              <Empty
                title={
                  statusError
                    ? "Local session unavailable"
                    : "Traces could not be loaded"
                }
              >
                <p>
                  {statusError
                    ? "The foreground Tracelet session may have stopped. Run tracelet ui in your project to open stored history."
                    : "Refresh to try again. If this continues, check the terminal running Tracelet."}
                </p>
              </Empty>
            ) : rows.length === 0 ? (
              <Empty
                title={query ? "No matching traces" : "No traces stored yet"}
              >
                <p>
                  {query
                    ? "Try a different operation name or clear the search."
                    : "Run an OpenTelemetry-instrumented application with Tracelet, then refresh this view."}
                </p>
                {query ? (
                  <button
                    onClick={() => {
                      setDraft("");
                      setQuery("");
                      resetPage();
                    }}
                  >
                    Clear search
                  </button>
                ) : (
                  <code className="command compact">
                    tracelet run -- your-command
                  </code>
                )}
              </Empty>
            ) : (
              <div
                className="table-scroll"
                tabIndex={0}
                role="region"
                aria-label="Trace results, scroll horizontally for all columns"
              >
                <table>
                  <caption className="sr-only">
                    Retained traces. Times are local; select a summary for exact
                    UTC timestamps.
                  </caption>
                  <thead>
                    <tr>
                      <th scope="col">Operation / trace</th>
                      <th scope="col">Service</th>
                      <th scope="col">
                        Started <span className="quiet">(local)</span>
                      </th>
                      <th scope="col" className="numeric">
                        Duration
                      </th>
                      <th scope="col" className="numeric">
                        Spans
                      </th>
                      <th scope="col">Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((row) => (
                      <tr
                        key={row.trace_id}
                        className={
                          selected?.trace_id === row.trace_id
                            ? "selected-row"
                            : ""
                        }
                      >
                        <td>
                          <button
                            className="operation"
                            ref={(element) => {
                              if (element)
                                traceButtons.current.set(row.trace_id, element);
                              else traceButtons.current.delete(row.trace_id);
                            }}
                            aria-label={`View summary for ${row.operation || "Unnamed operation"}`}
                            aria-pressed={selected?.trace_id === row.trace_id}
                            onClick={() => setSelected(row)}
                          >
                            {row.operation || "Unnamed operation"}
                          </button>
                          <span className="short-id" title={row.trace_id}>
                            {row.trace_id.slice(0, 8)}…{row.trace_id.slice(-8)}
                          </span>
                        </td>
                        <td className="service-name">
                          {row.service_name || "Not provided"}
                        </td>
                        <td>
                          <time
                            dateTime={exactUTC(row.start_time_unix_nano)}
                            title={exactUTC(row.start_time_unix_nano)}
                          >
                            {localTime(row.start_time_unix_nano)}
                          </time>
                        </td>
                        <td className="numeric duration">
                          {duration(row.duration_nano)}
                        </td>
                        <td className="numeric">
                          {row.span_count.toLocaleString()}
                        </td>
                        <td>
                          <span
                            className={row.has_error ? "state error" : "state"}
                          >
                            {row.has_error ? "Error" : "No error"}
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
          <div className="pagination">
            <span className="quiet" role="status">
              {loading
                ? ""
                : rows.length
                  ? `${rows.length} traces on page ${previous.length + 1}`
                  : ""}
            </span>
            <div>
              <button
                disabled={loading || previous.length === 0}
                onClick={() => {
                  setCursor(previous[previous.length - 1]);
                  setPrevious(previous.slice(0, -1));
                }}
              >
                Previous page
              </button>
              <button
                disabled={loading || !page?.next_cursor}
                onClick={() => {
                  setPrevious([...previous, cursor]);
                  setCursor(page?.next_cursor ?? "");
                }}
              >
                Next page
              </button>
            </div>
          </div>
        </section>
        {selected && (
          <TraceView
            key={selected.trace_id}
            selected={selected}
            token={token}
            close={() => {
              const id = selected.trace_id;
              setSelected(null);
              requestAnimationFrame(() =>
                traceButtons.current.get(id)?.focus(),
              );
            }}
            onUnauthorized={onUnauthorized}
          />
        )}
        {status && <Settings status={status} />}
        <footer>
          Tracelet receives the spans your application emits. It does not
          instrument your code.
        </footer>
      </main>
    </div>
  );
}
