import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import ToolList from "./ToolList";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";
import { clearIdentity } from "../utils/identityStore";
import { resetWorkspaceNames } from "../components/workspaces/workspaceFields";

// Workspaces E3: the tool list shows each tool's Workspace, filters by
// Workspace, and gates edit/delete/publish in the tool's Workspace.

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
jest.mock("../utils/pubClient", () => ({ __esModule: true, default: { get: jest.fn() } }));
jest.mock("../hooks/useSystemFeatures", () => ({ __esModule: true, default: () => ({ features: {} }) }));
jest.mock("../hooks/useConfig", () => ({ __esModule: true, default: () => ({ getDocsLink: () => "" }) }));
jest.mock("../components/tools/ImportOpenAPIWizard", () => ({ __esModule: true, default: () => null }));
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
}));

const tools = [
  { id: "1", attributes: { name: "W1 Weather", description: "d", privacy_score: 2, active: true, workspace_id: 2 } },
  { id: "2", attributes: { name: "W2 Search", description: "d", privacy_score: 2, active: true, workspace_id: 3 } },
];

// An Editor scoped to W1 (id 2): tools:read is read-global, write and
// publish only in W1.
const scoped = {
  id: "9",
  attributes: {
    is_admin: false,
    has_admin_access: true,
    rbac_enabled: true,
    permissions: [],
    workspaces: [{ id: 2, name: "W1", permissions: ["tools:read", "tools:write", "tools:delete", "tools:publish"] }],
  },
};
const community = { id: "1", attributes: { is_admin: true, has_admin_access: true, rbac_enabled: false, permissions: ["*"] } };

const renderWith = (identity, path = "/admin/tools") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <PermissionsProvider identity={identity}>
          <ToolList />
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

describe("ToolList Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    apiClient.get.mockImplementation((url) =>
      url === "/workspaces"
        ? Promise.resolve({ data: { data: [{ id: "2", attributes: { name: "W1" } }] } })
        : Promise.resolve({ data: { data: tools }, headers: { "x-total-count": "2", "x-total-pages": "1" } }),
    );
  });

  it("Enterprise: shows the Workspace column and filter; another Workspace's name is not shown", async () => {
    renderWith(scoped);
    await screen.findByText("W1 Weather");
    expect(screen.getByRole("columnheader", { name: "Workspace" })).toBeInTheDocument();
    expect(screen.getByTestId("workspace-filter")).toBeInTheDocument();
    expect(within(screen.getByRole("row", { name: /W1 Weather/ })).getByTestId("workspace-cell")).toHaveTextContent("W1");
    expect(within(screen.getByRole("row", { name: /W2 Search/ })).getByTestId("workspace-cell")).toHaveTextContent("Other Workspace");
  });

  it("Community: no Workspace column or filter", async () => {
    renderWith(community);
    await screen.findByText("W1 Weather");
    expect(screen.queryByRole("columnheader", { name: "Workspace" })).not.toBeInTheDocument();
    expect(screen.queryByTestId("workspace-filter")).not.toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/workspaces");
  });

  it("a tool of another Workspace shows no edit, delete or publish controls", async () => {
    renderWith(scoped);
    let menu = await openRowMenu("W1 Weather");
    expect(within(menu).getByRole("menuitem", { name: "Edit tool" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Delete tool" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Deactivate tool" })).toBeInTheDocument();
    fireEvent.keyDown(menu, { key: "Escape" });

    menu = await openRowMenu("W2 Search");
    expect(within(menu).queryByRole("menuitem")).not.toBeInTheDocument();
  });

  it("the Add and Import buttons show for a caller who may write in a Workspace", async () => {
    renderWith(scoped);
    await screen.findByText("W1 Weather");
    expect(screen.getByRole("button", { name: /Add tool/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Import OpenAPI/ })).toBeInTheDocument();
  });

  it("?workspace_id= filters the list request", async () => {
    renderWith(scoped, "/admin/tools?workspace_id=2");
    await screen.findByText("W1 Weather");
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith("/tools", { params: expect.objectContaining({ workspace_id: 2 }) }),
    );
  });
});
