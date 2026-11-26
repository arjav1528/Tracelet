import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { Waterfall } from "./Waterfall";
import { buildTree } from "../features/traces/tree";
import { span } from "../../test/span";

describe("accessible virtual waterfall", () => {
  it("preserves selection through zoom and returns focus to a collapsed parent", async () => {
    const tree = buildTree([
      span("root", "", 0n, 100n),
      span("child", "root", 10n, 50n),
      span("leaf", "child", 20n, 30n),
    ]);
    render(<Waterfall tree={tree} />);
    const root = screen.getByRole("button", { name: "Select span root" });
    root.focus();
    await userEvent.keyboard("{ArrowDown}{Enter}");
    expect(
      screen.getByRole("heading", { name: "Selected span: child" }),
    ).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Zoom in" }));
    expect(
      screen.getByRole("heading", { name: "Selected span: child" }),
    ).toBeTruthy();
    await userEvent.click(
      screen.getByRole("button", { name: "Collapse root" }),
    );
    expect(
      screen.queryByRole("button", { name: "Select span child" }),
    ).toBeNull();
    expect(
      screen.getByRole("heading", { name: "Selected span: root" }),
    ).toBeTruthy();
    expect(document.activeElement).toBe(root);
    await userEvent.keyboard(
      "{ArrowRight}{ArrowDown}{ArrowRight}{ArrowDown}{Enter}",
    );
    expect(
      screen.getByRole("heading", { name: "Selected span: leaf" }),
    ).toBeTruthy();
  });
  it("bounds rendered rows while keeping a 2001-span trace navigable", async () => {
    const input = [
      span("root", "", 0n, 3000n),
      ...Array.from({ length: 2000 }, (_, i) =>
        span(
          `child-${String(i).padStart(4, "0")}`,
          "root",
          BigInt(i),
          BigInt(i + 1),
        ),
      ),
    ];
    render(<Waterfall tree={buildTree(input)} />);
    const grid = screen.getByRole("treegrid", { name: "Trace waterfall" });
    expect(within(grid).getAllByRole("row").length).toBeLessThan(40);
    screen.getByRole("button", { name: "Select span root" }).focus();
    await userEvent.keyboard("{End}{Enter}");
    expect(
      await screen.findByRole("heading", { name: "Selected span: child-1999" }),
    ).toBeTruthy();
    expect(within(grid).getAllByRole("row").length).toBeLessThan(40);
    const viewport = screen.getByTestId("waterfall-scroll");
    fireEvent.scroll(viewport, { target: { scrollTop: 0 } });
    expect(
      screen.getByRole("heading", { name: "Selected span: child-1999" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Select span root" }),
    ).toBeTruthy();
  });
  it("labels orphans, cycles, error states and zero-duration spans", async () => {
    const root = span("root", "", 0n, 100n);
    const orphan = { ...span("orphan", "missing", 10n, 10n), status: 2 };
    render(
      <Waterfall
        tree={buildTree([root, orphan, span("loop", "loop", 20n, 30n)])}
      />,
    );
    expect(screen.getByText(/Missing parent: 1 span/)).toBeTruthy();
    expect(screen.getByText(/Cyclic parent: 1 span/)).toBeTruthy();
    expect(screen.getByText("Orphan")).toBeTruthy();
    expect(screen.getByText("Cycle")).toBeTruthy();
    await userEvent.click(
      screen.getByRole("button", { name: "Select span orphan" }),
    );
    expect(
      screen.getByRole("heading", { name: "Selected span: orphan" }),
    ).toBeTruthy();
    expect(screen.getAllByText("0 ns").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Error").length).toBeGreaterThan(0);
  });
});
