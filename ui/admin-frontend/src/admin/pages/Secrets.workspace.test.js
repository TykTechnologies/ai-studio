import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import Secrets from "./Secrets";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";
import { clearIdentity } from "../utils/identityStore";
import { resetWorkspaceNames } from "../components/workspaces/workspaceFields";

// Workspaces E3: the secret list shows each one's Workspace, filters by
// Workspace, and gates edit/delete in the object's Workspace.

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
jest.mock("../utils/pubClient", () => ({ __esModule: true, default: { get: jest.fn() } }));
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
}));

const rows = [
  { id: "1", attributes: { var_name: "W1_KEY", has_value: true, referenced_by: [], workspace_id: 2 } },
  { id: "2", attributes: { var_name: "W2_KEY", has_value: true, referenced_by: [], workspace_id: 3 } },
];
const listResponse = { data: { data: rows }, headers: { "x-total-count": "2", "x-total-pages": "1" } };

// An Editor scoped to W1 (id 2): secrets:read is read-global, the write
// permissions only in W1.
const scoped = {
  id: "9",
  attributes: {
    is_admin: false,
    has_admin_access: true,
    rbac_enabled: true,
    permissions: [],
    workspaces: [{ id: 2, name: "W1", permissions: ["secrets:read", "secrets:write"] }],
  },
};
const community = { id: "1", attributes: { is_admin: true, has_admin_access: true, rbac_enabled: false, permissions: ["*"] } };

const renderWith = (identity, path = "/admin/secrets") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <PermissionsProvider identity={identity}>
          <Secrets />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const openRowMenu = async (name) => {
  const row = await screen.findByRole("row", { name: new RegExp(name) });
  fireEvent.click(within(row).getByRole("button", { name: `Actions for ${name}` }));
  return screen.findByRole("menu");
};

describe("Secrets Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    apiClient.get.mockImplementation((url) =>
      url === "/workspaces"
        ? Promise.resolve({ data: { data: [{ id: "2", attributes: { name: "W1" } }] } })
        : Promise.resolve(listResponse),
    );
  });

  it("Enterprise: shows the Workspace column and filter; another Workspace's name is not shown", async () => {
    renderWith(scoped);
    await screen.findByText("W1_KEY");
    expect(screen.getByRole("columnheader", { name: "Workspace" })).toBeInTheDocument();
    expect(screen.getByTestId("workspace-filter")).toBeInTheDocument();
    expect(within(screen.getByRole("row", { name: /W1_KEY/ })).getByTestId("workspace-cell")).toHaveTextContent("W1");
    expect(within(screen.getByRole("row", { name: /W2_KEY/ })).getByTestId("workspace-cell")).toHaveTextContent("Other Workspace");
  });

  it("Community: no Workspace column or filter", async () => {
    renderWith(community);
    await screen.findByText("W1_KEY");
    expect(screen.queryByRole("columnheader", { name: "Workspace" })).not.toBeInTheDocument();
    expect(screen.queryByTestId("workspace-filter")).not.toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/workspaces");
  });

  it("an object of another Workspace shows no edit or delete controls", async () => {
    renderWith(scoped);
    let menu = await openRowMenu("W1_KEY");
    expect(within(menu).getByRole("menuitem", { name: "Edit secret" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Delete secret" })).toBeInTheDocument();
    fireEvent.keyDown(menu, { key: "Escape" });

    menu = await openRowMenu("W2_KEY");
    expect(within(menu).queryByRole("menuitem")).not.toBeInTheDocument();
  });

  it("the Add button shows for a caller who may write in a Workspace", async () => {
    renderWith(scoped);
    await screen.findByText("W1_KEY");
    expect(screen.getByRole("button", { name: /Add secret/ })).toBeInTheDocument();
  });

  it("?workspace_id= filters the list request", async () => {
    renderWith(scoped, "/admin/secrets?workspace_id=2");
    await screen.findByText("W1_KEY");
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith("/secrets", { params: expect.objectContaining({ workspace_id: 2 }) }),
    );
  });
});
