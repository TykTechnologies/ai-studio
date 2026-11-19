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

// Workspaces .54: cloning a data source asks where the copy goes when the
// caller can write data sources in more than one Workspace.

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
const two = {
  id: "9",
  attributes: {
    is_admin: false,
    has_admin_access: true,
    rbac_enabled: true,
    permissions: [],
    workspaces: [
      { id: 2, name: "W1", permissions: ["datasources:read", "datasources:write"] },
      { id: 3, name: "W2", permissions: ["datasources:read", "datasources:write"] },
    ],
  },
};

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

describe("DatasourceList clone and Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    apiClient.get.mockImplementation((url) =>
      url === "/workspaces"
        ? Promise.resolve({ data: { data: [] } })
        : Promise.resolve({ data: { data: datasources }, headers: { "x-total-count": "2", "x-total-pages": "1" } }),
    );
    apiClient.post.mockResolvedValue({ data: { data: { id: "9" } } });
  });

  it("with one Workspace clones straight away (landing rule)", async () => {
    renderWith(scoped);
    const menu = await openRowMenu("W1 Docs");
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Clone data source" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/datasources/1/clone", undefined));
    expect(screen.queryByTestId("workspace-pick-dialog")).not.toBeInTheDocument();
  });

  it("with several Workspaces asks where the copy goes and sends it", async () => {
    renderWith(two);
    const menu = await openRowMenu("W1 Docs");
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Clone data source" }));
    const dialog = await screen.findByTestId("workspace-pick-dialog");
    expect(apiClient.post).not.toHaveBeenCalled();
    fireEvent.mouseDown(within(dialog).getByRole("combobox", { name: "Workspace" }));
    fireEvent.click(await screen.findByRole("option", { name: "W2" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Clone" }));
    await waitFor(() =>
      expect(apiClient.post).toHaveBeenCalledWith("/datasources/1/clone", { data: { attributes: { workspace_id: 3 } } }),
    );
  });

  it("a Workspace refusal is shown in the dialog", async () => {
    apiClient.post.mockRejectedValue({
      response: { status: 422, data: { errors: [{ title: "Invalid Workspace", detail: "x", code: "invalid_workspace" }] } },
    });
    renderWith(two);
    const menu = await openRowMenu("W1 Docs");
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Clone data source" }));
    const dialog = await screen.findByTestId("workspace-pick-dialog");
    fireEvent.mouseDown(within(dialog).getByRole("combobox", { name: "Workspace" }));
    fireEvent.click(await screen.findByRole("option", { name: "W1" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Clone" }));
    expect(await within(dialog).findByText(/does not exist or you cannot create/)).toBeInTheDocument();
  });
});
