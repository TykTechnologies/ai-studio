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

// CRA resets mock implementations between tests, so every describe sets
// them again here.
const setupMocks = () => {
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
};

describe("ToolForm governed metadata embedding", () => {
  beforeEach(setupMocks);

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

// The API answers 200 with meta.governed_metadata_error when the tool saved
// but its metadata did not; the form must say so instead of "updated
// successfully".
describe("ToolForm governed metadata save warning", () => {
  beforeEach(() => {
    setupMocks();
    mockParams = { id: "7" };
    // The save runs operation, dependency and filter syncs after the PATCH;
    // each must resolve for the form to reach navigate().
    apiClient.get.mockImplementation((url) => {
      if (url === "/filters") return Promise.resolve({ data: [] });
      if (url === "/tools/7") {
        return Promise.resolve({ data: { data: { id: "7", governed_metadata: { team: "platform" }, attributes: { name: "weather", description: "d", tool_type: "REST", oas_spec: "", privacy_score: 5, operations: [], file_stores: [], filters: [] } } } });
      }
      if (url === "/tools/7/operations") return Promise.resolve({ data: { data: { operations: [] } } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.post.mockResolvedValue({ data: { data: { id: "11" } } });
    apiClient.delete.mockResolvedValue({ data: {} });
  });

  const submitUpdate = async () => {
    renderForm();
    await screen.findByDisplayValue("weather");
    fireEvent.click(screen.getByRole("button", { name: "Update tool" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalledWith("/tools/7", expect.anything()));
    await waitFor(() => expect(mockNavigate).toHaveBeenCalled());
    return mockNavigate.mock.calls[0][1].state.snackbar;
  };

  it("reports success when the tool and its metadata both saved", async () => {
    const snackbar = await submitUpdate();
    expect(snackbar).toEqual({ message: "Tool updated successfully", severity: "success" });
  });

  it("reports a warning, not success, when the tool saved but a plugin rejected its metadata", async () => {
    apiClient.patch.mockResolvedValue({
      data: {
        data: { id: "7" },
        meta: { governed_metadata_error: { code: "hook_rejected", detail: "policy says no" } },
      },
    });
    const snackbar = await submitUpdate();
    expect(snackbar.severity).toBe("warning");
    expect(snackbar.message).toMatch(/Tool updated, but Governance metadata was rejected by a plugin: policy says no/);
    expect(snackbar.message).not.toMatch(/updated successfully/);
  });

  it("reports a warning when the metadata write itself failed", async () => {
    apiClient.patch.mockResolvedValue({
      data: {
        data: { id: "7" },
        meta: { governed_metadata_error: { code: "metadata_write_failed", detail: "UNIQUE constraint failed" } },
      },
    });
    const snackbar = await submitUpdate();
    expect(snackbar.severity).toBe("warning");
    expect(snackbar.message).toMatch(/Governance metadata was not saved: UNIQUE constraint failed/);
    expect(snackbar.message).not.toMatch(/updated successfully/);
  });
});
