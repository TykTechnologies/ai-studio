import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import FilterForm from "./FilterForm";
import apiClient from "../../utils/apiClient";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";
import { resetWorkspaceNames } from "../workspaces/workspaceFields";

// Workspaces E3: the filter form's Workspace select (landing on create,
// move on edit) and its errors.

jest.mock("prismjs/themes/prism-tomorrow.css", () => ({}));
jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
jest.mock("../common/EdgeAvailabilitySection", () => () => null);
jest.mock("./ScriptTestPanel", () => () => null);
jest.mock("./ScriptTemplateSelector", () => () => null);
jest.mock("react-simple-code-editor", () => ({ value, onValueChange }) => (
  <textarea data-testid="script-editor" value={value} onChange={(e) => onValueChange(e.target.value)} />
));
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
  workspaces: [{ id: 2, name: "W1", permissions: ["filters:read", "filters:write"] }],
});
const scopedTwo = identity({
  rbac_enabled: true,
  workspaces: [
    { id: 2, name: "W1", permissions: ["filters:read", "filters:write"] },
    { id: 3, name: "W2", permissions: ["filters:read", "filters:write"] },
  ],
});
const community = identity({ is_admin: true, rbac_enabled: false, permissions: ["*"] });

const stored = {
  attributes: {
    name: "PII",
    description: "",
    script: btoa("x := 1"),
    kind: "script",
    response_filter: false,
    namespace: "",
    workspace_id: 2,
  },
};

const renderForm = (who) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={who}>
          <FilterForm />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const fillRequired = () => {
  fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "PII" } });
  fireEvent.change(screen.getByTestId("script-editor"), { target: { value: "x := 1" } });
};

const sentAttributes = (method) => apiClient[method].mock.calls[0][1].data.attributes;

describe("FilterForm Workspace select", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    mockParams = {};
    apiClient.get.mockImplementation((url) => {
      if (url === "/filters/5") return Promise.resolve({ data: stored });
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
    fireEvent.click(screen.getByRole("button", { name: "Add filter" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    expect(sentAttributes("post").workspace_id).toBe(2);
  });

  it("shows workspace_required on the Workspace field", async () => {
    apiClient.post.mockRejectedValue({
      response: { status: 422, data: { errors: [{ title: "Workspace required", detail: "name one", code: "workspace_required" }] } },
    });
    renderForm(scopedTwo);
    await screen.findByTestId("workspace-select");
    fillRequired();
    fireEvent.click(screen.getByRole("button", { name: "Add filter" }));
    expect(await screen.findAllByText(/choose one/)).not.toHaveLength(0);
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("edit: an unchanged Workspace is not sent; picking another is a move", async () => {
    mockParams = { id: "5" };
    renderForm(scopedTwo);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    fireEvent.click(screen.getByRole("button", { name: "Update filter" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch")).not.toHaveProperty("workspace_id");

    apiClient.patch.mockClear();
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Workspace" }));
    fireEvent.click(await screen.findByRole("option", { name: "W2" }));
    fireEvent.click(screen.getByRole("button", { name: "Update filter" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch").workspace_id).toBe(3);
  });

  it("Community: no Workspace select and no workspace_id sent", async () => {
    renderForm(community);
    fillRequired();
    expect(screen.queryByTestId("workspace-select")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add filter" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    expect(sentAttributes("post")).not.toHaveProperty("workspace_id");
  });
});
