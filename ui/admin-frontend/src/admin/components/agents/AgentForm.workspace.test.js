import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import AgentForm from "./AgentForm";
import apiClient from "../../utils/apiClient";
import pluginService from "../../services/pluginService";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";
import { resetWorkspaceNames } from "../workspaces/workspaceFields";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));
jest.mock("../../services/pluginService", () => ({
  __esModule: true,
  default: { listPlugins: jest.fn(), getPlugin: jest.fn() },
}));
const mockNavigate = jest.fn();
let mockParams = {};
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => mockParams,
}));

// Workspaces E4: the agent form's Workspace select (landing on create, move
// on edit), the App picker for an App the caller cannot see, and the
// not-found panel for an agent in another Workspace.

const identity = (attributes) => ({ id: "9", attributes: { has_admin_access: true, permissions: [], ...attributes } });
const scopedW1 = identity({
  rbac_enabled: true,
  workspaces: [{ id: 2, name: "W1", permissions: ["agents:read", "agents:write", "agents:publish", "apps:read"] }],
});
const scopedTwo = identity({
  rbac_enabled: true,
  workspaces: [
    { id: 2, name: "W1", permissions: ["agents:read", "agents:write"] },
    { id: 3, name: "W2", permissions: ["agents:read", "agents:write"] },
  ],
});

// The visible Apps (the server hides those in other Workspaces).
const apps = [{ id: "10", attributes: { name: "Visible App" } }];

const storedAgent = (attrs = {}) => ({
  data: {
    data: {
      id: 5,
      name: "Helper",
      description: "",
      plugin_id: 1,
      app_id: 10,
      config: {},
      groups: [],
      is_active: true,
      namespace: "",
      workspace_id: 2,
      ...attrs,
    },
  },
});

const renderForm = (who) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={who}>
          <AgentForm />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const pick = async (label, option) => {
  fireEvent.mouseDown(screen.getByRole("combobox", { name: label }));
  fireEvent.click(await screen.findByRole("option", { name: option }));
};

const mockGets = (agentResult) => {
  apiClient.get.mockImplementation((url) => {
    if (url === "/apps") return Promise.resolve({ data: { data: apps } });
    if (url === "/agents/5") return agentResult;
    return Promise.resolve({ data: { data: [] } });
  });
};

describe("AgentForm Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    mockParams = {};
    pluginService.listPlugins.mockResolvedValue({ data: [{ id: 1, name: "Agent plugin", hookType: "agent", isActive: true }] });
    pluginService.getPlugin.mockResolvedValue({});
    mockGets(Promise.resolve(storedAgent()));
    apiClient.post.mockResolvedValue({ data: { data: storedAgent().data.data } });
    apiClient.put.mockResolvedValue({ data: { data: storedAgent().data.data } });
    jest.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    console.error.mockRestore();
  });

  it("create with one Workspace preselects it and sends it", async () => {
    renderForm(scopedW1);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Helper" } });
    await pick("Plugin", "Agent plugin");
    await pick("App", "Visible App");
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    expect(apiClient.post.mock.calls[0][1].workspace_id).toBe(2);
  });

  it("shows workspace_required on the Workspace field", async () => {
    apiClient.post.mockRejectedValue({
      response: { status: 422, data: { errors: [{ title: "Workspace required", detail: "name one", code: "workspace_required" }] } },
    });
    renderForm(scopedTwo);
    await screen.findByTestId("workspace-select");
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Helper" } });
    await pick("Plugin", "Agent plugin");
    await pick("App", "Visible App");
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(await screen.findAllByText(/choose one/)).not.toHaveLength(0);
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("edit: an unchanged Workspace is not sent; picking another is a move", async () => {
    mockParams = { id: "5" };
    renderForm(scopedTwo);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    fireEvent.click(screen.getByRole("button", { name: "Update" }));
    await waitFor(() => expect(apiClient.put).toHaveBeenCalled());
    expect(apiClient.put.mock.calls[0][1]).not.toHaveProperty("workspace_id");

    apiClient.put.mockClear();
    await waitFor(() => expect(screen.getByRole("button", { name: "Update" })).toBeEnabled());
    await pick("Workspace", "W2");
    fireEvent.click(screen.getByRole("button", { name: "Update" }));
    await waitFor(() => expect(apiClient.put).toHaveBeenCalled());
    expect(apiClient.put.mock.calls[0][1].workspace_id).toBe(3);
  });

  it("keeps an App the caller cannot see, under a neutral label, and sends it unchanged", async () => {
    // An App id that is not in the /apps list the caller gets.
    const HIDDEN_APP_ID = 77;
    mockParams = { id: "5" };
    mockGets(Promise.resolve(storedAgent({ app_id: HIDDEN_APP_ID })));
    renderForm(scopedW1);
    const appSelect = await screen.findByRole("combobox", { name: "App" });
    await waitFor(() => expect(appSelect).toHaveTextContent("App in another Workspace"));
    fireEvent.click(screen.getByRole("button", { name: "Update" }));
    await waitFor(() => expect(apiClient.put).toHaveBeenCalled());
    expect(apiClient.put.mock.calls[0][1].app_id).toBe(HIDDEN_APP_ID);
  });

  it("an agent the caller cannot see (404) renders the not-found panel", async () => {
    mockParams = { id: "5" };
    mockGets(Promise.reject({ response: { status: 404, data: { errors: [{ detail: "Agent not found" }] } } }));
    renderForm(scopedW1);
    expect(await screen.findByTestId("admin-not-found")).toBeInTheDocument();
  });
});
