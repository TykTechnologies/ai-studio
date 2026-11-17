import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import EmbedderList from "./EmbedderList";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";
import { clearIdentity } from "../utils/identityStore";
import { resetWorkspaceNames } from "../components/workspaces/workspaceFields";

// Workspaces E3: the embedder list shows each one's Workspace, filters by
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
  { id: "1", attributes: { name: "W1 Embed", model: "text-embedding-3-small", vendor: "openai", privacy_score: 10, workspace_id: 2 } },
  { id: "2", attributes: { name: "W2 Embed", model: "nomic", vendor: "ollama", privacy_score: 10, workspace_id: 3 } },
];
const listResponse = { data: { data: rows, meta: { total_count: 2, total_pages: 1 } }, headers: {} };

// An Editor scoped to W1 (id 2): embedders:read is read-global, the write
// permissions only in W1.
const scoped = {
  id: "9",
  attributes: {
    is_admin: false,
    has_admin_access: true,
    rbac_enabled: true,
    permissions: [],
    workspaces: [{ id: 2, name: "W1", permissions: ["embedders:read", "embedders:write", "embedders:delete"] }],
  },
};
const community = { id: "1", attributes: { is_admin: true, has_admin_access: true, rbac_enabled: false, permissions: ["*"] } };

const renderWith = (identity, path = "/admin/embedders") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <PermissionsProvider identity={identity}>
          <EmbedderList />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const openRowMenu = async (name) => {
  const row = await screen.findByRole("row", { name: new RegExp(name) });
  fireEvent.click(within(row).getByRole("button", { name: `Actions for ${name}` }));
  return screen.findByRole("menu");
};

describe("EmbedderList Workspaces", () => {
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
    await screen.findByText("W1 Embed");
    expect(screen.getByRole("columnheader", { name: "Workspace" })).toBeInTheDocument();
    expect(screen.getByTestId("workspace-filter")).toBeInTheDocument();
    expect(within(screen.getByRole("row", { name: /W1 Embed/ })).getByTestId("workspace-cell")).toHaveTextContent("W1");
    expect(within(screen.getByRole("row", { name: /W2 Embed/ })).getByTestId("workspace-cell")).toHaveTextContent("Other Workspace");
  });

  it("Community: no Workspace column or filter", async () => {
    renderWith(community);
    await screen.findByText("W1 Embed");
    expect(screen.queryByRole("columnheader", { name: "Workspace" })).not.toBeInTheDocument();
    expect(screen.queryByTestId("workspace-filter")).not.toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/workspaces");
  });

  it("an object of another Workspace shows no edit or delete controls", async () => {
    renderWith(scoped);
    let menu = await openRowMenu("W1 Embed");
    expect(within(menu).getByRole("menuitem", { name: "Edit embedder" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Delete embedder" })).toBeInTheDocument();
    fireEvent.keyDown(menu, { key: "Escape" });

    menu = await openRowMenu("W2 Embed");
    expect(within(menu).queryByRole("menuitem")).not.toBeInTheDocument();
  });

  it("the Add button shows for a caller who may write in a Workspace", async () => {
    renderWith(scoped);
    await screen.findByText("W1 Embed");
    expect(screen.getByRole("button", { name: /Add embedder/ })).toBeInTheDocument();
  });

  it("?workspace_id= filters the list request", async () => {
    renderWith(scoped, "/admin/embedders?workspace_id=2");
    await screen.findByText("W1 Embed");
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith("/embedders", { params: expect.objectContaining({ workspace_id: 2 }) }),
    );
  });
});
