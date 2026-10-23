import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import DatasourceForm from "./DatasourceForm";
import apiClient from "../../utils/apiClient";
import { useEdition } from "../../context/EditionContext";
import { usePermissions } from "../../context/PermissionsContext";
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
jest.mock("../../context/PermissionsContext", () => ({ usePermissions: jest.fn() }));
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
    active: true, namespace: "", files: [], allow_internal_host: true,
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

// w4e: the data source's Allow internal host switch loads with the record,
// and an administrator's change is sent.
describe("DatasourceForm Allow internal host", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = { id: "4" };
    useEdition.mockReturnValue({ isEnterprise: false });
    usePermissions.mockReturnValue({ can: () => true, isFullAdmin: true });
    apiClient.get.mockImplementation((url) => {
      if (url === "/datasources/4") return Promise.resolve({ data: { data: DS } });
      if (url === "/users") return Promise.resolve({ data: { data: [{ id: 1, attributes: { name: "Alice", email: "a@x.io" } }] } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "4" } } });
    resolveMetadataSchema.mockResolvedValue({ fields: [], enforcement: "advisory" });
    getMetadataVocabularies.mockResolvedValue([]);
    getMetadataUsers.mockResolvedValue([]);
    validateObjectMetadata.mockResolvedValue({ valid: true, errors: [], warnings: [] });
    Element.prototype.scrollIntoView = jest.fn();
  });

  it("shows the stored switch and sends an administrator's change", async () => {
    renderForm();
    fireEvent.click(await screen.findByText("Vector Database Access Details"));
    const toggle = await screen.findByRole("checkbox", { name: "Allow internal host" });
    await waitFor(() => expect(toggle).toBeChecked());
    fireEvent.click(toggle);
    fireEvent.click(await screen.findByRole("button", { name: "Update data source" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(apiClient.patch.mock.calls[0][1].data.attributes.allow_internal_host).toBe(false);
  });
});
