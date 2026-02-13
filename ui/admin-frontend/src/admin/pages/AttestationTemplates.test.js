import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../utils/testTheme";
import { MemoryRouter } from "react-router-dom";
import AttestationTemplates from "./AttestationTemplates";

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: {
    get: jest.fn(),
    post: jest.fn(),
    patch: jest.fn(),
    delete: jest.fn(),
  },
}));

const apiClient = require("../utils/apiClient").default;

const renderWithProviders = (ui) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>{ui}</MemoryRouter>
    </ThemeProvider>
  );

const mockTemplates = [
  {
    id: 1,
    name: "Data Authority",
    text: "I confirm I have authority to share these credentials",
    applies_to_type: "all",
    required: true,
    active: true,
    sort_order: 1,
  },
  {
    id: 2,
    name: "No PII",
    text: "This data source contains no personally identifiable information",
    applies_to_type: "datasource",
    required: false,
    active: true,
    sort_order: 2,
  },
];

describe("AttestationTemplates", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockResolvedValue({ data: { data: [] } });
  });

  it("renders the page title and add button", async () => {
    renderWithProviders(<AttestationTemplates />);
    await waitFor(() => {
      expect(screen.getByText("Attestation Templates")).toBeInTheDocument();
      expect(screen.getByText("Add Template")).toBeInTheDocument();
    });
  });

  it("renders templates in table", async () => {
    apiClient.get.mockResolvedValue({ data: { data: mockTemplates } });
    renderWithProviders(<AttestationTemplates />);
    await waitFor(() => {
      expect(screen.getByText("Data Authority")).toBeInTheDocument();
      expect(screen.getByText("No PII")).toBeInTheDocument();
    });
  });

  it("shows required/optional chips", async () => {
    apiClient.get.mockResolvedValue({ data: { data: mockTemplates } });
    renderWithProviders(<AttestationTemplates />);
    await waitFor(() => {
      const requiredChips = screen.getAllByText("Required");
      expect(requiredChips.length).toBeGreaterThan(0);
      expect(screen.getByText("Optional")).toBeInTheDocument();
    });
  });

  it("shows empty state when no templates", async () => {
    renderWithProviders(<AttestationTemplates />);
    await waitFor(() => {
      expect(screen.getByText(/No attestation templates yet/)).toBeInTheDocument();
    });
  });

  it("calls API with correct endpoint", async () => {
    renderWithProviders(<AttestationTemplates />);
    await waitFor(() => {
      expect(apiClient.get).toHaveBeenCalledWith("/attestation-templates");
    });
  });
});
