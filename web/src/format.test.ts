import { describe, expect, it } from "vitest";
import { duration, exactUTC, localTime, bytes } from "./format";

describe("precise presentation", () => {
  it("preserves nanoseconds before formatting bounded display values", () => {
    expect(exactUTC("1795679780123456789")).toBe(
      "2026-11-26T07:56:20.123456789Z",
    );
    expect(duration("1")).toBe("1 ns");
    expect(duration("1234567")).toBe("1.234 ms");
    expect(duration("1234567890")).toBe("1.234 s");
    expect(localTime("1795679780123456789")).not.toBe("Unavailable");
  });
  it("handles invalid data without crashing the page", () => {
    expect(exactUTC("bad")).toBe("Unavailable");
    expect(duration("-1")).toBe("Unavailable");
    expect(bytes(1048576)).toBe("1 MiB");
  });
});
