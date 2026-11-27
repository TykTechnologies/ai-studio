import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import LLMForm from "./LLMForm";
import apiClient from "../../utils/apiClient";
import pluginService from "../../services/pluginService";
import { useEdition } from "../../context/EditionContext";

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
        <LLMForm />
      </MemoryRouter>
    </ThemeProvider>,
  );

const pickVendor = async (name) => {
  fireEvent.mouseDown(screen.getByRole("combobox", { name: /Vendor/ }));
  fireEvent.click(await screen.findByRole("option", { name }));
};

// Gemini Platform (Vertex) connection help (midsommar-clean-1af.6).
describe("LLMForm Vertex connection help", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = {};
    useEdition.mockReturnValue({ isEnterprise: false });
    pluginService.listPlugins.mockResolvedValue({ data: [] });
    pluginService.getAvailableHookTypes.mockReturnValue([]);
    pluginService.getHookTypeLabel.mockImplementation((t) => t);
    apiClient.get.mockImplementation(() => Promise.resolve({ data: { data: [] } }));
  });

  it("explains the endpoint and credential and warns while the key is empty", async () => {
    renderForm();
    await screen.findByRole("button", { name: "Add model provider" });
    await pickVendor(/Vertex/);

    expect(screen.getByText(/project:location: the Google Cloud project ID/)).toBeInTheDocument();
    expect(screen.getByText(/Workload Identity Federation config/)).toBeInTheDocument();
    expect(screen.getByTestId("vertex-keyless-warning")).toHaveTextContent("AI_STUDIO_VERTEX_ADC=true");

    fireEvent.change(screen.getByLabelText("API Key"), { target: { value: "$SECRET/vertex-sa" } });
    expect(screen.queryByTestId("vertex-keyless-warning")).not.toBeInTheDocument();
  });

  it("shows no Vertex help for other vendors", async () => {
    renderForm();
    await screen.findByRole("button", { name: "Add model provider" });
    await pickVendor(/OpenAI/);
    expect(screen.queryByText(/project:location: the Google Cloud project ID/)).not.toBeInTheDocument();
    expect(screen.queryByTestId("vertex-keyless-warning")).not.toBeInTheDocument();
  });
});
