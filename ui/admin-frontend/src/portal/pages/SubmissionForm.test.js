import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../admin/utils/testTheme";
import { MemoryRouter } from "react-router-dom";
import SubmissionForm from "./SubmissionForm";

// Mock react-markdown (ESM module that Jest can't handle)
jest.mock("react-markdown", () => ({
  __esModule: true,
  default: ({ children }) => <div data-testid="markdown">{children}</div>,
}));

jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: {
    get: jest.fn(),
    post: jest.fn(),
    patch: jest.fn(),
  },
}));

jest.mock("../../admin/utils/vendorUtils", () => ({
  fetchVendors: jest.fn().mockResolvedValue({
    embedders: [{ code: "openai" }, { code: "ollama" }],
    vectorStores: [{ code: "pgvector" }, { code: "chroma" }],
  }),
  getEmbedderDefaultModel: jest.fn().mockReturnValue("text-embedding-3-small"),
  getEmbedderDefaultUrl: jest.fn().mockReturnValue("https://api.openai.com/v1"),
}));

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => ({}),
}));

const pubClient = require("../../admin/utils/pubClient").default;

const renderWithProviders = (ui) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>{ui}</MemoryRouter>
    </ThemeProvider>
  );

describe("SubmissionForm", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pubClient.get.mockResolvedValue({ data: { data: [] } });
    // Re-set vendorUtils mock after clearAllMocks
    const { fetchVendors } = require("../../admin/utils/vendorUtils");
    fetchVendors.mockResolvedValue({
      embedders: [{ code: "openai" }, { code: "ollama" }],
      vectorStores: [{ code: "pgvector" }, { code: "chroma" }],
    });
  });

  it("renders the page title", async () => {
    renderWithProviders(<SubmissionForm />);
    await waitFor(() => {
      expect(screen.getByText("Submit a Resource")).toBeInTheDocument();
    });
  });

  it("shows resource type selector label", async () => {
    renderWithProviders(<SubmissionForm />);
    await waitFor(() => {
      const elements = screen.getAllByText("Resource Type");
      expect(elements.length).toBeGreaterThan(0);
    });
  });

  it("fetches attestation templates on load", async () => {
    renderWithProviders(<SubmissionForm />);

    await waitFor(() => {
      expect(pubClient.get).toHaveBeenCalledWith(
        "/common/submissions/attestation-templates"
      );
    });
  });

  it("shows back navigation", () => {
    renderWithProviders(<SubmissionForm />);
    expect(
      screen.getByText("Back to My Contributions")
    ).toBeInTheDocument();
  });
});
