import React from "react";
import { render, screen, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import PolicyViolationsTab, { actionBreakdown } from "./PolicyViolationsTab";

let mockChart = null;
jest.mock("../common/MemoizedChart", () => ({
  MemoizedLineChart: (props) => {
    mockChart = props.data;
    return <div data-testid="chart" />;
  },
}));
jest.mock("../../utils/apiClient", () => ({ __esModule: true, default: { get: jest.fn() } }));

// The rc6 sandbox's numbers (o4a.4): no blocked request, 106 events (58
// warning, 48 critical), all quarantines, removals and logs.
const sandbox = {
  total_blocks: 0,
  total_flagged: 106,
  events_by_action: { quarantine: 70, remove: 30, log: 6 },
  filter_blocks: [
    { app_id: 1, app_name: "Blog Sandbox App", violation_types: ["guardrail.injection.instruction_override"], count: 64, blocks: 0, events: 64, events_by_action: { quarantine: 60, log: 4 }, last_occurred: "2026-11-26T10:00:00Z" },
    { app_id: 5, app_name: "Agent", violation_types: ["guardrail.secrets.aws_access_key"], count: 42, blocks: 0, events: 42, events_by_action: { quarantine: 10, remove: 30, log: 2 }, last_occurred: "2026-11-26T11:00:00Z" },
  ],
  timeline: [{ date: "2026-11-25", count: 28 }, { date: "2026-11-26", count: 78 }],
  timeline_blocks: [{ date: "2026-11-25", count: 0 }, { date: "2026-11-26", count: 0 }],
  timeline_events: [{ date: "2026-11-25", count: 28 }, { date: "2026-11-26", count: 78 }],
};

const renderTab = (data) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PolicyViolationsTab data={data} startDate="2026-11-20" endDate="2026-11-27" />
      </MemoryRouter>
    </ThemeProvider>
  );

describe("PolicyViolationsTab", () => {
  it("never counts quarantines and removals as blocked requests", () => {
    renderTab(sandbox);
    const blocked = screen.getByText("Blocked Requests").parentElement;
    expect(within(blocked).getByText("0")).toBeInTheDocument();
    const events = screen.getByText("Filter and guardrail events").parentElement;
    expect(within(events).getByText("106")).toBeInTheDocument();
    expect(screen.getByTestId("events-by-action")).toHaveTextContent("70 quarantined · 30 removed · 6 logged");
    expect(screen.queryByText("Flagged Requests")).not.toBeInTheDocument();

    expect(screen.getByText("Blocks and filter events over time")).toBeInTheDocument();
    expect(mockChart.datasets.map((d) => [d.label, d.data])).toEqual([
      ["Blocked requests", [0, 0]],
      ["Filter and guardrail events", [28, 78]],
    ]);

    expect(screen.getByText("Blocks and filter events by application")).toBeInTheDocument();
    const row = screen.getByText("Agent").closest("tr");
    const cells = within(row).getAllByRole("cell");
    expect(cells[3]).toHaveTextContent("0");
    expect(cells[4]).toHaveTextContent("42");
    expect(cells[4]).toHaveTextContent("10 quarantined · 30 removed · 2 logged");
  });

  it("still renders an older server's data without the split", () => {
    const { timeline_blocks, timeline_events, events_by_action, ...old } = sandbox;
    renderTab({ ...old, filter_blocks: [{ app_id: 1, app_name: "A", violation_types: [], count: 9, last_occurred: "2026-11-26T10:00:00Z" }] });
    expect(mockChart.datasets).toHaveLength(1);
    const cells = within(screen.getByText("A").closest("tr")).getAllByRole("cell");
    expect(cells[3]).toHaveTextContent("-");
    expect(cells[4]).toHaveTextContent("9");
  });

  it("orders the breakdown by action and leaves out zeros", () => {
    expect(actionBreakdown({ other: 1, block: 2, quarantine: 0, redact: 3 })).toBe("2 blocked · 3 redacted · 1 other");
    expect(actionBreakdown(undefined)).toBe("");
    // An action this page does not know yet is shown by name, after the known ones.
    expect(actionBreakdown({ throttle: 4, log: 1, alpha: 2 })).toBe("1 logged · 2 alpha · 4 throttle");
  });
});
