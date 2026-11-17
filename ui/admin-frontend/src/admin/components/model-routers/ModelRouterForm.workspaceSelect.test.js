import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import apiClient from "../../utils/apiClient";
import ModelRouterForm from "./ModelRouterForm";
import { useEdition } from "../../context/EditionContext";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";
import { resetWorkspaceNames } from "../workspaces/workspaceFields";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn() },
}));
jest.mock("../../utils/pubClient", () => ({ __esModule: true, default: { get: jest.fn() } }));
jest.mock("../../context/EditionContext");
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

// Workspaces E4: the model router form's Workspace select (landing on
// create, move on edit), its errors, and a router the caller cannot see.

const identity = (attributes) => ({ id: "9", attributes: { has_admin_access: true, permissions: [], ...attributes } });
const scopedW1 = identity({
  rbac_enabled: true,
  workspaces: [{ id: 2, name: "W1", permissions: ["model-routers:read", "model-routers:write", "model-routers:publish"] }],
});
const scopedTwo = identity({
  rbac_enabled: true,
  workspaces: [
    { id: 2, name: "W1", permissions: ["model-routers:read", "model-routers:write"] },
    { id: 3, name: "W2", permissions: ["model-routers:read", "model-routers:write"] },
  ],
});
const community = identity({ is_admin: true, rbac_enabled: false, permissions: ["*"] });

const storedRouter = {
  data: {
    data: {
      id: "3",
      attributes: {
        name: "Prod",
        slug: "prod",
        description: "",
        api_compat: "openai",
        active: false,
        namespace: "",
        workspace_id: 2,
        catalogues: [],
        pools: [
          {
            name: "gpt",
            model_pattern: "gpt-4o",
            selection_algorithm: "round_robin",
            priority: 0,
            vendors: [{ llm_id: 1, llm_slug: "openai", weight: 1, is_active: true, mappings: [] }],
          },
        ],
      },
    },
  },
};

const renderForm = (who) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={who}>
          <ModelRouterForm />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const fillRequired = () => {
  fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Prod" } });
  fireEvent.click(screen.getByRole("button", { name: "Add Pool" }));
  fireEvent.change(screen.getByLabelText(/^Pool Name/), { target: { value: "all" } });
  fireEvent.click(screen.getByRole("button", { name: "Add Vendor" }));
};

const sentAttributes = (method) => apiClient[method].mock.calls.find((c) => String(c[0]).startsWith("/model-routers"))[1].data.attributes;

describe("ModelRouterForm Workspace select", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    mockParams = {};
    useEdition.mockReturnValue({ isEnterprise: false });
    apiClient.get.mockImplementation((url) => {
      if (url === "/llms") return Promise.resolve({ data: { data: [{ id: 1, attributes: { name: "OpenAI", slug: "openai" } }] } });
      if (url === "/model-routers/3") return Promise.resolve(storedRouter);
      return Promise.resolve({ data: { data: [] }, headers: {} });
    });
    apiClient.post.mockResolvedValue({ data: { data: { id: "9", attributes: {} } } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "3", attributes: {} } } });
    apiClient.put.mockResolvedValue({ data: { data: { catalogues: [] } } });
  });

  it("create with one Workspace preselects it and sends it", async () => {
    renderForm(scopedW1);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    expect(screen.getByRole("combobox", { name: "Workspace" })).toHaveAttribute("aria-disabled", "true");
    fillRequired();
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
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
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(await screen.findAllByText(/choose one/)).not.toHaveLength(0);
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("edit: an unchanged Workspace is not sent; picking another is a move", async () => {
    mockParams = { id: "3" };
    renderForm(scopedTwo);
    await screen.findByDisplayValue("Prod");
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    fireEvent.click(screen.getByRole("button", { name: "Update" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch")).not.toHaveProperty("workspace_id");

    apiClient.patch.mockClear();
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Workspace" }));
    fireEvent.click(await screen.findByRole("option", { name: "W2" }));
    fireEvent.click(screen.getByRole("button", { name: "Update" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch").workspace_id).toBe(3);
  });

  it("edit: the Workspace is not movable with write in one Workspace only", async () => {
    mockParams = { id: "3" };
    renderForm(scopedW1);
    await screen.findByDisplayValue("Prod");
    expect(screen.getByRole("combobox", { name: "Workspace" })).toHaveAttribute("aria-disabled", "true");
  });

  it("says catalogs in other Workspaces are kept when the caller cannot read every catalog", async () => {
    mockParams = { id: "3" };
    renderForm(scopedTwo);
    await screen.findByDisplayValue("Prod");
    expect(screen.getByTestId("router-catalogues-hidden-note")).toBeInTheDocument();
  });

  it("a router the caller cannot see (404) renders the not-found panel", async () => {
    mockParams = { id: "3" };
    apiClient.get.mockImplementation((url) =>
      url === "/model-routers/3"
        ? Promise.reject({ response: { status: 404, data: {} } })
        : Promise.resolve({ data: { data: [] }, headers: {} }),
    );
    renderForm(scopedW1);
    expect(await screen.findByTestId("admin-not-found")).toBeInTheDocument();
  });

  it("Community: no Workspace select, no catalog note and no workspace_id sent", async () => {
    useEdition.mockReturnValue({ isEnterprise: false });
    renderForm(community);
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/llms", expect.anything()));
    expect(screen.queryByTestId("workspace-select")).not.toBeInTheDocument();
    expect(screen.queryByTestId("router-catalogues-hidden-note")).not.toBeInTheDocument();
    fillRequired();
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    expect(sentAttributes("post")).not.toHaveProperty("workspace_id");
  });
});
