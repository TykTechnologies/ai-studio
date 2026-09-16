import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import MCPServers from "./MCPServers";
import MCPServerDetail from "./MCPServerDetail";
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

jest.mock("../styles/sharedStyles", () => ({
  TitleBox: ({ children }) => <div>{children}</div>,
  StyledPaper: ({ children, sx, ...props }) => <div {...props}>{children}</div>,
  StyledTableHeaderCell: ({ children, sx, ...props }) => <th {...props}>{children}</th>,
  StyledTableCell: ({ children, sx, ...props }) => <td {...props}>{children}</td>,
  StyledTableRow: ({ children, hover, sx, ...props }) => <tr {...props}>{children}</tr>,
}));

const theme = createTheme();

const server = {
  id: 7,
  connection_id: 1,
  connection_name: "Prod",
  tyk_api_id: "api-weather",
  name: "Weather MCP proxy",
  slug: "weather-mcp-proxy",
  description: "",
  kind: "remote",
  listen_path: "/weather/",
  transport_path: "/mcp",
  endpoint_url: "https://gw.example.com/weather/mcp",
  endpoint_urls: { "edge-eu": "https://eu.gw.example.com/weather/mcp" },
  upstream_url: "https://weather.example.com",
  auth_mode: "auth_token",
  auth_details: { header_name: "Authorization", schemes: [{ name: "authToken", type: "auth_token" }] },
  primitives: [{ type: "tool", name: "get-weather" }],
  gateway_tags: { enabled: true, tags: ["edge-eu"] },
  dashboard_state: "active",
  origin: "dashboard",
  privacy_score: null,
  is_active: false,
  brokerable: false,
  lock_version: 2,
  definition: JSON.stringify({ openapi: "3.0.3" }),
  group_ids: [],
  bundle: [],
};

const policies = [
  { tyk_policy_id: "pol-acl", name: "Weather access", is_partitioned: true, partitions: { acl: true }, api_ids: ["api-weather"] },
  { tyk_policy_id: "pol-gold", name: "Gold plan", is_partitioned: true, partitions: { rate_limit: true, quota: true }, api_ids: [] },
  { tyk_policy_id: "pol-other", name: "Other API", is_partitioned: true, partitions: { acl: true }, api_ids: ["api-other"] },
];

const renderList = () =>
  render(
    <ThemeProvider theme={theme}>
      <MemoryRouter initialEntries={["/admin/mcp-servers"]}>
        <Routes>
          <Route path="/admin/mcp-servers" element={<MCPServers />} />
          <Route path="/admin/mcp-servers/:id" element={<div>detail page</div>} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

const renderDetail = () =>
  render(
    <ThemeProvider theme={theme}>
      <MemoryRouter initialEntries={["/admin/mcp-servers/7"]}>
        <Routes>
          <Route path="/admin/mcp-servers/:id" element={<MCPServerDetail />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

describe("MCPServers", () => {
  beforeEach(() => {
    apiClient.get.mockReset();
    apiClient.post.mockReset();
    apiClient.patch.mockReset();
    apiClient.put.mockReset();
    apiClient.delete.mockReset();
  });

  it("lists servers and runs a sync", async () => {
    apiClient.get.mockImplementation((path) => {
      if (path === "/tyk-mcp/status") return Promise.resolve({ data: { available: true, enabled: true } });
      if (path === "/tyk-connections") return Promise.resolve({ data: [{ id: 1, name: "Prod", status: "active" }] });
      if (path === "/mcp-servers") return Promise.resolve({ data: { servers: [server], total: 1, page: 1, page_size: 25 } });
      return Promise.reject(new Error("unexpected " + path));
    });
    apiClient.post.mockResolvedValue({ data: { status: "ok", proxies_added: 1, proxies_updated: 0, proxies_missing: 0 } });
    renderList();
    expect(await screen.findByText("Weather MCP proxy")).toBeInTheDocument();
    expect(screen.getByTestId("state-active")).toBeInTheDocument();
    expect(screen.getByText("Unpublished")).toBeInTheDocument();
    expect(screen.getByText("edge-eu")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("sync-now"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/tyk-connections/1/sync?wait=true", {}));
    expect(await screen.findByText(/Sync ok: 1 added/)).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("server-row-7"));
    expect(await screen.findByText("detail page")).toBeInTheDocument();
  });

  it("shows the upsell when unavailable", async () => {
    apiClient.get.mockResolvedValue({ data: { available: false, enabled: false } });
    renderList();
    expect(await screen.findByText("Enterprise Feature")).toBeInTheDocument();
  });
});

describe("MCPServerDetail", () => {
  beforeEach(() => {
    apiClient.get.mockReset();
    apiClient.post.mockReset();
    apiClient.patch.mockReset();
    apiClient.put.mockReset();
  });

  it("renders the server, saves a privacy score, publishes and pins a bundle", async () => {
    apiClient.get.mockImplementation((path) => {
      if (path === "/mcp-servers/7") return Promise.resolve({ data: server });
      if (path === "/tyk-connections/1/policies") return Promise.resolve({ data: policies });
      if (path === "/groups") return Promise.resolve({ data: [{ id: 3, name: "AI team" }] });
      return Promise.reject(new Error("unexpected " + path));
    });
    apiClient.patch.mockResolvedValue({ data: { ...server, privacy_score: 40, lock_version: 3 } });
    apiClient.post.mockResolvedValue({ data: { ...server, privacy_score: 40, is_active: true, lock_version: 4 } });
    apiClient.put.mockResolvedValue({ data: { ...server, brokerable: true, bundle: [{ role: "access", policy: policies[0] }, { role: "consumption", policy: policies[1] }] } });
    renderDetail();

    expect(await screen.findByText("https://gw.example.com/weather/mcp")).toBeInTheDocument();
    expect(screen.getByText("https://eu.gw.example.com/weather/mcp")).toBeInTheDocument();
    expect(screen.getByText(/Set a privacy score before publishing/)).toBeInTheDocument();
    expect(screen.getByLabelText("Published")).toBeDisabled();

    fireEvent.change(screen.getByTestId("privacy-score"), { target: { value: "40" } });
    fireEvent.click(screen.getByTestId("save-presentation"));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalledWith("/mcp-servers/7", expect.objectContaining({ privacy_score: 40, lock_version: 2 })));
    expect(await screen.findByText("Saved")).toBeInTheDocument();
    expect(screen.getByLabelText("Published")).not.toBeDisabled();

    fireEvent.click(screen.getByLabelText("Published"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/mcp-servers/7/activate", {}));
    expect(await screen.findByText("Published to the portal")).toBeInTheDocument();

    // Only policies granting this proxy are offered as access policies.
    fireEvent.change(screen.getByTestId("access-select"), { target: { value: "pol-acl" } });
    fireEvent.click(screen.getByTestId("save-bundle"));
    await waitFor(() =>
      expect(apiClient.put).toHaveBeenCalledWith("/mcp-servers/7/bundle", { pins: [{ tyk_policy_id: "pol-acl", role: "access" }] })
    );
    expect(await screen.findByText("Bundle saved")).toBeInTheDocument();
    expect(screen.getByText("Brokerable")).toBeInTheDocument();
  });
});
