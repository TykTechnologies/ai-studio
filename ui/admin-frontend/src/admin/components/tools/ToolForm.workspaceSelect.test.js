import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import ToolForm from "./ToolForm";
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

// Workspaces E3: the tool form's Workspace select (landing on create, move
// on edit) and its errors.

const identity = (attributes) => ({ id: "9", attributes: { has_admin_access: true, permissions: [], ...attributes } });
const scopedW1 = identity({
  rbac_enabled: true,
  workspaces: [{ id: 2, name: "W1", permissions: ["tools:read", "tools:write", "tools:publish"] }],
});
const scopedTwo = identity({
  rbac_enabled: true,
  workspaces: [
    { id: 2, name: "W1", permissions: ["tools:read", "tools:write"] },
    { id: 3, name: "W2", permissions: ["tools:read", "tools:write"] },
  ],
});
const community = identity({ is_admin: true, rbac_enabled: false, permissions: ["*"] });

const storedTool = (attrs) => ({
  data: {
    data: {
      id: "7",
      governed_metadata: {},
      attributes: {
        name: "weather", description: "Forecasts", tool_type: "REST", oas_spec: "",
        privacy_score: 40, active: false, operations: [], file_stores: [], filters: [], namespace: "",
        rest_access_enabled: false, mcp_access_enabled: false,
        ...attrs,
      },
    },
  },
});

const renderForm = (who) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={who}>
          <ToolForm />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const fillRequired = async () => {
  fireEvent.change(await screen.findByLabelText(/^Name/), { target: { value: "Time" } });
  fireEvent.change(screen.getByLabelText(/^Description/), { target: { value: "Clock" } });
};

const sentAttributes = (method) => {
  const call = apiClient[method].mock.calls.find((c) => String(c[0]).startsWith("/tools"));
  return call[1].data.attributes;
};

describe("ToolForm Workspace select", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    mockParams = {};
    useEdition.mockReturnValue({ isEnterprise: true });
    apiClient.get.mockImplementation((url) => {
      if (url === "/filters") return Promise.resolve({ data: [] });
      if (url === "/tools/7") return Promise.resolve(storedTool({ workspace_id: 2 }));
      if (url === "/tools/7/operations") return Promise.resolve({ data: { data: { operations: [] } } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.post.mockResolvedValue({ data: { data: { id: "11" } } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "7" } } });
    apiClient.put.mockResolvedValue({ data: {} });
    apiClient.delete.mockResolvedValue({ data: {} });
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
    fireEvent.click(screen.getByRole("button", { name: "Add tool" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/tools", expect.anything()));
    expect(sentAttributes("post").workspace_id).toBe(2);
  });

  it("shows workspace_required on the Workspace field", async () => {
    apiClient.post.mockRejectedValue({
      response: { status: 422, data: { errors: [{ title: "Workspace required", detail: "name one", code: "workspace_required" }] } },
    });
    renderForm(scopedTwo);
    await screen.findByTestId("workspace-select");
    await fillRequired();
    fireEvent.click(screen.getByRole("button", { name: "Add tool" }));
    expect(await screen.findAllByText(/choose one/)).not.toHaveLength(0);
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("edit: an unchanged Workspace is not sent; picking another is a move", async () => {
    mockParams = { id: "7" };
    renderForm(scopedTwo);
    await screen.findByDisplayValue("weather");
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    fireEvent.click(screen.getByRole("button", { name: "Update tool" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch")).not.toHaveProperty("workspace_id");

    apiClient.patch.mockClear();
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Workspace" }));
    fireEvent.click(await screen.findByRole("option", { name: "W2" }));
    fireEvent.click(screen.getByRole("button", { name: "Update tool" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch").workspace_id).toBe(3);
  });

  it("Community: no Workspace select and no workspace_id sent", async () => {
    useEdition.mockReturnValue({ isEnterprise: false });
    renderForm(community);
    await fillRequired();
    expect(screen.queryByTestId("workspace-select")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add tool" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/tools", expect.anything()));
    expect(sentAttributes("post")).not.toHaveProperty("workspace_id");
  });
});
