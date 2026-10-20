import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import apiClient from "../../utils/apiClient";
import MCPServerUsage from "./MCPServerUsage";
import AppMCPUsage from "./AppMCPUsage";
import MCPTrafficPanel from "./MCPTrafficPanel";

jest.mock("react-chartjs-2", () => ({ Line: () => <div data-testid="chart" /> }));
jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));

const wrap = (ui) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>{ui}</MemoryRouter>
    </ThemeProvider>,
  );

const summary = { calls: 1200, errors: 60, error_rate: 0.05, p50_ms: 40, p95_ms: 310, last_seen_at: "2026-10-20T10:00:00Z" };
const routes = (overrides = {}) => (path) => {
  const table = {
    "/analytics/mcp/servers/4/summary": { data: summary },
    "/analytics/mcp/servers/4/timeseries": { data: { points: [{ bucket: "2026-10-19T00:00:00Z", calls: 1200, errors: 60 }] } },
    "/analytics/mcp/servers/4/tools": {
      data: { data: [{ primitive_type: "tool", primitive_name: "get_forecast", calls: 1000, errors: 50, avg_ms: 50, p95_ms: 300 }], total: 1 },
    },
    "/analytics/mcp/servers/4/apps": {
      data: { data: [{ app_id: 3, app_name: "Weather bot", calls: 900 }, { app_id: null, calls: 300 }], total: 2 },
    },
    "/analytics/mcp/overview": {
      data: { calls: 50, errors: 5, top_tools: [{ mcp_server_id: 4, server_name: "Weather", primitive_name: "get_forecast", calls: 40, errors: 2 }] },
    },
    ...overrides,
  };
  if (table[path] instanceof Error) return Promise.reject(table[path]);
  return table[path] ? Promise.resolve(table[path]) : Promise.reject(new Error("unexpected " + path));
};

describe("MCPServerUsage", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockImplementation(routes());
  });

  it("renders the summary, the per-tool table and the top Apps", async () => {
    wrap(<MCPServerUsage serverId={4} connectionId={7} />);
    expect(await screen.findByTestId("mcp-usage-calls")).toHaveTextContent("1,200");
    expect(screen.getByTestId("mcp-usage-error-rate")).toHaveTextContent("5.0%");
    expect(screen.getByTestId("mcp-usage-p95")).toHaveTextContent("310 ms");
    expect(await screen.findByText("get_forecast")).toBeInTheDocument();
    expect(await screen.findByText("Weather bot")).toHaveAttribute("href", "/admin/apps/3");
    expect(screen.getByText("Unattributed")).toBeInTheDocument();
    expect(apiClient.get).toHaveBeenCalledWith(
      "/analytics/mcp/servers/4/tools",
      expect.objectContaining({ params: expect.objectContaining({ page: 1, page_size: 10 }) }),
    );
  });

  it("shows the Pump setup empty state linking to the connection's Analytics section", async () => {
    apiClient.get.mockImplementation(routes({ "/analytics/mcp/servers/4/summary": { data: { calls: 0 } } }));
    wrap(<MCPServerUsage serverId={4} connectionId={7} />);
    expect(await screen.findByTestId("mcp-analytics-empty")).toBeInTheDocument();
    expect(screen.getByTestId("mcp-analytics-setup-link")).toHaveAttribute("href", "/admin/tyk-connections/edit/7#analytics");
  });

  it("shows an error when the summary fails", async () => {
    apiClient.get.mockImplementation(
      routes({ "/analytics/mcp/servers/4/summary": Object.assign(new Error("x"), { response: { data: { errors: [{ detail: "denied" }] } } }) }),
    );
    wrap(<MCPServerUsage serverId={4} connectionId={7} />);
    expect(await screen.findByTestId("mcp-usage-error")).toHaveTextContent("denied");
  });

  it("refetches when the range changes", async () => {
    wrap(<MCPServerUsage serverId={4} connectionId={7} />);
    await screen.findByTestId("mcp-usage-calls");
    fireEvent.change(screen.getByLabelText("Start Date"), { target: { value: "2026-10-19" } });
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith(
        "/analytics/mcp/servers/4/timeseries",
        expect.objectContaining({ params: expect.objectContaining({ start_date: "2026-10-19", interval: expect.any(String) }) }),
      ),
    );
  });
});

describe("AppMCPUsage", () => {
  const client = { get: jest.fn() };
  beforeEach(() => {
    client.get.mockReset();
  });

  it("renders a row per bound server, linking to the server in admin", async () => {
    client.get.mockResolvedValue({ data: { servers: [{ mcp_server_id: 4, server_name: "Weather", calls: 10, errors: 1, p95_ms: 90 }] } });
    wrap(<AppMCPUsage client={client} path="/analytics/mcp/apps/3" startDate="a" endDate="b" admin />);
    expect(await screen.findByText("Weather")).toHaveAttribute("href", "/admin/mcp-servers/4");
    expect(screen.getByTestId("app-mcp-usage-row-4")).toHaveTextContent("10.0%");
  });

  it("renders no links and no setup link in the portal", async () => {
    client.get.mockResolvedValueOnce({ data: { servers: [{ mcp_server_id: 4, server_name: "Weather", calls: 10 }] } });
    const { unmount } = wrap(<AppMCPUsage client={client} path="/common/apps/3/analytics/mcp" startDate="a" endDate="b" />);
    expect(await screen.findByText("Weather")).not.toHaveAttribute("href");
    expect(client.get).toHaveBeenCalledWith("/common/apps/3/analytics/mcp", { params: { start_date: "a", end_date: "b" } });
    unmount();

    client.get.mockResolvedValueOnce({ data: { servers: [{ mcp_server_id: 4, server_name: "Weather", calls: 0 }] } });
    wrap(<AppMCPUsage client={client} path="/common/apps/3/analytics/mcp" startDate="a" endDate="b" />);
    expect(await screen.findByTestId("mcp-analytics-empty")).toBeInTheDocument();
    expect(screen.queryByTestId("mcp-analytics-setup-link")).not.toBeInTheDocument();
  });

  it("renders nothing for an App without MCP servers or when the route is missing", async () => {
    client.get.mockResolvedValueOnce({ data: { servers: [] } });
    const { container, unmount } = wrap(<AppMCPUsage client={client} path="/p" startDate="a" endDate="b" />);
    await waitFor(() => expect(container).toBeEmptyDOMElement());
    unmount();

    client.get.mockRejectedValueOnce({ response: { status: 404 } });
    const second = wrap(<AppMCPUsage client={client} path="/p" startDate="a" endDate="b" />);
    await waitFor(() => expect(second.container).toBeEmptyDOMElement());
  });

  it("shows the Pump empty state for an App with bound but silent MCP servers", async () => {
    client.get.mockResolvedValueOnce({ data: { servers: [] } });
    wrap(
      <AppMCPUsage client={client} path="/p" startDate="a" endDate="b" boundServers={[{ id: 4, connection_id: 7 }]} admin />,
    );
    expect(await screen.findByTestId("mcp-analytics-empty")).toBeInTheDocument();
    expect(screen.getByTestId("mcp-analytics-setup-link")).toHaveAttribute("href", "/admin/tyk-connections/edit/7#analytics");
  });

  it("shows other errors", async () => {
    client.get.mockRejectedValueOnce({ response: { status: 500, data: { errors: [{ detail: "boom" }] } } });
    wrap(<AppMCPUsage client={client} path="/p" startDate="a" endDate="b" admin />);
    expect(await screen.findByTestId("app-mcp-usage-error")).toHaveTextContent("boom");
  });
});

describe("MCPTrafficPanel", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockImplementation(routes());
  });

  it("renders totals and top tools", async () => {
    wrap(<MCPTrafficPanel startDate="2026-10-01" endDate="2026-10-20" />);
    expect(await screen.findByTestId("mcp-traffic-calls")).toHaveTextContent("50");
    expect(screen.getByTestId("mcp-traffic-error-rate")).toHaveTextContent("10.0%");
    expect(screen.getAllByTestId("mcp-traffic-tool")).toHaveLength(1);
    expect(screen.getByText("Weather")).toHaveAttribute("href", "/admin/mcp-servers/4");
  });

  it("shows the empty state without traffic", async () => {
    apiClient.get.mockImplementation(routes({ "/analytics/mcp/overview": { data: { calls: 0 } } }));
    wrap(<MCPTrafficPanel startDate="2026-10-01" endDate="2026-10-20" />);
    expect(await screen.findByTestId("mcp-analytics-empty")).toBeInTheDocument();
  });

  it("shows an error", async () => {
    apiClient.get.mockRejectedValue(new Error("down"));
    wrap(<MCPTrafficPanel startDate="2026-10-01" endDate="2026-10-20" />);
    expect(await screen.findByTestId("mcp-traffic-error")).toHaveTextContent("down");
  });
});
