import { useLayoutEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, KeyboardEvent } from "react";
import { duration, exactUTC } from "../format";
import {
  barGeometry,
  rowHeight,
  virtualWindow,
  visibleRows,
} from "../features/traces/tree";
import type { Tree, TreeRow } from "../features/traces/tree";
import "./waterfall.css";

const labels: Record<string, string> = {
  missing_parent: "Missing parent",
  cycle: "Cyclic parent",
  time_skew: "Time skew",
  invalid_time: "Invalid timing",
  duplicate_span: "Duplicate metadata",
};

export function Waterfall({ tree }: { tree: Tree }) {
  const [collapsed, setCollapsed] = useState(new Set<string>());
  const [selectedID, setSelectedID] = useState(
    tree.rows[0]?.span.span_id ?? "",
  );
  const [focusedID, setFocusedID] = useState(tree.rows[0]?.span.span_id ?? "");
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportWidth, setViewportWidth] = useState(900);
  const [zoom, setZoom] = useState(1);
  const viewport = useRef<HTMLDivElement>(null);
  const buttons = useRef(new Map<string, HTMLButtonElement>());
  const pendingFocus = useRef<string | null>(null);
  const visible = useMemo(
    () => visibleRows(tree, collapsed),
    [tree, collapsed],
  );
  const positions = useMemo(
    () => new Map(visible.map((row, index) => [row.span.span_id, index])),
    [visible],
  );
  const selected = tree.byID.get(selectedID);
  const window = virtualWindow(
    Math.max(0, scrollTop - 40),
    520,
    visible.length,
  );
  const rendered = visible.slice(window.start, window.end);
  const focusedInWindow = rendered.some(
    (row) => row.span.span_id === focusedID,
  );
  const trackWidth = Math.max(320, viewportWidth - 384) * zoom;
  const contentWidth = 384 + trackWidth;
  const counts = useMemo(() => {
    const result = new Map<string, number>();
    for (const warning of tree.warnings)
      result.set(warning.code, (result.get(warning.code) ?? 0) + 1);
    return result;
  }, [tree]);

  useLayoutEffect(() => {
    const element = viewport.current;
    if (!element) return;
    setViewportWidth(element.clientWidth || 900);
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(() =>
      setViewportWidth(element.clientWidth || 900),
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  useLayoutEffect(() => {
    if (pendingFocus.current) {
      const button = buttons.current.get(pendingFocus.current);
      if (button) {
        button.focus({ preventScroll: true });
        pendingFocus.current = null;
      }
    }
  });

  function focusRow(id: string) {
    const index = positions.get(id);
    if (index === undefined) return;
    pendingFocus.current = id;
    setFocusedID(id);
    const element = viewport.current;
    if (!element) return;
    const top = index * rowHeight + 40;
    let next = element.scrollTop;
    const height = element.clientHeight || 520;
    if (top < next + 40) next = top - 40;
    else if (top + rowHeight > next + height) next = top + rowHeight - height;
    element.scrollTop = next;
    setScrollTop(next);
  }
  function toggle(row: TreeRow) {
    const closing = !collapsed.has(row.span.span_id);
    const next = new Set(collapsed);
    if (closing) next.add(row.span.span_id);
    else next.delete(row.span.span_id);
    if (
      closing &&
      selected &&
      selected.index > row.index &&
      selected.index < row.subtreeEnd
    )
      setSelectedID(row.span.span_id);
    setCollapsed(next);
    focusRow(row.span.span_id);
  }
  function key(event: KeyboardEvent, row: TreeRow) {
    const index = positions.get(row.span.span_id)!;
    let target: string | undefined;
    switch (event.key) {
      case "ArrowDown":
        target = visible[Math.min(index + 1, visible.length - 1)]?.span.span_id;
        break;
      case "ArrowUp":
        target = visible[Math.max(index - 1, 0)]?.span.span_id;
        break;
      case "Home":
        target = visible[0]?.span.span_id;
        break;
      case "End":
        target = visible[visible.length - 1]?.span.span_id;
        break;
      case "ArrowLeft":
        if (row.hasChildren && !collapsed.has(row.span.span_id)) toggle(row);
        else if (row.parentID) target = row.parentID;
        break;
      case "ArrowRight":
        if (row.hasChildren && collapsed.has(row.span.span_id)) toggle(row);
        else if (row.hasChildren) target = visible[index + 1]?.span.span_id;
        break;
      default:
        return;
    }
    event.preventDefault();
    if (target) focusRow(target);
  }

  return (
    <section className="waterfall" aria-labelledby="waterfall-heading">
      <div className="section-heading">
        <div>
          <h3 id="waterfall-heading">Span waterfall</h3>
          <p className="quiet small">
            {tree.rows.length.toLocaleString()} observed spans ·{" "}
            {duration(String(tree.duration))} total elapsed
          </p>
        </div>
        <div className="zoom-controls" aria-label="Timeline scale">
          <button
            onClick={() => {
              setZoom(1);
              if (viewport.current) viewport.current.scrollLeft = 0;
            }}
          >
            Fit trace
          </button>
          <button
            aria-label="Zoom out"
            disabled={zoom === 1}
            onClick={() => setZoom(Math.max(1, zoom / 2))}
          >
            −
          </button>
          <span aria-live="polite">{zoom}×</span>
          <button
            aria-label="Zoom in"
            disabled={zoom === 16}
            onClick={() => setZoom(Math.min(16, zoom * 2))}
          >
            +
          </button>
        </div>
      </div>
      {counts.size > 0 && (
        <ul className="tree-warnings" aria-label="Trace relationship warnings">
          {[...counts].map(([code, count]) => (
            <li key={code}>
              {code === "missing_root"
                ? "No root span was received."
                : `${labels[code] ?? "Relationship warning"}: ${count} ${count === 1 ? "span" : "spans"}.`}
            </li>
          ))}
        </ul>
      )}
      <p id="waterfall-keys" className="quiet small waterfall-help">
        Arrow keys move through spans. Left/right collapse and expand. Enter
        selects. Drag or scroll horizontally after zooming.
      </p>
      <div
        ref={viewport}
        className="waterfall-scroll"
        data-testid="waterfall-scroll"
        style={{ height: Math.min(520, visible.length * rowHeight + 40) }}
        onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}
      >
        <div
          role="treegrid"
          aria-label="Trace waterfall"
          aria-describedby="waterfall-keys"
          aria-rowcount={visible.length + 1}
          aria-colcount={3}
          style={{ width: contentWidth }}
        >
          <div
            className="waterfall-header"
            role="row"
            aria-rowindex={1}
            style={
              {
                width: contentWidth,
                "--track-width": `${trackWidth}px`,
              } as CSSProperties
            }
          >
            <div role="columnheader">Operation / service</div>
            <div role="columnheader" className="numeric">
              Duration
            </div>
            <div
              className="time-axis"
              role="columnheader"
              aria-label="Elapsed from earliest span"
            >
              {[0, 1, 2, 3, 4].map((tick) => (
                <span key={tick} style={{ left: `${tick * 25}%` }}>
                  {duration(String((tree.duration * BigInt(tick)) / 4n))}
                </span>
              ))}
            </div>
          </div>
          <div
            className="waterfall-canvas"
            role="rowgroup"
            style={{ height: visible.length * rowHeight, width: contentWidth }}
          >
            {rendered.map((row, localIndex) => {
              const id = row.span.span_id;
              const index = window.start + localIndex;
              const geometry = barGeometry(row, tree);
              const error = row.span.status === 2;
              return (
                <div
                  role="row"
                  key={id}
                  aria-rowindex={index + 2}
                  aria-level={row.depth + 1}
                  aria-expanded={
                    row.hasChildren ? !collapsed.has(id) : undefined
                  }
                  aria-selected={id === selectedID}
                  className={`waterfall-row${id === selectedID ? " active" : ""}`}
                  style={
                    {
                      top: index * rowHeight,
                      width: contentWidth,
                      "--track-width": `${trackWidth}px`,
                    } as CSSProperties
                  }
                  onKeyDown={(event) => key(event, row)}
                >
                  <div
                    role="gridcell"
                    className="span-cell"
                    style={{ paddingLeft: 8 + Math.min(row.depth, 8) * 12 }}
                  >
                    {row.hasChildren ? (
                      <button
                        className="tree-toggle"
                        tabIndex={-1}
                        aria-label={`${collapsed.has(id) ? "Expand" : "Collapse"} ${row.span.name}`}
                        onClick={() => toggle(row)}
                      >
                        {collapsed.has(id) ? "▸" : "▾"}
                      </button>
                    ) : (
                      <span className="tree-leaf" aria-hidden="true">
                        ·
                      </span>
                    )}
                    <button
                      ref={(element) => {
                        if (element) buttons.current.set(id, element);
                        else buttons.current.delete(id);
                      }}
                      className="span-select"
                      tabIndex={
                        focusedInWindow
                          ? id === focusedID
                            ? 0
                            : -1
                          : localIndex === 0
                            ? 0
                            : -1
                      }
                      aria-label={`Select span ${row.span.name || "Unnamed operation"}`}
                      onFocus={() => setFocusedID(id)}
                      onClick={() => {
                        setSelectedID(id);
                        setFocusedID(id);
                      }}
                      title={`${row.span.name}\n${row.span.service_name || "Service not provided"}\nDepth ${row.depth}`}
                    >
                      <span>{row.span.name || "Unnamed operation"}</span>
                      <small>
                        {row.span.service_name || "Service not provided"}
                        {row.depth > 8 ? ` · depth ${row.depth}` : ""}
                      </small>
                    </button>
                    {row.relationship !== "child" && (
                      <span className={`relationship ${row.relationship}`}>
                        {row.relationship === "root"
                          ? "Root"
                          : row.relationship === "orphan"
                            ? "Orphan"
                            : "Cycle"}
                      </span>
                    )}
                    {error && <span className="span-error">Error</span>}
                  </div>
                  <div role="gridcell" className="numeric span-duration">
                    {duration(row.span.duration_nano)}
                  </div>
                  <div role="gridcell" className="span-track">
                    <span className="sr-only">
                      Starts {duration(String(row.start - tree.start))} after
                      the earliest span.
                    </span>
                    <button
                      className={`span-bar${error ? " error" : ""}${row.warnings.length ? " has-warning" : ""}`}
                      aria-label={`Select timing bar for ${row.span.name}`}
                      tabIndex={-1}
                      title={`${row.span.name}: ${duration(row.span.duration_nano)}`}
                      style={{
                        left: `min(${geometry.left}%, calc(100% - 2px))`,
                        width: `${geometry.width}%`,
                        minWidth: 2,
                      }}
                      onClick={() => {
                        setSelectedID(id);
                        focusRow(id);
                      }}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </div>
      <p className="quiet small waterfall-count">
        {visible.length.toLocaleString()} visible /{" "}
        {tree.rows.length.toLocaleString()} observed spans. Only rows in view
        are drawn.
      </p>
      {selected && (
        <section
          className="span-summary"
          aria-labelledby="span-summary-heading"
        >
          <h3 id="span-summary-heading">
            Selected span: {selected.span.name || "Unnamed operation"}
          </h3>
          <dl className="selection-grid">
            <div>
              <dt>Span ID</dt>
              <dd className="trace-id">{selected.span.span_id}</dd>
            </div>
            <div>
              <dt>Parent span ID</dt>
              <dd className="trace-id">
                {selected.span.parent_span_id || "None (root span)"}
              </dd>
            </div>
            <div>
              <dt>Service</dt>
              <dd>{selected.span.service_name || "Not provided"}</dd>
            </div>
            <div>
              <dt>Status</dt>
              <dd>
                {selected.span.status === 2
                  ? "Error"
                  : selected.span.status === 1
                    ? "OK"
                    : "Unset"}
              </dd>
            </div>
            <div>
              <dt>Start in UTC</dt>
              <dd>{exactUTC(selected.span.start_time_unix_nano)}</dd>
            </div>
            <div>
              <dt>Duration</dt>
              <dd>{duration(selected.span.duration_nano)}</dd>
            </div>
          </dl>
          {selected.warnings.length > 0 && (
            <p className="notice warning">
              {selected.warnings
                .map((code) => labels[code] ?? "Relationship warning")
                .join(". ")}
              . Stored parent identity is preserved; the displayed hierarchy may
              differ.
            </p>
          )}
        </section>
      )}
    </section>
  );
}
