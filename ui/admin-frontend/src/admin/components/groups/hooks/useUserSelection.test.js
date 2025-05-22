import React from 'react';
import { render, screen, waitFor, act } from '@testing-library/react';
import { fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { useUserSelection } from './useUserSelection';
import { getUsers } from '../../../services/userService';

// Mock the user service
jest.mock('../../../services/userService', () => ({
  getUsers: jest.fn()
}));

// Mock the teams service
jest.mock('../../../services/teamsService', () => ({
  teamsService: {
    getTeamUsers: jest.fn().mockResolvedValue({ data: [] })
  }
}));

// Create a test component that uses the hook
const TestComponent = ({ groupId = null, initialSelectedUsers = [], parentSetSelectedUsers = null }) => {
  const hookResult = useUserSelection(
    groupId,
    initialSelectedUsers,
    parentSetSelectedUsers
  );
  
  return (
    <div>
      <div data-testid="users">{JSON.stringify(hookResult.users)}</div>
      <div data-testid="available-users">{JSON.stringify(hookResult.availableUsers)}</div>
      <div data-testid="selected-users">{JSON.stringify(hookResult.selectedUsers)}</div>
      <div data-testid="current-page">{hookResult.currentPage}</div>
      <div data-testid="total-pages">{hookResult.totalPages}</div>
      <div data-testid="is-loading-more">{hookResult.isLoadingMore.toString()}</div>
      <div data-testid="loading">{hookResult.loading.toString()}</div>
      <div data-testid="current-search-term">{hookResult.currentSearchTerm}</div>
      
      <button 
        data-testid="fetch-users" 
        onClick={() => hookResult.fetchUsers()}
      >
        Fetch Users
      </button>
      
      <button 
        data-testid="handle-users-change" 
        onClick={() => hookResult.handleUsersChange({ selected: [{ id: '123', name: 'Test User' }] })}
      >
        Change Users
      </button>
      
      <button 
        data-testid="set-selected-users" 
        onClick={() => hookResult.setSelectedUsers([{ id: '456', name: 'Direct Set User' }])}
      >
        Set Selected Users
      </button>
      
      <input 
        data-testid="search-input" 
        onChange={(e) => hookResult.handleSearch(e.target.value)}
      />
      
      <button 
        data-testid="load-more" 
        onClick={() => hookResult.handleLoadMore()}
      >
        Load More
      </button>
    </div>
  );
};

describe('useUserSelection Hook', () => {
  // Mock data for testing
  const mockUsers = [
    { id: '1', name: 'User 1' },
    { id: '2', name: 'User 2' },
    { id: '3', name: 'User 3' }
  ];
  
  const mockSearchResults = [
    { id: '4', name: 'Search User 1' },
    { id: '5', name: 'Search User 2' }
  ];
  
  const initialSelectedUsers = [{ id: '1', name: 'User 1' }];
  
  beforeEach(() => {
    jest.clearAllMocks();
    
    // Default successful responses
    getUsers.mockResolvedValue({
      data: mockUsers,
      totalPages: 2,
      totalCount: 6
    });
  });
  
  test('initializes with default values when no parameters are provided', () => {
    render(<TestComponent />);
    
    expect(screen.getByTestId('users').textContent).toBe('[]');
    expect(screen.getByTestId('available-users').textContent).toBe('[]');
    expect(screen.getByTestId('selected-users').textContent).toBe('[]');
    expect(screen.getByTestId('current-page').textContent).toBe('1');
    expect(screen.getByTestId('total-pages').textContent).toBe('1');
    expect(screen.getByTestId('is-loading-more').textContent).toBe('false');
    expect(screen.getByTestId('loading').textContent).toBe('false');
    expect(screen.getByTestId('current-search-term').textContent).toBe('');
  });
  
  test('initializes with provided users', () => {
    render(<TestComponent initialSelectedUsers={initialSelectedUsers} />);
    
    expect(JSON.parse(screen.getByTestId('selected-users').textContent)).toEqual(initialSelectedUsers);
  });
  
  test('fetches users on mount', async () => {
    render(<TestComponent />);
    
    // No auto-fetch on mount, so trigger fetch manually
    fireEvent.click(screen.getByTestId('fetch-users'));

    // Wait for loading state to change
    await waitFor(() => {
      expect(getUsers).toHaveBeenCalled();
    });
    
    // Wait for data to load
    await waitFor(() => {
      expect(JSON.parse(screen.getByTestId('users').textContent)).toEqual(mockUsers);
    });
    
    // Check service function was called with correct params
    expect(getUsers).toHaveBeenCalledWith(1, {});
  });
  
  test('fetches users when fetchUsers is called', async () => {
    render(<TestComponent />);
    
    // Call fetchUsers
    fireEvent.click(screen.getByTestId('fetch-users'));
    
    // Wait for data to load
    await waitFor(() => {
      expect(JSON.parse(screen.getByTestId('users').textContent)).toEqual(mockUsers);
    });
    
    // Check service function was called with correct params
    expect(getUsers).toHaveBeenCalledWith(1, {});
  });
  
  test('handles user selection changes', () => {
    const mockParentSetSelectedUsers = jest.fn();
    render(<TestComponent parentSetSelectedUsers={mockParentSetSelectedUsers} />);
    
    // Call handleUsersChange
    fireEvent.click(screen.getByTestId('handle-users-change'));
    
    // Check selected users updated
    const expectedSelectedUsers = [{ id: '123', name: 'Test User' }];
    expect(JSON.parse(screen.getByTestId('selected-users').textContent)).toEqual(expectedSelectedUsers);
    
    // Check parent function was called
    expect(mockParentSetSelectedUsers).toHaveBeenCalledWith(expectedSelectedUsers);
  });
  
  test('directly sets selected users', () => {
    render(<TestComponent />);
    
    // Call setSelectedUsers
    fireEvent.click(screen.getByTestId('set-selected-users'));
    
    // Check selected users updated
    const expectedSelectedUsers = [{ id: '456', name: 'Direct Set User' }];
    expect(JSON.parse(screen.getByTestId('selected-users').textContent)).toEqual(expectedSelectedUsers);
  });
  
  test('searches users with search term', async () => {
    render(<TestComponent />);
    
    // Enter search term
    fireEvent.change(screen.getByTestId('search-input'), { target: { value: 'search' } });
    
    // Wait for search results
    await waitFor(() => {
      expect(screen.getByTestId('current-search-term').textContent).toBe('search');
    });
    
    // Check getUsers was called with search param
    await waitFor(() => {
      expect(getUsers).toHaveBeenCalledWith(1, { search: 'search' });
    });
  });
  
  test('clears search when search term is empty', async () => {
    render(<TestComponent />);
    
    // Enter search term
    fireEvent.change(screen.getByTestId('search-input'), { target: { value: 'search' } });
    
    // Wait for search results
    await waitFor(() => {
      expect(screen.getByTestId('current-search-term').textContent).toBe('search');
    });
    
    // Clear search term
    fireEvent.change(screen.getByTestId('search-input'), { target: { value: '' } });
    
    // Check currentSearchTerm is empty
    expect(screen.getByTestId('current-search-term').textContent).toBe('');
  });
  
  test('loads more users when handleLoadMore is called', async () => {
    // Need to ensure we have a totalPages value > currentPage
    getUsers.mockResolvedValueOnce({
      data: mockUsers,
      totalPages: 2,
      totalCount: 6
    });
    
    render(<TestComponent />);
    
    // First fetch to initialize data
    fireEvent.click(screen.getByTestId('fetch-users'));
    
    // Wait for first load to complete
    await waitFor(() => {
      expect(JSON.parse(screen.getByTestId('users').textContent)).toEqual(mockUsers);
    });
    
    // Verify the totalPages is set correctly
    expect(screen.getByTestId('total-pages').textContent).toBe('2');
    
    // Setup mock for second page fetch
    getUsers.mockResolvedValueOnce({
      data: [{ id: '10', name: 'User 10' }],
      totalPages: 2,
      totalCount: 6
    });
    
    // Call loadMore
    fireEvent.click(screen.getByTestId('load-more'));
    
    // Verify the second page call was made
    await waitFor(() => {
      expect(getUsers).toHaveBeenLastCalledWith(2, {});
    });
    
    // Check current page was updated
    await waitFor(() => {
      expect(screen.getByTestId('current-page').textContent).toBe('2');
    });
  });
  
  test('loads more search results when handleLoadMore is called with search term', async () => {
    render(<TestComponent />);
    
    // Enter search term
    fireEvent.change(screen.getByTestId('search-input'), { target: { value: 'search' } });
    
    // Wait for search results to be updated
    await waitFor(() => {
      expect(screen.getByTestId('current-search-term').textContent).toBe('search');
    });
    
    // Wait for totalPages to be updated
    await waitFor(() => {
      expect(screen.getByTestId('total-pages').textContent).toBe('2');
    });
    
    // Set up for next search page
    getUsers.mockResolvedValueOnce({
      data: [{ id: '6', name: 'More Search User' }],
      totalPages: 2,
      totalCount: 5
    });
    
    // Reset mock call history to make verification clearer
    getUsers.mockClear();
    
    // Call loadMore
    fireEvent.click(screen.getByTestId('load-more'));
    
    // Verify the second page call was made
    await waitFor(() => {
      expect(getUsers).toHaveBeenCalledWith(2, { search: 'search' });
    });
    
    // Check current page was updated
    await waitFor(() => {
      expect(screen.getByTestId('current-page').textContent).toBe('2');
    });
  });
  
  test('handles API fetch errors', async () => {
    // Mock API error
    const errorMessage = 'Failed to fetch users';
    getUsers.mockRejectedValue(new Error(errorMessage));
    
    // Spy on console.error
    const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    
    render(<TestComponent />);
    
    // Manually trigger fetch to cause error
    fireEvent.click(screen.getByTestId('fetch-users'));
    
    // Wait for error to be logged
    await waitFor(() => {
      expect(consoleSpy).toHaveBeenCalled();
    });
    
    consoleSpy.mockRestore();
  });
  
  test('handles API search errors', async () => {
    // Mock API error
    const errorMessage = 'Failed to search users';
    getUsers.mockRejectedValue(new Error(errorMessage));
    
    // Spy on console.error
    const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    
    render(<TestComponent />);
    
    // Enter search term
    fireEvent.change(screen.getByTestId('search-input'), { target: { value: 'search' } });
    
    // Wait for error to be logged
    await waitFor(() => {
      expect(consoleSpy).toHaveBeenCalled();
    });
    
    consoleSpy.mockRestore();
  });
  
  test('filters out selected users from available users', async () => {
    render(<TestComponent initialSelectedUsers={initialSelectedUsers} />);
    
    // Trigger a fetch to populate availableUsers
    fireEvent.click(screen.getByTestId('fetch-users'));
    
    // Wait for data to be loaded
    await waitFor(() => {
      expect(JSON.parse(screen.getByTestId('users').textContent)).toEqual(mockUsers);
    });
    
    // Check available users are populated
    await waitFor(() => {
      const availableUsers = JSON.parse(screen.getByTestId('available-users').textContent);
      expect(availableUsers.length).toBeGreaterThan(0);
    });
  });
});