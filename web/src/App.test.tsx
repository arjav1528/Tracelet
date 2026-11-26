import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { page, status, summary } from "../test/fixtures";
import contractStatus from "../../internal/server/testdata/status.json";
import contractTraces from "../../internal/server/testdata/traces.json";

const token = "a".repeat(64);
const json = (value: unknown, code = 200) =>
  new Response(JSON.stringify(value), {
    status: code,
    headers: { "Content-Type": "application/json" },
  });
function mockAPI(overrides: { status?: unknown; page?: unknown } = {}) {
  return vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string) =>
      json(
        path.endsWith("/status")
          ? (overrides.status ?? status)
          : (overrides.page ?? page),
      ),
    ),
  );
}
afterEach(() => vi.unstubAllGlobals());

describe("local trace list", () => {
  it("renders the backend's checked response fixtures", async () => {
    mockAPI({ status: contractStatus, page: contractTraces });
    render(<App token={token} />);
    expect(
      await screen.findByRole("heading", { name: "synthetic-project" }),
    ).toBeTruthy();
    expect(await screen.findByText("GET /synthetic")).toBeTruthy();
    expect(screen.getByText("999.9 µs")).toBeTruthy();
  });
  it("explains project setup without making unauthenticated requests", () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    render(<App token="" />);
    expect(
      screen.getByRole("heading", { name: "Open your local project" }),
    ).toBeTruthy();
    expect(screen.getByText("tracelet ui")).toBeTruthy();
    expect(screen.getByText("tracelet init")).toBeTruthy();
    expect(fetch).not.toHaveBeenCalled();
  });
  it("shows loading while requests are pending", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise(() => {})),
    );
    render(<App token={token} />);
    expect(screen.getByText("Loading traces…")).toBeTruthy();
  });
  it("shows real summaries with precise timing and keyboard selection", async () => {
    mockAPI();
    render(<App token={token} />);
    const row = await screen.findByRole("button", {
      name: "View summary for GET /checkout",
    });
    expect(screen.getByText("123.456 ms")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "checkout-api" })).toBeTruthy();
    expect(screen.getByText("Capturing traces")).toBeTruthy();
    row.focus();
    await userEvent.keyboard("{Enter}");
    expect(
      await screen.findByRole("heading", { name: "Trace summary" }),
    ).toBeTruthy();
    expect(screen.getByText("2026-11-26T07:56:20.123456789Z")).toBeTruthy();
    expect(screen.getByText(summary.trace_id)).toBeTruthy();
    await userEvent.click(
      screen.getByRole("button", { name: "Close summary" }),
    );
    await waitFor(() => expect(document.activeElement).toBe(row));
  });
  it("renders malicious names as plain text", async () => {
    const name = '<img src=x onerror="alert(1)">';
    mockAPI({ page: { ...page, traces: [{ ...summary, operation: name }] } });
    const { container } = render(<App token={token} />);
    expect(await screen.findByText(name)).toBeTruthy();
    expect(container.querySelector("img")).toBeNull();
  });
  it("distinguishes stored history with no telemetry from an empty search", async () => {
    mockAPI({
      status: {
        ...status,
        session: { ...status.session, role: "history" },
        receiver: { ...status.receiver, running: false },
        storage: { ...status.storage, trace_count: 0, span_count: 0 },
      },
      page: { traces: [], revision: "0" },
    });
    render(<App token={token} />);
    expect(
      await screen.findByRole("heading", { name: "No traces stored yet" }),
    ).toBeTruthy();
    expect(screen.getByText("History session")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Search operations"), {
      target: { value: "unmatched" },
    });
    fireEvent.submit(screen.getByRole("search"));
    expect(
      await screen.findByRole("heading", { name: "No matching traces" }),
    ).toBeTruthy();
  });
  it("shows rejected telemetry and effective privacy settings", async () => {
    mockAPI({
      status: {
        ...status,
        receiver: {
          ...status.receiver,
          rejected_spans: 4,
          rejected_requests: 2,
        },
        storage: { ...status.storage, pressure: true },
      },
    });
    render(<App token={token} />);
    expect(
      await screen.findByText(/4 spans and 2 requests rejected/),
    ).toBeTruthy();
    expect(screen.getByText(/Storage is under pressure/)).toBeTruthy();
    await userEvent.click(screen.getByText("Effective settings"));
    expect(screen.getByText("Request and response bodies")).toBeTruthy();
    expect(screen.getByText("Not captured")).toBeTruthy();
  });
  it("reports a disconnected session and safe errors", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("PRIVATE")));
    render(<App token={token} />);
    expect(
      await screen.findByRole("heading", { name: "Local session unavailable" }),
    ).toBeTruthy();
    expect(document.body.textContent).not.toContain("PRIVATE");
    expect(screen.getByRole("button", { name: "Refresh" })).toBeTruthy();
  });
  it("treats rejected authorization as expired session access", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        json({ error: { code: "unauthorized", message: "PRIVATE" } }, 401),
      ),
    );
    render(<App token={token} />);
    expect(
      await screen.findByRole("heading", { name: "Reconnect to this project" }),
    ).toBeTruthy();
    expect(document.body.textContent).not.toContain("PRIVATE");
  });
  it("resets stale paging instead of mixing revisions", async () => {
    const fetch = vi.fn(async (path: string) => {
      if (path.endsWith("/status")) return json(status);
      if (path.includes("cursor="))
        return json({ error: { code: "revision_changed" } }, 409);
      return json({ ...page, next_cursor: "next" });
    });
    vi.stubGlobal("fetch", fetch);
    render(<App token={token} />);
    await screen.findByText(summary.operation);
    await userEvent.click(screen.getByRole("button", { name: "Next page" }));
    expect(
      await screen.findByText(
        "Trace data changed. Showing the latest first page.",
      ),
    ).toBeTruthy();
    await waitFor(() =>
      expect(
        fetch.mock.calls.filter(
          ([path]) => path.includes("/traces?") && !path.includes("cursor="),
        ).length,
      ).toBe(2),
    );
    expect(
      screen
        .getByRole("button", { name: "Previous page" })
        .hasAttribute("disabled"),
    ).toBe(true);
  });
  it("aborts and ignores a superseded search response", async () => {
    let resolveOld!: (response: Response) => void;
    let oldSignal: AbortSignal | undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn((path: string, init: RequestInit) => {
        if (path.endsWith("/status")) return Promise.resolve(json(status));
        if (path.includes("q=new"))
          return Promise.resolve(
            json({
              ...page,
              traces: [{ ...summary, operation: "new result" }],
            }),
          );
        oldSignal = init.signal as AbortSignal;
        return new Promise<Response>((resolve) => {
          resolveOld = resolve;
        });
      }),
    );
    render(<App token={token} />);
    fireEvent.change(screen.getByLabelText("Search operations"), {
      target: { value: "new" },
    });
    fireEvent.submit(screen.getByRole("search"));
    expect(await screen.findByText("new result")).toBeTruthy();
    expect(oldSignal?.aborted).toBe(true);
    await act(async () => resolveOld(json(page)));
    expect(screen.queryByText(summary.operation)).toBeNull();
  });
});
