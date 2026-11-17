import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import testTheme from "../utils/testTheme";
import MCPServerRegister, { buildPayload } from "./MCPServerRegister";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";
import { clearIdentity } from "../utils/identityStore";
import { resetWorkspaceNames } from "../components/workspaces/workspaceFields";

// Workspaces E3 (D5): register creates a server, so it lands in a Workspace
// where the caller holds mcp-servers:write.

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));

const identity = (attributes) => ({ id: "9", attributes: { is_admin: false, has_admin_access: true, permissions: [], ...attributes } });
const scopedW1 = identity({
  rbac_enabled: true,
  workspaces: [{ id: 2, name: "W1", permissions: ["mcp-servers:read", "mcp-servers:write", "mcp-servers:execute"] }],
});
const community = identity({ is_admin: true, rbac_enabled: false, permissions: ["*"] });

const renderWith = (who) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/mcp-servers/register"]}>
        <PermissionsProvider identity={who}>
          <Routes>
            <Route path="/admin/mcp-servers/register" element={<MCPServerRegister />} />
            <Route path="/admin/mcp-servers/:id" element={<div data-testid="detail-page" />} />
          </Routes>
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const pickConnection = async (label) => {
  fireEvent.mouseDown(screen.getByRole("combobox", { name: "Tyk connection" }));
  fireEvent.click(await screen.findByRole("option", { name: label }));
};

// Drives the wizard to the review step and creates the server.
const walkToCreate = async () => {
  await screen.findByTestId("connection");
  await pickConnection("Prod");
  fireEvent.click(screen.getByTestId("next"));
  fireEvent.change(await screen.findByTestId("name"), { target: { value: "Weather MCP" } });
  fireEvent.change(screen.getByTestId("listen-path"), { target: { value: "/weather-mcp/" } });
  fireEvent.change(screen.getByTestId("upstream-url"), { target: { value: "https://weather.example.com" } });
  fireEvent.click(screen.getByTestId("next"));
  await screen.findByText(/Publish to the portal right away/);
  fireEvent.click(screen.getByTestId("next"));
  fireEvent.click(await screen.findByTestId("create"));
};

describe("MCPServerRegister Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    apiClient.get.mockImplementation((path) => {
      if (path === "/tyk-mcp/status") return Promise.resolve({ data: { available: true, enabled: true } });
      if (path === "/tyk-connections") return Promise.resolve({ data: [{ id: 1, name: "Prod", status: "active", effective_mode: "full", capabilities: {} }] });
      if (path === "/tool-catalogues") return Promise.resolve({ data: { data: [] } });
      if (path === "/tyk-connections/1/gateway-tags") return Promise.resolve({ data: [] });
      if (path === "/workspaces") return Promise.resolve({ data: { data: [{ id: "2", attributes: { name: "W1" } }] } });
      return Promise.reject(new Error("unexpected " + path));
    });
    apiClient.post.mockImplementation((path) =>
      path === "/mcp-servers/register?dry_run=1"
        ? Promise.resolve({ data: { definition: {}, warnings: [], endpoint_url: "https://gw/x/mcp" } })
        : Promise.resolve({ data: { server: { id: 42 }, warnings: [] } }),
    );
  });

  it("with one Workspace, preselects it and sends workspace_id", async () => {
    renderWith(scopedW1);
    await screen.findByTestId("workspace-select");
    await waitFor(() => expect(screen.getByRole("combobox", { name: "Workspace" })).toHaveTextContent("W1"));
    await walkToCreate();
    await screen.findByTestId("detail-page");
    const calls = apiClient.post.mock.calls.filter(([p]) => p.startsWith("/mcp-servers/register"));
    expect(calls).toHaveLength(2);
    calls.forEach(([, body]) => expect(body).toMatchObject({ workspace_id: 2 }));
  });

  it("Community: no Workspace select and no workspace_id", async () => {
    renderWith(community);
    await screen.findByTestId("connection");
    expect(screen.queryByTestId("workspace-select")).not.toBeInTheDocument();
    await walkToCreate();
    await screen.findByTestId("detail-page");
    const [, body] = apiClient.post.mock.calls.find(([p]) => p === "/mcp-servers/register");
    expect(body).not.toHaveProperty("workspace_id");
  });

  it("a workspace_required answer returns to the Workspace field", async () => {
    apiClient.post.mockRejectedValue({ response: { status: 422, data: { errors: [{ code: "workspace_required" }] } } });
    renderWith(scopedW1);
    await screen.findByTestId("connection");
    await pickConnection("Prod");
    fireEvent.click(screen.getByTestId("next"));
    fireEvent.change(await screen.findByTestId("name"), { target: { value: "Weather MCP" } });
    fireEvent.change(screen.getByTestId("listen-path"), { target: { value: "/weather-mcp/" } });
    fireEvent.change(screen.getByTestId("upstream-url"), { target: { value: "https://weather.example.com" } });
    fireEvent.click(screen.getByTestId("next"));
    await screen.findByText(/Publish to the portal right away/);
    fireEvent.click(screen.getByTestId("next"));
    expect(await screen.findByTestId("workspace-select")).toBeInTheDocument();
    expect((await screen.findAllByText(/choose one/)).length).toBeGreaterThan(0);
  });

  it("buildPayload sends workspace_id only when chosen", () => {
    const f = { connection_id: "1", kind: "remote", tags: "", privacy_score: "", tool_catalogues: [], consumer_auth: "auth_token", upstream_url: "u", tool_access: "all" };
    expect(buildPayload(f)).not.toHaveProperty("workspace_id");
    expect(buildPayload({ ...f, workspace_id: 3 })).toMatchObject({ workspace_id: 3 });
  });
});
