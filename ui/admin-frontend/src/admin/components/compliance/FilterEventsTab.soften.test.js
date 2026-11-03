import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import FilterEventsTab from "./FilterEventsTab";
import apiClient from "../../utils/apiClient";

jest.mock("../../utils/apiClient");
jest.mock("../common/MemoizedChart", () => ({ MemoizedLineChart: () => <div /> }));
jest.mock("../../styles/sharedStyles", () => ({
  StyledTableHeaderCell: ({ children, ...props }) => <th {...props}>{children}</th>,
  StyledTableCell: ({ children, ...props }) => <td {...props}>{children}</td>,
  StyledTableRow: ({ children, ...props }) => <tr {...props}>{children}</tr>,
  StyledPaper: ({ children, ...props }) => <div {...props}>{children}</div>,
  PrimaryButton: ({ children, ...props }) => <button {...props}>{children}</button>,
  SecondaryOutlineButton: ({ children, ...props }) => <button {...props}>{children}</button>,
}));
let mockCan = true;
jest.mock("../../context/PermissionsContext", () => ({
  usePermissions: () => ({ isFullAdmin: true, can: () => mockCan, canAny: () => mockCan, canAll: () => mockCan }),
}));

const ev = (id, extra) => ({
  id, timestamp: "2026-11-03T10:30:00Z", app_id: 9, app_name: "Visor", filter_name: "Wolf", filter_scope: "proxy_request",
  event_type: "guardrail.prompt_attack", severity: "critical", description: `blocked ${id}`,
  metadata: { filter_id: 13, detector: "prompt_attack", action: "block", segments: [{ index: 0, role: "user", kind: "text" }] },
  ...extra,
});

const renderTab = () =>
  render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter>
        <FilterEventsTab startDate="2026-11-01" endDate="2026-11-03" />
      </MemoryRouter>
    </ThemeProvider>
  );

// midsommar-clean-9u1j.7: "Not a threat? Soften for this app" on a guardrail
// detector event of an app, from a filter the event names by id, for users
// who can change apps.
describe("FilterEventsTab: soften from an event", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockCan = true;
    apiClient.get.mockImplementation((url) => {
      if (url === "/compliance/events") {
        return Promise.resolve({
          data: {
            events: [
              ev(1),
              ev(2, { app_id: 0, app_name: "" }),
              ev(3, { metadata: { detector: "prompt_attack", roles: ["user"] } }),
              ev(4, { event_type: "guardrail.unscanned" }),
              ev(5, { metadata: { filter_id: 13, detector: "prompt_attack", exceptions: [41], segments: [{ index: 0, role: "user", exception: { id: 41, from: "block", to: "log" } }] } }),
            ],
            total_count: 5, by_severity: {}, by_type: {}, by_filter: {}, timeline: [],
          },
        });
      }
      if (url === "/apps/9/guardrail-capability") return Promise.resolve({ data: { data: { value: "read_only", suggestion: "read_only", reasons: [] } } });
      if (url === "/apps/9/guardrail-exceptions") {
        return Promise.resolve({ data: { data: [], meta: { actions: { text_user: ["log", "quarantine"] }, floors: { read_only: { text_user: "log" } } } } });
      }
      return Promise.resolve({ data: { data: [] } });
    });
  });

  const expandAll = async () => {
    await screen.findByText("blocked 1");
    screen.getAllByTestId("ExpandMoreIcon").forEach((icon) => fireEvent.click(icon.closest("button")));
  };

  test("offers the action only where an exception could apply, and opens the dialog pre-filled", async () => {
    renderTab();
    await expandAll();
    expect(await screen.findByTestId("soften-1")).toBeInTheDocument();
    expect(screen.queryByTestId("soften-2")).not.toBeInTheDocument(); // no app
    expect(screen.queryByTestId("soften-3")).not.toBeInTheDocument(); // no filter_id
    expect(screen.queryByTestId("soften-4")).not.toBeInTheDocument(); // not a detector's verdict

    fireEvent.click(screen.getByTestId("soften-1"));
    expect(await screen.findByTestId("soften-scope")).toHaveTextContent("Wolf flagged prompt_attack");
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/apps/9/guardrail-capability"));
  });

  test("shows which exception softened a segment", async () => {
    renderTab();
    await expandAll();
    expect(await screen.findByTestId("flagged-exception")).toHaveTextContent("Softened by exception #41 (block to log)");
  });

  test("hides the action from users who cannot change apps", async () => {
    mockCan = false;
    renderTab();
    await expandAll();
    await screen.findAllByTestId("flagged-text");
    expect(screen.queryByTestId("soften-1")).not.toBeInTheDocument();
  });
});
