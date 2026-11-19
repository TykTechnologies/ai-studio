import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import ModelRouterList from "./ModelRouterList";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";
import { clearIdentity } from "../utils/identityStore";
import { resetWorkspaceNames } from "../components/workspaces/workspaceFields";

// Workspaces E4: the model router list shows each router's Workspace, filters by
// Workspace, and gates edit/delete/toggle in the router's Workspace.

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
jest.mock("../utils/pubClient", () => ({ __esModule: true, default: { get: jest.fn() } }));
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
}));

const routers = [
  { id: "1", attributes: { name: "W1 router", slug: "w1", active: true, pools: [], routes: [], workspace_id: 2 } },
  { id: "2", attributes: { name: "W2 router", slug: "w2", active: true, pools: [], routes: [], workspace_id: 3 } },
];

// An Editor scoped to W1 (id 2): read, write, publish and delete only in W1.
const scoped = {
  id: "9",
  attributes: {
    is_admin: false,
    has_admin_access: true,
    rbac_enabled: true,
    permissions: [],
    workspaces: [{ id: 2, name: "W1", permissions: ["model-routers:read", "model-routers:write", "model-routers:delete", "model-routers:publish"] }],
  },
};
const community = { id: "1", attributes: { is_admin: true, has_admin_access: true, rbac_enabled: false, permissions: ["*"] } };

const renderWith = (identity, path = "/admin/model-routers") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <PermissionsProvider identity={identity}>
          <ModelRouterList />
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

describe("ModelRouterList Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    apiClient.get.mockImplementation((url) =>
      url === "/workspaces"
        ? Promise.resolve({ data: { data: [{ id: "2", attributes: { name: "W1" } }] } })
        : Promise.resolve({ data: { data: routers }, headers: { "x-total-count": "2", "x-total-pages": "1" } }),
    );
  });

  it("Enterprise: shows the Workspace column and filter; another Workspace's name is not shown", async () => {
    renderWith(scoped);
    await screen.findByText("W1 router");
    expect(screen.getByRole("columnheader", { name: "Workspace" })).toBeInTheDocument();
    expect(screen.getByTestId("workspace-filter")).toBeInTheDocument();
    expect(within(screen.getByRole("row", { name: /W1 router/ })).getByTestId("workspace-cell")).toHaveTextContent("W1");
    expect(within(screen.getByRole("row", { name: /W2 router/ })).getByTestId("workspace-cell")).toHaveTextContent("Other Workspace");
  });

  it("Community: no Workspace column or filter", async () => {
    renderWith(community);
    await screen.findByText("W1 router");
    expect(screen.queryByRole("columnheader", { name: "Workspace" })).not.toBeInTheDocument();
    expect(screen.queryByTestId("workspace-filter")).not.toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/workspaces");
  });

  it("a router of another Workspace shows no edit, delete or toggle controls", async () => {
    renderWith(scoped);
    let menu = await openRowMenu("W1 router");
    expect(within(menu).getByRole("menuitem", { name: "Edit Router" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Delete Router" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Deactivate Router" })).toBeInTheDocument();
    fireEvent.keyDown(menu, { key: "Escape" });

    menu = await openRowMenu("W2 router");
    expect(within(menu).queryByRole("menuitem")).not.toBeInTheDocument();
  });

  it("the Add button shows for a caller who may write in a Workspace", async () => {
    renderWith(scoped);
    await screen.findByText("W1 router");
    expect(screen.getByRole("button", { name: /Add Router/ })).toBeInTheDocument();
  });

  it("?workspace_id= filters the list request", async () => {
    renderWith(scoped, "/admin/model-routers?workspace_id=2");
    await screen.findByText("W1 router");
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith("/model-routers", { params: expect.objectContaining({ workspace_id: 2 }) }),
    );
  });
});
