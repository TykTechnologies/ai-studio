import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import LLMList from "./LLMList";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";
import { clearIdentity } from "../utils/identityStore";
import { resetWorkspaceNames } from "../components/workspaces/workspaceFields";

// Workspaces E3: the model provider list shows each one's Workspace, filters
// by Workspace, and gates edit/delete/publish in the object's Workspace.

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
jest.mock("../utils/pubClient", () => ({ __esModule: true, default: { get: jest.fn() } }));
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
}));

const llms = [
  { id: "1", attributes: { name: "W1 OpenAI", vendor: "openai", privacy_score: 2, active: true, credential_status: "ok", workspace_id: 2 } },
  { id: "2", attributes: { name: "W2 Claude", vendor: "anthropic", privacy_score: 2, active: true, credential_status: "ok", workspace_id: 3 } },
];

// An Editor scoped to W1 (id 2): llms:read is read-global, write and
// publish only in W1.
const scoped = {
  id: "9",
  attributes: {
    is_admin: false,
    has_admin_access: true,
    rbac_enabled: true,
    permissions: [],
    workspaces: [{ id: 2, name: "W1", permissions: ["llms:read", "llms:write", "llms:publish"] }],
  },
};
const community = { id: "1", attributes: { is_admin: true, has_admin_access: true, rbac_enabled: false, permissions: ["*"] } };

const renderWith = (identity, path = "/admin/llms") =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <PermissionsProvider identity={identity}>
          <LLMList />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const openRowMenu = async (name) => {
  const row = (await screen.findByText(name)).closest("tr");
  fireEvent.click(within(row).getByRole("button", { name: `Actions for ${name}` }));
  return screen.findByRole("menu");
};

describe("LLMList Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    apiClient.get.mockImplementation((url) =>
      url === "/workspaces"
        ? Promise.resolve({ data: { data: [{ id: "2", attributes: { name: "W1" } }] } })
        : Promise.resolve({ data: { data: llms }, headers: { "x-total-count": "2", "x-total-pages": "1" } }),
    );
  });

  it("Enterprise: shows the Workspace column and filter; another Workspace's name is not shown", async () => {
    renderWith(scoped);
    await screen.findByText("W1 OpenAI");
    expect(screen.getByRole("columnheader", { name: "Workspace" })).toBeInTheDocument();
    expect(screen.getByTestId("workspace-filter")).toBeInTheDocument();
    expect(within(screen.getByText("W1 OpenAI").closest("tr")).getByTestId("workspace-cell")).toHaveTextContent("W1");
    expect(within(screen.getByText("W2 Claude").closest("tr")).getByTestId("workspace-cell")).toHaveTextContent("Other Workspace");
  });

  it("Community: no Workspace column or filter", async () => {
    renderWith(community);
    await screen.findByText("W1 OpenAI");
    expect(screen.queryByRole("columnheader", { name: "Workspace" })).not.toBeInTheDocument();
    expect(screen.queryByTestId("workspace-filter")).not.toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/workspaces");
  });

  it("an object of another Workspace shows no edit, delete or publish controls", async () => {
    renderWith(scoped);
    let menu = await openRowMenu("W1 OpenAI");
    expect(within(menu).getByRole("menuitem", { name: "Edit model provider" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Delete model provider" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Deactivate model provider" })).toBeInTheDocument();
    fireEvent.keyDown(menu, { key: "Escape" });

    menu = await openRowMenu("W2 Claude");
    expect(within(menu).queryByRole("menuitem")).not.toBeInTheDocument();
  });

  it("the Add button shows for a caller who may write in a Workspace", async () => {
    renderWith(scoped);
    await screen.findByText("W1 OpenAI");
    expect(screen.getByRole("button", { name: /Add model provider/ })).toBeInTheDocument();
  });

  it("?workspace_id= filters the list request", async () => {
    renderWith(scoped, "/admin/llms?workspace_id=2");
    await screen.findByText("W1 OpenAI");
    await waitFor(() =>
      expect(apiClient.get).toHaveBeenCalledWith("/llms", { params: expect.objectContaining({ workspace_id: 2 }) }),
    );
  });
});
