import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import SecretForm from "./SecretForm";
import apiClient from "../../utils/apiClient";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";
import { resetWorkspaceNames } from "../workspaces/workspaceFields";

// Workspaces E3: the secret form's Workspace select (landing on create,
// move on edit) and the server's refusal to move a referenced secret.

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
  workspaces: [{ id: 2, name: "W1", permissions: ["secrets:read", "secrets:write"] }],
});
const scopedTwo = identity({
  rbac_enabled: true,
  workspaces: [
    { id: 2, name: "W1", permissions: ["secrets:read", "secrets:write"] },
    { id: 3, name: "W2", permissions: ["secrets:read", "secrets:write"] },
  ],
});
const community = identity({ is_admin: true, rbac_enabled: false, permissions: ["*"] });

const renderForm = (who) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={who}>
          <SecretForm />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const fillRequired = () => {
  fireEvent.change(screen.getByLabelText(/^Variable Name/), { target: { value: "OPENAI_KEY" } });
  fireEvent.change(screen.getByLabelText(/^Secret Value/), { target: { value: "sk-1" } });
};

const sentAttributes = (method) => apiClient[method].mock.calls[0][1].data.attributes;

const moveToW2 = async () => {
  fireEvent.mouseDown(screen.getByRole("combobox", { name: "Workspace" }));
  fireEvent.click(await screen.findByRole("option", { name: "W2" }));
  fireEvent.click(screen.getByRole("button", { name: "Update secret" }));
};

describe("SecretForm Workspace select", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    mockParams = {};
    apiClient.get.mockImplementation((url) => {
      if (url === "/secrets/5") {
        return Promise.resolve({ data: { data: { id: "5", attributes: { var_name: "OPENAI_KEY", value: "$SECRET/OPENAI_KEY", workspace_id: 2 } } } });
      }
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.post.mockResolvedValue({ data: {} });
    apiClient.patch.mockResolvedValue({ data: {} });
  });

  it("create with one Workspace preselects it and sends it", async () => {
    renderForm(scopedW1);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    expect(screen.getByRole("combobox", { name: "Workspace" })).toHaveAttribute("aria-disabled", "true");
    fillRequired();
    fireEvent.click(screen.getByRole("button", { name: "Add secret" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    expect(sentAttributes("post").workspace_id).toBe(2);
  });

  it("edit: an unchanged Workspace is not sent; picking another is a move", async () => {
    mockParams = { id: "5" };
    renderForm(scopedTwo);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    fireEvent.click(screen.getByRole("button", { name: "Update secret" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch")).not.toHaveProperty("workspace_id");

    apiClient.patch.mockClear();
    await moveToW2();
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch").workspace_id).toBe(3);
  });

  it("a referenced secret's refused move shows the server's reason", async () => {
    mockParams = { id: "5" };
    apiClient.patch.mockRejectedValue({
      response: { status: 422, data: { errors: [{ title: "Unprocessable", detail: "secret is referenced by LLM Claude; it cannot move" }] } },
    });
    renderForm(scopedTwo);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    await moveToW2();
    expect(await screen.findByText(/referenced by LLM Claude; it cannot move/)).toBeInTheDocument();
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("Community: no Workspace select and no workspace_id sent", async () => {
    renderForm(community);
    fillRequired();
    expect(screen.queryByTestId("workspace-select")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add secret" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    expect(sentAttributes("post")).not.toHaveProperty("workspace_id");
  });
});
