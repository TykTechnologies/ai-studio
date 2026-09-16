import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import testTheme from "../utils/testTheme";
import MCPServers from "./MCPServers";
import apiClient from "../utils/apiClient";

jest.mock("../utils/apiClient");

jest.mock("../context/PermissionsContext", () => ({
  usePermissions: () => ({
    can: () => true,
    canAny: () => true,
    canAll: () => true,
    isFullAdmin: true,
    hasAdminAccess: true,
    rbacEnabled: false,
  }),
}));
jest.mock("../hooks/useConfig", () => ({
  __esModule: true,
  default: () => ({ config: {}, loading: false, getDocsLink: () => "https://docs.example.com/mcp" }),
}));

const server = {
  id: 7,
  connection_id: 1,
  connection_name: "Prod",
  tyk_api_id: "api-weather",
  name: "Weather MCP proxy",
  slug: "weather-mcp-proxy",
  kind: "remote",
  listen_path: "/weather/",
  auth_mode: "auth_token",
  gateway_tags: { enabled: true, tags: ["edge-eu"] },
  dashboard_state: "active",
  origin: "dashboard",
  is_active: false,
  brokerable: true,
  last_seen_at: "2026-09-16T10:00:00Z",
};

const renderList = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/mcp-servers"]}>
        <Routes>
          <Route path="/admin/mcp-servers" element={<MCPServers />} />
          <Route path="/admin/mcp-servers/register" element={<div>register page</div>} />
          <Route path="/admin/mcp-servers/:id" element={<div>detail page</div>} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

const mockGets = (servers = [server], connections = [{ id: 1, name: "Prod", status: "active", effective_mode: "full" }]) => {
  apiClient.get.mockImplementation((path) => {
    if (path === "/tyk-mcp/status") return Promise.resolve({ data: { available: true, enabled: true } });
    if (path === "/tyk-connections") return Promise.resolve({ data: connections });
    if (path === "/mcp-servers") return Promise.resolve({ data: { servers, total: servers.length, page: 1, page_size: 25 } });
    return Promise.reject(new Error("unexpected " + path));
  });
};

describe("MCPServers", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("lists servers in the data table and runs a sync", async () => {
    mockGets();
    apiClient.post.mockResolvedValue({ data: { status: "ok", proxies_added: 1, proxies_updated: 0, proxies_missing: 0 } });
    renderList();
    expect(await screen.findByText("Weather MCP proxy")).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "MCP servers" })).toBeInTheDocument();
    expect(screen.getByTestId("state-active")).toBeInTheDocument();
    expect(screen.getByTestId("active-status-dot")).toHaveAttribute("data-active", "false");
    expect(screen.getByTestId("active-status-dot")).toHaveTextContent("Unpublished");
    expect(screen.getByText("Brokerable")).toBeInTheDocument();
    expect(screen.getByText("edge-eu")).toBeInTheDocument();
    expect(apiClient.get).toHaveBeenCalledWith("/mcp-servers", { params: { page: 1, page_size: 25 } });

    fireEvent.click(screen.getByTestId("sync-now"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/tyk-connections/1/sync?wait=true", {}));
    expect(await screen.findByText(/Sync ok: 1 added/)).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("server-row-7"));
    expect(await screen.findByText("detail page")).toBeInTheDocument();
  });

  it("sends the search term as q and the filters as query params", async () => {
    mockGets();
    renderList();
    await screen.findByText("Weather MCP proxy");

    fireEvent.change(screen.getByPlaceholderText("Search MCP servers by name..."), { target: { value: "weather" } });
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/mcp-servers", { params: { page: 1, page_size: 25, q: "weather" } }));

    fireEvent.change(screen.getByTestId("filter-published"), { target: { value: "true" } });
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith("/mcp-servers", { params: { page: 1, page_size: 25, q: "weather", published: "true" } })
    );
  });

  it("offers Register only when a full-mode connection is active", async () => {
    mockGets([server], [{ id: 1, name: "Prod", status: "active", effective_mode: "catalogue" }]);
    renderList();
    await screen.findByText("Weather MCP proxy");
    expect(screen.queryByTestId("register-server")).not.toBeInTheDocument();
    expect(screen.getByTestId("sync-now")).toBeInTheDocument();
  });

  it("shows the empty state with actions when nothing is imported yet", async () => {
    mockGets([]);
    renderList();
    expect(await screen.findByText("No MCP servers yet")).toBeInTheDocument();
    expect(screen.getAllByTestId("register-server").length).toBeGreaterThan(0);
    fireEvent.click(screen.getAllByTestId("register-server")[0]);
    expect(await screen.findByText("register page")).toBeInTheDocument();
  });

  it("points at Settings → Tyk Connections when nothing is connected", async () => {
    mockGets([], []);
    renderList();
    expect(await screen.findByText(/Settings → Tyk Connections/)).toBeInTheDocument();
    expect(screen.queryByTestId("sync-now")).not.toBeInTheDocument();
  });

  it("shows the upsell when unavailable", async () => {
    apiClient.get.mockResolvedValue({ data: { available: false, enabled: false } });
    renderList();
    expect(await screen.findByText("Enterprise Feature")).toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/mcp-servers", expect.anything());
  });
});
