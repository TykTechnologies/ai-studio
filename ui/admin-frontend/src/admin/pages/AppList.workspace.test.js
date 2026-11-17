import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import AppList from "./AppList";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";
import { clearIdentity } from "../utils/identityStore";
import { resetWorkspaceNames } from "../components/workspaces/workspaceFields";

// Workspaces E4: the App list shows each App's Workspace, filters by
// Workspace, and gates edit/delete/credential actions in the App's Workspace.

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
jest.mock("../../components/common/Icon", () => (props) => <div data-testid="mock-icon">{props.name}</div>);
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
}));

const apps = [
  { id: "1", attributes: { name: "W1 Bot", description: "", user_id: 1, credential_id: 11, credential_active: false, workspace_id: 2 } },
  { id: "2", attributes: { name: "W2 Bot", description: "", user_id: 1, credential_id: 12, credential_active: false, workspace_id: 3 } },
];

// An Editor scoped to W1 (id 2) who reads Apps everywhere (a global reader):
// write, delete and credential approval only in W1.
const scoped = {
  id: "9",
  attributes: {
    is_admin: false,
    has_admin_access: true,
    rbac_enabled: true,
    permissions: ["apps:read"],
    workspaces: [{ id: 2, name: "W1", permissions: ["apps:read", "apps:write", "apps:delete", "credentials:write"] }],
  },
};
const community = { id: "1", attributes: { is_admin: true, has_admin_access: true, rbac_enabled: false, permissions: ["*"] } };

const renderWith = (identity, path = "/admin/apps") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <PermissionsProvider identity={identity}>
          <AppList />
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

describe("AppList Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    apiClient.get.mockImplementation((url) => {
      if (url === "/workspaces") return Promise.resolve({ data: { data: [{ id: "2", attributes: { name: "W1" } }] } });
      if (url === "/users") return Promise.resolve({ data: { data: [{ id: "1", attributes: { name: "Ada" } }] } });
      return Promise.resolve({ data: { data: apps }, headers: { "x-total-count": "2", "x-total-pages": "1" } });
    });
  });

  it("Enterprise: shows the Workspace column and filter; another Workspace's name is not shown", async () => {
    renderWith(scoped);
    await screen.findByText("W1 Bot");
    expect(screen.getByRole("columnheader", { name: "Workspace" })).toBeInTheDocument();
    expect(screen.getByTestId("workspace-filter")).toBeInTheDocument();
    expect(within(screen.getByRole("row", { name: /W1 Bot/ })).getByTestId("workspace-cell")).toHaveTextContent("W1");
    expect(within(screen.getByRole("row", { name: /W2 Bot/ })).getByTestId("workspace-cell")).toHaveTextContent("Other Workspace");
  });

  it("Community: no Workspace column or filter", async () => {
    renderWith(community);
    await screen.findByText("W1 Bot");
    expect(screen.queryByRole("columnheader", { name: "Workspace" })).not.toBeInTheDocument();
    expect(screen.queryByTestId("workspace-filter")).not.toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/workspaces");
  });

  it("an App of another Workspace shows no edit, delete or credential controls", async () => {
    renderWith(scoped);
    let menu = await openRowMenu("W1 Bot");
    expect(within(menu).getByRole("menuitem", { name: "Edit app" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Delete app" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: /Approve credentials/ })).toBeInTheDocument();
    fireEvent.keyDown(menu, { key: "Escape" });

    menu = await openRowMenu("W2 Bot");
    expect(within(menu).queryByRole("menuitem")).not.toBeInTheDocument();
  });

  it("the Add button shows for a caller who may write in a Workspace", async () => {
    renderWith(scoped);
    await screen.findByText("W1 Bot");
    expect(screen.getByRole("button", { name: /Add app/ })).toBeInTheDocument();
  });

  it("?workspace_id= filters the list request", async () => {
    renderWith(scoped, "/admin/apps?workspace_id=2");
    await screen.findByText("W1 Bot");
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith("/apps", { params: expect.objectContaining({ workspace_id: 2 }) }),
    );
  });
});
