import React from 'react';
import { render, screen, waitFor, act } from '@testing-library/react';
import { fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { useTransferListAvailableUsers } from './useTransferListAvailableUsers';
import { getUsers } from '../services/userService';

jest.mock('../services/userService', () => ({
  getUsers: jest.fn()
}));

const TestComponent = ({ 
  groupId = '123', 
  excludeIds = [], 
  pageSize = 10, 
  searchDebounceMs = 0 
}) => {
  const hookResult = useTransferListAvailableUsers({
    groupId,
    excludeIds,
    pageSize,
    searchDebounceMs
  });
  
  return (
    <div>
      <div data-testid="items">{JSON.stringify(hookResult.items)}</div>
      <div data-testid="loading">{hookResult.loading.toString()}</div>
      <div data-testid="is-searching">{hookResult.isSearching.toString()}</div>
      <div data-testid="is-loading-more">{hookResult.isLoadingMore.toString()}</div>
      <div data-testid="has-more">{hookResult.hasMore.toString()}</div>
      <div data-testid="search-term">{hookResult.searchTerm}</div>
      
      <input 
        data-testid="search-input" 
        value={hookResult.searchTerm || ''} 
        onChange={(e) => hookResult.search(e.target.value)} 
      />
      
      <button 
        data-testid="load-more-button" 
        onClick={() => hookResult.loadMore()}
      >
        Load More
      </button>
      
      <button 
        data-testid="add-item-button" 
        onClick={() => hookResult.addItem({ id: '999', name: 'Added User' })}
      >
        Add Item
      </button>
      
      <button 
        data-testid="remove-item-button" 
        onClick={() => {
          if (hookResult.items.length > 0) {
            hookResult.removeItem(hookResult.items[0]);
          }
        }}
      >
        Remove Item
      </button>
      
      <button 
        data-testid="reset-button" 
        onClick={() => hookResult.reset()}
      >
        Reset
      </button>
    </div>
  );
};

describe('useTransferListAvailableUsers Hook', () => {
  const mockUsers = [
    { id: '1', name: 'User 1' },
    { id: '2', name: 'User 2' },
    { id: '3', name: 'User 3' }
  ];
  
  const mockApiResponse = {
    data: mockUsers,
    meta: {
      total_pages: 3,
      last_page: 3
    }
  };
  
  beforeEach(() => {
    jest.clearAllMocks();
    getUsers.mockResolvedValue(mockApiResponse);
    getUsers.mockClear();
  });
  
  test('initializes with loading state and fetches users', async () => {
    render(<TestComponent />);
    
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    expect(getUsers).toHaveBeenCalledWith(1, {
      exclude_group_id: '123',
      page: 1,
      page_size: 10,
      search: ''
    });
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    const itemsElement = screen.getByTestId('items');
    const items = JSON.parse(itemsElement.textContent);
    expect(items).toEqual(mockUsers);
    
    expect(screen.getByTestId('is-searching').textContent).toBe('false');
    expect(screen.getByTestId('is-loading-more').textContent).toBe('false');
    expect(screen.getByTestId('has-more').textContent).toBe('true');
    expect(screen.getByTestId('search-term').textContent).toBe('');
  });
  
  test('excludes specified IDs from displayed items', async () => {
    render(<TestComponent excludeIds={['1']} />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    const itemsElement = screen.getByTestId('items');
    const items = JSON.parse(itemsElement.textContent);
    
    expect(items.find(item => item.id === '1')).toBeUndefined();
    expect(items.find(item => item.id === '2')).toBeDefined();
    expect(items.find(item => item.id === '3')).toBeDefined();
  });
  
  test('handles search term changes', async () => {
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    const searchResults = [{ id: '4', name: 'Searched User' }];
    getUsers.mockResolvedValue({
      data: searchResults,
      meta: { total_pages: 1 }
    });
    
    const searchInput = screen.getByTestId('search-input');
    fireEvent.change(searchInput, { target: { value: 'test-search' } });
    
    expect(screen.getByTestId('is-searching').textContent).toBe('true');
    expect(screen.getByTestId('search-term').textContent).toBe('test-search');
    
    expect(getUsers).toHaveBeenCalledWith(1, {
      exclude_group_id: '123',
      page: 1,
      page_size: 10,
      search: 'test-search'
    });
    
    await waitFor(() => {
      expect(screen.getByTestId('is-searching').textContent).toBe('false');
    });
    
    const itemsElement = screen.getByTestId('items');
    const items = JSON.parse(itemsElement.textContent);
    expect(items).toEqual(searchResults);
  });
  
  test('loads more items when loadMore is called', async () => {
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    getUsers.mockClear();
    
    const nextPageUsers = [{ id: '4', name: 'User 4' }, { id: '5', name: 'User 5' }];
    getUsers.mockResolvedValue({
      data: nextPageUsers,
      meta: { total_pages: 3 }
    });
    
    fireEvent.click(screen.getByTestId('load-more-button'));
    
    await waitFor(() => {
      expect(getUsers).toHaveBeenCalled();
    });
    
    expect(getUsers).toHaveBeenCalledWith(2, {
      exclude_group_id: '123',
      page: 2,
      page_size: 10,
      search: ''
    });
    
    await waitFor(() => {
      expect(screen.getByTestId('is-loading-more').textContent).toBe('false');
    });
    
    const itemsElement = screen.getByTestId('items');
    const items = JSON.parse(itemsElement.textContent);
    
    expect(items).toEqual([...mockUsers, ...nextPageUsers]);
  });
  
  test('prevents duplicate items when loading more', async () => {
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    getUsers.mockClear();
    
    const nextPageUsers = [{ id: '4', name: 'User 4' }, { id: '5', name: 'User 5' }];
    getUsers.mockResolvedValue({
      data: nextPageUsers,
      meta: { total_pages: 3 }
    });
    
    fireEvent.click(screen.getByTestId('load-more-button'));
    
    await waitFor(() => {
      expect(getUsers).toHaveBeenCalled();
    });
    
    await waitFor(() => {
      expect(screen.getByTestId('is-loading-more').textContent).toBe('false');
    });
    
    const itemsElement = screen.getByTestId('items');
    const items = JSON.parse(itemsElement.textContent);
    
    expect(items.length).toBe(5);
    expect(items).toEqual([
      { id: '1', name: 'User 1' },
      { id: '2', name: 'User 2' },
      { id: '3', name: 'User 3' },
      { id: '4', name: 'User 4' },
      { id: '5', name: 'User 5' }
    ]);
  });
  
  test('handles adding items to recently removed', async () => {
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    fireEvent.click(screen.getByTestId('add-item-button'));
    
    const itemsElement = screen.getByTestId('items');
    const items = JSON.parse(itemsElement.textContent);
    
    expect(items[0].id).toBe('999');
    expect(items[0].name).toBe('Added User');
    expect(items.length).toBe(mockUsers.length + 1);
  });
  
  test('handles removing items', async () => {
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    fireEvent.click(screen.getByTestId('remove-item-button'));
    
    const itemsElement = screen.getByTestId('items');
    const items = JSON.parse(itemsElement.textContent);
    
    expect(items.find(item => item.id === '1')).toBeUndefined();
    expect(items.length).toBe(mockUsers.length - 1);
  });
  
  test('handles reset correctly', async () => {
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    fireEvent.change(screen.getByTestId('search-input'), { target: { value: 'test-search' } });
    
    await waitFor(() => {
      expect(screen.getByTestId('is-searching').textContent).toBe('false');
    });
    
    getUsers.mockClear();
    
    fireEvent.click(screen.getByTestId('reset-button'));
    
    await waitFor(() => {
      expect(screen.getByTestId('search-term').textContent).toBe('');
    });
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('true');
    });
  });
  
  test('handles API fetch errors', async () => {
    getUsers.mockRejectedValue(new Error('Failed to fetch users'));
    
    const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    
    render(<TestComponent />);
    
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    const itemsElement = screen.getByTestId('items');
    const items = JSON.parse(itemsElement.textContent);
    expect(items).toEqual([]);
    
    consoleSpy.mockRestore();
  });
  
  test('does not fetch if groupId is not provided', async () => {
    render(<TestComponent groupId={null} />);
    
    expect(screen.getByTestId('loading').textContent).toBe('true');
    expect(getUsers).not.toHaveBeenCalled();
  });
  
  test('does not show recentlyRemoved items when searching', async () => {
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    fireEvent.click(screen.getByTestId('add-item-button'));
    
    getUsers.mockResolvedValue({
      data: [{ id: '5', name: 'Search Result' }],
      meta: { total_pages: 1 }
    });
    
    fireEvent.change(screen.getByTestId('search-input'), { target: { value: 'search' } });
    
    await waitFor(() => {
      expect(screen.getByTestId('is-searching').textContent).toBe('false');
    });
    
    const itemsElement = screen.getByTestId('items');
    const items = JSON.parse(itemsElement.textContent);
    
    expect(items.find(item => item.id === '999')).toBeUndefined();
    expect(items.find(item => item.id === '5')).toBeDefined();
  });
});