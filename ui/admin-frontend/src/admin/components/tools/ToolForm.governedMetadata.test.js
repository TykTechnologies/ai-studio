import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import ToolForm from "./ToolForm";
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

const renderForm = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <ToolForm />
      </MemoryRouter>
    </ThemeProvider>
  );

describe("ToolForm governed metadata embedding", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = {};
    useEdition.mockReturnValue({ isEnterprise: true });
    apiClient.get.mockImplementation((url) => {
      if (url === "/filters") return Promise.resolve({ data: [] });
      if (url === "/tools/7") {
        return Promise.resolve({ data: { data: { id: "7", governed_metadata: { team: "platform" }, attributes: { name: "weather", description: "d", tool_type: "REST", oas_spec: "", privacy_score: 5, operations: [], file_stores: [], filters: [] } } } });
      }
      if (url.startsWith("/tools/7/")) return Promise.resolve({ data: { data: [] } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.put.mockResolvedValue({ data: {} });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "7" } } });
    resolveMetadataSchema.mockResolvedValue({ fields: [{ key: "team", label: "Team", type: "string", required: true, order: 1 }], enforcement: "advisory" });
    getMetadataVocabularies.mockResolvedValue([]);
    getMetadataUsers.mockResolvedValue([]);
    validateObjectMetadata.mockResolvedValue({ valid: true, errors: [], warnings: [] });
    Element.prototype.scrollIntoView = jest.fn();
  });

  it("prefills stored metadata when editing and sends it back on save", async () => {
    mockParams = { id: "7" };
    renderForm();
    const section = await screen.findByText("Governance Metadata");
    fireEvent.click(section);
    expect(await screen.findByLabelText(/Team/)).toHaveValue("platform");
    fireEvent.change(screen.getByLabelText(/Team/), { target: { value: "data" } });

    fireEvent.click(screen.getByRole("button", { name: "Update tool" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    expect(apiClient.patch.mock.calls[0][1].data.attributes.governed_metadata).toEqual({ team: "data" });
  });

  it("maps a 422 onto the field on create", async () => {
    apiClient.post.mockRejectedValue({ response: { status: 422, data: { errors: [
      { detail: "Team is required", source: { pointer: "/data/attributes/governed_metadata/team" } },
    ] } } });
    renderForm();
    await screen.findByText("Governance Metadata");
    fireEvent.change(await screen.findByLabelText(/^Name/), { target: { value: "weather" } });
    fireEvent.change(screen.getByLabelText(/^Description/), { target: { value: "forecasts" } });
    const specInput = screen.getAllByLabelText(/OpenAPI|OAS/i).find((el) => el.tagName === "TEXTAREA");
    fireEvent.change(specInput, { target: { value: '{"openapi":"3.0.0"}' } });
    fireEvent.click(screen.getByRole("button", { name: "Add tool" }));
    expect(await screen.findByText("Team is required")).toBeInTheDocument();
    expect(screen.queryByText(/Failed to save tool/)).not.toBeInTheDocument();
  });
});
