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
    db_conn_string: "", db_source_type: "qdrant", db_conn_api_key: "", db_name: "", embed_vendor: "openai", embed_url: "", embed_api_key: "", embed_model: "m",
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
