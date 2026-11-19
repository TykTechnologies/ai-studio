import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import AgentList from "./AgentList";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";
import { clearIdentity } from "../utils/identityStore";
import { resetWorkspaceNames } from "../components/workspaces/workspaceFields";

// Workspaces E4: the agent list shows each agent's Workspace, filters by
// Workspace, and gates edit/delete/publish in the agent's Workspace.

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
}));

const agents = [
  { id: 1, name: "W1 Agent", plugin: { name: "P" }, app: { name: "A" }, groups: [], is_active: true, workspace_id: 2 },
  { id: 2, name: "W2 Agent", plugin: { name: "P" }, app: { name: "B" }, groups: [], is_active: true, workspace_id: 3 },
];

// Reads agents everywhere (a global reader); write, delete and publish in W1 (id 2) only.
const scoped = {
  id: "9",
  attributes: {
    is_admin: false,
    has_admin_access: true,
    rbac_enabled: true,
    permissions: ["agents:read"],
    workspaces: [{ id: 2, name: "W1", permissions: ["agents:read", "agents:write", "agents:delete", "agents:publish"] }],
  },
};
const community = { id: "1", attributes: { is_admin: true, has_admin_access: true, rbac_enabled: false, permissions: ["*"] } };

const renderWith = (identity, path = "/admin/agents") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <PermissionsProvider identity={identity}>
          <AgentList />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const openRowMenu = async (name) => {
  await screen.findByText(name);
  const row = screen.getByRole("row", { name: new RegExp(name) });
  fireEvent.click(within(row).getByRole("button", { name: `Actions for ${name}` }));
  return screen.findByRole("menu");
};

describe("AgentList Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    apiClient.get.mockImplementation((url) =>
      url === "/workspaces"
        ? Promise.resolve({ data: { data: [{ id: "2", attributes: { name: "W1" } }] } })
        : Promise.resolve({ data: { data: agents, meta: { total: 2 } } }),
    );
  });

  it("Enterprise: shows the Workspace column and filter; another Workspace's name is not shown", async () => {
    renderWith(scoped);
    await screen.findByText("W1 Agent");
    expect(screen.getByRole("columnheader", { name: "Workspace" })).toBeInTheDocument();
    expect(screen.getByTestId("workspace-filter")).toBeInTheDocument();
    await waitFor(() =>
      expect(within(screen.getByRole("row", { name: /W1 Agent/ })).getByTestId("workspace-cell")).toHaveTextContent("W1"),
    );
    expect(within(screen.getByRole("row", { name: /W2 Agent/ })).getByTestId("workspace-cell")).toHaveTextContent("Other Workspace");
  });

  it("Community: no Workspace column or filter", async () => {
    renderWith(community);
    await screen.findByText("W1 Agent");
    expect(screen.queryByRole("columnheader", { name: "Workspace" })).not.toBeInTheDocument();
    expect(screen.queryByTestId("workspace-filter")).not.toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/workspaces");
  });

  it("an agent of another Workspace offers only View Details", async () => {
    renderWith(scoped);
    let menu = await openRowMenu("W1 Agent");
    expect(within(menu).getByRole("menuitem", { name: "Edit" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Delete" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Deactivate" })).toBeInTheDocument();
    fireEvent.keyDown(menu, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("menu")).not.toBeInTheDocument());

    menu = await openRowMenu("W2 Agent");
    expect(within(menu).getAllByRole("menuitem").map((el) => el.textContent)).toEqual(["View Details"]);
  });

  it("the Create button shows for a caller who may write in a Workspace", async () => {
    renderWith(scoped);
    await screen.findByText("W1 Agent");
    expect(screen.getByRole("button", { name: /Create Agent/ })).toBeInTheDocument();
  });

  it("?workspace_id= filters the list request", async () => {
    renderWith(scoped, "/admin/agents?workspace_id=2");
    await screen.findByText("W1 Agent");
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith("/agents", { params: expect.objectContaining({ workspace_id: 2 }) }),
    );
  });
});
