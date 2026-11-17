import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import testTheme from "../utils/testTheme";
import MCPServerDetail from "./MCPServerDetail";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";
import { clearIdentity } from "../utils/identityStore";
import { resetWorkspaceNames } from "../components/workspaces/workspaceFields";

// Workspaces E3 (B4b rules): edit, delete, publish, push and link are
// checked in the server's Workspace, as is the handoff with the credential.

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));
jest.mock("../components/mcp-analytics/MCPServerUsage", () => () => null);

const MCP_ALL = ["mcp-servers:read", "mcp-servers:write", "mcp-servers:execute", "mcp-servers:delete", "mcp-servers:publish"];
const identity = (attributes) => ({ id: "9", attributes: { is_admin: false, has_admin_access: true, permissions: [], ...attributes } });
// Everything on MCP servers in W1 (id 2), read everywhere.
const scoped = identity({ rbac_enabled: true, permissions: ["mcp-servers:read"], workspaces: [{ id: 2, name: "W1", permissions: MCP_ALL }] });
const scopedTwo = identity({
  rbac_enabled: true,
  permissions: ["mcp-servers:read"],
  workspaces: [
    { id: 2, name: "W1", permissions: MCP_ALL },
    { id: 3, name: "W2", permissions: MCP_ALL },
  ],
});

const common = {
  connection_id: 1,
  connection_name: "Prod",
  slug: "s",
  kind: "remote",
  listen_path: "/s/",
  auth_mode: "auth_token",
  auth_details: {},
  primitives: [],
  gateway_tags: { enabled: false, tags: [] },
  privacy_score: 20,
  is_active: false,
  brokerable: true,
  lock_version: 1,
  definition: "{}",
  tool_catalogues: [],
  bundle: [],
};
// On the Dashboard of a full-mode connection: edit, publish, push, delete.
const active = (ws) => ({ ...common, id: 9, name: "Weather", tyk_api_id: "api-w", dashboard_state: "active", origin: "studio", workspace_id: ws });
// Awaiting the platform team: link and handoff with the credential.
const pending = (ws) => ({ ...common, id: 9, name: "Tickets", tyk_api_id: "", dashboard_state: "pending_platform", origin: "submission", workspace_id: ws });
const handoff = { instructions: [], candidates: [{ id: 12, name: "tickets", listen_path: "/t/", tyk_api_id: "api-t" }] };

const mockGets = (server) => {
  apiClient.get.mockImplementation((path) => {
    if (path === "/mcp-servers/9") return Promise.resolve({ data: server });
    if (path === "/mcp-servers/9/handoff") return Promise.resolve({ data: handoff });
    if (path === "/tyk-connections/1/policies") return Promise.resolve({ data: [] });
    if (path === "/tyk-connections/1") return Promise.resolve({ data: { id: 1, name: "Prod", status: "active", effective_mode: "full", kind: "dashboard" } });
    if (path === "/tool-catalogues") return Promise.resolve({ data: { data: [] } });
    if (path === "/workspaces") return Promise.resolve({ data: { data: [{ id: "2", attributes: { name: "W1" } }, { id: "3", attributes: { name: "W2" } }] } });
    return Promise.reject(new Error("unexpected " + path));
  });
};

const renderWith = (who) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/mcp-servers/9"]}>
        <PermissionsProvider identity={who}>
          <Routes>
            <Route path="/admin/mcp-servers/:id" element={<MCPServerDetail />} />
          </Routes>
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

describe("MCPServerDetail Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
  });

  it("a server in the caller's Workspace shows edit, publish, push and delete", async () => {
    mockGets(active(2));
    renderWith(scoped);
    expect(await screen.findByTestId("save-presentation")).toBeInTheDocument();
    expect(await screen.findByTestId("push-definition")).toBeInTheDocument();
    expect(screen.getByTestId("delete-server")).toBeInTheDocument();
    expect(screen.getByTestId("save-catalogues")).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Published" })).toBeEnabled();
  });

  it("a server of another Workspace shows no edit, delete, publish or push controls", async () => {
    mockGets(active(3));
    renderWith(scoped);
    await screen.findByText("Server information");
    await screen.findByText(/Edit the Tyk OAS document/);
    expect(screen.queryByTestId("save-presentation")).not.toBeInTheDocument();
    expect(screen.queryByTestId("save-catalogues")).not.toBeInTheDocument();
    expect(screen.queryByTestId("push-definition")).not.toBeInTheDocument();
    expect(screen.queryByTestId("open-policy-creator")).not.toBeInTheDocument();
    expect(screen.queryByTestId("delete-server")).not.toBeInTheDocument();
    expect(screen.queryByTestId("workspace-select")).not.toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Published" })).toBeDisabled();
  });

  it("a pending server of another Workspace shows no link or credential download", async () => {
    mockGets(pending(3));
    renderWith(scoped);
    await screen.findByTestId("link-candidates");
    expect(screen.getByTestId("download-handoff")).toBeInTheDocument();
    expect(screen.queryByTestId("download-handoff-secrets")).not.toBeInTheDocument();
    expect(screen.queryByTestId("link-api-t")).not.toBeInTheDocument();
    expect(screen.queryByTestId("delete-server")).not.toBeInTheDocument();
  });

  it("a pending server in the caller's Workspace shows link and credential download", async () => {
    mockGets(pending(2));
    renderWith(scoped);
    expect(await screen.findByTestId("link-api-t")).toBeInTheDocument();
    expect(screen.getByTestId("download-handoff-secrets")).toBeInTheDocument();
  });

  it("moving the server sends the new workspace_id; an unchanged one is not sent", async () => {
    mockGets(active(2));
    apiClient.patch.mockImplementation((_, body) => Promise.resolve({ data: { ...active(body.workspace_id ?? 2) } }));
    renderWith(scopedTwo);
    fireEvent.click(await screen.findByTestId("save-presentation"));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalledTimes(1));
    expect(apiClient.patch.mock.calls[0][1]).not.toHaveProperty("workspace_id");

    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Workspace" }));
    fireEvent.click(await screen.findByRole("option", { name: "W2" }));
    fireEvent.click(screen.getByTestId("save-presentation"));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalledTimes(2));
    expect(apiClient.patch.mock.calls[1][1]).toMatchObject({ workspace_id: 3 });
  });

  it("an invalid_workspace answer is shown on the Workspace field", async () => {
    mockGets(active(2));
    apiClient.patch.mockRejectedValue({ response: { status: 422, data: { errors: [{ code: "invalid_workspace" }] } } });
    renderWith(scopedTwo);
    fireEvent.mouseDown(await screen.findByRole("combobox", { name: "Workspace" }));
    fireEvent.click(await screen.findByRole("option", { name: "W2" }));
    fireEvent.click(screen.getByTestId("save-presentation"));
    expect(await screen.findAllByText(/This Workspace does not exist/)).not.toHaveLength(0);
  });
});
