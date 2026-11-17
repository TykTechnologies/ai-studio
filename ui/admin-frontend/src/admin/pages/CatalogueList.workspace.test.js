import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import CatalogueList from "./CatalogueList";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";
import { clearIdentity } from "../utils/identityStore";
import { resetWorkspaceNames } from "../components/workspaces/workspaceFields";

// Workspaces E4: the model provider catalog list shows each catalog's Workspace, filters by
// Workspace, and gates edit/delete in the catalog's Workspace (catalogs are
// full-scoped: the server lists only the ones the caller can see).

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
}));

const catalogs = [
  { id: "1", attributes: { name: "W1 Catalog", llm_names: [], workspace_id: 2 } },
  { id: "2", attributes: { name: "W2 Catalog", llm_names: [], workspace_id: 3 } },
];

// An Editor scoped to W1 (id 2): read, write and delete only in W1 (W2's
// catalog is listed here to check the per-row gating).
const scoped = {
  id: "9",
  attributes: {
    is_admin: false,
    has_admin_access: true,
    rbac_enabled: true,
    permissions: [],
    workspaces: [{ id: 2, name: "W1", permissions: ["catalogues:read", "catalogues:write", "catalogues:delete"] }],
  },
};
const community = { id: "1", attributes: { is_admin: true, has_admin_access: true, rbac_enabled: false, permissions: ["*"] } };

const renderWith = (identity, path = "/admin/catalogs/llms") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <PermissionsProvider identity={identity}>
          <CatalogueList />
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

describe("CatalogueList Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    apiClient.get.mockImplementation((url) =>
      url === "/workspaces"
        ? Promise.resolve({ data: { data: [{ id: "2", attributes: { name: "W1" } }] } })
        : Promise.resolve({ data: { data: catalogs }, headers: { "x-total-count": "2", "x-total-pages": "1" } }),
    );
  });

  it("Enterprise: shows the Workspace column and filter; another Workspace's name is not shown", async () => {
    renderWith(scoped);
    await screen.findByText("W1 Catalog");
    expect(screen.getByRole("columnheader", { name: "Workspace" })).toBeInTheDocument();
    expect(screen.getByTestId("workspace-filter")).toBeInTheDocument();
    expect(within(screen.getByRole("row", { name: /W1 Catalog/ })).getByTestId("workspace-cell")).toHaveTextContent("W1");
    expect(within(screen.getByRole("row", { name: /W2 Catalog/ })).getByTestId("workspace-cell")).toHaveTextContent("Other Workspace");
  });

  it("Community: no Workspace column or filter", async () => {
    renderWith(community);
    await screen.findByText("W1 Catalog");
    expect(screen.queryByRole("columnheader", { name: "Workspace" })).not.toBeInTheDocument();
    expect(screen.queryByTestId("workspace-filter")).not.toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/workspaces");
  });

  it("a catalog of another Workspace shows no edit or delete controls", async () => {
    renderWith(scoped);
    let menu = await openRowMenu("W1 Catalog");
    expect(within(menu).getByRole("menuitem", { name: "Edit catalog" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Delete catalog" })).toBeInTheDocument();
    fireEvent.keyDown(menu, { key: "Escape" });

    menu = await openRowMenu("W2 Catalog");
    expect(within(menu).queryByRole("menuitem")).not.toBeInTheDocument();
  });

  it("the Add button shows for a caller who may write in a Workspace", async () => {
    renderWith(scoped);
    await screen.findByText("W1 Catalog");
    expect(screen.getByRole("button", { name: /Add catalog/ })).toBeInTheDocument();
  });

  it("?workspace_id= filters the list request", async () => {
    renderWith(scoped, "/admin/catalogs/llms?workspace_id=2");
    await screen.findByText("W1 Catalog");
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith("/catalogues", { params: expect.objectContaining({ workspace_id: 2 }) }),
    );
  });
});
