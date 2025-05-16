import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import GroupMembersSection from './GroupMembersSection';

// Mock Material-UI components
jest.mock('@mui/material', () => ({
  Box: ({ children, sx, ...props }) => (
    <div data-testid="box" data-sx={JSON.stringify(sx)} {...props}>
      {children}
    </div>
  ),
  Typography: ({ children, variant, color, sx, ...props }) => (
    <div data-testid="typography" data-variant={variant} data-color={color} data-sx={JSON.stringify(sx)} {...props}>
      {children}
    </div>
  ),
}));

// Mock CollapsibleSection component
jest.mock('../../common/CollapsibleSection', () => ({
  __esModule: true,
  default: ({ children, title, defaultExpanded, ...props }) => (
    <div data-testid="collapsible-section" data-title={title} data-default-expanded={defaultExpanded.toString()} {...props}>
      {children}
    </div>
  )
}));

// Mock TransferList component
jest.mock('../../common/transfer-list/TransferList', () => ({
  __esModule: true,
  default: jest.fn(props => (
    <div data-testid="transfer-list">
      <button 
        data-testid="mock-transfer-button" 
        onClick={() => props.onChange && props.onChange({
          selected: [...props.selectedItems, props.availableItems[0]],
          available: props.availableItems.slice(1)
        })}
      >
        Transfer Item
      </button>
      <button 
        data-testid="mock-search-button" 
        onClick={() => props.onSearch && props.onSearch('test search')}
      >
        Search
      </button>
      <button 
        data-testid="mock-load-more-button" 
        onClick={() => props.onLoadMore && props.onLoadMore()}
      >
        Load More
      </button>
      <div data-left-title={props.leftTitle}></div>
      <div data-right-title={props.rightTitle}></div>
      <div data-has-more={props.hasMore.toString()}></div>
      <div data-is-loading-more={props.isLoadingMore.toString()}></div>
      <div data-columns={JSON.stringify(props.columns)}></div>
    </div>
  ))
}));

// Mock CustomSelectBadge component
jest.mock('../../common/CustomSelectBadge', () => ({
  __esModule: true,
  default: jest.fn(({ config }) => (
    <div data-testid="custom-select-badge" data-config={JSON.stringify(config)}></div>
  ))
}));

// Mock roleBadgeConfigs
jest.mock('../utils/roleBadgeConfig', () => ({
  roleBadgeConfigs: {
    'Admin': { color: 'primary', label: 'Admin' },
    'Chat user': { color: 'info', label: 'Chat user' },
    'Developer': { color: 'secondary', label: 'Developer' }
  }
}));

describe('GroupMembersSection Component', () => {
  // Test data
  const mockAvailableUsers = [
    { 
      id: '1', 
      attributes: { name: 'John Doe', email: 'john@example.com', role: 'Admin' } 
    },
    { 
      id: '2', 
      attributes: { name: 'Jane Smith', email: 'jane@example.com', role: 'Developer' } 
    }
  ];
  
  const mockSelectedUsers = [
    { 
      id: '3', 
      attributes: { name: 'Bob Johnson', email: 'bob@example.com', role: 'Chat user' } 
    }
  ];
  
  // Mock functions
  const mockHandleUsersChange = jest.fn();
  const mockHandleSearch = jest.fn();
  const mockHandleLoadMore = jest.fn();
  
  beforeEach(() => {
    jest.clearAllMocks();
  });
  
  test('renders GroupMembersSection component with correct props', () => {
    const { container } = render(
      <GroupMembersSection
        availableUsers={mockAvailableUsers}
        selectedUsers={mockSelectedUsers}
        handleUsersChange={mockHandleUsersChange}
        handleSearch={mockHandleSearch}
        handleLoadMore={mockHandleLoadMore}
        currentPage={1}
        totalPages={3}
        isLoadingMore={false}
      />
    );
    
    // Check that CollapsibleSection is rendered with correct props
    const collapsibleSection = screen.getByTestId('collapsible-section');
    expect(collapsibleSection).toBeInTheDocument();
    expect(collapsibleSection).toHaveAttribute('data-title', 'Manage team members');
    expect(collapsibleSection).toHaveAttribute('data-default-expanded', 'false');
    
    // Since we can't directly test the TransferList rendering due to the mock structure,
    // let's verify key props passed to TransferList via the mock implementation
    const mockTransferList = require('../../common/transfer-list/TransferList').default;
    expect(mockTransferList).toHaveBeenCalled();
    
    const lastCall = mockTransferList.mock.calls[mockTransferList.mock.calls.length - 1][0];
    expect(lastCall.availableItems).toEqual(mockAvailableUsers);
    expect(lastCall.selectedItems).toEqual(mockSelectedUsers);
    expect(lastCall.leftTitle).toBe("Current members");
    expect(lastCall.rightTitle).toBe("Add members");
    expect(lastCall.enableSearch).toBe(true);
    expect(lastCall.hasMore).toBe(true);
    expect(lastCall.isLoadingMore).toBe(false);
  });
  
  test('calls handleUsersChange when users are transferred', () => {
    render(
      <GroupMembersSection
        availableUsers={mockAvailableUsers}
        selectedUsers={mockSelectedUsers}
        handleUsersChange={mockHandleUsersChange}
        handleSearch={mockHandleSearch}
        handleLoadMore={mockHandleLoadMore}
        currentPage={1}
        totalPages={3}
        isLoadingMore={false}
      />
    );
    
    // Extract the onChange handler that was passed to TransferList
    const mockCalls = require('../../common/transfer-list/TransferList').default.mock.calls;
    const onChangeProp = mockCalls[mockCalls.length - 1][0].onChange;
    
    // Create mock data to simulate what TransferList would pass to onChange
    const mockChangeData = {
      selected: [...mockSelectedUsers, mockAvailableUsers[0]],
      available: mockAvailableUsers.slice(1)
    };
    
    // Call the onChange handler directly
    onChangeProp(mockChangeData);
    
    // Check that handleUsersChange was called with the expected parameters
    expect(mockHandleUsersChange).toHaveBeenCalledTimes(1);
    expect(mockHandleUsersChange).toHaveBeenCalledWith(mockChangeData);
  });
  
  test('calls handleSearch when search is performed', () => {
    render(
      <GroupMembersSection
        availableUsers={mockAvailableUsers}
        selectedUsers={mockSelectedUsers}
        handleUsersChange={mockHandleUsersChange}
        handleSearch={mockHandleSearch}
        handleLoadMore={mockHandleLoadMore}
        currentPage={1}
        totalPages={3}
        isLoadingMore={false}
      />
    );
    
    // Extract the onSearch handler that was passed to TransferList
    const mockCalls = require('../../common/transfer-list/TransferList').default.mock.calls;
    const onSearchProp = mockCalls[mockCalls.length - 1][0].onSearch;
    
    // Call the onSearch handler directly with a test term
    onSearchProp('test search');
    
    // Check that handleSearch was called with the expected parameters
    expect(mockHandleSearch).toHaveBeenCalledTimes(1);
    expect(mockHandleSearch).toHaveBeenCalledWith('test search', 1);
  });
  
  test('calls handleLoadMore when loading more users', () => {
    render(
      <GroupMembersSection
        availableUsers={mockAvailableUsers}
        selectedUsers={mockSelectedUsers}
        handleUsersChange={mockHandleUsersChange}
        handleSearch={mockHandleSearch}
        handleLoadMore={mockHandleLoadMore}
        currentPage={1}
        totalPages={3}
        isLoadingMore={false}
      />
    );
    
    // Extract the onLoadMore handler that was passed to TransferList
    const mockCalls = require('../../common/transfer-list/TransferList').default.mock.calls;
    const onLoadMoreProp = mockCalls[mockCalls.length - 1][0].onLoadMore;
    
    // Call the onLoadMore handler directly
    onLoadMoreProp();
    
    // Check that handleLoadMore was called
    expect(mockHandleLoadMore).toHaveBeenCalledTimes(1);
  });
  
  test('sets hasMore to false when currentPage equals totalPages', () => {
    render(
      <GroupMembersSection
        availableUsers={mockAvailableUsers}
        selectedUsers={mockSelectedUsers}
        handleUsersChange={mockHandleUsersChange}
        handleSearch={mockHandleSearch}
        handleLoadMore={mockHandleLoadMore}
        currentPage={3}
        totalPages={3}
        isLoadingMore={false}
      />
    );
    
    // Extract the hasMore prop that was passed to TransferList
    const mockCalls = require('../../common/transfer-list/TransferList').default.mock.calls;
    const hasMoreProp = mockCalls[mockCalls.length - 1][0].hasMore;
    
    // Check hasMore value
    expect(hasMoreProp).toBe(false);
  });
  
  test('passes isLoadingMore prop to TransferList', () => {
    render(
      <GroupMembersSection
        availableUsers={mockAvailableUsers}
        selectedUsers={mockSelectedUsers}
        handleUsersChange={mockHandleUsersChange}
        handleSearch={mockHandleSearch}
        handleLoadMore={mockHandleLoadMore}
        currentPage={1}
        totalPages={3}
        isLoadingMore={true}
      />
    );
    
    // Extract the isLoadingMore prop that was passed to TransferList
    const mockCalls = require('../../common/transfer-list/TransferList').default.mock.calls;
    const isLoadingMoreProp = mockCalls[mockCalls.length - 1][0].isLoadingMore;
    
    // Check isLoadingMore value
    expect(isLoadingMoreProp).toBe(true);
  });
});