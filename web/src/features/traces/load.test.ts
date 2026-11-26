import { afterEach, describe, expect, it, vi } from "vitest";
import { loadTrace, TimelineError } from "./load";
import { span } from "../../../test/span";
import { summary } from "../../../test/fixtures";

const id = summary.trace_id;
const child = { ...span("child", "parent", 1n, 3n), trace_id: id };
const parent = { ...span("parent", "", 0n, 4n), trace_id: id };
const detail = {
  summary: { ...summary, span_count: 2 },
  spans: [child],
  warnings: [],
  next_cursor: "next",
};
const last = { trace_id: id, spans: [parent], revision: "1", span_count: 2 };
const json = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
afterEach(() => vi.unstubAllGlobals());
describe("revision-consistent timeline loading", () => {
  it("collects all node pages before classifying missing parents", async () => {
    const fetch = vi.fn(async (path: string) =>
      json(path.includes("/spans?") ? last : detail),
    );
    vi.stubGlobal("fetch", fetch);
    const result = await loadTrace(
      id,
      "a".repeat(64),
      new AbortController().signal,
    );
    expect(result.spans).toHaveLength(2);
    expect(fetch.mock.calls[1][0]).toContain("limit=500&cursor=next");
    expect(result.restarted).toBe(false);
  });
  it("restarts once on changed revisions without mixing old nodes", async () => {
    let starts = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string) => {
        if (path.includes("/spans?")) return json({}, 409);
        starts++;
        return json(
          starts === 1
            ? detail
            : {
                ...detail,
                summary: { ...summary, revision: "2", span_count: 1 },
                spans: [parent],
                next_cursor: "",
              },
        );
      }),
    );
    const result = await loadTrace(
      id,
      "a".repeat(64),
      new AbortController().signal,
    );
    expect(result.spans.map((s) => s.span_id)).toEqual(["parent"]);
    expect(result.summary.revision).toBe("2");
    expect(result.restarted).toBe(true);
  });
  it("bounds retries and rejects excessive or incomplete traces", async () => {
    const fetch = vi.fn(async (path: string) =>
      path.includes("/spans?") ? json({}, 409) : json(detail),
    );
    vi.stubGlobal("fetch", fetch);
    await expect(
      loadTrace(id, "a".repeat(64), new AbortController().signal),
    ).rejects.toMatchObject({ status: 409 });
    expect(fetch).toHaveBeenCalledTimes(4);
    fetch.mockImplementation(async () =>
      json({ ...detail, summary: { ...detail.summary, span_count: 100001 } }),
    );
    await expect(
      loadTrace(id, "a".repeat(64), new AbortController().signal),
    ).rejects.toBeInstanceOf(TimelineError);
    fetch.mockImplementation(async () => json({ ...detail, next_cursor: "" }));
    await expect(
      loadTrace(id, "a".repeat(64), new AbortController().signal),
    ).rejects.toThrow("incomplete");
  });
  it("rejects duplicate identities and non-advancing cursors", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string) =>
        json(
          path.includes("/spans?")
            ? { ...last, spans: [child], next_cursor: "next" }
            : detail,
        ),
      ),
    );
    await expect(
      loadTrace(id, "a".repeat(64), new AbortController().signal),
    ).rejects.toBeInstanceOf(TimelineError);
  });
  it("treats a mismatched successful page revision as changed data", async () => {
    let starts = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string) => {
        if (path.includes("/spans?")) return json({ ...last, revision: "2" });
        starts++;
        return json(
          starts === 1
            ? detail
            : { ...detail, summary: { ...detail.summary, revision: "2" } },
        );
      }),
    );
    const result = await loadTrace(
      id,
      "a".repeat(64),
      new AbortController().signal,
    );
    expect(result.restarted).toBe(true);
    expect(result.summary.revision).toBe("2");
  });
  it("does not retry cancellation or load a partially aborted timeline", async () => {
    const controller = new AbortController();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        controller.abort();
        return json(detail);
      }),
    );
    await expect(
      loadTrace(id, "a".repeat(64), controller.signal),
    ).rejects.toMatchObject({ name: "AbortError" });
    expect(fetch).toHaveBeenCalledTimes(1);
  });
});
