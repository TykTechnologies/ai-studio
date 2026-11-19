import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import DatasourceForm from "./DatasourceForm";
import apiClient from "../../utils/apiClient";
import { useEdition } from "../../context/EditionContext";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";
import { resetWorkspaceNames } from "../workspaces/workspaceFields";
import {
  resolveMetadataSchema,
  getMetadataVocabularies,
  getMetadataUsers,
  validateObjectMetadata,
} from "../../services/governedMetadataService";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));
jest.mock("../../context/EditionContext");
jest.mock("../../hooks/useSystemFeatures", () => ({ __esModule: true, default: () => ({ features: {} }) }));
jest.mock("../../services/governedMetadataService", () => ({
  ...jest.requireActual("../../services/governedMetadataService"),
  resolveMetadataSchema: jest.fn(),
  getMetadataVocabularies: jest.fn(),
  getMetadataUsers: jest.fn(),
  validateObjectMetadata: jest.fn(),
}));
// The embedder picker has its own tests; here it only has to set an id.
jest.mock("../embedders/EmbedderPicker", () => ({
  __esModule: true,
  default: ({ onChange }) => (
    <button type="button" onClick={() => onChange("3", { id: 3, attributes: { name: "Docs embedder" } })}>
      Pick embedder
    </button>
  ),
}));
const mockNavigate = jest.fn();
let mockParams = {};
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => mockParams,
}));

// Workspaces E3: the data source form's Workspace select (landing on
// create, move on edit), its errors, and Start Processing (execute) gated in
// the data source's Workspace.

const identity = (attributes) => ({ id: "9", attributes: { has_admin_access: true, permissions: [], ...attributes } });
const scopedW1 = identity({
  rbac_enabled: true,
  workspaces: [{ id: 2, name: "W1", permissions: ["datasources:read", "datasources:write", "datasources:publish"] }],
});
const scopedTwo = identity({
  rbac_enabled: true,
  workspaces: [
    { id: 2, name: "W1", permissions: ["datasources:read", "datasources:write", "datasources:execute"] },
    { id: 3, name: "W2", permissions: ["datasources:read", "datasources:write"] },
  ],
});
const community = identity({ is_admin: true, rbac_enabled: false, permissions: ["*"] });

const storedDatasource = (attrs) => ({
  id: "4",
  governed_metadata: {},
  attributes: {
    name: "docs", short_description: "d", long_description: "", icon: "", url: "", privacy_score: 10, user_id: 1, tags: [],
    db_conn_string: "", db_source_type: "qdrant", db_conn_api_key: "", db_name: "",
    embedder_id: 3, embedder_name: "Docs embedder",
    active: false, namespace: "", files: [{ id: 1, attributes: { file_name: "a.txt" } }],
    ...attrs,
  },
});

const renderForm = (who) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={who}>
          <DatasourceForm />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const pickOption = async (label, optionIndex = 0) => {
  fireEvent.mouseDown(screen.getByRole("combobox", { name: label }));
  const listbox = await screen.findByRole("listbox");
  fireEvent.click(within(listbox).getAllByRole("option")[optionIndex]);
};

const fillRequired = async () => {
  fireEvent.change(await screen.findByLabelText(/^Name/), { target: { value: "Wiki" } });
  await pickOption(/^User/);
  await pickOption(/^Vector Database Type/);
  fireEvent.click(screen.getByRole("button", { name: "Pick embedder" }));
};

const sentAttributes = (method) => {
  const call = apiClient[method].mock.calls.find((c) => String(c[0]).startsWith("/datasources"));
  return call[1].data.attributes;
};

describe("DatasourceForm Workspace select", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    mockParams = {};
    useEdition.mockReturnValue({ isEnterprise: true });
    apiClient.get.mockImplementation((url) => {
      if (url === "/datasources/4") return Promise.resolve({ data: { data: storedDatasource({ workspace_id: 2 }) } });
      if (url === "/users") return Promise.resolve({ data: { data: [{ id: 1, attributes: { name: "Alice", email: "a@x.io" } }] } });
      if (url === "/vendors/vector-stores") return Promise.resolve({ data: { data: ["qdrant"] } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.post.mockResolvedValue({ data: { data: { id: "11" } } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "4" } } });
    resolveMetadataSchema.mockResolvedValue({ fields: [], enforcement: "advisory" });
    getMetadataVocabularies.mockResolvedValue([]);
    getMetadataUsers.mockResolvedValue([]);
    validateObjectMetadata.mockResolvedValue({ valid: true, errors: [], warnings: [] });
    Element.prototype.scrollIntoView = jest.fn();
  });

  it("create with one Workspace preselects it and sends it", async () => {
    renderForm(scopedW1);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    expect(screen.getByRole("combobox", { name: "Workspace" })).toHaveAttribute("aria-disabled", "true");
    await fillRequired();
    fireEvent.click(screen.getByRole("button", { name: "Add data source" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/datasources", expect.anything()));
    expect(sentAttributes("post").workspace_id).toBe(2);
  });

  it("shows workspace_required on the Workspace field", async () => {
    apiClient.post.mockRejectedValue({
      response: { status: 422, data: { errors: [{ title: "Workspace required", detail: "name one", code: "workspace_required" }] } },
    });
    renderForm(scopedTwo);
    await screen.findByTestId("workspace-select");
    await fillRequired();
    fireEvent.click(screen.getByRole("button", { name: "Add data source" }));
    expect(await screen.findAllByText(/choose one/)).not.toHaveLength(0);
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("edit: an unchanged Workspace is not sent; picking another is a move", async () => {
    mockParams = { id: "4" };
    renderForm(scopedTwo);
    await screen.findByDisplayValue("docs");
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    fireEvent.click(screen.getByRole("button", { name: "Update data source" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch")).not.toHaveProperty("workspace_id");

    apiClient.patch.mockClear();
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Workspace" }));
    fireEvent.click(await screen.findByRole("option", { name: "W2" }));
    fireEvent.click(screen.getByRole("button", { name: "Update data source" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch").workspace_id).toBe(3);
  });

  it("Start Processing shows only with datasources:execute in the data source's Workspace", async () => {
    mockParams = { id: "4" };
    renderForm(scopedTwo);
    await screen.findByDisplayValue("docs");
    fireEvent.click(screen.getByText("File Quick Upload"));
    expect(await screen.findByRole("button", { name: "Start Processing" })).toBeInTheDocument();
  });

  it("Start Processing is hidden without execute in its Workspace", async () => {
    mockParams = { id: "4" };
    renderForm(scopedW1);
    await screen.findByDisplayValue("docs");
    fireEvent.click(screen.getByText("File Quick Upload"));
    expect(await screen.findByRole("button", { name: /Upload Additional Datasource Documentation/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Start Processing" })).not.toBeInTheDocument();
  });

  it("Community: no Workspace select and no workspace_id sent", async () => {
    useEdition.mockReturnValue({ isEnterprise: false });
    renderForm(community);
    await fillRequired();
    expect(screen.queryByTestId("workspace-select")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add data source" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/datasources", expect.anything()));
    expect(sentAttributes("post")).not.toHaveProperty("workspace_id");
  });
});
