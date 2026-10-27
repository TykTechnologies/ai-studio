import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import { SessionDetailProvider, useSessionDetail } from "./SessionDetailContext";
import TimelineStrip, { dragRange } from "./TimelineStrip";
import { T0, detail, spans } from "./testFixture";

// The fixture session runs 6 s; the track is mocked 600 px wide, so 100 px
// is 1 s.
let ctx;
const Probe = () => {
  ctx = useSessionDetail();
  return null;
};
const renderStrip = () => {
  render(
    <ThemeProvider theme={testTheme}>
      <SessionDetailProvider detail={detail()} spans={spans} resources={[]}>
        <TimelineStrip />
        <Probe />
      </SessionDetailProvider>
    </ThemeProvider>,
  );
  screen.getByTestId("timeline-strip-track").getBoundingClientRect = () => ({ left: 0, top: 0, width: 600, height: 40, right: 600, bottom: 40 });
};
const drag = (el, from, to) => {
  fireEvent.mouseDown(el, { button: 0, clientX: from });
  fireEvent.mouseMove(window, { clientX: to });
  fireEvent.mouseUp(window, { clientX: to });
};

describe("dragRange", () => {
  const bounds = { from: 0, to: 1000 };
  const range = { from: 200, to: 400 };
  it("draws, resizes and moves a range inside the bounds", () => {
    expect(dragRange("new", { t0: 500, t: 300 }, bounds, 1)).toEqual({ from: 300, to: 500 });
    expect(dragRange("start", { range, t: 100 }, bounds, 1)).toEqual({ from: 100, to: 400 });
    // An edge never crosses the other one.
    expect(dragRange("start", { range, t: 900 }, bounds, 10)).toEqual({ from: 390, to: 400 });
    expect(dragRange("end", { range, t: 5000 }, bounds, 1)).toEqual({ from: 200, to: 1000 });
    expect(dragRange("move", { range, t0: 300, t: 1300 }, bounds, 1)).toEqual({ from: 800, to: 1000 });
  });
});

describe("TimelineStrip", () => {
  it("keeps a new range when the drag leaves the strip, then resizes and moves it by its handles and body", () => {
    renderStrip();
    // Released well outside the 600 px track: clamped to the session's end.
    drag(screen.getByTestId("timeline-strip-track"), 100, 900);
    expect(ctx.range).toEqual({ from: T0 + 1000, to: T0 + 6000 });

    drag(screen.getByTestId("range-handle-start"), 100, 300);
    expect(ctx.range).toEqual({ from: T0 + 3000, to: T0 + 6000 });

    drag(screen.getByTestId("range-handle-end"), 600, 500);
    expect(ctx.range).toEqual({ from: T0 + 3000, to: T0 + 5000 });

    drag(screen.getByTestId("range-rect"), 400, 200);
    expect(ctx.range).toEqual({ from: T0 + 1000, to: T0 + 3000 });
  });

  it("nudges an edge with the arrow keys", () => {
    renderStrip();
    drag(screen.getByTestId("timeline-strip-track"), 100, 300);
    const end = screen.getByRole("slider", { name: "Range end" });
    fireEvent.keyDown(end, { key: "ArrowRight", shiftKey: true });
    // Shift moves by 10% of the 6 s session.
    expect(ctx.range.to).toBeCloseTo(T0 + 3600, 0);
    fireEvent.keyDown(screen.getByRole("slider", { name: "Range start" }), { key: "ArrowLeft" });
    expect(ctx.range.from).toBeCloseTo(T0 + 940, 0);
  });
});
