import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import TransferList from '../transfer-list/TransferList';

jest.mock('@mui/material', () => require('../../../../test-utils/mui-mocks').muiMaterialMock);
jest.mock('@mui/styled-engine', () => require('../../../../test-utils/mui-mocks').muiStyledEngineMock);
jest.mock('@mui/material/styles', () => require('../../../../test-utils/mui-mocks').muiStylesMock);
jest.mock('@mui/material/IconButton', () => require('../../../../test-utils/mui-mocks').muiIconButtonMock);
jest.mock('@mui/icons-material/Search', () => require('../../../../test-utils/mui-mocks').muiSearchIconMock);
jest.mock('@mui/icons-material/Add', () => require('../../../../test-utils/mui-mocks').muiAddIconMock);
jest.mock('@mui/icons-material/Close', () => require('../../../../test-utils/mui-mocks').muiCloseIconMock);

jest.mock('../InfiniteScrollContainer', () => require('../../../../test-utils/component-mocks').infiniteScrollContainerMock);
jest.mock('../transfer-list/TransferListTable', () => require('../../../../test-utils/component-mocks').transferListTableMock);
jest.mock('../transfer-list/styles', () => require('../../../../test-utils/styled-component-mocks').transferListStylesMock);
jest.mock('../../../styles/sharedStyles', () => require('../../../../test-utils/styled-component-mocks').sharedStylesMock);

describe('TransferList Component', () => {
  const mockAvailableItems = [
    { id: '1', name: 'Item 1' },
    { id: '2', name: 'Item 2' },
  ];
  
  const mockSelectedItems = [
    { id: '3', name: 'Item 3' },
  ];
  
  const mockColumns = [
    { field: 'name', headerName: 'Name', width: '70%' },
  ];
  
  const mockOnAdd = jest.fn();
  const mockOnRemove = jest.fn();
  const mockOnSearchTermChange = jest.fn();
  const mockOnLoadMore = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders with default props', () => {
    render(
      <TransferList
        availableItems={mockAvailableItems}
        selectedItems={mockSelectedItems}
        columns={mockColumns}
        idField="id"
      />
    );
    
    expect(screen.getByTestId('transfer-list-container')).toBeInTheDocument();
    expect(screen.getByTestId('left-table')).toBeInTheDocument();
    expect(screen.getByTestId('right-table')).toBeInTheDocument();
  });

  test('displays titles and subtitles', () => {
    render(
      <TransferList
        leftTitle="Available"
        leftSubtitle="Available items"
        rightTitle="Selected"
        rightSubtitle="Selected items"
      />
    );
    
    expect(screen.getByText('Available')).toBeInTheDocument();
    expect(screen.getByText('Available items')).toBeInTheDocument();
    expect(screen.getByText('Selected')).toBeInTheDocument();
    expect(screen.getByText('Selected items')).toBeInTheDocument();
  });

  test('passes correct props to TransferListTable components', () => {
    render(
      <TransferList
        availableItems={mockAvailableItems}
        selectedItems={mockSelectedItems}
        columns={mockColumns}
        idField="id"
      />
    );
    
    const leftTable = screen.getByTestId('left-table');
    expect(leftTable.dataset.items).toBe(JSON.stringify(mockSelectedItems));
    expect(leftTable.dataset.columns).toBe(JSON.stringify(mockColumns));
    expect(leftTable.dataset.idField).toBe('id');

    const rightTable = screen.getByTestId('right-table');
    expect(rightTable.dataset.items).toBe(JSON.stringify(mockAvailableItems)); 
    expect(rightTable.dataset.columns).toBe(JSON.stringify(mockColumns));
    expect(rightTable.dataset.idField).toBe('id');
  });

  test('renders search field when enableSearch is true', () => {
    render(
      <TransferList
        enableSearch={true}
        searchTerm="test"
        onSearchTermChange={mockOnSearchTermChange}
      />
    );
    
    expect(screen.getByTestId('search-container')).toBeInTheDocument();
    const searchInput = screen.getByTestId('styled-text-field');
    expect(searchInput).toBeInTheDocument();
    expect(searchInput.value).toBe('test');
  });

  test('does not render search field when enableSearch is false', () => {
    render(<TransferList />);
    expect(screen.queryByTestId('search-box')).not.toBeInTheDocument();
  });

  test('handles item addition', () => {
    render(
      <TransferList
        availableItems={mockAvailableItems}
        onAdd={mockOnAdd}
      />
    );
    
    const addButton = screen.getByTestId('add-button');
    fireEvent.click(addButton);
    
    expect(mockOnAdd).toHaveBeenCalledWith(mockAvailableItems[0]);
  });

  test('handles item removal', () => {
    render(
      <TransferList
        selectedItems={mockSelectedItems}
        onRemove={mockOnRemove}
      />
    );
    
    const removeButton = screen.getByTestId('remove-button');
    fireEvent.click(removeButton);
    
    expect(mockOnRemove).toHaveBeenCalledWith(mockSelectedItems[0]);
  });

  test('shows loading state when isSearching is true', () => {
    render(
      <TransferList
        enableSearch={true}
        isSearching={true}
      />
    );
    
    expect(screen.getByText('Searching...')).toBeInTheDocument();
    expect(screen.queryByTestId('right-table')).not.toBeInTheDocument();
  });

  test('shows loading more state when isLoadingMore is true', () => {
    render(
      <TransferList
        isLoadingMore={true}
        isSearching={false}
      />
    );
    
    expect(screen.getByText('Loading more users...')).toBeInTheDocument();
  });
});