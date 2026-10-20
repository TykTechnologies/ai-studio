import React from "react";
import { screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { renderWithRoutesAndTheme } from "../../test-utils/render-with-theme";
import MCPServerDetail from "./MCPServerDetail";
import apiClient from "../utils/apiClient";
import { P } from "../rbac/permissions";

jest.mock("../utils/apiClient");
jest.mock("react-chartjs-2", () => ({ Line: () => <div data-testid="chart" /> }));

let mockCan;
jest.mock("../context/PermissionsContext", () => ({
  usePermissions: () => ({ can: (p) => mockCan(p), canAny: () => true, canAll: () => true, isFullAdmin: true, hasAdminAccess: true, rbacEnabled: false }),
}));

const server = {
  id: 4,
  connection_id: 7,
  name: "Weather MCP",
  slug: "weather-mcp",
  kind: "remote",
  listen_path: "/weather-mcp/",
  auth_mode: "auth_token",
  auth_details: {},
  primitives: [],
  dashboard_state: "active",
  origin: "dashboard",
  privacy_score: 20,
  is_active: true,
  brokerable: true,
  lock_version: 1,
  definition: "{}",
  tool_catalogue_ids: [],
  tool_catalogues: [],
  bundle: [],
};

const renderDetail = (entry = "/admin/mcp-servers/4") =>
  renderWithRoutesAndTheme(null, {
    initialEntry: entry,
    routes: [{ path: "/admin/mcp-servers/:id", element: <MCPServerDetail /> }],
  });

describe("MCPServerDetail Usage tab", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockCan = () => true;
    apiClient.get.mockImplementation((path) => {
      if (path === "/mcp-servers/4") return Promise.resolve({ data: server });
      if (path === "/tyk-connections/7/policies") return Promise.resolve({ data: [] });
      if (path === "/tyk-connections/7") return Promise.resolve({ data: { id: 7, status: "active", effective_mode: "full" } });
      if (path === "/tool-catalogues") return Promise.resolve({ data: { data: [] } });
      if (path === "/mcp-servers/4/credentials") return Promise.resolve({ data: [] });
      if (path.startsWith("/analytics/mcp/servers/4/summary")) return Promise.resolve({ data: { calls: 0 } });
      if (path.startsWith("/analytics/mcp/servers/4/")) return Promise.resolve({ data: { points: [], data: [], total: 0 } });
      return Promise.reject(new Error("unexpected " + path));
    });
  });

  it("opens Usage from the tab and shows the Pump setup empty state", async () => {
    renderDetail();
    fireEvent.click(await screen.findByTestId("tab-usage"));
    expect(await screen.findByTestId("mcp-server-usage")).toBeInTheDocument();
    expect(await screen.findByTestId("mcp-analytics-setup-link")).toHaveAttribute("href", "/admin/tyk-connections/edit/7#analytics");
    expect(screen.queryByText("Server information")).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId("tab-details"));
    expect(await screen.findByText("Server information")).toBeInTheDocument();
  });

  it("deep-links to the Usage tab", async () => {
    renderDetail("/admin/mcp-servers/4?tab=usage");
    expect(await screen.findByTestId("mcp-server-usage")).toBeInTheDocument();
  });

  it("has no Usage tab without analytics read", async () => {
    mockCan = (p) => p !== P.ANALYTICS_READ;
    renderDetail("/admin/mcp-servers/4?tab=usage");
    expect(await screen.findByText("Server information")).toBeInTheDocument();
    expect(screen.queryByTestId("tab-usage")).not.toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith(expect.stringMatching(/^\/analytics\/mcp/), expect.anything());
  });
});
