import { describe, expect, it } from "vitest";
import { buildTree, barGeometry, visibleRows, virtualWindow } from "./tree";
import { span } from "../../../test/span";

const epoch = 1795679780123456789n;
describe("honest trace tree", () => {
  it("sorts siblings, retains multiple roots/orphans and never sums overlap", () => {
    const input = [
      span("c", "r", epoch + 10n, epoch + 90n),
      span("orphan", "missing", epoch + 30n, epoch + 40n),
      span("r", "", epoch, epoch + 100n),
      span("b", "r", epoch + 10n, epoch + 80n),
      span("other", "", epoch + 200n, epoch + 210n),
    ];
    const tree = buildTree(input);
    expect(tree.rows.map((r) => r.span.span_id)).toEqual([
      "r",
      "b",
      "c",
      "orphan",
      "other",
    ]);
    expect(tree.duration).toBe(210n);
    expect(tree.rows[3].relationship).toBe("orphan");
    expect(input[0].span_id).toBe("c");
    expect(
      visibleRows(tree, new Set(["r"])).map((r) => r.span.span_id),
    ).toEqual(["r", "orphan", "other"]);
    expect(tree.rows[0].subtreeEnd).toBe(3);
  });
  it("resolves a late parent without mutating identities", () => {
    const child = span("child", "parent", epoch, epoch + 100n);
    expect(buildTree([child]).warnings.map((w) => w.code)).toContain(
      "missing_parent",
    );
    const tree = buildTree([child, span("parent", "", epoch, epoch + 110n)]);
    expect(tree.warnings).toEqual([]);
    expect(tree.rows[1].parentID).toBe("parent");
  });
  it("breaks cycles deterministically and keeps every member", () => {
    const input = [
      span("a", "b", epoch, epoch + 1n),
      span("b", "c", epoch, epoch + 1n),
      span("c", "a", epoch, epoch + 1n),
      span("self", "self", epoch, epoch + 1n),
    ];
    const tree = buildTree(input);
    expect(tree.rows).toHaveLength(4);
    expect(tree.rows[0].parentID).toBe("");
    expect(tree.rows[0].span.parent_span_id).toBe("b");
    expect(tree.warnings.filter((w) => w.code === "cycle")).toHaveLength(4);
    expect(buildTree([...input].reverse()).rows).toEqual(tree.rows);
  });
  it("walks ten thousand levels iteratively", () => {
    const input = Array.from({ length: 10000 }, (_, i) =>
      span(
        String(i).padStart(5, "0"),
        i ? String(i - 1).padStart(5, "0") : "",
        epoch,
        epoch + 1n,
      ),
    );
    const tree = buildTree(input);
    expect(tree.rows[9999].depth).toBe(9999);
    expect(visibleRows(tree, new Set(["00001"]))).toHaveLength(2);
  });
  it("uses precise relative offsets and keeps zero-length bars selectable", () => {
    const tree = buildTree([
      span("r", "", epoch, epoch + 100n),
      span("c", "r", epoch + 1n, epoch + 2n),
      span("zero", "r", epoch + 100n, epoch + 100n),
    ]);
    expect(barGeometry(tree.rows[1], tree)).toEqual({ left: 1, width: 1 });
    expect(barGeometry(tree.rows[2], tree)).toEqual({ left: 100, width: 0 });
    expect(
      barGeometry(
        buildTree([span("z", "", epoch, epoch)]).rows[0],
        buildTree([span("z", "", epoch, epoch)]),
      ),
    ).toEqual({ left: 0, width: 0 });
  });
  it("flags time skew and returns a bounded rendering window", () => {
    const tree = buildTree([
      span("r", "", epoch + 5n, epoch + 10n),
      span("c", "r", epoch, epoch + 20n),
    ]);
    expect(tree.warnings.map((w) => w.code)).toContain("time_skew");
    const window = virtualWindow(36000, 520, 2001);
    expect(window.end - window.start).toBeLessThan(32);
    expect(window.start).toBeLessThanOrEqual(1000);
    expect(window.end).toBeGreaterThan(1000);
  });
});
