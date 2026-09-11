import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import WebhookDeliveries from "./WebhookDeliveries";
import apiClient from "../utils/apiClient";

jest.mock("../utils/apiClient");

jest.mock("../styles/sharedStyles", () => ({
  TitleBox: ({ children }) => <div>{children}</div>,
  StyledPaper: ({ children, sx, ...props }) => <div {...props}>{children}</div>,
  StyledTableHeaderCell: ({ children, sx, ...props }) => <th {...props}>{children}</th>,
  StyledTableCell: ({ children, sx, ...props }) => <td {...props}>{children}</td>,
  StyledTableRow: ({ children, hover, sx, ...props }) => <tr {...props}>{children}</tr>,
}));

jest.mock("../components/common/DateRangePicker", () => () => <div data-testid="date-range-picker" />);

const theme = createTheme();

const renderPage = () =>
  render(
    <ThemeProvider theme={theme}>
      <MemoryRouter>
        <WebhookDeliveries />
      </MemoryRouter>
    </ThemeProvider>
  );

const healthyStatus = {
  available: true,
  enabled: true,
  bus_connected: true,
  worker_enabled: true,
  queue_depth: 1,
  retrying: 0,
  in_flight: 0,
  dead_lettered: 1,
  dropped_events: 0,
};

const deliveries = [
  {
    id: "d-ok",
    event_id: "evt-1",
    target_id: "t-1",
    topic: "system.llm.created",
    target_url: "https://siem.example.com/ingest",
    status: "succeeded",
    attempt_count: 1,
    max_attempts: 10,
    last_status_code: 200,
    last_error: "",
    kind: "event",
    created_at: "2026-09-10T10:00:00Z",
  },
  {
    id: "d-dead",
    event_id: "evt-2",
    target_id: "t-1",
    topic: "system.app.created",
    target_url: "https://siem.example.com/ingest",
    status: "dead_lettered",
    attempt_count: 10,
    max_attempts: 10,
    last_status_code: 503,
    last_error: "HTTP 503 (gave up after 10 attempts)",
    kind: "event",
    created_at: "2026-09-10T11:00:00Z",
  },
];

const stats = {
  window: "24h",
  by_status: { succeeded: 5, dead_lettered: 1, in_flight: 0 },
  by_target: [],
  queue_depth: 1,
};

const mockApi = ({ status = healthyStatus } = {}) => {
  apiClient.get.mockImplementation((url) => {
    if (url === "/webhooks/status") return Promise.resolve({ data: status });
    if (url === "/webhooks/targets") return Promise.resolve({ data: { targets: [{ id: "t-1", name: "SIEM" }], pending_count: 0 } });
    if (url === "/webhooks/deliveries") return Promise.resolve({ data: { deliveries, total: 2, page: 1, page_size: 25 } });
    if (url === "/webhooks/stats") return Promise.resolve({ data: stats });
    if (url === "/webhooks/deliveries/d-dead") {
      return Promise.resolve({
        data: {
          delivery: { ...deliveries[1], rendered_payload: '{"id":"evt-2"}' },
          attempts: [
            { id: 1, attempt: 1, started_at: "2026-09-10T11:00:00Z", outcome: "retry", status_code: 503, duration_ms: 12, error: "HTTP 503", response_snippet: "busy" },
            { id: 2, attempt: 10, started_at: "2026-09-10T12:00:00Z", outcome: "dead_letter", status_code: 503, duration_ms: 9, error: "HTTP 503 (gave up after 10 attempts)" },
          ],
          event: { id: "evt-2", topic: "system.app.created", payload: { object: { name: "app", api_key: "[REDACTED]" } } },
        },
      });
    }
    return Promise.reject(new Error(`unexpected GET ${url}`));
  });
  apiClient.post.mockImplementation((url) => {
    if (url === "/webhooks/deliveries/d-dead/replay") return Promise.resolve({ data: { delivery_id: "d-replay" } });
    if (url === "/webhooks/deliveries/replay") return Promise.resolve({ data: { replayed: 1, skipped: 0 } });
    return Promise.reject(new Error(`unexpected POST ${url}`));
  });
};

beforeEach(() => {
  jest.clearAllMocks();
});

describe("WebhookDeliveries", () => {
  it("shows the enterprise upsell when unavailable", async () => {
    mockApi({ status: { available: false } });
    renderPage();
    expect(await screen.findByText("Enterprise Feature")).toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/webhooks/deliveries", expect.anything());
  });

  it("lists deliveries with the default window and summary tiles", async () => {
    mockApi();
    renderPage();
    expect(await screen.findByText("system.llm.created")).toBeInTheDocument();
    expect(screen.getByText("system.app.created")).toBeInTheDocument();
    expect(screen.getByTestId("status-succeeded")).toBeInTheDocument();
    expect(screen.getByTestId("status-dead_lettered")).toBeInTheDocument();
    expect(screen.getAllByText("SIEM").length).toBeGreaterThan(0);
    expect(screen.getByText("Succeeded (24h)")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();

    const call = apiClient.get.mock.calls.find((c) => c[0] === "/webhooks/deliveries");
    expect(call[1].params).toEqual(
      expect.objectContaining({
        start_date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/),
        end_date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/),
        page: 1,
        page_size: 25,
      })
    );
    expect(call[1].params.status).toBeUndefined();
  });

  it("expands a delivery, loads attempts and payload, and replays it", async () => {
    mockApi();
    renderPage();
    await screen.findByText("system.app.created");
    fireEvent.click(screen.getByTestId("delivery-row-d-dead"));

    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/webhooks/deliveries/d-dead"));
    expect(await screen.findByTestId("delivery-attempts")).toBeInTheDocument();
    expect(screen.getByText("busy")).toBeInTheDocument();
    expect(screen.getByTestId("rendered-payload")).toHaveTextContent("evt-2");
    expect(screen.getByText(/\[REDACTED\]/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Replay" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/webhooks/deliveries/d-dead/replay", {}));
    expect(await screen.findByText(/Replay queued as delivery d-replay/)).toBeInTheDocument();
  });

  it("applies filters and the dead-letter shortcut with bulk replay", async () => {
    mockApi();
    renderPage();
    await screen.findByText("system.llm.created");

    fireEvent.change(screen.getByLabelText("Topic"), { target: { value: "system.llm.created" } });
    fireEvent.change(screen.getByLabelText("Search"), { target: { value: "siem" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply filters" }));
    await waitFor(() => {
      const calls = apiClient.get.mock.calls.filter((c) => c[0] === "/webhooks/deliveries");
      expect(calls[calls.length - 1][1].params).toEqual(
        expect.objectContaining({ topic: "system.llm.created", search: "siem", page: 1 })
      );
    });

    // Buttons are disabled while a fetch is in flight; wait for it to settle.
    await waitFor(() => expect(screen.getByRole("button", { name: "Dead letters" })).not.toBeDisabled());
    fireEvent.click(screen.getByRole("button", { name: "Dead letters" }));
    await waitFor(() => {
      const calls = apiClient.get.mock.calls.filter((c) => c[0] === "/webhooks/deliveries");
      expect(calls[calls.length - 1][1].params).toEqual(expect.objectContaining({ status: "dead_lettered" }));
    });
    await waitFor(() => expect(screen.getByRole("button", { name: /Replay all dead letters/ })).not.toBeDisabled());
    fireEvent.click(screen.getByRole("button", { name: /Replay all dead letters/ }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/webhooks/deliveries/replay", { max: 500 }));
    expect(await screen.findByText(/Replayed 1 dead letter/)).toBeInTheDocument();
  });

  it("warns about dropped events", async () => {
    mockApi({ status: { ...healthyStatus, dropped_events: 4 } });
    renderPage();
    expect(await screen.findByText(/4 event\(s\) could not be persisted/)).toBeInTheDocument();
  });
});
