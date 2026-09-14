import React from "react";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import ManageTeamMembersModal from "./ManageTeamMembersModal";
import { useTransferListSelectedUsers } from "../../../hooks/useTransferListSelectedUsers";
import { getUsers } from "../../../services/userService";
import { teamsService } from "../../../services/teamsService";
import { renderWithTheme } from "../../../../test-utils/render-with-theme";

jest.mock("../../../hooks/useTransferListSelectedUsers");
jest.mock("../../../services/userService");
jest.mock("../../../services/teamsService");

jest.mock('@mui/material', () => require('../../../../test-utils/mui-mocks').muiMaterialMock);
jest.mock('@mui/styled-engine', () => require('../../../../test-utils/mui-mocks').muiStyledEngineMock);
jest.mock('@mui/material/styles', () => require('../../../../test-utils/mui-mocks').muiStylesMock);
jest.mock('@mui/material/IconButton', () => require('../../../../test-utils/mui-mocks').muiIconButtonMock);
jest.mock('../../../styles/sharedStyles', () => require('../../../../test-utils/styled-component-mocks').sharedStylesMock);
jest.mock('../../../components/common/styles', () => require('../../../../test-utils/styled-component-mocks').actionModalStylesMock);

jest.mock("../../../components/common/relationship-picker", () => require('../../../../test-utils/component-mocks').relationshipPickerMock);

const pickerMock = require('../../../../test-utils/component-mocks').relationshipPickerMock;

describe("ManageTeamMembersModal", () => {
  const mockOnClose = jest.fn();
  const mockOnSuccess = jest.fn();
  const mockOnError = jest.fn();
  const mockGroup = { id: "1", attributes: { name: "Test Group" } };
  const mockSetMembers = jest.fn();

  const mockSelectedUsers = [
    { id: "3", attributes: { name: "User Three", email: "three@example.com" } },
  ];

  beforeEach(() => {
    jest.clearAllMocks();
    pickerMock.clearLastProps();
    const mockUpdateGroupUsers = jest.fn().mockResolvedValue({});
    teamsService.updateGroupUsers = mockUpdateGroupUsers;
    getUsers.mockResolvedValue({ data: [], totalPages: 0 });

    useTransferListSelectedUsers.mockReturnValue({
      members: mockSelectedUsers,
      setMembers: mockSetMembers,
      addMember: jest.fn(),
      removeMember: jest.fn(),
      loading: false
    });
  });

  it("renders loading state while the current members load", () => {
    useTransferListSelectedUsers.mockReturnValueOnce({
      members: [],
      setMembers: mockSetMembers,
      addMember: jest.fn(),
      removeMember: jest.fn(),
      loading: true
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
    expect(screen.queryByTestId("relationship-picker")).not.toBeInTheDocument();
    // Save is disabled while loading
    expect(screen.getByText("Save")).toBeDisabled();
  });

  it("renders a dual RelationshipPicker for users when not loading", () => {
    renderWithTheme(
      <ManageTeamMembersModal
        open={true}
        group={mockGroup}
        onClose={mockOnClose}
        onSuccess={mockOnSuccess}
        onError={mockOnError}
      />
    );
    const picker = screen.getByTestId("relationship-picker");
    expect(picker).toHaveAttribute("data-variant", "dual");
    expect(picker).toHaveAttribute("data-item-label", "user");
    expect(picker).toHaveAttribute("aria-label", "Team members");

    const lastProps = pickerMock.getLastProps();
    expect(lastProps.value).toEqual(mockSelectedUsers);
    expect(lastProps.onChange).toBe(mockSetMembers);
    expect(lastProps.getOptionLabel(mockSelectedUsers[0])).toBe("User Three");
    expect(lastProps.getOptionSecondary(mockSelectedUsers[0])).toBe("three@example.com");
    expect(typeof lastProps.source.search).toBe("function");
  });

  it("feeds the picker a paged source that excludes the group and searches", async () => {
    getUsers.mockResolvedValue({
      data: [{ id: "9", attributes: { name: "Nine" } }],
      totalPages: 2
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

    const { source } = pickerMock.getLastProps();
    const result = await source.search("ni", 1);

    expect(getUsers).toHaveBeenCalledWith(1, {
      exclude_group_id: "1",
      page_size: 10,
      search: "ni"
    });
    expect(result).toEqual({
      items: [{ id: "9", attributes: { name: "Nine" } }],
      hasMore: true
    });
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
