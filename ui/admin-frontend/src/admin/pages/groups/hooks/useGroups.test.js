import { renderHook, act, waitFor } from '@testing-library/react';
import useGroups from './useGroups';
import { teamsService } from '../../../services/teamsService';
import * as useDebounceLibrary from 'use-debounce';

jest.mock('../../../services/teamsService', () => ({
  teamsService: {
    getTeams: jest.fn(),
  },
}));

const mockTeamData = {
  data: {
    data: [
      { id: '1', name: 'Group 1' },
      { id: '2', name: 'Group 2' },
    ],
  },
  headers: {
    'x-total-count': '2',
    'x-total-pages': '1',
  },
};

const mockError = new Error('Failed to fetch');

describe('useGroups', () => {
  let useDebounceSpy;

  beforeEach(() => {
    jest.clearAllMocks();
    teamsService.getTeams.mockResolvedValue(mockTeamData);

    const mockControls = {
      isPending: jest.fn(() => false),
      cancel: jest.fn(),
      flush: jest.fn(),
    };
    useDebounceSpy = jest.spyOn(useDebounceLibrary, 'useDebounce').mockImplementation((value) => [value, mockControls]);
  });

  afterEach(() => {
    if (useDebounceSpy) {
      useDebounceSpy.mockRestore();
    }
  });

  it('should initialize with default values', async () => {
    const { result } = renderHook(() => useGroups());

    expect(result.current.groups).toEqual([]);
    expect(result.current.loading).toBe(true);
    expect(result.current.error).toBe('');
    expect(result.current.page).toBe(1);
    expect(result.current.pageSize).toBe(10);
    expect(result.current.totalPages).toBe(0);
    expect(result.current.sortConfig).toEqual({ field: 'id', direction: 'asc' });

    await waitFor(() => expect(result.current.loading).toBe(false));
  });

  it('should fetch groups successfully', async () => {
    const { result } = renderHook(() => useGroups());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(teamsService.getTeams).toHaveBeenCalledWith({
      page: 1,
      page_size: 10,
      search: '',
      sort: 'id',
    });
    expect(result.current.groups).toEqual(mockTeamData.data.data);
    expect(result.current.totalPages).toBe(1);
    expect(result.current.error).toBe('');
  });

  it('should handle fetch groups error', async () => {
    teamsService.getTeams.mockRejectedValueOnce(mockError);
    const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    const { result } = renderHook(() => useGroups());

    await waitFor(() => expect(result.current.loading).toBe(false));
    
    expect(result.current.groups).toEqual([]);
    expect(result.current.error).toBe('Failed to load groups');
    expect(consoleErrorSpy).toHaveBeenCalledWith('Error fetching groups', mockError);
    consoleErrorSpy.mockRestore();
  });

  it('should handle search', async () => {
    const { result } = renderHook(() => useGroups());
    await waitFor(() => expect(teamsService.getTeams).toHaveBeenCalledTimes(1)); // Initial fetch

    act(() => {
      result.current.handleSearch('Test Search');
    });
    
    await waitFor(() => expect(teamsService.getTeams).toHaveBeenCalledTimes(2));

    expect(teamsService.getTeams).toHaveBeenLastCalledWith({
      page: 1,
      page_size: 10,
      search: 'Test Search',
      sort: 'id',
    });
  });

  it('should handle sort change', async () => {
    const { result } = renderHook(() => useGroups());
    await waitFor(() => expect(teamsService.getTeams).toHaveBeenCalledTimes(1)); 

    act(() => {
      result.current.handleSortChange({ field: 'name', direction: 'desc' });
    });
    
    await waitFor(() => expect(teamsService.getTeams).toHaveBeenCalledTimes(2));

    expect(result.current.sortConfig).toEqual({ field: 'name', direction: 'desc' });
    expect(teamsService.getTeams).toHaveBeenLastCalledWith({
      page: 1,
      page_size: 10,
      search: '',
      sort: '-name',
    });
  });

  it('should handle page change', async () => {
    const { result } = renderHook(() => useGroups());
    await waitFor(() => expect(teamsService.getTeams).toHaveBeenCalledTimes(1));

    act(() => {
      result.current.handlePageChange(null, 2); // mui pagination specific
    });
    await waitFor(() => expect(teamsService.getTeams).toHaveBeenCalledTimes(2));
    
    expect(result.current.page).toBe(2);
    expect(teamsService.getTeams).toHaveBeenLastCalledWith({
      page: 2,
      page_size: 10,
      search: '',
      sort: 'id',
    });
  });

  it('should handle page size change', async () => {
    const { result } = renderHook(() => useGroups());
    await waitFor(() => expect(teamsService.getTeams).toHaveBeenCalledTimes(1));

    act(() => {
      result.current.handlePageSizeChange({ target: { value: 20 } });
    });
    await waitFor(() => expect(teamsService.getTeams).toHaveBeenCalledTimes(2));

    expect(result.current.pageSize).toBe(20);
    expect(teamsService.getTeams).toHaveBeenLastCalledWith({
      page: 1, // Page should reset to 1
      page_size: 20,
      search: '',
      sort: 'id',
    });
  });
  
  it('should refresh groups when refreshGroups is called', async () => {
    const { result } = renderHook(() => useGroups());
    await waitFor(() => expect(teamsService.getTeams).toHaveBeenCalledTimes(1)); 
    
    teamsService.getTeams.mockClear(); // Clear previous calls for this specific test logic

    act(() => {
      result.current.refreshGroups();
    });
    await waitFor(() => expect(teamsService.getTeams).toHaveBeenCalledTimes(1)); // Called once after clear

     expect(teamsService.getTeams).toHaveBeenLastCalledWith({
      page: 1,
      page_size: 10,
      search: '',
      sort: 'id',
    });
  });
}); 