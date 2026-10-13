import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import FilterEventsTab from "./FilterEventsTab";
import apiClient from "../../utils/apiClient";

jest.mock("../../utils/apiClient");

// MemoizedLineChart wraps chart.js which needs canvas; stub it out so we can
// assert the component behaviour without dragging in the chart runtime.
jest.mock("../common/MemoizedChart", () => ({
  MemoizedLineChart: () => <div data-testid="line-chart" />,
}));

jest.mock("../../styles/sharedStyles", () => ({
  StyledTableHeaderCell: ({ children, ...props }) => <th {...props}>{children}</th>,
  StyledTableCell: ({ children, ...props }) => <td {...props}>{children}</td>,
  StyledTableRow: ({ children, ...props }) => <tr {...props}>{children}</tr>,
  StyledPaper: ({ children, ...props }) => <div {...props}>{children}</div>,
}));

const theme = createTheme();

const renderTab = (props = {}) =>
  render(
    <ThemeProvider theme={theme}>
      <MemoryRouter>
        <FilterEventsTab
          startDate="2026-04-22"
          endDate="2026-04-29"
          {...props}
        />
      </MemoryRouter>
    </ThemeProvider>
  );

const buildResponse = (overrides = {}) => ({
  data: {
    events: [],
    total_count: 0,
    by_severity: { critical: 0, warning: 0, info: 0 },
    by_type: {},
    by_filter: {},
    timeline: [],
    ...overrides,
  },
});

describe("FilterEventsTab", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test("does not fetch when no date range is provided", async () => {
    apiClient.get.mockResolvedValue(buildResponse());
    renderTab({ startDate: "", endDate: "" });

    // Settle pending micro-tasks; the effect should have early-returned.
    await waitFor(() => {
      expect(apiClient.get).not.toHaveBeenCalled();
    });
  });

  test("fetches events with the supplied date range and default pagination", async () => {
    apiClient.get.mockResolvedValue(buildResponse());

    renderTab();

    await waitFor(() => {
      expect(apiClient.get).toHaveBeenCalledWith("/compliance/events", {
        params: {
          start_date: "2026-04-22",
          end_date: "2026-04-29",
          limit: 25,
          offset: 0,
        },
      });
    });
  });

  test("renders severity totals from by_severity in the summary cards", async () => {
    apiClient.get.mockResolvedValue(
      buildResponse({
        total_count: 12,
        by_severity: { critical: 3, warning: 7, info: 2 },
      })
    );

    renderTab();

    expect(await screen.findByText("Total Events")).toBeInTheDocument();

    // Locate each card by its label and assert the count above it.
    const cardFor = (label) => screen.getByText(label).closest("div");
    expect(within(cardFor("Total Events")).getByText("12")).toBeInTheDocument();
    expect(within(cardFor("Critical")).getByText("3")).toBeInTheDocument();
    expect(within(cardFor("Warning")).getByText("7")).toBeInTheDocument();
    expect(within(cardFor("Info")).getByText("2")).toBeInTheDocument();
  });

  test("shows empty-state row when no events are returned", async () => {
    apiClient.get.mockResolvedValue(buildResponse());

    renderTab();

    expect(
      await screen.findByText(/no filter events recorded in the selected period/i)
    ).toBeInTheDocument();
  });

  test("renders an event row with description and severity chip", async () => {
    apiClient.get.mockResolvedValue(
      buildResponse({
        events: [
          {
            id: 1,
            timestamp: "2026-04-29T10:30:00Z",
            app_id: 42,
            app_name: "Banking App",
            filter_name: "pii-redactor",
            filter_scope: "proxy_request",
            event_type: "pii_redacted",
            severity: "warning",
            description: "Removed sensitive customer data",
            metadata: {},
          },
        ],
        total_count: 1,
        by_severity: { warning: 1 },
        by_type: { pii_redacted: 1 },
      })
    );

    renderTab();

    expect(await screen.findByText("Banking App")).toBeInTheDocument();
    expect(screen.getByText("pii-redactor")).toBeInTheDocument();
    expect(screen.getByText("proxy_request")).toBeInTheDocument();
    expect(screen.getByText("pii_redacted")).toBeInTheDocument();
    expect(screen.getByText("warning")).toBeInTheDocument();
    expect(
      screen.getByText("Removed sensitive customer data")
    ).toBeInTheDocument();
  });

  test("re-fetches with severity param when severity filter changes", async () => {
    apiClient.get.mockResolvedValue(buildResponse());

    renderTab();

    // Wait for initial fetch.
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledTimes(1));

    // MUI's Select renders a custom popover, not a native <select>.
    // Open it via the combobox and pick the option from the popped listbox.
    fireEvent.mouseDown(screen.getByRole("combobox", { name: /severity/i }));
    const listbox = await screen.findByRole("listbox");
    fireEvent.click(within(listbox).getByText("Critical"));

    await waitFor(() => {
      expect(apiClient.get).toHaveBeenLastCalledWith(
        "/compliance/events",
        expect.objectContaining({
          params: expect.objectContaining({ severity: "critical", offset: 0 }),
        })
      );
    });
  });

  test("populates event-type dropdown options from by_type aggregation", async () => {
    apiClient.get.mockResolvedValue(
      buildResponse({
        by_type: { pii_redacted: 5, content_rewritten: 2 },
      })
    );

    renderTab();

    await screen.findByText("Total Events");

    const eventTypeSelect = screen.getByRole("combobox", { name: /event type/i });
    fireEvent.mouseDown(eventTypeSelect);

    const listbox = await screen.findByRole("listbox");
    expect(within(listbox).getByText(/pii_redacted \(5\)/)).toBeInTheDocument();
    expect(
      within(listbox).getByText(/content_rewritten \(2\)/)
    ).toBeInTheDocument();
  });

  test("expanding a row reveals its metadata JSON", async () => {
    apiClient.get.mockResolvedValue(
      buildResponse({
        events: [
          {
            id: 7,
            timestamp: "2026-04-29T10:30:00Z",
            app_id: 1,
            app_name: "App One",
            filter_name: "pii-redactor",
            filter_scope: "proxy_request",
            event_type: "pii_redacted",
            severity: "warning",
            description: "Removed PII",
            metadata: { redacted_types: ["account_number", "ssn"] },
          },
        ],
        total_count: 1,
      })
    );

    renderTab();

    expect(await screen.findByText("App One")).toBeInTheDocument();

    // Metadata should not be visible until the row is expanded.
    expect(screen.queryByText(/redacted_types/)).not.toBeInTheDocument();

    // The expand toggle is the only button rendered with the ExpandMoreIcon.
    const expandIcon = screen.getByTestId("ExpandMoreIcon");
    fireEvent.click(expandIcon.closest("button"));

    expect(await screen.findByText(/redacted_types/)).toBeInTheDocument();
    expect(screen.getByText(/account_number/)).toBeInTheDocument();
  });
});

// 4uc.8: a guardrail event names where the flagged text came from.
describe("FilterEventsTab: event source", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  const event = (id, metadata) => ({
    id,
    timestamp: "2026-10-13T10:30:00Z",
    app_id: 42,
    app_name: "Agent App",
    filter_name: "Prompt injection heuristics (log)",
    filter_scope: "proxy_request",
    event_type: "guardrail.injection.ignore_instructions",
    severity: "warning",
    description: "guardrail detected",
    metadata,
  });

  test("shows the tool name for a tool-result event, the role for a user message, and - for old events", async () => {
    apiClient.get.mockResolvedValue(
      buildResponse({
        events: [
          event(1, { action: "quarantine", roles: ["tool"], kinds: ["tool_result"], tool_names: ["fetch_url", "read_file"] }),
          event(2, { action: "block", roles: ["user"], kinds: ["text"] }),
          event(3, {}),
        ],
        total_count: 3,
        by_severity: { warning: 3 },
        by_type: { "guardrail.injection.ignore_instructions": 3 },
      })
    );

    renderTab();

    expect(await screen.findByTestId("event-source-1")).toHaveTextContent("Tool result from fetch_url, read_file");
    expect(screen.getByTestId("event-source-2")).toHaveTextContent("User message");
    expect(screen.getByTestId("event-source-3")).toHaveTextContent("-");
    expect(screen.getByText("Source")).toBeInTheDocument();
  });
});
