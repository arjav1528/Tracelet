import { afterEach, describe, expect, it, vi } from "vitest";
import { readToken, apiGet, APIError } from "./api";

afterEach(() => vi.unstubAllGlobals());

describe("private session access", () => {
  it("removes the fragment and remembers its token in this tab only", () => {
    const token = "a".repeat(64);
    history.replaceState(null, "", "/#token=" + token);
    expect(readToken()).toBe(token);
    expect(location.hash).toBe("");
    expect(readToken()).toBe(token);
    expect(localStorage.length).toBe(0);
  });
  it("rejects malformed and ambiguous tokens, including an old stored token", () => {
    sessionStorage.setItem("tracelet.token", "b".repeat(64));
    history.replaceState(null, "", "/#token=bad");
    expect(readToken()).toBe("");
    expect(location.hash).toBe("");
    history.replaceState(
      null,
      "",
      "/#token=" + "a".repeat(64) + "&token=" + "b".repeat(64),
    );
    expect(readToken()).toBe("");
  });
  it("authenticates bounded same-origin requests without caching", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response('{"revision":"1"}', {
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetch);
    const controller = new AbortController();
    expect(
      await apiGet("/api/v1/traces", "a".repeat(64), controller.signal),
    ).toEqual({ revision: "1" });
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/traces",
      expect.objectContaining({
        cache: "no-store",
        credentials: "same-origin",
        redirect: "error",
        signal: controller.signal,
        headers: expect.objectContaining({
          "X-Tracelet-Token": "a".repeat(64),
        }),
      }),
    );
    await expect(
      apiGet("https://elsewhere.invalid", "a".repeat(64), controller.signal),
    ).rejects.toThrow();
  });
  it("never displays untrusted backend error text", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response(
            '{"error":{"code":"revision_changed","message":"PRIVATE"}}',
            { status: 409, headers: { "Content-Type": "application/json" } },
          ),
        ),
    );
    try {
      await apiGet(
        "/api/v1/traces",
        "a".repeat(64),
        new AbortController().signal,
      );
      throw new Error("expected rejection");
    } catch (error) {
      expect(error).toBeInstanceOf(APIError);
      expect((error as APIError).status).toBe(409);
      expect(String(error)).not.toContain("PRIVATE");
    }
  });
});
