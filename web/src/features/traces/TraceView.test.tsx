import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TraceView } from "./TraceView";
import { summary } from "../../../test/fixtures";
import { span } from "../../../test/span";

afterEach(() => vi.unstubAllGlobals());
describe("trace inspection", () => {
  it("does not classify an unloaded parent as orphaned", async () => {
    let finish!: (response: Response) => void;
    const child = {
      ...span("child", "root", 1n, 2n),
      trace_id: summary.trace_id,
    };
    const root = { ...span("root", "", 0n, 3n), trace_id: summary.trace_id };
    const json = (value: unknown) =>
      new Response(JSON.stringify(value), {
        headers: { "Content-Type": "application/json" },
      });
    vi.stubGlobal(
      "fetch",
      vi.fn((path: string) =>
        path.includes("/spans?")
          ? new Promise<Response>((resolve) => {
              finish = resolve;
            })
          : Promise.resolve(
              json({
                summary: { ...summary, span_count: 2 },
                spans: [child],
                warnings: [],
                next_cursor: "next",
              }),
            ),
      ),
    );
    render(
      <TraceView
        selected={summary}
        token={"a".repeat(64)}
        close={() => {}}
        onUnauthorized={() => {}}
      />,
    );
    await waitFor(() => expect(finish).toBeTruthy());
    expect(screen.getByText(/Loading timeline: 1 of 2 spans/)).toBeTruthy();
    expect(screen.queryByText(/Missing parent/)).toBeNull();
    finish(
      json({
        trace_id: summary.trace_id,
        spans: [root],
        revision: "1",
        span_count: 2,
      }),
    );
    expect(
      await screen.findByRole("treegrid", { name: "Trace waterfall" }),
    ).toBeTruthy();
    expect(screen.queryByText(/Missing parent/)).toBeNull();
  });
  it("explains retention removal without inventing an empty trace", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("", { status: 404 })),
    );
    render(
      <TraceView
        selected={summary}
        token={"a".repeat(64)}
        close={() => {}}
        onUnauthorized={() => {}}
      />,
    );
    expect(
      await screen.findByText(
        "This trace is no longer available. It may have been removed by retention. Refresh the trace list.",
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("treegrid")).toBeNull();
  });
});
