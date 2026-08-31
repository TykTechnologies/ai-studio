import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../utils/testTheme";
import { MemoryRouter } from "react-router-dom";
import SubmissionReviewQueue from "./SubmissionReviewQueue";

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: {
    get: jest.fn(),
  },
}));

// The mock must return the SAME callbacks on every render. Returning fresh
// jest.fn()s made updatePaginationData a new identity each time, so the
// component's fetchSubmissions useCallback never stabilised, its effect
// re-ran forever and the queue stayed in its loading state -- which made the
// empty-state assertion pass or fail depending on which other suite had run
// first.
const mockPagination = {
  page: 1,
  pageSize: 20,
  totalPages: 1,
  handlePageChange: jest.fn(),
  handlePageSizeChange: jest.fn(),
  updatePaginationData: jest.fn(),
};

jest.mock("../hooks/usePagination", () => ({
  __esModule: true,
  default: () => mockPagination,
}));

jest.mock("../components/common/PaginationControls", () => ({
  __esModule: true,
  default: () => <div data-testid="pagination-controls" />,
}));

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const apiClient = require("../utils/apiClient").default;

const renderWithProviders = (ui) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>{ui}</MemoryRouter>
    </ThemeProvider>
  );

describe("SubmissionReviewQueue", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockResolvedValue({
      data: { data: [], status_counts: {}, total_count: 0, total_pages: 0 },
    });
  });

  it("renders the page title", () => {
    renderWithProviders(<SubmissionReviewQueue />);
    expect(screen.getByText("Submission Queue")).toBeInTheDocument();
  });

  it("renders submissions in table", async () => {
    apiClient.get.mockResolvedValue({
      data: {
        data: [
          {
            id: 1,
            resource_type: "datasource",
            status: "submitted",
            is_update: false,
            resource_payload: { name: "Test DS" },
            submitter: { name: "Dev User" },
            submitter_id: 1,
            submitted_at: "2024-01-15T10:00:00Z",
          },
        ],
        status_counts: { submitted: 1 },
        total_count: 1,
        total_pages: 1,
      },
    });

    renderWithProviders(<SubmissionReviewQueue />);

    await waitFor(() => {
      expect(screen.getByText("Test DS")).toBeInTheDocument();
    });
  });

  it("shows empty state when no submissions", async () => {
    renderWithProviders(<SubmissionReviewQueue />);

    await waitFor(() => {
      expect(screen.getByText("No submissions found")).toBeInTheDocument();
    });
  });

  it("calls API with submissions endpoint", async () => {
    renderWithProviders(<SubmissionReviewQueue />);

    await waitFor(() => {
      expect(apiClient.get).toHaveBeenCalledWith(
        "/submissions",
        expect.any(Object)
      );
    });
  });
});
