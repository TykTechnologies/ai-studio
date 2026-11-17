import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import apiClient from "../../utils/apiClient";
import EmbedderCreateDialog from "./EmbedderCreateDialog";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";
import { resetWorkspaceNames } from "../workspaces/workspaceFields";
import { validateDraft } from "./embedderModel";

// Workspaces .54: an embedder created inline (from a data source or a
// semantic router form) lands in the Workspace chosen in the dialog.

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn() },
}));
jest.mock("./useEmbedderOptions", () => ({ __esModule: true, default: () => ({ llms: [], vendors: [] }) }));
jest.mock("./embedderModel", () => ({
  ...jest.requireActual("./embedderModel"),
  validateDraft: jest.fn(() => ({})),
}));

const identity = (workspaces) => ({
  id: "9",
  attributes: { is_admin: false, has_admin_access: true, rbac_enabled: true, permissions: [], workspaces },
});
const one = identity([{ id: 2, name: "W1", permissions: ["embedders:read", "embedders:write"] }]);
const two = identity([
  { id: 2, name: "W1", permissions: ["embedders:write"] },
  { id: 3, name: "W2", permissions: ["embedders:write"] },
]);

const renderDialog = (who, onCreated = jest.fn()) =>
  render(
    <ThemeProvider theme={testTheme}>
      <PermissionsProvider identity={who}>
        <EmbedderCreateDialog open onClose={jest.fn()} onCreated={onCreated} />
      </PermissionsProvider>
    </ThemeProvider>,
  );

const sent = () => apiClient.post.mock.calls[0][1].data.attributes;

describe("EmbedderCreateDialog Workspace", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    validateDraft.mockReturnValue({});
    apiClient.get.mockResolvedValue({ data: { data: [] } });
    apiClient.post.mockResolvedValue({ data: { data: { id: "7" } } });
  });

  it("with one Workspace preselects it and sends it", async () => {
    const onCreated = jest.fn();
    renderDialog(one, onCreated);
    await waitFor(() => expect(screen.getByTestId("workspace-select")).toHaveTextContent("W1"));
    fireEvent.click(screen.getByRole("button", { name: "Create embedder" }));
    await waitFor(() => expect(onCreated).toHaveBeenCalled());
    expect(sent().workspace_id).toBe(2);
  });

  it("shows workspace_required on the field and stays open", async () => {
    apiClient.post.mockRejectedValue({
      response: { status: 422, data: { errors: [{ title: "Workspace required", detail: "x", code: "workspace_required" }] } },
    });
    const onCreated = jest.fn();
    renderDialog(two, onCreated);
    await screen.findByTestId("workspace-select");
    fireEvent.click(screen.getByRole("button", { name: "Create embedder" }));
    expect(await screen.findByText(/choose one/)).toBeInTheDocument();
    expect(onCreated).not.toHaveBeenCalled();
    expect(sent()).not.toHaveProperty("workspace_id");
  });
});
