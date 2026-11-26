import type { SpanSummary, TreeWarning } from "../../types";

export type TreeRow = {
  span: SpanSummary;
  depth: number;
  parentID: string;
  hasChildren: boolean;
  relationship: "root" | "child" | "orphan" | "cycle";
  warnings: string[];
  start: bigint;
  end: bigint;
  index: number;
  subtreeEnd: number;
};
export type Tree = {
  rows: TreeRow[];
  byID: Map<string, TreeRow>;
  warnings: TreeWarning[];
  start: bigint;
  end: bigint;
  duration: bigint;
};

function ns(value: string): bigint | null {
  if (!/^\d{1,19}$/.test(value)) return null;
  const n = BigInt(value);
  return n <= 9_223_372_036_854_775_807n ? n : null;
}

// Call only after all node pages for one trace revision have been collected.
// Parent identities remain unchanged; a separate parentID defines the display tree.
export function buildTree(input: SpanSummary[]): Tree {
  const spans = [...input].sort((a, b) => {
    const startA = ns(a.start_time_unix_nano) ?? 0n;
    const startB = ns(b.start_time_unix_nano) ?? 0n;
    return startA < startB
      ? -1
      : startA > startB
        ? 1
        : a.span_id < b.span_id
          ? -1
          : a.span_id > b.span_id
            ? 1
            : 0;
  });
  const result: Tree = {
    rows: [],
    byID: new Map(),
    warnings: [],
    start: 0n,
    end: 0n,
    duration: 0n,
  };
  const n = spans.length;
  const parent = new Int32Array(n).fill(-1);
  const indices = new Map<string, number>();
  const starts: bigint[] = [];
  const ends: bigint[] = [];
  const valid = new Uint8Array(n);
  const warningCodes: string[][] = Array.from({ length: n }, () => []);
  const warn = (code: string, i: number, message: string) => {
    result.warnings.push({ code, span_id: spans[i].span_id, message });
    warningCodes[i].push(code);
  };
  let hasTime = false;
  let hasRoot = false;
  spans.forEach((span, i) => {
    const key = span.trace_id + "/" + span.span_id;
    if (indices.has(key))
      warn(
        "duplicate_span",
        i,
        "Duplicate span metadata is displayed separately.",
      );
    else indices.set(key, i);
    const start = ns(span.start_time_unix_nano);
    const end = ns(span.end_time_unix_nano);
    starts.push(start ?? 0n);
    ends.push(end ?? start ?? 0n);
    if (start === null || end === null || end < start) {
      warn("invalid_time", i, "Span timing metadata is invalid.");
      return;
    }
    valid[i] = 1;
    if (!hasTime || start < result.start) result.start = start;
    if (!hasTime || end > result.end) result.end = end;
    hasTime = true;
  });
  spans.forEach((span, i) => {
    if (!span.parent_span_id) {
      hasRoot = true;
      return;
    }
    const index = indices.get(span.trace_id + "/" + span.parent_span_id);
    if (index === undefined)
      warn("missing_parent", i, "Parent span was not received.");
    else parent[i] = index;
  });
  if (n && !hasRoot)
    result.warnings.push({
      code: "missing_root",
      message: "No root span was received.",
    });
  const colors = new Uint8Array(n);
  for (let i = 0; i < n; i++) {
    if (colors[i]) continue;
    const chain: number[] = [];
    let current = i;
    while (current >= 0 && colors[current] === 0) {
      colors[current] = 1;
      chain.push(current);
      current = parent[current];
    }
    if (current >= 0 && colors[current] === 1) {
      let cut = current;
      for (let node = parent[current]; node !== current; node = parent[node])
        cut = Math.min(cut, node);
      let node = current;
      do {
        warn(
          "cycle",
          node,
          "Cyclic parent relationship was broken for display.",
        );
        node = parent[node];
      } while (node !== current);
      parent[cut] = -1;
    }
    for (const node of chain) colors[node] = 2;
  }
  const children: number[][] = Array.from({ length: n }, () => []);
  const roots: number[] = [];
  parent.forEach((p, i) => {
    if (p < 0) roots.push(i);
    else {
      children[p].push(i);
      if (valid[i] && valid[p] && (starts[i] < starts[p] || ends[i] > ends[p]))
        warn(
          "time_skew",
          i,
          "Span interval extends beyond its parent interval.",
        );
    }
  });
  const stack = roots.reverse().map((index) => ({ index, depth: 0 }));
  while (stack.length) {
    const current = stack.pop()!;
    const i = current.index;
    const row: TreeRow = {
      span: spans[i],
      depth: current.depth,
      parentID: parent[i] < 0 ? "" : spans[parent[i]].span_id,
      hasChildren: children[i].length > 0,
      relationship: warningCodes[i].includes("cycle")
        ? "cycle"
        : warningCodes[i].includes("missing_parent")
          ? "orphan"
          : spans[i].parent_span_id
            ? "child"
            : "root",
      warnings: warningCodes[i],
      start: starts[i],
      end: ends[i],
      index: result.rows.length,
      subtreeEnd: n,
    };
    result.rows.push(row);
    result.byID.set(row.span.span_id, row);
    for (let j = children[i].length - 1; j >= 0; j--)
      stack.push({ index: children[i][j], depth: current.depth + 1 });
  }
  const open: TreeRow[] = [];
  result.rows.forEach((row) => {
    while (open.length && open[open.length - 1].depth >= row.depth)
      open.pop()!.subtreeEnd = row.index;
    open.push(row);
  });
  result.duration = result.end - result.start;
  return result;
}

export function visibleRows(tree: Tree, collapsed: Set<string>): TreeRow[] {
  const result: TreeRow[] = [];
  let index = 0;
  while (index < tree.rows.length) {
    const row = tree.rows[index];
    result.push(row);
    index = collapsed.has(row.span.span_id) ? row.subtreeEnd : index + 1;
  }
  return result;
}

export function barGeometry(
  row: TreeRow,
  tree: Tree,
): { left: number; width: number } {
  if (tree.duration <= 0n || row.warnings.includes("invalid_time"))
    return { left: 0, width: 0 };
  const percent = (n: bigint) =>
    Number((n * 1_000_000n) / tree.duration) / 10_000;
  return {
    left: Math.max(0, Math.min(100, percent(row.start - tree.start))),
    width: Math.max(0, Math.min(100, percent(row.end - row.start))),
  };
}

export const rowHeight = 36;
export function virtualWindow(
  scrollTop: number,
  height: number,
  count: number,
) {
  const first = Math.min(
    Math.max(0, count - 1),
    Math.floor(Math.max(0, scrollTop) / rowHeight),
  );
  return {
    start: Math.max(0, first - 6),
    end: Math.min(count, first + Math.ceil(height / rowHeight) + 6),
  };
}
