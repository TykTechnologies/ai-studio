import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import ManageTeamMembersModal from "./ManageTeamMembersModal";
import { useTeamMembersModal } from "../hooks/useTeamMembersModal";
import { teamsService } from "../../../services/teamsService";

jest.mock("../hooks/useTeamMembersModal");
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

describe("ManageTeamMembersModal", () => {
  const mockOnClose = jest.fn();
  const mockOnSuccess = jest.fn();
  const mockOnError = jest.fn();
  const mockUpdateGroupUsers = jest.fn();

  const mockGroup = {
    id: 1,
    attributes: {
      name: "Test Group"
    }
  };

  const mockAvailableUsers = [
    { id: "1", attributes: { name: "User 1", email: "user1@example.com", role: "Admin" } },
    { id: "2", attributes: { name: "User 2", email: "user2@example.com", role: "Chat user" } }
  ];

  const mockSelectedUsers = [
    { id: "3", attributes: { name: "User 3", email: "user3@example.com", role: "Admin" } }
  ];

  beforeEach(() => {
    jest.clearAllMocks();
    
    teamsService.updateGroupUsers = mockUpdateGroupUsers;
    
    useTeamMembersModal.mockReturnValue({
      availableUsers: mockAvailableUsers,
      selectedUsers: mockSelectedUsers,
      isLoadingMore: false,
      loading: false,
      hasMore: false,
      handleUsersChange: jest.fn(),
      handleSearch: jest.fn(),
      handleLoadMore: jest.fn(),
      handleUserAdded: jest.fn(),
      handleUserRemoved: jest.fn(),
    });
  });

  it("renders loading state when loading is true", () => {
    useTeamMembersModal.mockReturnValueOnce({
      availableUsers: [],
      selectedUsers: [],
      isLoadingMore: false,
      loading: true,
      hasMore: false,
      handleUsersChange: jest.fn(),
      handleSearch: jest.fn(),
      handleLoadMore: jest.fn(),
      handleUserAdded: jest.fn(),
      handleUserRemoved: jest.fn(),
    });

    renderWithTheme(
      <ManageTeamMembersModal
        open={true}
        onClose={mockOnClose}
        group={mockGroup}
        onSuccess={mockOnSuccess}
        onError={mockOnError}
      />
    );

    expect(screen.getByText("Manage Team Members")).toBeInTheDocument();
    expect(screen.getByRole("progressbar")).toBeInTheDocument();
  });

  it("renders TransferList when not loading", () => {
    renderWithTheme(
      <ManageTeamMembersModal
        open={true}
        onClose={mockOnClose}
        group={mockGroup}
        onSuccess={mockOnSuccess}
        onError={mockOnError}
      />
    );

    expect(screen.getByText("Manage Team Members")).toBeInTheDocument();
    expect(screen.getByText("Current members")).toBeInTheDocument();
    expect(screen.getByText("Add members")).toBeInTheDocument();
    expect(screen.getByText("Users currently on this team")).toBeInTheDocument();
    expect(screen.getByText("Add users to this team")).toBeInTheDocument();
  });

  it("handles saving successfully", async () => {
    mockUpdateGroupUsers.mockResolvedValueOnce({});

    renderWithTheme(
      <ManageTeamMembersModal
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
      expect(mockUpdateGroupUsers).toHaveBeenCalledWith(1, [3]);
    });
    
    await waitFor(() => {
      expect(mockOnSuccess).toHaveBeenCalledWith('Team members for "Test Group" updated successfully!');
    });
    
    await waitFor(() => {
      expect(mockOnClose).toHaveBeenCalled();
    });
  });

  it("handles saving with error", async () => {
    const mockError = new Error("Update failed");
    mockUpdateGroupUsers.mockRejectedValueOnce(mockError);

    renderWithTheme(
      <ManageTeamMembersModal
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
      expect(mockUpdateGroupUsers).toHaveBeenCalledWith(1, [3]);
    });
    
    await waitFor(() => {
      expect(mockOnError).toHaveBeenCalledWith("Failed to update team members. Please try again.");
    });
    
    await waitFor(() => {
      expect(mockOnClose).not.toHaveBeenCalled();
    });
    
    await waitFor(() => {
      expect(saveButton).not.toBeDisabled();
    });
  });

  it("does not call updateGroupUsers when group is not provided", async () => {
    renderWithTheme(
      <ManageTeamMembersModal
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
      expect(mockUpdateGroupUsers).not.toHaveBeenCalled();
    });
  });

  it("closes the modal when Cancel is clicked", () => {
    renderWithTheme(
      <ManageTeamMembersModal
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