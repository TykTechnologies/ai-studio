import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import TykConnections from "./TykConnections";
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

const renderPage = () =>
  render(
    <ThemeProvider theme={theme}>
      <MemoryRouter>
        <TykConnections />
      </MemoryRouter>
    </ThemeProvider>
  );

const connections = [
  {
    id: 1,
    name: "Prod Dashboard",
    dashboard_url: "https://dash.example.com",
    declared_mode: "full",
    effective_mode: "broker",
    status: "active",
    degraded: false,
    capabilities: {
      mcp_read: { state: "ok" },
      mcp_write: { state: "denied", detail: "403" },
      keys_write: { state: "unverified" },
    },
    gateway_tags: ["edge-eu"],
    data_planes: [{ group_id: "eu", tags: ["edge-eu"], node_count: 2, healthy: true }],
    lock_version: 3,
    has_token: true,
    token_hint: "abcd",
  },
];

describe("TykConnections", () => {
  beforeEach(() => {
    apiClient.get.mockReset();
    apiClient.post.mockReset();
    apiClient.patch.mockReset();
    apiClient.delete.mockReset();
  });

  it("shows the enterprise upsell when the feature is unavailable", async () => {
    apiClient.get.mockResolvedValue({ data: { available: false, enabled: false } });
    renderPage();
    expect(await screen.findByText("Enterprise Feature")).toBeInTheDocument();
    expect(apiClient.get).toHaveBeenCalledTimes(1);
  });

  it("shows why the feature is disabled", async () => {
    apiClient.get.mockResolvedValue({ data: { available: true, enabled: false, disabled_reason: "TYK_AI_SECRET_KEY is not set" } });
    renderPage();
    expect(await screen.findByText(/TYK_AI_SECRET_KEY is not set/)).toBeInTheDocument();
  });

  it("lists connections with the capped mode and probe capabilities", async () => {
    apiClient.get.mockImplementation((path) => {
      if (path === "/tyk-mcp/status") return Promise.resolve({ data: { available: true, enabled: true } });
      if (path === "/tyk-connections") return Promise.resolve({ data: connections });
      return Promise.reject(new Error("unexpected " + path));
    });
    renderPage();
    expect(await screen.findByText("Prod Dashboard")).toBeInTheDocument();
    expect(screen.getByTestId("mode-chip")).toHaveTextContent("Broker (declared Full)");
    expect(screen.getByTestId("connection-status-active")).toBeInTheDocument();
    expect(screen.getByText("1 tag(s)")).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText("details"));
    expect(await screen.findByTestId("capability-mcp_write")).toHaveTextContent("Create MCP proxies: denied");
    expect(screen.getByTestId("capability-keys_write")).toHaveTextContent("Mint keys: unverified");
    expect(screen.getByText(/eu: 2 node\(s\), tags edge-eu/)).toBeInTheDocument();
  });

  it("tests unsaved settings and creates a connection without echoing the token", async () => {
    apiClient.get.mockImplementation((path) => {
      if (path === "/tyk-mcp/status") return Promise.resolve({ data: { available: true, enabled: true } });
      if (path === "/tyk-connections") return Promise.resolve({ data: [] });
      return Promise.reject(new Error("unexpected " + path));
    });
    apiClient.post.mockImplementation((path) => {
      if (path === "/tyk-connections/probe") {
        return Promise.resolve({
          data: { reachable: true, effective_mode: "catalogue", org_id: "org-1", capabilities: { mcp_read: { state: "ok" } }, warnings: [], data_planes: [] },
        });
      }
      if (path === "/tyk-connections") return Promise.resolve({ data: { id: 5, name: "New", status: "pending", has_token: true } });
      return Promise.reject(new Error("unexpected " + path));
    });
    renderPage();
    fireEvent.click(await screen.findByTestId("add-connection"));

    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "New" } });
    fireEvent.change(screen.getByLabelText(/Dashboard URL/), { target: { value: "https://dash.example.com" } });
    fireEvent.change(screen.getByTestId("token-input"), { target: { value: "secret-token-1234" } });

    fireEvent.click(screen.getByTestId("probe-button"));
    expect(await screen.findByTestId("probe-panel")).toHaveTextContent("Effective mode: Catalogue");
    expect(apiClient.post).toHaveBeenCalledWith(
      "/tyk-connections/probe",
      expect.objectContaining({ dashboard_url: "https://dash.example.com", dashboard_access_token: "secret-token-1234", declared_mode: "catalogue" })
    );

    fireEvent.click(screen.getByTestId("save-button"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/tyk-connections", expect.objectContaining({ name: "New" })));
    expect(await screen.findByText(/activate it to start syncing/)).toBeInTheDocument();
  });

  it("activates through the row menu", async () => {
    const pending = [{ ...connections[0], status: "pending", effective_mode: "catalogue", declared_mode: "catalogue" }];
    apiClient.get.mockImplementation((path) => {
      if (path === "/tyk-mcp/status") return Promise.resolve({ data: { available: true, enabled: true } });
      if (path === "/tyk-connections") return Promise.resolve({ data: pending });
      return Promise.reject(new Error("unexpected " + path));
    });
    apiClient.post.mockResolvedValue({ data: { ...pending[0], status: "active" } });
    renderPage();
    fireEvent.click(await screen.findByTestId("menu-1"));
    fireEvent.click(await screen.findByTestId("menu-activate"));
    fireEvent.click(await screen.findByTestId("confirm-action"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/tyk-connections/1/activate", {}));
    expect(await screen.findByText("Prod Dashboard activated")).toBeInTheDocument();
  });
});
