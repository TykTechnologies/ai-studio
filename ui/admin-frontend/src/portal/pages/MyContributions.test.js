import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../admin/utils/testTheme";
import { MemoryRouter } from "react-router-dom";
import MyContributions from "./MyContributions";

// Mock pubClient
jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: {
    get: jest.fn(),
    delete: jest.fn(),
  },
}));

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const pubClient = require("../../admin/utils/pubClient").default;

const renderWithProviders = (ui) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>{ui}</MemoryRouter>
    </ThemeProvider>
  );

describe("MyContributions", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("renders the page title", async () => {
    pubClient.get.mockResolvedValueOnce({
      data: { data: [], total_count: 0 },
    });

    renderWithProviders(<MyContributions />);
    expect(screen.getByText("My Contributions")).toBeInTheDocument();
  });

  it("renders tabs for status filters", async () => {
    pubClient.get.mockResolvedValueOnce({
      data: { data: [], total_count: 0 },
    });

    renderWithProviders(<MyContributions />);
    expect(screen.getByText("All")).toBeInTheDocument();
    expect(screen.getByText("Published")).toBeInTheDocument();
    expect(screen.getByText("Pending Review")).toBeInTheDocument();
    expect(screen.getByText("Drafts")).toBeInTheDocument();
    expect(screen.getByText("Rejected")).toBeInTheDocument();
  });

  it("shows empty state when no submissions", async () => {
    pubClient.get.mockResolvedValueOnce({
      data: { data: [], total_count: 0 },
    });

    renderWithProviders(<MyContributions />);

    await waitFor(() => {
      expect(screen.getByText("No submissions found")).toBeInTheDocument();
    });
  });

  it("renders submission cards", async () => {
    pubClient.get.mockResolvedValueOnce({
      data: {
        data: [
          {
            id: 1,
            resource_type: "datasource",
            status: "submitted",
            is_update: false,
            resource_payload: {
              name: "Test Vector DB",
              short_description: "A test datasource",
            },
            submitted_at: "2024-01-15T10:00:00Z",
            submitter_feedback: null,
          },
          {
            id: 2,
            resource_type: "tool",
            status: "approved",
            is_update: false,
            resource_payload: {
              name: "Weather API",
              description: "Get weather data",
            },
            submitted_at: "2024-01-10T10:00:00Z",
            submitter_feedback: null,
          },
        ],
        total_count: 2,
      },
    });

    renderWithProviders(<MyContributions />);

    await waitFor(() => {
      expect(screen.getByText("Test Vector DB")).toBeInTheDocument();
      expect(screen.getByText("Weather API")).toBeInTheDocument();
    });
  });

  it("shows admin feedback for rejected submissions", async () => {
    pubClient.get.mockResolvedValueOnce({
      data: {
        data: [
          {
            id: 1,
            resource_type: "datasource",
            status: "rejected",
            is_update: false,
            resource_payload: { name: "Rejected DS" },
            submitted_at: "2024-01-15T10:00:00Z",
            submitter_feedback: "Missing documentation",
          },
        ],
        total_count: 1,
      },
    });

    renderWithProviders(<MyContributions />);

    await waitFor(() => {
      expect(screen.getByText("Missing documentation")).toBeInTheDocument();
    });
  });

  it("calls API with correct endpoint", async () => {
    pubClient.get.mockResolvedValueOnce({
      data: { data: [], total_count: 0 },
    });

    renderWithProviders(<MyContributions />);

    await waitFor(() => {
      expect(pubClient.get).toHaveBeenCalledWith(
        "/common/submissions",
        expect.objectContaining({
          params: expect.objectContaining({
            page_size: 50,
            page_number: 1,
          }),
        })
      );
    });
  });

  it("shows Submit Resource button", async () => {
    pubClient.get.mockResolvedValueOnce({
      data: { data: [], total_count: 0 },
    });

    renderWithProviders(<MyContributions />);
    const buttons = screen.getAllByText("Submit Resource");
    expect(buttons.length).toBeGreaterThan(0);
  });
});
