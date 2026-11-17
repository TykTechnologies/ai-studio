import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import AppForm from "./AppForm";
import apiClient, { appToolAPI } from "../../utils/apiClient";
import { useEdition } from "../../context/EditionContext";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";
import { resetWorkspaceNames } from "../workspaces/workspaceFields";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn(), delete: jest.fn() },
  appToolAPI: { listAvailableTools: jest.fn() },
}));
jest.mock("../../context/EditionContext");
jest.mock("../../hooks/useSystemFeatures", () => ({
  __esModule: true,
  default: () => ({ features: {} }),
}));
jest.mock("../../../components/common/Icon", () => {
  return function MockIcon(props) {
    return <div data-testid="mock-icon">{props.name}</div>;
  };
});
jest.mock("../common/relationship-picker", () =>
  require("../../../test-utils/component-mocks").relationshipPickerMock,
);
const mockNavigate = jest.fn();
let mockParams = {};
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => mockParams,
}));

// Workspaces E4: the App form's Workspace select (landing on create, move on
// edit), its errors, and the not-found panel for an App the caller cannot see.

const identity = (attributes) => ({ id: "9", attributes: { has_admin_access: true, permissions: [], ...attributes } });
const scopedW1 = identity({
  rbac_enabled: true,
  workspaces: [{ id: 2, name: "W1", permissions: ["apps:read", "apps:write"] }],
});
const scopedTwo = identity({
  rbac_enabled: true,
  workspaces: [
    { id: 2, name: "W1", permissions: ["apps:read", "apps:write"] },
    { id: 3, name: "W2", permissions: ["apps:read", "apps:write"] },
  ],
});
const community = identity({ is_admin: true, rbac_enabled: false, permissions: ["*"] });

const storedApp = {
  data: {
    data: {
      id: "5",
      attributes: {
        name: "Sales bot",
        description: "",
        user_id: 1,
        llm_ids: [],
        datasource_ids: [],
        tool_ids: [],
        monthly_budget: null,
        budget_start_date: null,
        namespace: "",
        metadata: {},
        workspace_id: 2,
      },
    },
  },
};

const renderForm = (who) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={who}>
          <AppForm />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const fillRequired = async () => {
  fireEvent.change(await screen.findByLabelText(/^Name/), { target: { value: "New app" } });
  fireEvent.mouseDown(screen.getByRole("combobox", { name: "User" }));
  fireEvent.click(await screen.findByRole("option", { name: "Ada" }));
};

const sentAttributes = (method) => apiClient[method].mock.calls.find((c) => String(c[0]).startsWith("/apps"))[1].data.attributes;

describe("AppForm Workspace select", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    mockParams = {};
    useEdition.mockReturnValue({ isEnterprise: false });
    apiClient.get.mockImplementation((url) => {
      if (url === "/users") return Promise.resolve({ data: { data: [{ id: "1", attributes: { name: "Ada" } }] } });
      if (url === "/apps/5") return Promise.resolve(storedApp);
      return Promise.resolve({ data: { data: [] } });
    });
    appToolAPI.listAvailableTools.mockResolvedValue({ data: { data: [] } });
    apiClient.post.mockResolvedValue({ data: { data: { id: "6" } } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "5" } } });
    jest.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    console.error.mockRestore();
  });

  it("create with one Workspace preselects it and sends it", async () => {
    renderForm(scopedW1);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    expect(screen.getByRole("combobox", { name: "Workspace" })).toHaveAttribute("aria-disabled", "true");
    await fillRequired();
    fireEvent.click(screen.getByRole("button", { name: "Add app" }));
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
    fireEvent.click(screen.getByRole("button", { name: "Add app" }));
    expect(await screen.findAllByText(/choose one/)).not.toHaveLength(0);
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("edit: an unchanged Workspace is not sent; picking another is a move", async () => {
    mockParams = { id: "5" };
    renderForm(scopedTwo);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    fireEvent.click(screen.getByRole("button", { name: "Update app" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch")).not.toHaveProperty("workspace_id");

    apiClient.patch.mockClear();
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Workspace" }));
    fireEvent.click(await screen.findByRole("option", { name: "W2" }));
    fireEvent.click(screen.getByRole("button", { name: "Update app" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch").workspace_id).toBe(3);
  });

  it("edit: the move control is disabled for a caller who writes in one Workspace", async () => {
    mockParams = { id: "5" };
    renderForm(scopedW1);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    expect(screen.getByRole("combobox", { name: "Workspace" })).toHaveAttribute("aria-disabled", "true");
  });

  it("an App the caller cannot see (404) renders the not-found panel", async () => {
    mockParams = { id: "5" };
    apiClient.get.mockImplementation((url) => {
      if (url === "/apps/5") return Promise.reject({ response: { status: 404, data: {} } });
      return Promise.resolve({ data: { data: [] } });
    });
    renderForm(scopedW1);
    expect(await screen.findByTestId("admin-not-found")).toBeInTheDocument();
  });

  it("Community: no Workspace select and no workspace_id sent", async () => {
    renderForm(community);
    await fillRequired();
    expect(screen.queryByTestId("workspace-select")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add app" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    expect(sentAttributes("post")).not.toHaveProperty("workspace_id");
  });
});
