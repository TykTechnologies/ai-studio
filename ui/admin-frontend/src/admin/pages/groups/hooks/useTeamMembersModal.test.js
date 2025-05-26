import { renderHook, act, waitFor } from "@testing-library/react";
import { useTeamMembersModal } from "./useTeamMembersModal";
import { getUsers } from "../../../services/userService";
import { teamsService } from "../../../services/teamsService";

jest.mock("../../../services/userService");
jest.mock("../../../services/teamsService");

describe("useTeamMembersModal", () => {
  const groupId = "123";
  const mockUsers = [
    { id: "1", attributes: { name: "User 1", email: "user1@example.com" } },
    { id: "2", attributes: { name: "User 2", email: "user2@example.com" } },
    { id: "3", attributes: { name: "User 3", email: "user3@example.com" } },
  ];
  
  const mockGroupUsers = [
    { id: "4", attributes: { name: "User 4", email: "user4@example.com" } },
    { id: "5", attributes: { name: "User 5", email: "user5@example.com" } },
  ];

  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(console, "error").mockImplementation(() => {});
    
    getUsers.mockResolvedValue({
      data: mockUsers,
      totalPages: 2,
      meta: { last_page: 2 }
    });
    
    teamsService.getTeamUsers.mockResolvedValue({
      data: mockGroupUsers,
      totalPages: 1,
      meta: { last_page: 1 }
    });
  });

  afterEach(() => {
    console.error.mockRestore();
  });

  it("initializes with correct state", async () => {
    const { result } = renderHook(() => useTeamMembersModal(groupId));
    
    expect(result.current.loading).toBe(true);
    expect(result.current.availableUsers).toEqual([]);
    expect(result.current.selectedUsers).toEqual([]);
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    expect(teamsService.getTeamUsers).toHaveBeenCalledWith(groupId, { all: true });
    expect(getUsers).toHaveBeenCalledWith(1, { exclude_group_id: groupId, page: 1 });
    expect(result.current.selectedUsers).toEqual(mockGroupUsers);
    expect(result.current.availableUsers).toEqual(mockUsers);
  });
  
  it("does not fetch data when groupId is not provided", async () => {
    const { result } = renderHook(() => useTeamMembersModal(null));
    
    expect(result.current.loading).toBe(false);
    expect(teamsService.getTeamUsers).not.toHaveBeenCalled();
    expect(getUsers).not.toHaveBeenCalled();
  });
  
  it("resets state when groupId changes", async () => {
    const { result, rerender } = renderHook((props) => useTeamMembersModal(props), {
      initialProps: groupId
    });
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    expect(result.current.selectedUsers).toEqual(mockGroupUsers);
    
    rerender("456");
    
    expect(result.current.loading).toBe(true);
    expect(result.current.selectedUsers).toEqual([]);
    expect(result.current.availableUsers).toEqual([]);
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    expect(teamsService.getTeamUsers).toHaveBeenCalledWith("456", { all: true });
    expect(getUsers).toHaveBeenCalledWith(1, { exclude_group_id: "456", page: 1 });
  });
  
  it("handles user addition correctly", async () => {
    const { result } = renderHook(() => useTeamMembersModal(groupId));
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    const userToAdd = { id: "1", attributes: { name: "User 1" } };
    
    act(() => {
      result.current.handleUserAdded(userToAdd);
    });
    
    expect(result.current.selectedUsers).toContainEqual(userToAdd);
  });
  
  it("handles user removal correctly", async () => {
    const { result } = renderHook(() => useTeamMembersModal(groupId));
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    const userToRemove = mockGroupUsers[0];
    
    act(() => {
      result.current.handleUserRemoved(userToRemove);
    });
    
    expect(result.current.selectedUsers).not.toContainEqual(userToRemove);
  });
  
  it("handles loading more users", async () => {
    const { result } = renderHook(() => useTeamMembersModal(groupId));
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    const page2Users = [
      { id: "6", attributes: { name: "User 6", email: "user6@example.com" } },
      { id: "7", attributes: { name: "User 7", email: "user7@example.com" } },
    ];
    
    getUsers.mockResolvedValueOnce({
      data: page2Users,
      totalPages: 2,
      meta: { last_page: 2 }
    });
    
    act(() => {
      result.current.handleLoadMore();
    });
    
    expect(result.current.isLoadingMore).toBe(true);
    
    await waitFor(() => {
      expect(result.current.isLoadingMore).toBe(false);
    });
    
    expect(getUsers).toHaveBeenCalledWith(2, { exclude_group_id: groupId, page: 2 });
  });
  
  it("handles search correctly", async () => {
    const { result } = renderHook(() => useTeamMembersModal(groupId));
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    const searchTerm = "searchQuery";
    const searchResults = [
      { id: "8", attributes: { name: "Matching User", email: "match@example.com" } }
    ];
    
    getUsers.mockResolvedValueOnce({
      data: searchResults,
      totalPages: 1,
      meta: { last_page: 1 }
    });
    
    act(() => {
      result.current.handleSearch(searchTerm);
    });
    
    expect(result.current.currentSearchTerm).toBe(searchTerm);
    
    await waitFor(() => {
      expect(getUsers).toHaveBeenCalledWith(1, {
        exclude_group_id: groupId,
        search: searchTerm,
        page: 1
      });
    });
  });
  
  it("clears search when empty search term is provided", async () => {
    const { result } = renderHook(() => useTeamMembersModal(groupId));
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    act(() => {
      result.current.handleSearch("  ");
    });
    
    expect(result.current.currentSearchTerm).toBe("  ");
    
    await waitFor(() => {
      expect(getUsers).toHaveBeenCalledTimes(1); // Only the initial call
    });
  });
  
  it("handles error when fetching group members", async () => {
    teamsService.getTeamUsers.mockRejectedValueOnce(new Error("API error"));
    
    const { result } = renderHook(() => useTeamMembersModal(groupId));
    
    await waitFor(() => {
      expect(console.error).toHaveBeenCalledWith(
        expect.stringContaining("Error fetching group members:"),
        expect.any(Error)
      );
    });
    
    expect(result.current.selectedUsers).toEqual([]);
  });
  
  it("handles error when fetching users", async () => {
    getUsers.mockRejectedValueOnce(new Error("API error"));
    
    const { result } = renderHook(() => useTeamMembersModal(groupId));
    
    await waitFor(() => {
      expect(console.error).toHaveBeenCalledWith(
        expect.stringContaining("Error fetching users:"),
        expect.any(Error)
      );
    });
  });
  
  it("updates available users when selected users change", async () => {
    const { result } = renderHook(() => useTeamMembersModal(groupId));
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    const initialAvailableCount = result.current.availableUsers.length;
    const userToAdd = mockUsers[0];
    
    act(() => {
      result.current.handleUserAdded(userToAdd);
    });
    
    expect(result.current.availableUsers.length).toBeLessThan(initialAvailableCount);
    expect(result.current.availableUsers).not.toContainEqual(userToAdd);
  });
  
  it("handles batch user changes correctly", async () => {
    const { result } = renderHook(() => useTeamMembersModal(groupId));
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    const newSelection = [mockUsers[0], mockUsers[1]];
    
    act(() => {
      result.current.handleUsersChange({ selected: newSelection });
    });
    
    expect(result.current.selectedUsers).toEqual(newSelection);
  });
});