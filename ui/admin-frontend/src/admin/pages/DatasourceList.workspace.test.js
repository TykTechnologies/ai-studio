import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import DatasourceList from "./DatasourceList";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";
import { clearIdentity } from "../utils/identityStore";
import { resetWorkspaceNames } from "../components/workspaces/workspaceFields";

// Workspaces E3: the data source list shows each one's Workspace, filters by
// Workspace, and gates edit/delete/publish in the data source's Workspace.

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
jest.mock("../utils/pubClient", () => ({ __esModule: true, default: { get: jest.fn() } }));
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
}));

const datasources = [
  { id: "1", attributes: { name: "W1 Docs", short_description: "d", db_source_type: "qdrant", privacy_score: 2, tags: [], active: true, workspace_id: 2 } },
  { id: "2", attributes: { name: "W2 Wiki", short_description: "d", db_source_type: "qdrant", privacy_score: 2, tags: [], active: true, workspace_id: 3 } },
];

// An Editor scoped to W1 (id 2): datasources:read is read-global, write and
// publish only in W1.
const scoped = {
  id: "9",
  attributes: {
    is_admin: false,
    has_admin_access: true,
    rbac_enabled: true,
    permissions: [],
    workspaces: [{ id: 2, name: "W1", permissions: ["datasources:read", "datasources:write", "datasources:publish"] }],
  },
};
const community = { id: "1", attributes: { is_admin: true, has_admin_access: true, rbac_enabled: false, permissions: ["*"] } };

const renderWith = (identity, path = "/admin/datasources") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <PermissionsProvider identity={identity}>
          <DatasourceList />
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

describe("DatasourceList Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    apiClient.get.mockImplementation((url) => {
      if (url === "/workspaces") return Promise.resolve({ data: { data: [{ id: "2", attributes: { name: "W1" } }] } });
      if (url.startsWith("/vendors/")) return Promise.resolve({ data: { data: [] } });
      return Promise.resolve({ data: { data: datasources }, headers: { "x-total-count": "2", "x-total-pages": "1" } });
    });
  });

  it("Enterprise: shows the Workspace column and filter; another Workspace's name is not shown", async () => {
    renderWith(scoped);
    await screen.findByText("W1 Docs");
    expect(screen.getByRole("columnheader", { name: "Workspace" })).toBeInTheDocument();
    expect(screen.getByTestId("workspace-filter")).toBeInTheDocument();
    expect(within(screen.getByRole("row", { name: /W1 Docs/ })).getByTestId("workspace-cell")).toHaveTextContent("W1");
    expect(within(screen.getByRole("row", { name: /W2 Wiki/ })).getByTestId("workspace-cell")).toHaveTextContent("Other Workspace");
  });

  it("Community: no Workspace column or filter", async () => {
    renderWith(community);
    await screen.findByText("W1 Docs");
    expect(screen.queryByRole("columnheader", { name: "Workspace" })).not.toBeInTheDocument();
    expect(screen.queryByTestId("workspace-filter")).not.toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/workspaces");
  });

  it("a data source of another Workspace shows no edit, clone, delete or publish controls", async () => {
    renderWith(scoped);
    let menu = await openRowMenu("W1 Docs");
    expect(within(menu).getByRole("menuitem", { name: "Edit data source" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Clone data source" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Delete data source" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Deactivate data source" })).toBeInTheDocument();
    fireEvent.keyDown(menu, { key: "Escape" });

    menu = await openRowMenu("W2 Wiki");
    expect(within(menu).queryByRole("menuitem")).not.toBeInTheDocument();
  });

  it("the Add button shows for a caller who may write in a Workspace", async () => {
    renderWith(scoped);
    await screen.findByText("W1 Docs");
    expect(screen.getByRole("button", { name: /Add data source/ })).toBeInTheDocument();
  });

  it("?workspace_id= filters the list request", async () => {
    renderWith(scoped, "/admin/datasources?workspace_id=2");
    await screen.findByText("W1 Docs");
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith("/datasources", { params: expect.objectContaining({ workspace_id: 2 }) }),
    );
  });
});
