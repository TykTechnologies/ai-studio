import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';

// Mock the custom hook
jest.mock('../transfer-list/useTransferList', () => ({
  __esModule: true,
  default: jest.fn(),
}));

// Mock Material-UI components
jest.mock('@mui/material', () => ({
  Box: ({ children, display, justifyContent, p, ...props }) => (
    <div 
      data-testid="box" 
      data-display={display}
      data-justifycontent={justifyContent}
      data-p={p}
      {...props}
    >
      {children}
    </div>
  ),
  Typography: ({ children, variant, color, ...props }) => (
    <div data-testid="typography" data-variant={variant} data-color={color} {...props}>
      {children}
    </div>
  ),
  InputAdornment: ({ children, position, ...props }) => (
    <div data-testid="input-adornment" data-position={position} {...props}>
      {children}
    </div>
  ),
}));

// Mock icons
jest.mock('@mui/icons-material/Search', () => () => <div data-testid="search-icon" />);

// Mock the styled components - with a safe approach that doesn't use React.forwardRef directly
jest.mock('../transfer-list/styles', () => ({
  TransferListContainer: ({ children, ...props }) => (
    <div data-testid="transfer-list-container" {...props}>
      {children}
    </div>
  ),
  TransferBox: ({ children, ...props }) => {
    // Safely ignore the ref but keep all other props
    const { ref, ...otherProps } = props;
    return (
      <div data-testid="transfer-box" {...otherProps}>
        {children}
      </div>
    );
  },
  HeaderBox: ({ children, ...props }) => (
    <div data-testid="header-box" {...props}>
      {children}
    </div>
  ),
  SearchContainer: ({ children, ...props }) => (
    <div data-testid="search-container" {...props}>
      {children}
    </div>
  ),
}));

// Mock the StyledTextField - fix issue with input children
jest.mock('../../../styles/sharedStyles', () => ({
  StyledTextField: ({ value, onChange, placeholder, InputProps, ...props }) => {
    // Create a safe implementation of StyledTextField
    return (
      <div data-testid="styled-text-field-wrapper">
        <input
          data-testid="styled-text-field"
          value={value}
          onChange={onChange}
          placeholder={placeholder}
          {...props}
        />
        {InputProps?.startAdornment && (
          <div data-testid="input-adornment-container">
            {InputProps.startAdornment}
          </div>
        )}
      </div>
    );
  },
}));

// Mock the TransferListTable component
jest.mock('../transfer-list/TransferListTable', () => {
  const MockTransferListTable = ({ items, columns, idField, isLeftSide, onAddItem, onRemoveItem }) => (
    <div
      data-testid={isLeftSide ? "left-table" : "right-table"}
      data-items={items ? JSON.stringify(items) : '[]'}
      data-columns={columns ? JSON.stringify(columns) : '[]'}
      data-id-field={idField}
    >
      {isLeftSide ? (
        <button data-testid="remove-button" onClick={() => items && items.length > 0 && onRemoveItem(items[0])}>
          Remove
        </button>
      ) : (
        <button data-testid="add-button" onClick={() => items && items.length > 0 && onAddItem(items[0])}>
          Add
        </button>
      )}
    </div>
  );
  
  return {
    __esModule: true,
    default: MockTransferListTable
  };
});

// Import the component under test
const useTransferList = require('../transfer-list/useTransferList').default;
const TransferList = require('../transfer-list/TransferList').default;

describe('TransferList Component', () => {
  // Mock data
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
  
  // Mock hook return values
  const mockHookReturn = {
    leftBoxRef: { current: null },
    rightBoxRef: { current: null },
    available: mockAvailableItems,
    selected: mockSelectedItems,
    searchTerm: '',
    isSearching: false,
    handleSearchChange: jest.fn(),
    handleAddItem: jest.fn(),
    handleRemoveItem: jest.fn(),
  };

  beforeEach(() => {
    useTransferList.mockReturnValue(mockHookReturn);
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  test('renders with default props', () => {
    render(<TransferList />);
    
    expect(screen.getByTestId('transfer-list-container')).toBeInTheDocument();
    expect(screen.getAllByTestId('transfer-box').length).toBe(2);
    expect(screen.getByTestId('left-table')).toBeInTheDocument();
    expect(screen.getByTestId('right-table')).toBeInTheDocument();
  });

  test('renders with custom props', () => {
    render(
      <TransferList
        availableItems={mockAvailableItems}
        selectedItems={mockSelectedItems}
        columns={mockColumns}
        leftTitle="Left Title"
        leftSubtitle="Left Subtitle"
        rightTitle="Right Title"
        rightSubtitle="Right Subtitle"
        idField="id"
      />
    );
    
    expect(screen.getByText('Left Title')).toBeInTheDocument();
    expect(screen.getByText('Left Subtitle')).toBeInTheDocument();
    expect(screen.getByText('Right Title')).toBeInTheDocument();
    expect(screen.getByText('Right Subtitle')).toBeInTheDocument();
    
    // Verify tables received the correct props
    const leftTable = screen.getByTestId('left-table');
    const rightTable = screen.getByTestId('right-table');
    
    expect(JSON.parse(leftTable.dataset.items)).toEqual(mockSelectedItems);
    expect(JSON.parse(rightTable.dataset.items)).toEqual(mockAvailableItems);
    expect(JSON.parse(leftTable.dataset.columns)).toEqual(mockColumns);
    expect(JSON.parse(rightTable.dataset.columns)).toEqual(mockColumns);
    expect(leftTable.dataset.idField).toBe('id');
    expect(rightTable.dataset.idField).toBe('id');
  });

  test('calls useTransferList with correct props', () => {
    const onChangeMock = jest.fn();
    const onSearchMock = jest.fn();
    const onLoadMoreMock = jest.fn();
    const onItemAddedMock = jest.fn();
    const onItemRemovedMock = jest.fn();
    
    render(
      <TransferList
        availableItems={mockAvailableItems}
        selectedItems={mockSelectedItems}
        columns={mockColumns}
        idField="id"
        onChange={onChangeMock}
        onSearch={onSearchMock}
        onLoadMore={onLoadMoreMock}
        hasMore={true}
        isLoadingMore={false}
        onItemAdded={onItemAddedMock}
        onItemRemoved={onItemRemovedMock}
      />
    );
    
    expect(useTransferList).toHaveBeenCalledWith({
      availableItems: mockAvailableItems,
      selectedItems: mockSelectedItems,
      idField: 'id',
      onChange: onChangeMock,
      onSearch: onSearchMock,
      onLoadMore: onLoadMoreMock,
      hasMore: true,
      isLoadingMore: false,
      onItemAdded: onItemAddedMock,
      onItemRemoved: onItemRemovedMock,
    });
  });

  test('renders search field when enableSearch is true', () => {
    useTransferList.mockReturnValue({
      ...mockHookReturn,
      searchTerm: 'test',
    });
    
    render(
      <TransferList
        enableSearch={true}
      />
    );
    
    expect(screen.getByTestId('search-container')).toBeInTheDocument();
    expect(screen.getByTestId('styled-text-field')).toBeInTheDocument();
    expect(screen.getByTestId('styled-text-field')).toHaveValue('test');
  });

  test('does not render search field when enableSearch is false', () => {
    render(
      <TransferList
        enableSearch={false}
      />
    );
    
    expect(screen.queryByTestId('search-container')).not.toBeInTheDocument();
    expect(screen.queryByTestId('styled-text-field')).not.toBeInTheDocument();
  });

  test('handles item addition', () => {
    useTransferList.mockReturnValue({
      ...mockHookReturn,
      available: [{ id: '1', name: 'Item 1' }],
    });
    
    render(<TransferList />);
    
    const addButton = screen.getByTestId('add-button');
    fireEvent.click(addButton);
    
    expect(mockHookReturn.handleAddItem).toHaveBeenCalled();
  });

  test('handles item removal', () => {
    useTransferList.mockReturnValue({
      ...mockHookReturn,
      selected: [{ id: '3', name: 'Item 3' }],
    });
    
    render(<TransferList />);
    
    const removeButton = screen.getByTestId('remove-button');
    fireEvent.click(removeButton);
    
    expect(mockHookReturn.handleRemoveItem).toHaveBeenCalled();
  });

  test('shows loading state when isSearching is true', () => {
    useTransferList.mockReturnValue({
      ...mockHookReturn,
      isSearching: true,
    });
    
    render(<TransferList enableSearch={true} />);
    
    expect(screen.getByText('Searching...')).toBeInTheDocument();
    expect(screen.queryByTestId('right-table')).not.toBeInTheDocument();
  });

  test('shows loading more state when isLoadingMore is true', () => {
    useTransferList.mockReturnValue({
      ...mockHookReturn,
      isLoadingMore: true,
      isSearching: false,
    });
    
    render(<TransferList isLoadingMore={true} />);
    
    expect(screen.getByText('Loading more users...')).toBeInTheDocument();
  });
});