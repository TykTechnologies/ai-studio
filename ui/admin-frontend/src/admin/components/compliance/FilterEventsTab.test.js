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

    // Locate each card by its label and assert the count above it. The
    // labels render before the response arrives (with 0s), so wait for the
    // total; the other counts come from the same response.
    const cardFor = (label) => screen.getByText(label).closest("div");
    expect(await within(cardFor("Total Events")).findByText("12")).toBeInTheDocument();
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
    // "Total Events" renders before the response; the options come from it.
    expect(await within(listbox).findByText(/pii_redacted \(5\)/)).toBeInTheDocument();
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

// midsommar-clean-9u1j.1: events can be narrowed to one application.
describe("FilterEventsTab: application filter", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test("picking an application re-fetches with its app_id; clearing it drops the param", async () => {
    apiClient.get.mockImplementation((url) => {
      if (url === "/apps") {
        return Promise.resolve({
          data: { data: [{ id: "7", attributes: { name: "Visor" } }, { id: "9", attributes: { name: "Support desk" } }] },
        });
      }
      return Promise.resolve(buildResponse());
    });

    renderTab();
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/compliance/events", expect.anything()));
    // The app list is loaded only when the picker opens.
    expect(apiClient.get).not.toHaveBeenCalledWith("/apps", expect.anything());

    const input = screen.getByRole("combobox", { name: /application/i });
    fireEvent.mouseDown(input);
    fireEvent.change(input, { target: { value: "vis" } });
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith("/apps", { params: { page_size: 20, search: "vis" } })
    );
    fireEvent.click(await screen.findByText("Visor"));

    await waitFor(() => {
      expect(apiClient.get).toHaveBeenLastCalledWith(
        "/compliance/events",
        expect.objectContaining({ params: expect.objectContaining({ app_id: 7, offset: 0 }) })
      );
    });

    fireEvent.click(screen.getByLabelText("Clear"));
    await waitFor(() => {
      const last = apiClient.get.mock.calls.filter(([u]) => u === "/compliance/events").pop();
      expect(last[1].params).not.toHaveProperty("app_id");
    });
  });
});

// midsommar-clean-9u1j.13: the judge's agreement per filter, from the
// period's guardrail.judge events, and its opinion on a flagged segment.
describe("FilterEventsTab: second opinion", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  const judgeEvent = (id, filter, judge, requestId = `r${id}`) => ({
    id, timestamp: "2026-11-03T10:30:00Z", app_id: 9, app_name: "Visor", filter_name: filter, filter_scope: "proxy_request",
    event_type: "guardrail.judge", severity: "info", description: "guardrail judge", metadata: { request_id: requestId, judge },
  });

  test("summarises how often the judge found a filter's flags benign", async () => {
    const judges = [
      judgeEvent(1, "Wolf", { verdict: "benign", reason: "code_or_data", coverage: "full", sha256: "aa" }),
      judgeEvent(2, "Wolf", { verdict: "benign", reason: "quoted_example", coverage: "partial", sha256: "bb" }),
      judgeEvent(3, "Wolf", { verdict: "attack", reason: "role_override", coverage: "full", sha256: "cc" }),
      judgeEvent(4, "Wolf", { verdict: "none", error: "timeout" }),
      judgeEvent(5, "PII", { verdict: "suspicious", reason: "unclear", coverage: "full", sha256: "dd" }),
    ];
    const detector = {
      id: 10, timestamp: "2026-11-03T10:30:00Z", app_id: 9, app_name: "Visor", filter_name: "Wolf", filter_scope: "proxy_request",
      event_type: "guardrail.prompt_attack", severity: "critical", description: "blocked",
      metadata: { request_id: "r1", action: "block", segments: [{ index: 0, role: "user", sha256: "aa" }] },
    };
    apiClient.get.mockImplementation((url, { params }) => {
      if (params.event_type === "guardrail.judge") {
        return Promise.resolve({ data: { events: judges } });
      }
      return Promise.resolve(buildResponse({
        events: [detector], total_count: 6,
        by_type: { "guardrail.judge": 5, "guardrail.prompt_attack": 1 },
      }));
    });

    renderTab();

    const summary = await screen.findByTestId("judge-agreement");
    expect(within(summary).getByTestId("judge-agreement-Wolf")).toHaveTextContent(
      "Wolf: judge said benign on 2 of 3 flags (1 not shown in full); no opinion on 1"
    );
    expect(within(summary).getByTestId("judge-agreement-PII")).toHaveTextContent("PII: judge said benign on 0 of 1 flags");
    expect(apiClient.get).toHaveBeenCalledWith("/compliance/events", {
      params: expect.objectContaining({ event_type: "guardrail.judge", limit: 500, offset: 0 }),
    });

    // The detector event's flagged segment shows the judge's opinion.
    fireEvent.click(screen.getAllByTestId("ExpandMoreIcon")[0].closest("button"));
    expect(await screen.findByTestId("flagged-judge")).toHaveTextContent("Judge: benign (code or data)");
  });

  test("asks for no judge events when the period has none", async () => {
    apiClient.get.mockResolvedValue(buildResponse({ by_type: { pii_redacted: 2 } }));
    renderTab();
    await screen.findByText("Total Events");
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledTimes(1));
    expect(screen.queryByTestId("judge-agreement")).not.toBeInTheDocument();
  });

  // A link to one event (/admin/compliance?event=<id>, carried by guardrail
  // alerts and outcome events) shows it open above the table, whatever the
  // period (midsommar-clean-9u1j.67).
  describe("linked event", () => {
    const linked = {
      id: 4242,
      timestamp: "2026-02-01T09:00:00Z",
      app_id: 409,
      app_name: "Visor",
      filter_name: "Wolf Defender: stop conversation",
      filter_scope: "proxy_request",
      event_type: "guardrail.user_message.block",
      severity: "critical",
      description: "guardrail blocked a user message",
      metadata: { action: "block", detector: "user_message.block", roles: ["user"] },
    };
    const mockApi = (single) =>
      apiClient.get.mockImplementation((url) => {
        if (url === "/compliance/events/4242") return single();
        return Promise.resolve(buildResponse());
      });

    test("fetches the event by id and shows it open, with its details", async () => {
      mockApi(() => Promise.resolve({ data: linked }));
      renderTab({ eventId: "4242", onCloseEvent: jest.fn() });

      const panel = await screen.findByTestId("linked-event");
      expect(apiClient.get).toHaveBeenCalledWith("/compliance/events/4242");
      expect(within(panel).getByText("Linked event #4242")).toBeInTheDocument();
      expect(within(panel).getByText("guardrail.user_message.block")).toBeInTheDocument();
      expect(within(panel).getByText("critical")).toBeInTheDocument();
      expect(within(panel).getByText("Visor")).toHaveAttribute("href", "/admin/apps/409");
      expect(within(panel).getByTestId("linked-event-description")).toHaveTextContent("guardrail blocked a user message");
      // Open, with no click: the metadata the row would only show expanded.
      expect(within(panel).getByText(/"detector": "user_message.block"/)).toBeInTheDocument();
      // The table still loads for the period.
      expect(apiClient.get).toHaveBeenCalledWith("/compliance/events", expect.anything());
    });

    test("Show all events drops the link", async () => {
      mockApi(() => Promise.resolve({ data: linked }));
      const onCloseEvent = jest.fn();
      renderTab({ eventId: "4242", onCloseEvent });
      await screen.findByTestId("linked-event");
      fireEvent.click(screen.getByTestId("linked-event-close"));
      expect(onCloseEvent).toHaveBeenCalledTimes(1);
    });

    test("an event that no longer exists says so", async () => {
      mockApi(() => Promise.reject({ response: { status: 404 } }));
      const onCloseEvent = jest.fn();
      renderTab({ eventId: "4242", onCloseEvent });
      const alert = await screen.findByTestId("linked-event-error");
      expect(alert).toHaveTextContent("Compliance event 4242 was not found");
      fireEvent.click(within(alert).getByTestId("linked-event-close"));
      expect(onCloseEvent).toHaveBeenCalledTimes(1);
    });

    test("a permission refusal says so", async () => {
      mockApi(() => Promise.reject({ isPermissionDenied: true, response: { status: 403 } }));
      renderTab({ eventId: "4242", onCloseEvent: jest.fn() });
      expect(await screen.findByTestId("linked-event-error")).toHaveTextContent("You do not have permission to view this compliance event.");
    });

    test("without a link nothing is fetched by id", async () => {
      apiClient.get.mockResolvedValue(buildResponse());
      renderTab();
      await screen.findByText("Total Events");
      await waitFor(() => expect(apiClient.get).toHaveBeenCalled());
      expect(apiClient.get.mock.calls.some(([u]) => /^\/compliance\/events\/\d/.test(u))).toBe(false);
      expect(screen.queryByTestId("linked-event")).not.toBeInTheDocument();
    });
  });
});
