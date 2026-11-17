import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import AgentDetail from "./AgentDetail";
import apiClient from "../../utils/apiClient";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";
import { resetWorkspaceNames } from "../workspaces/workspaceFields";

// Workspaces E4: the agent page's actions are checked in the agent's
// Workspace, and an agent the caller cannot see (404) shows the not-found
// panel.

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));

beforeAll(() => {
  // The page scrolls its test transcript into view; jsdom has no layout.
  window.HTMLElement.prototype.scrollIntoView = jest.fn();
});

const agent = (workspaceId) => ({
  id: 5,
  name: "Helper",
  slug: "helper",
  description: "",
  plugin: { id: 1, name: "Agent plugin" },
  app: { id: 10, name: "Visible App" },
  config: {},
  groups: [],
  is_active: true,
  workspace_id: workspaceId,
  created_at: "2026-01-01T00:00:00Z",
});

// Reads agents everywhere; write, delete and publish in W1 (id 2) only.
const scoped = {
  id: "9",
  attributes: {
    has_admin_access: true,
    rbac_enabled: true,
    permissions: ["agents:read"],
    workspaces: [{ id: 2, name: "W1", permissions: ["agents:read", "agents:write", "agents:delete", "agents:publish"] }],
  },
};

const renderDetail = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={["/admin/agents/5"]}>
        <PermissionsProvider identity={scoped}>
          <Routes>
            <Route path="/admin/agents/:id" element={<AgentDetail />} />
          </Routes>
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

describe("AgentDetail Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    jest.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    console.error.mockRestore();
  });

  it("offers Edit, Deactivate and Delete for an agent in the caller's Workspace", async () => {
    apiClient.get.mockResolvedValue({ data: { data: agent(2) } });
    renderDetail();
    expect(await screen.findByRole("button", { name: "Edit" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Deactivate" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete" })).toBeInTheDocument();
  });

  it("an agent of another Workspace shows no Edit, Deactivate or Delete", async () => {
    apiClient.get.mockResolvedValue({ data: { data: agent(3) } });
    renderDetail();
    await screen.findByText("Helper");
    expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Deactivate" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
  });

  it("the test interface needs agents:execute in the agent's Workspace", async () => {
    apiClient.get.mockResolvedValue({ data: { data: agent(2) } });
    renderDetail();
    await screen.findByText("Helper");
    expect(screen.queryByText("Test Interface")).not.toBeInTheDocument();
  });

  it("a 404 (an agent in another Workspace looks missing) renders the not-found panel", async () => {
    apiClient.get.mockRejectedValue({ response: { status: 404, data: { errors: [{ detail: "Agent not found" }] } } });
    renderDetail();
    expect(await screen.findByTestId("admin-not-found")).toBeInTheDocument();
  });
});
