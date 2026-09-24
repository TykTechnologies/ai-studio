import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import DatasourceForm from "./DatasourceForm";
import apiClient from "../../utils/apiClient";
import { useEdition } from "../../context/EditionContext";
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
const mockNavigate = jest.fn();
let mockParams = {};
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => mockParams,
}));

const DS = {
  id: "4",
  governed_metadata: { team: "data" },
  governed_metadata_status: "valid",
  attributes: {
    name: "docs", short_description: "d", long_description: "", icon: "", url: "", privacy_score: 10, user_id: 1, tags: [],
    db_conn_string: "", db_source_type: "qdrant", db_conn_api_key: "", db_name: "", embed_vendor: "openai", embed_url: "", embed_api_key: "[redacted]", embed_model: "m",
    embedder_id: 3, embedder_name: "Docs embedder",
    active: true, namespace: "", files: [],
  },
};

const renderForm = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <DatasourceForm />
      </MemoryRouter>
    </ThemeProvider>
  );

describe("DatasourceForm governed metadata embedding", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = { id: "4" };
    useEdition.mockReturnValue({ isEnterprise: true });
    apiClient.get.mockImplementation((url) => {
      if (url === "/datasources/4") return Promise.resolve({ data: { data: DS } });
      if (url === "/users") return Promise.resolve({ data: { data: [{ id: 1, attributes: { name: "Alice", email: "a@x.io" } }] } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "4" } } });
    resolveMetadataSchema.mockResolvedValue({ fields: [{ key: "team", label: "Team", type: "string", order: 1 }], enforcement: "enforce" });
    getMetadataVocabularies.mockResolvedValue([]);
    getMetadataUsers.mockResolvedValue([]);
    validateObjectMetadata.mockResolvedValue({ valid: true, errors: [], warnings: [] });
    Element.prototype.scrollIntoView = jest.fn();
  });

  it("prefills stored metadata, keeps the section open under enforcement and sends values on save", async () => {
    renderForm();
    expect(await screen.findByText("Enforced")).toBeInTheDocument();
    expect(await screen.findByLabelText(/Team/)).toHaveValue("data");
    fireEvent.change(screen.getByLabelText(/Team/), { target: { value: "" } });

    fireEvent.click(await screen.findByRole("button", { name: "Update data source" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    const attrs = apiClient.patch.mock.calls[0][1].data.attributes;
    expect(attrs.governed_metadata).toEqual({});
    expect(attrs.governed_metadata_status).toBeUndefined();
    // The embedder goes by id; the flattened legacy fields are not sent back.
    expect(attrs.embedder_id).toBe(3);
    expect(attrs).not.toHaveProperty("embed_vendor");
    expect(attrs).not.toHaveProperty("embed_api_key");
  });

  it("reports success when the data source and its metadata both saved", async () => {
    renderForm();
    await screen.findByLabelText(/Team/);
    fireEvent.click(await screen.findByRole("button", { name: "Update data source" }));
    await waitFor(() => expect(mockNavigate).toHaveBeenCalled());
    const [, opts] = mockNavigate.mock.calls[0];
    expect(opts.state.snackbar).toEqual({ message: "Data source updated successfully", severity: "success" });
  });

  it("reports a warning, not success, when the data source saved but a plugin rejected its metadata", async () => {
    apiClient.patch.mockResolvedValue({
      data: {
        data: { id: "4" },
        meta: { governed_metadata_error: { code: "hook_rejected", detail: "policy says no" } },
      },
    });
    renderForm();
    await screen.findByLabelText(/Team/);
    fireEvent.click(await screen.findByRole("button", { name: "Update data source" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalledWith("/datasources/4", expect.anything()));
    await waitFor(() => expect(mockNavigate).toHaveBeenCalled());
    const [, opts] = mockNavigate.mock.calls[0];
    expect(opts.state.snackbar.severity).toBe("warning");
    expect(opts.state.snackbar.message).toMatch(/Data source updated, but Governance metadata was rejected by a plugin: policy says no/);
    expect(opts.state.snackbar.message).not.toMatch(/updated successfully/);
  });

  it("reports a warning when the metadata write itself failed", async () => {
    apiClient.patch.mockResolvedValue({
      data: {
        data: { id: "4" },
        meta: { governed_metadata_error: { code: "metadata_write_failed", detail: "UNIQUE constraint failed" } },
      },
    });
    renderForm();
    await screen.findByLabelText(/Team/);
    fireEvent.click(await screen.findByRole("button", { name: "Update data source" }));
    await waitFor(() => expect(mockNavigate).toHaveBeenCalled());
    const [, opts] = mockNavigate.mock.calls[0];
    expect(opts.state.snackbar.severity).toBe("warning");
    expect(opts.state.snackbar.message).toMatch(/Governance metadata was not saved: UNIQUE constraint failed/);
    expect(opts.state.snackbar.message).not.toMatch(/updated successfully/);
  });

  it("maps a 422 onto the field and scrolls to the section", async () => {
    apiClient.patch.mockRejectedValue({ response: { status: 422, data: { errors: [
      { detail: "Team is required", source: { pointer: "/data/attributes/governed_metadata/team" } },
      { detail: "policy says no", code: "hook_rejected" },
    ] } } });
    renderForm();
    await screen.findByLabelText(/Team/);
    fireEvent.click(await screen.findByRole("button", { name: "Update data source" }));
    expect(await screen.findByText("Team is required")).toBeInTheDocument();
    expect(screen.getAllByText(/policy says no/).length).toBeGreaterThanOrEqual(1);
    expect(Element.prototype.scrollIntoView).toHaveBeenCalled();
    expect(screen.queryByText(/Failed to save datasource/)).not.toBeInTheDocument();
  });
});
