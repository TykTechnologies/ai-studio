import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import EmbedderForm from "./EmbedderForm";
import apiClient from "../../utils/apiClient";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";
import { resetWorkspaceNames } from "../workspaces/workspaceFields";

// Workspaces E3: the embedder form's Workspace select (landing on create,
// move on edit) and its errors.

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
const mockNavigate = jest.fn();
let mockParams = {};
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => mockParams,
}));

const identity = (attributes) => ({ id: "9", attributes: { has_admin_access: true, permissions: [], ...attributes } });
const scopedW1 = identity({
  rbac_enabled: true,
  workspaces: [{ id: 2, name: "W1", permissions: ["embedders:read", "embedders:write"] }],
});
const scopedTwo = identity({
  rbac_enabled: true,
  workspaces: [
    { id: 2, name: "W1", permissions: ["embedders:read", "embedders:write"] },
    { id: 3, name: "W2", permissions: ["embedders:read", "embedders:write"] },
  ],
});
const community = identity({ is_admin: true, rbac_enabled: false, permissions: ["*"] });

const saved = {
  id: "1",
  attributes: {
    name: "Shared",
    vendor: "openai",
    endpoint: "https://api.openai.com/v1",
    api_key: "[redacted]",
    has_api_key: true,
    model: "text-embedding-3-small",
    privacy_score: 60,
    workspace_id: 2,
  },
};

const renderForm = (who) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={who}>
          <EmbedderForm />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const fillRequired = async () => {
  fireEvent.change(screen.getByTestId("embedder-name"), { target: { value: "Docs" } });
  fireEvent.change(screen.getByTestId("embedder-model"), { target: { value: "nomic-embed-text" } });
  fireEvent.mouseDown(screen.getByRole("combobox", { name: /API compatibility/ }));
  const options = await screen.findAllByRole("option");
  fireEvent.click(options[0]);
};

const sentAttributes = (method) => apiClient[method].mock.calls[0][1].data.attributes;

describe("EmbedderForm Workspace select", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    mockParams = {};
    apiClient.get.mockImplementation((url) => {
      if (url === "/embedders/1") return Promise.resolve({ data: { data: saved } });
      if (url === "/embedders/1/dependents") return Promise.resolve({ data: { data: { attributes: { datasources: [], total: 0 } } } });
      if (url === "/embedders/vendors") return Promise.resolve({ data: { data: ["openai", "ollama"] } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.post.mockResolvedValue({ data: { data: { id: "7" } } });
    apiClient.patch.mockResolvedValue({ data: { data: saved } });
  });

  it("create with one Workspace preselects it and sends it", async () => {
    renderForm(scopedW1);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    expect(screen.getByRole("combobox", { name: "Workspace" })).toHaveAttribute("aria-disabled", "true");
    await fillRequired();
    fireEvent.click(screen.getByRole("button", { name: "Create embedder" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    expect(sentAttributes("post").workspace_id).toBe(2);
  });

  it("shows workspace_required on the Workspace field", async () => {
    apiClient.post.mockRejectedValue({
      response: { status: 422, data: { errors: [{ title: "Workspace required", detail: "name one", code: "workspace_required" }] } },
    });
    renderForm(scopedTwo);
    await screen.findByTestId("workspace-select");
    await fillRequired();
    fireEvent.click(screen.getByRole("button", { name: "Create embedder" }));
    expect(await screen.findAllByText(/choose one/)).not.toHaveLength(0);
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("edit: an unchanged Workspace is not sent; picking another is a move", async () => {
    mockParams = { id: "1" };
    renderForm(scopedTwo);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    await waitFor(() => expect(screen.getByTestId("embedder-model")).toHaveValue("text-embedding-3-small"));
    fireEvent.click(screen.getByRole("button", { name: "Save embedder" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch")).not.toHaveProperty("workspace_id");

    apiClient.patch.mockClear();
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Workspace" }));
    fireEvent.click(await screen.findByRole("option", { name: "W2" }));
    fireEvent.click(screen.getByRole("button", { name: "Save embedder" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch").workspace_id).toBe(3);
  });

  it("Community: no Workspace select and no workspace_id sent", async () => {
    renderForm(community);
    await screen.findByRole("combobox", { name: /API compatibility/ });
    await fillRequired();
    expect(screen.queryByTestId("workspace-select")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Create embedder" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    expect(sentAttributes("post")).not.toHaveProperty("workspace_id");
  });
});
