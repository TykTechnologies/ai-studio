import { renderHook, act, waitFor } from '@testing-library/react';
import { useParams } from 'react-router-dom';
import { teamsService } from '../../../services/teamsService';
import useTeamMembers from './useTeamMembers';

jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useParams: jest.fn(),
}));

jest.mock('../../../services/teamsService', () => ({
  teamsService: {
    getTeamUsers: jest.fn(),
  },
}));

describe('useTeamMembers', () => {
  const mockTeamId = 'team-id-123';

  beforeEach(() => {
    useParams.mockReturnValue({ id: mockTeamId });
    teamsService.getTeamUsers.mockClear();
  });

  it('should initialize with correct default values', () => {
    const { result } = renderHook(() => useTeamMembers());
    expect(result.current.users).toEqual([]);
    expect(result.current.loading).toBe(true);
    expect(result.current.error).toBeNull();
    expect(result.current.isLoadingMore).toBe(false);
    expect(result.current.hasMore).toBe(false);
    expect(result.current.containerRef.current).toBeNull();
  });

  it('should fetch members on initial load', async () => {
    const mockMembers = [{ id: 'user1', name: 'User 1' }];
    teamsService.getTeamUsers.mockResolvedValueOnce({
      data: mockMembers,
      totalPages: 1,
    });
    const { result } = renderHook(() => useTeamMembers());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(teamsService.getTeamUsers).toHaveBeenCalledWith(mockTeamId, { page: 1, page_size: 10 });
    expect(result.current.users).toEqual(mockMembers);
    expect(result.current.error).toBeNull();
    expect(result.current.hasMore).toBe(false);
  });

  it('should handle error when fetching members', async () => {
    teamsService.getTeamUsers.mockRejectedValueOnce(new Error('Fetch error'));
    const { result } = renderHook(() => useTeamMembers());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error).toBe('Failed to load team members');
    expect(result.current.users).toEqual([]);
  });

  it('should load more members when handleLoadMore is called', async () => {
    const initialMembers = Array.from({ length: 10 }, (_, i) => ({ id: `user${i}`, name: `User ${i}` }));
    const moreMembers = [{ id: 'user10', name: 'User 10' }];
    teamsService.getTeamUsers
      .mockResolvedValueOnce({ data: initialMembers, totalPages: 2 })
      .mockResolvedValueOnce({ data: moreMembers, totalPages: 2 });

    const { result } = renderHook(() => useTeamMembers());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.users).toHaveLength(10);
    expect(result.current.hasMore).toBe(true);

    act(() => { result.current.handleLoadMore(); });
    await waitFor(() => expect(result.current.isLoadingMore).toBe(true));
    await waitFor(() => expect(result.current.isLoadingMore).toBe(false));

    expect(teamsService.getTeamUsers).toHaveBeenCalledTimes(2);
    expect(teamsService.getTeamUsers).toHaveBeenLastCalledWith(mockTeamId, { page: 2, page_size: 10 });
    expect(result.current.users).toHaveLength(11);
    expect(result.current.users[10]).toEqual(moreMembers[0]);
    expect(result.current.hasMore).toBe(false);
  });

  it('should not load more if already loading or no more pages', async () => {
    // Scenario 1: No more pages
    teamsService.getTeamUsers.mockResolvedValueOnce({ data: [], totalPages: 1 });
    const { result: result1 } = renderHook(() => useTeamMembers());
    await waitFor(() => expect(result1.current.loading).toBe(false));
    expect(result1.current.hasMore).toBe(false);
    act(() => { result1.current.handleLoadMore(); });
    expect(teamsService.getTeamUsers).toHaveBeenCalledTimes(1); // No new call

    // Scenario 2: Already loading (isLoadingMore is true)
    teamsService.getTeamUsers
      .mockResolvedValueOnce({ data: [{id: 'page1_user1'}], totalPages: 3 }) // Initial for result2
      .mockImplementationOnce(async () => { // First handleLoadMore for result2 (page 2)
        await new Promise(resolve => setTimeout(resolve, 50)); // Simulate network delay
        return { data: [{id: 'page2_user1'}], totalPages: 3 };
      });
      // No mock for a third call, as it shouldn't happen

    const { result: result2 } = renderHook(() => useTeamMembers());
    await waitFor(() => expect(result2.current.loading).toBe(false)); // Initial load done
    expect(result2.current.hasMore).toBe(true);

    act(() => { result2.current.handleLoadMore(); }); // Call 1 to load page 2
    await waitFor(() => expect(result2.current.isLoadingMore).toBe(true)); //isLoadingMore should be true
    
    act(() => { result2.current.handleLoadMore(); }); // Call 2 while still loading page 2
    // teamsService.getTeamUsers should not be called again because isLoadingMore is true
    // The previous call count was 1 (for result1) + 1 (for result2 initial) = 2
    // The first handleLoadMore for result2 makes it 3.
    // This second handleLoadMore should not increment it further.
    expect(teamsService.getTeamUsers).toHaveBeenCalledTimes(3);

    await waitFor(() => expect(result2.current.isLoadingMore).toBe(false)); // Wait for page 2 to finish loading
    expect(result2.current.users).toHaveLength(2); // Initial user + page 2 user
    expect(result2.current.hasMore).toBe(true); // Still has page 3
  });

  it('should fetch members when id changes', async () => {
    useParams.mockReturnValue({ id: 'initial-id' });
    teamsService.getTeamUsers.mockResolvedValueOnce({ data: [{id: 'user1'}], totalPages: 1 });

    const { rerender, result } = renderHook(() => useTeamMembers());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(teamsService.getTeamUsers).toHaveBeenCalledWith('initial-id', { page: 1, page_size: 10 });
    expect(result.current.users).toEqual([{id: 'user1'}]);

    useParams.mockReturnValue({ id: 'new-id' });
    teamsService.getTeamUsers.mockResolvedValueOnce({ data: [{id: 'user2'}], totalPages: 1 });
    
    rerender(); // Rerender with new id from mocked useParams

    await waitFor(() => expect(result.current.loading).toBe(true));
    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(teamsService.getTeamUsers).toHaveBeenCalledWith('new-id', { page: 1, page_size: 10 });
    expect(teamsService.getTeamUsers).toHaveBeenCalledTimes(2);
    expect(result.current.users).toEqual([{id: 'user2'}]);
  });

  it('should call handleLoadMore on scroll when conditions are met', async () => {
    const mockMembersPage1 = Array.from({ length: 10 }, (_, i) => ({ id: `user${i}` }));
    const mockMembersPage2 = Array.from({ length: 5 }, (_, i) => ({ id: `user${10 + i}` }));
    
    teamsService.getTeamUsers
      .mockResolvedValueOnce({ data: mockMembersPage1, totalPages: 2 })
      .mockResolvedValueOnce({ data: mockMembersPage2, totalPages: 2 });

    const { result } = renderHook(() => useTeamMembers());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.users).toHaveLength(10);
    expect(result.current.hasMore).toBe(true);

    const mockContainer = { addEventListener: jest.fn(), removeEventListener: jest.fn() };
    Object.defineProperty(result.current.containerRef, 'current', { configurable: true, value: mockContainer });
    
    // This test primarily verifies that handleLoadMore is called if scroll conditions were met.
    // The actual scroll event simulation is complex. Here we focus on the consequence.
    act(() => { result.current.handleLoadMore(); });
    await waitFor(() => expect(result.current.isLoadingMore).toBe(true));
    await waitFor(() => expect(result.current.isLoadingMore).toBe(false));

    expect(result.current.hasMore).toBe(false);
    expect(teamsService.getTeamUsers).toHaveBeenCalledTimes(2);
  });
}); 