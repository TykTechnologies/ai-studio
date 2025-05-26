import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import ManageGroupCatalogsModal from "./ManageGroupCatalogsModal";
import { useCatalogsModal } from "../hooks/useCatalogsModal";
import { teamsService } from "../../../services/teamsService";

jest.mock("../hooks/useCatalogsModal");
jest.mock("../../../services/teamsService");

const theme = createTheme({
  palette: {
    text: {
      primary: '#000000',
      secondary: '#666666',
      defaultSubdued: '#454545',
      neutralDisabled: '#AAAAAA'
    },
    background: {
      paper: '#FFFFFF',
      default: '#F5F5F5',
      surfaceNeutralHover: '#F0F0F0',
      surfaceNeutralDisabled: '#EEEEEE',
      buttonPrimaryDefault: '#3F51B5',
      buttonPrimaryDefaultHover: '#303F9F',
      buttonPrimaryOutlineHover: '#E8EAF6',
      defaultSubdued: '#FAFAFA'
    },
    border: {
      neutralDefault: '#E0E0E0',
      neutralHovered: '#BDBDBD',
      criticalDefault: '#F44336',
      criticalHover: '#D32F2F'
    },
    primary: {
      main: '#3F51B5',
      light: '#7986CB'
    },
    custom: {
      white: '#FFFFFF',
      purpleExtraDark: '#1A237E'
    }
  }
});

const renderWithTheme = (ui) => {
  return render(
    <ThemeProvider theme={theme}>
      {ui}
    </ThemeProvider>
  );
};

describe("ManageGroupCatalogsModal", () => {
  const mockOnClose = jest.fn();
  const mockOnSuccess = jest.fn();
  const mockOnError = jest.fn();
  const mockUpdateGroupCatalogs = jest.fn();

  const mockGroup = {
    id: 1,
    attributes: {
      name: "Test Group"
    }
  };

  const mockCatalogs = [
    { value: "1", label: "Catalog 1" },
    { value: "2", label: "Catalog 2" }
  ];

  const mockDataCatalogs = [
    { value: "3", label: "Data Catalog 1" },
    { value: "4", label: "Data Catalog 2" }
  ];

  const mockToolCatalogs = [
    { value: "5", label: "Tool Catalog 1" },
    { value: "6", label: "Tool Catalog 2" }
  ];

  const mockSelectedCatalogs = [mockCatalogs[0]];
  const mockSelectedDataCatalogs = [mockDataCatalogs[0]];
  const mockSelectedToolCatalogs = [mockToolCatalogs[0]];

  beforeEach(() => {
    jest.clearAllMocks();
    
    teamsService.updateGroupCatalogs = mockUpdateGroupCatalogs;
    
    useCatalogsModal.mockReturnValue({
      catalogs: mockCatalogs,
      selectedCatalogs: mockSelectedCatalogs,
      setSelectedCatalogs: jest.fn(),
      dataCatalogs: mockDataCatalogs,
      selectedDataCatalogs: mockSelectedDataCatalogs,
      setSelectedDataCatalogs: jest.fn(),
      toolCatalogs: mockToolCatalogs,
      selectedToolCatalogs: mockSelectedToolCatalogs,
      setSelectedToolCatalogs: jest.fn(),
      loading: false
    });
  });

  it("renders loading state when loading is true", () => {
    useCatalogsModal.mockReturnValueOnce({
      catalogs: [],
      selectedCatalogs: [],
      setSelectedCatalogs: jest.fn(),
      dataCatalogs: [],
      selectedDataCatalogs: [],
      setSelectedDataCatalogs: jest.fn(),
      toolCatalogs: [],
      selectedToolCatalogs: [],
      setSelectedToolCatalogs: jest.fn(),
      loading: true
    });

    renderWithTheme(
      <ManageGroupCatalogsModal
        open={true}
        onClose={mockOnClose}
        group={mockGroup}
        onSuccess={mockOnSuccess}
        onError={mockOnError}
      />
    );

    expect(screen.getByText("Manage Catalogs")).toBeInTheDocument();
    expect(screen.getByRole("progressbar")).toBeInTheDocument();
  });

  it("renders catalog selectors when not loading", () => {
    renderWithTheme(
      <ManageGroupCatalogsModal
        open={true}
        onClose={mockOnClose}
        group={mockGroup}
        onSuccess={mockOnSuccess}
        onError={mockOnError}
      />
    );

    expect(screen.getByText("Manage Catalogs")).toBeInTheDocument();
    expect(screen.getByText("Select one or more catalogs to make available to this team")).toBeInTheDocument();
    expect(screen.getByText("LLM providers catalogs")).toBeInTheDocument();
    expect(screen.getByText("Data sources catalogs")).toBeInTheDocument();
    expect(screen.getByText("Tools catalogs")).toBeInTheDocument();
  });

  it("handles saving successfully", async () => {
    mockUpdateGroupCatalogs.mockResolvedValueOnce({});

    renderWithTheme(
      <ManageGroupCatalogsModal
        open={true}
        onClose={mockOnClose}
        group={mockGroup}
        onSuccess={mockOnSuccess}
        onError={mockOnError}
      />
    );

    const saveButton = screen.getByRole("button", { name: "Save" });
    expect(saveButton).not.toBeDisabled();
    
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(mockUpdateGroupCatalogs).toHaveBeenCalledWith(1, {
        data: {
          type: "Group",
          attributes: {
            catalogues: [1],
            data_catalogues: [3],
            tool_catalogues: [5]
          }
        }
      });
    });
    
    await waitFor(() => {
      expect(mockOnSuccess).toHaveBeenCalledWith('Catalogs for "Test Group" updated successfully!');
    });
    
    await waitFor(() => {
      expect(mockOnClose).toHaveBeenCalled();
    });
  });

  it("handles saving with error", async () => {
    const mockError = new Error("Update failed");
    mockUpdateGroupCatalogs.mockRejectedValueOnce(mockError);

    renderWithTheme(
      <ManageGroupCatalogsModal
        open={true}
        onClose={mockOnClose}
        group={mockGroup}
        onSuccess={mockOnSuccess}
        onError={mockOnError}
      />
    );

    const saveButton = screen.getByRole("button", { name: "Save" });
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(mockUpdateGroupCatalogs).toHaveBeenCalledWith(1, {
        data: {
          type: "Group",
          attributes: {
            catalogues: [1],
            data_catalogues: [3],
            tool_catalogues: [5]
          }
        }
      });
    });
    
    await waitFor(() => {
      expect(mockOnError).toHaveBeenCalledWith("Failed to update catalogs. Please try again.");
    });
    
    await waitFor(() => {
      expect(mockOnClose).not.toHaveBeenCalled();
    });
    
    await waitFor(() => {
      expect(saveButton).not.toBeDisabled();
    });
  });

  it("does not call updateGroupCatalogs when group is not provided", async () => {
    renderWithTheme(
      <ManageGroupCatalogsModal
        open={true}
        onClose={mockOnClose}
        group={null}
        onSuccess={mockOnSuccess}
        onError={mockOnError}
      />
    );

    const saveButton = screen.getByRole("button", { name: "Save" });
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(mockUpdateGroupCatalogs).not.toHaveBeenCalled();
    });
  });

  it("closes the modal when Cancel is clicked", () => {
    renderWithTheme(
      <ManageGroupCatalogsModal
        open={true}
        onClose={mockOnClose}
        group={mockGroup}
        onSuccess={mockOnSuccess}
        onError={mockOnError}
      />
    );

    const cancelButton = screen.getByRole("button", { name: "Cancel" });
    fireEvent.click(cancelButton);

    expect(mockOnClose).toHaveBeenCalled();
  });
});