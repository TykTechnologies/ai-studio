import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import LLMForm from "./LLMForm";
import apiClient from "../../utils/apiClient";
import pluginService from "../../services/pluginService";
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
jest.mock("../../hooks/useSystemFeatures", () => ({
  __esModule: true,
  default: () => ({ features: {} }),
}));
jest.mock("../../services/pluginService", () => ({
  __esModule: true,
  default: { listPlugins: jest.fn(), getAvailableHookTypes: jest.fn(), getHookTypeLabel: jest.fn() },
}));
jest.mock("../../services/governedMetadataService", () => ({
  ...jest.requireActual("../../services/governedMetadataService"),
  resolveMetadataSchema: jest.fn(),
  getMetadataVocabularies: jest.fn(),
  getMetadataUsers: jest.fn(),
  validateObjectMetadata: jest.fn(),
}));
const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => ({}),
}));

const renderForm = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <LLMForm />
      </MemoryRouter>
    </ThemeProvider>
  );

const fillRequired = async () => {
  fireEvent.change(await screen.findByLabelText(/^Name/), { target: { value: "gpt" } });
  // Vendor is a MUI Select; pick the first vendor offered.
  fireEvent.mouseDown(screen.getByLabelText(/^Vendor/));
  const vendorOptions = await screen.findAllByRole("option");
  fireEvent.click(vendorOptions[0]);
  await waitFor(() => expect(screen.queryAllByRole("option")).toHaveLength(0));
};

describe("LLMForm governed metadata embedding", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    useEdition.mockReturnValue({ isEnterprise: true });
    // CRA resets mock implementations between tests, so set them here.
    pluginService.listPlugins.mockResolvedValue({ data: [] });
    pluginService.getAvailableHookTypes.mockReturnValue([]);
    pluginService.getHookTypeLabel.mockImplementation((t) => t);
    apiClient.get.mockImplementation((url) => {
      if (url === "/filters") return Promise.resolve({ data: [] });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.put.mockResolvedValue({ data: {} });
    resolveMetadataSchema.mockResolvedValue({
      fields: [{ key: "risk_tier", label: "Risk tier", type: "vocabulary", vocabulary_slug: "risk_tier", required: true, order: 1 }],
      enforcement: "enforce",
    });
    getMetadataVocabularies.mockResolvedValue([{ slug: "risk_tier", name: "Risk", terms: [{ value: "high", label: "High" }] }]);
    getMetadataUsers.mockResolvedValue([]);
    validateObjectMetadata.mockResolvedValue({ valid: true, errors: [], warnings: [] });
    Element.prototype.scrollIntoView = jest.fn();
  });

  it("sends governed_metadata inside attributes on create", async () => {
    apiClient.post.mockResolvedValue({ data: { data: { id: "9" } } });
    renderForm();
    expect(await screen.findByText("Governance Metadata")).toBeInTheDocument();
    await fillRequired();

    fireEvent.mouseDown(await screen.findByLabelText(/Risk tier/));
    fireEvent.click(await screen.findByRole("option", { name: "High" }));

    fireEvent.click(screen.getByRole("button", { name: "Add LLM" }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    const [url, body] = apiClient.post.mock.calls[0];
    expect(url).toBe("/llms");
    expect(body.data.attributes.governed_metadata).toEqual({ risk_tier: "high" });
  });

  it("maps a 422 to the field and suppresses the generic failure message", async () => {
    apiClient.post.mockRejectedValue({
      response: {
        status: 422,
        data: {
          errors: [
            { title: "Metadata Validation Failed", detail: "Risk tier is required", source: { pointer: "/data/attributes/governed_metadata/risk_tier" } },
          ],
        },
      },
    });
    renderForm();
    await screen.findByText("Governance Metadata");
    await fillRequired();

    fireEvent.click(screen.getByRole("button", { name: "Add LLM" }));
    expect(await screen.findByText("Risk tier is required")).toBeInTheDocument();
    expect(screen.getByText(/Governance metadata failed validation/)).toBeInTheDocument();
    expect(screen.queryByText(/Failed to save LLM/)).not.toBeInTheDocument();
    expect(Element.prototype.scrollIntoView).toHaveBeenCalled();
  });
});
