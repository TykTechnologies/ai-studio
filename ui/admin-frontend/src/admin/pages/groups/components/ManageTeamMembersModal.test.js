import React from "react";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import ManageTeamMembersModal from "./ManageTeamMembersModal";
import { useTransferListSelectedUsers } from "../../../hooks/useTransferListSelectedUsers";
import { useTransferListAvailableUsers } from "../../../hooks/useTransferListAvailableUsers";
import { teamsService } from "../../../services/teamsService";
import { renderWithTheme } from "../../../../test-utils/render-with-theme";

jest.mock("../../../hooks/useTransferListSelectedUsers");
jest.mock("../../../hooks/useTransferListAvailableUsers");
jest.mock("../../../services/teamsService");

jest.mock('@mui/material', () => require('../../../../test-utils/mui-mocks').muiMaterialMock);
jest.mock('@mui/styled-engine', () => require('../../../../test-utils/mui-mocks').muiStyledEngineMock);
jest.mock('@mui/material/styles', () => require('../../../../test-utils/mui-mocks').muiStylesMock);
jest.mock('@mui/material/IconButton', () => require('../../../../test-utils/mui-mocks').muiIconButtonMock);
jest.mock('../../../styles/sharedStyles', () => require('../../../../test-utils/styled-component-mocks').sharedStylesMock);
jest.mock('../../../components/common/styles', () => require('../../../../test-utils/styled-component-mocks').actionModalStylesMock);

jest.mock("../../../components/common/transfer-list/TransferList", () => require('../../../../test-utils/component-mocks').transferListMock);

describe("ManageTeamMembersModal", () => {
  const mockOnClose = jest.fn();
  const mockOnSuccess = jest.fn();
  const mockOnError = jest.fn();
  const mockGroup = { id: "1", attributes: { name: "Test Group" } };

  const mockAvailableUsers = [
    { id: "1", attributes: { name: "User One" } },
    { id: "2", attributes: { name: "User Two" } },
  ];
  const mockSelectedUsers = [
    { id: "3", attributes: { name: "User Three" } },
  ];

  beforeEach(() => {
    jest.clearAllMocks();
    const mockUpdateGroupUsers = jest.fn().mockResolvedValue({});
    teamsService.updateGroupUsers = mockUpdateGroupUsers;
    
    useTransferListSelectedUsers.mockReturnValue({
      members: mockSelectedUsers,
      addMember: jest.fn(),
      removeMember: jest.fn(),
      loading: false
    });
    
    useTransferListAvailableUsers.mockReturnValue({
      items: mockAvailableUsers,
      loading: false,
      isSearching: false,
      hasMore: false,
      isLoadingMore: false,
      searchTerm: "",
      loadMore: jest.fn(),
      search: jest.fn(),
      addItem: jest.fn(),
      removeItem: jest.fn()
    });
  });

  it("renders loading state when loading is true", () => {
    useTransferListAvailableUsers.mockReturnValueOnce({
      items: [],
      loading: true,
      isSearching: false,
      hasMore: false,
      isLoadingMore: false,
      searchTerm: "",
      loadMore: jest.fn(),
      search: jest.fn(),
      addItem: jest.fn(),
      removeItem: jest.fn()
    });

    renderWithTheme(
      <ManageTeamMembersModal
        open={true}
        group={mockGroup}
        onClose={mockOnClose}
        onSuccess={mockOnSuccess}
        onError={mockOnError}
      />
    );
    expect(screen.getByTestId("circular-progress")).toBeInTheDocument();
  });

  it("renders TransferList when not loading", () => {
    renderWithTheme(
      <ManageTeamMembersModal
        open={true}
        group={mockGroup}
        onClose={mockOnClose}
        onSuccess={mockOnSuccess}
        onError={mockOnError}
      />
    );
    expect(screen.getByTestId("transfer-list")).toBeInTheDocument();
  });

  it("calls onSave with selected users when primary button is clicked", async () => {
    renderWithTheme(
      <ManageTeamMembersModal
        open={true}
        group={mockGroup}
        onClose={mockOnClose}
        onSuccess={mockOnSuccess}
        onError={mockOnError}
      />
    );

    fireEvent.click(screen.getByText("Save"));

    await waitFor(() => {
      expect(teamsService.updateGroupUsers).toHaveBeenCalledWith("1", [3]);
    });
    await waitFor(() => {
      expect(mockOnSuccess).toHaveBeenCalledWith('Team members for "Test Group" updated successfully!');
    });
    await waitFor(() => {
      expect(mockOnClose).toHaveBeenCalled();
    });
  });

  it("calls onError when saving fails", async () => {
    teamsService.updateGroupUsers.mockRejectedValueOnce(new Error("Save failed"));
    renderWithTheme(
      <ManageTeamMembersModal
        open={true}
        group={mockGroup}
        onClose={mockOnClose}
        onSuccess={mockOnSuccess}
        onError={mockOnError}
      />
    );

    fireEvent.click(screen.getByText("Save"));

    await waitFor(() => {
      expect(mockOnError).toHaveBeenCalledWith("Failed to update team members. Please try again.");
    });
  });

  it("calls onClose when secondary button is clicked", () => {
    renderWithTheme(
      <ManageTeamMembersModal
        open={true}
        group={mockGroup}
        onClose={mockOnClose}
        onSuccess={mockOnSuccess}
        onError={mockOnError}
      />
    );
    fireEvent.click(screen.getByText("Cancel"));
    expect(mockOnClose).toHaveBeenCalled();
  });
});