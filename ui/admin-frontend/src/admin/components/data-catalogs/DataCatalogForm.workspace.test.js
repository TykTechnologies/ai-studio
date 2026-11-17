import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import DataCatalogForm from "./DataCatalogForm";
import DataCatalogDetail from "./DataCatalogDetail";
import apiClient from "../../utils/apiClient";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";
import { resetWorkspaceNames } from "../workspaces/workspaceFields";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), put: jest.fn(), delete: jest.fn() },
}));
jest.mock("../../hooks/useSystemFeatures", () => ({
  __esModule: true,
  default: () => ({ features: {}, loading: false }),
}));
const mockNavigate = jest.fn();
let mockParams = {};
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => mockParams,
}));

// Workspaces E4: the data catalog form's Workspace select (landing on create,
// move on edit), its errors, the details page's gated Edit button, and the
// not-found panel for a catalog the caller cannot see (the API answers 404).

const identity = (attributes) => ({ id: "9", attributes: { has_admin_access: true, permissions: [], ...attributes } });
const scopedW1 = identity({
  rbac_enabled: true,
  workspaces: [{ id: 2, name: "W1", permissions: ["data-catalogues:read", "data-catalogues:write"] }],
});
const scopedTwo = identity({
  rbac_enabled: true,
  workspaces: [
    { id: 2, name: "W1", permissions: ["data-catalogues:read", "data-catalogues:write"] },
    { id: 3, name: "W2", permissions: ["data-catalogues:read", "data-catalogues:write"] },
  ],
});
// Reads every catalog, writes only in W1.
const readerWriterW1 = identity({
  rbac_enabled: true,
  permissions: ["data-catalogues:read"],
  workspaces: [{ id: 2, name: "W1", permissions: ["data-catalogues:write"] }],
});
const community = identity({ is_admin: true, rbac_enabled: false, permissions: ["*"] });

const renderAs = (who, Component) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={who}>
          <Component />
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const stored = (workspaceId) => ({
  data: { data: { id: "5", type: "DataCatalogue", attributes: { name: "Stored catalog", short_description: "Short", long_description: "", icon: "", datasources: [], tags: [], workspace_id: workspaceId } } },
});

let storedWorkspace = 2;
let storedStatus = 200;

const fillRequired = async () => {
  fireEvent.change(await screen.findByLabelText(/Catalog Name/), { target: { value: "New catalog" } });
  fireEvent.change(screen.getByLabelText(/Short Description/), { target: { value: "Short" } });
};

const sentAttributes = (method) => {
  const call = apiClient[method].mock.calls.find((c) => c[0] === "/data-catalogues" || c[0] === "/data-catalogues/5");
  return call[1].data.attributes;
};

describe("DataCatalogForm Workspace select", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    mockParams = {};
    storedWorkspace = 2;
    storedStatus = 200;
    apiClient.get.mockImplementation((url) => {
      if (url === "/data-catalogues/5") {
        return storedStatus === 404
          ? Promise.reject({ response: { status: 404, data: { errors: [{ title: "Not Found" }] } } })
          : Promise.resolve(stored(storedWorkspace));
      }
      return Promise.resolve({ data: { data: [] }, headers: {} });
    });
    apiClient.post.mockResolvedValue({ data: { data: { id: "9" } } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "5" } } });
    apiClient.put.mockResolvedValue({ data: {} });
  });

  it("create with one Workspace preselects it and sends it", async () => {
    renderAs(scopedW1, DataCatalogForm);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    expect(screen.getByRole("combobox", { name: "Workspace" })).toHaveAttribute("aria-disabled", "true");
    await fillRequired();
    fireEvent.click(screen.getByRole("button", { name: "Create catalog" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    expect(sentAttributes("post").workspace_id).toBe(2);
  });

  it("shows workspace_required on the Workspace field", async () => {
    apiClient.post.mockRejectedValue({
      response: { status: 422, data: { errors: [{ title: "Workspace required", detail: "name one", code: "workspace_required" }] } },
    });
    renderAs(scopedTwo, DataCatalogForm);
    await screen.findByTestId("workspace-select");
    await fillRequired();
    fireEvent.click(screen.getByRole("button", { name: "Create catalog" }));
    expect(await screen.findAllByText(/choose one/)).not.toHaveLength(0);
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("edit: an unchanged Workspace is not sent", async () => {
    mockParams = { id: "5" };
    renderAs(scopedTwo, DataCatalogForm);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    fireEvent.click(screen.getByRole("button", { name: "Update catalog" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch")).not.toHaveProperty("workspace_id");
  });

  it("edit: picking another Workspace sends it (a move)", async () => {
    mockParams = { id: "5" };
    renderAs(scopedTwo, DataCatalogForm);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Workspace" }));
    fireEvent.click(await screen.findByRole("option", { name: "W2" }));
    fireEvent.click(screen.getByRole("button", { name: "Update catalog" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(sentAttributes("patch").workspace_id).toBe(3);
  });

  it("edit: a scoped user with write in one Workspace cannot move it", async () => {
    mockParams = { id: "5" };
    renderAs(scopedW1, DataCatalogForm);
    const select = await screen.findByTestId("workspace-select");
    await waitFor(() => expect(select).toHaveTextContent("W1"));
    expect(screen.getByRole("combobox", { name: "Workspace" })).toHaveAttribute("aria-disabled", "true");
  });

  it("edit: a catalog the caller cannot see (404) renders the not-found panel", async () => {
    mockParams = { id: "5" };
    storedStatus = 404;
    renderAs(scopedW1, DataCatalogForm);
    expect(await screen.findByTestId("admin-not-found")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Update catalog" })).not.toBeInTheDocument();
  });

  it("Community: no Workspace select and no workspace_id sent", async () => {
    renderAs(community, DataCatalogForm);
    await fillRequired();
    expect(screen.queryByTestId("workspace-select")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Create catalog" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    expect(sentAttributes("post")).not.toHaveProperty("workspace_id");
  });
});

describe("DataCatalogDetail Workspaces", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    resetWorkspaceNames();
    mockParams = { id: "5" };
    storedWorkspace = 2;
    storedStatus = 200;
    apiClient.get.mockImplementation((url) => {
      if (url === "/data-catalogues/5") {
        return storedStatus === 404
          ? Promise.reject({ response: { status: 404, data: { errors: [{ title: "Not Found" }] } } })
          : Promise.resolve(stored(storedWorkspace));
      }
      return Promise.resolve({ data: { data: [] }, headers: {} });
    });
  });

  it("shows Edit for a catalog in a Workspace the caller writes in", async () => {
    renderAs(readerWriterW1, DataCatalogDetail);
    expect(await screen.findByRole("button", { name: /Edit catalog/ })).toBeInTheDocument();
  });

  it("hides Edit for a catalog in another Workspace", async () => {
    storedWorkspace = 3;
    renderAs(readerWriterW1, DataCatalogDetail);
    expect(await screen.findByText("Stored catalog")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Edit catalog/ })).not.toBeInTheDocument();
  });

  it("a catalog the caller cannot see (404) renders the not-found panel", async () => {
    storedStatus = 404;
    renderAs(scopedW1, DataCatalogDetail);
    expect(await screen.findByTestId("admin-not-found")).toBeInTheDocument();
  });
});
