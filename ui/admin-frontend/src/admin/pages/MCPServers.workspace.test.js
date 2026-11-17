import React from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import MCPServers from "./MCPServers";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";
import { clearIdentity } from "../utils/identityStore";
import { resetWorkspaceNames } from "../components/workspaces/workspaceFields";

// Workspaces E3: the MCP server list shows each server's Workspace, filters
// by Workspace, and offers Register to a caller who may create in some
// Workspace.

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
jest.mock("../hooks/useConfig", () => ({
  __esModule: true,
  default: () => ({ config: {}, loading: false, getDocsLink: () => "https://docs.example.com/mcp" }),
}));
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
}));

const base = {
  connection_id: 1,
  connection_name: "Prod",
  kind: "remote",
  listen_path: "/x/",
  auth_mode: "auth_token",
  gateway_tags: { enabled: false, tags: [] },
  dashboard_state: "active",
  origin: "dashboard",
  is_active: false,
};
const servers = [
  { ...base, id: 1, name: "W1 Weather", workspace_id: 2 },
  { ...base, id: 2, name: "W2 Tickets", workspace_id: 3 },
];

const MCP_ALL = ["mcp-servers:read", "mcp-servers:write", "mcp-servers:execute", "mcp-servers:delete", "mcp-servers:publish"];
const identity = (attributes) => ({ id: "9", attributes: { is_admin: false, has_admin_access: true, permissions: [], ...attributes } });
const scoped = identity({ rbac_enabled: true, workspaces: [{ id: 2, name: "W1", permissions: MCP_ALL }] });
// Execute but no write anywhere: may not register (D5).
const executeOnly = identity({ rbac_enabled: true, workspaces: [{ id: 2, name: "W1", permissions: ["mcp-servers:read", "mcp-servers:execute"] }] });
const community = identity({ is_admin: true, rbac_enabled: false, permissions: ["*"] });

const renderWith = (who, path = "/admin/mcp-servers") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <PermissionsProvider identity={who}>
          <MCPServers />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

describe("MCPServers Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    apiClient.get.mockImplementation((path) => {
      if (path === "/tyk-mcp/status") return Promise.resolve({ data: { available: true, enabled: true } });
      if (path === "/tyk-connections") return Promise.resolve({ data: [{ id: 1, name: "Prod", status: "active", effective_mode: "full" }] });
      if (path === "/mcp-servers") return Promise.resolve({ data: { servers, total: 2 } });
      if (path === "/workspaces") return Promise.resolve({ data: { data: [{ id: "2", attributes: { name: "W1" } }] } });
      return Promise.reject(new Error("unexpected " + path));
    });
  });

  it("Enterprise: shows the Workspace column and filter; another Workspace's name is not shown", async () => {
    renderWith(scoped);
    await screen.findByText("W1 Weather");
    expect(screen.getByRole("columnheader", { name: "Workspace" })).toBeInTheDocument();
    expect(screen.getByTestId("workspace-filter")).toBeInTheDocument();
    expect(within(screen.getByTestId("server-row-1")).getByTestId("workspace-cell")).toHaveTextContent("W1");
    expect(within(screen.getByTestId("server-row-2")).getByTestId("workspace-cell")).toHaveTextContent("Other Workspace");
  });

  it("Community: no Workspace column or filter", async () => {
    renderWith(community);
    await screen.findByText("W1 Weather");
    expect(screen.queryByRole("columnheader", { name: "Workspace" })).not.toBeInTheDocument();
    expect(screen.queryByTestId("workspace-filter")).not.toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/workspaces");
    expect(screen.getByTestId("register-server")).toBeInTheDocument();
  });

  it("?workspace_id= filters the list request", async () => {
    renderWith(scoped, "/admin/mcp-servers?workspace_id=2");
    await screen.findByText("W1 Weather");
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith("/mcp-servers", { params: expect.objectContaining({ workspace_id: 2 }) }),
    );
  });

  it("Register shows for execute plus write in a Workspace, not for execute alone", async () => {
    const { unmount } = renderWith(scoped);
    await screen.findByText("W1 Weather");
    expect(screen.getByTestId("register-server")).toBeInTheDocument();
    unmount();

    clearIdentity();
    renderWith(executeOnly);
    await screen.findByText("W1 Weather");
    expect(screen.queryByTestId("register-server")).not.toBeInTheDocument();
  });
});
