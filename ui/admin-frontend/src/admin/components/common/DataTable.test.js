import React from 'react';
import { render, screen, fireEvent, within } from '@testing-library/react';
import DataTable from './DataTable';
import { ThemeProvider, createTheme } from '@mui/material/styles';

// Mock theme for testing
const theme = createTheme({
  palette: {
    text: {
      primary: '#000000',
      defaultSubdued: '#757575',
    },
    border: {
      neutralDefault: '#e0e0e0',
      neutralHovered: '#c0c0c0',
      neutralDefaultSubdued: '#f0f0f0',
      criticalDefault: '#ff0000',
    },
    background: {
      buttonCritical: '#ffebee',
      surfaceDefault: '#ffffff',
      iconSuccessDefault: '#4caf50',
    },
    custom: {
      white: '#ffffff',
      emptyStateBackground: '#f5f5f5',
    },
  },
  spacing: (factor) => `${factor * 8}px`,
});

// Mock PaginationControls component
jest.mock('./PaginationControls', () => {
  return function MockPaginationControls(props) {
    return (
      <div data-testid="pagination-controls">
        <span>Page {props.page} of {props.totalPages}</span>
      </div>
    );
  };
});

// Mock MUI icons
jest.mock('@mui/icons-material/MoreVert', () => {
  return function MockMoreVertIcon() {
    return <div data-testid="MoreVertIcon" />;
  };
});

const TestWrapper = ({ children }) => (
  <ThemeProvider theme={theme}>{children}</ThemeProvider>
);

describe('DataTable', () => {
  const mockColumns = [
    { field: 'name', headerName: 'Name', sortable: true },
    { field: 'age', headerName: 'Age', sortable: true },
    { field: 'email', headerName: 'Email', sortable: false },
  ];

  const mockData = [
    { id: '1', name: 'John Doe', age: 30, email: 'john@example.com' },
    { id: '2', name: 'Jane Smith', age: 25, email: 'jane@example.com' },
  ];

  const mockActions = [
    { label: 'Edit', onClick: jest.fn() },
    { label: 'Delete', onClick: jest.fn() },
  ];

  const mockPagination = {
    page: 1,
    pageSize: 10,
    totalPages: 2,
    onPageChange: jest.fn(),
    onPageSizeChange: jest.fn(),
  };

  const mockOnRowClick = jest.fn();
  const mockOnSortChange = jest.fn();

  const defaultProps = {
    columns: mockColumns,
    data: mockData,
    actions: mockActions,
    pagination: mockPagination,
    onRowClick: mockOnRowClick,
    sortConfig: { field: 'name', direction: 'asc' },
    onSortChange: mockOnSortChange,
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders table with correct columns and data', () => {
    render(
      <TestWrapper>
        <DataTable {...defaultProps} />
      </TestWrapper>
    );
    
    // Check column headers - using regex to match text that might include sort indicators
    expect(screen.getByText(/Name/)).toBeInTheDocument();
    expect(screen.getByText(/Age/)).toBeInTheDocument();
    expect(screen.getByText(/Email/)).toBeInTheDocument();
    
    // Check data rows
    expect(screen.getByText('John Doe')).toBeInTheDocument();
    expect(screen.getByText('30')).toBeInTheDocument();
    expect(screen.getByText('jane@example.com')).toBeInTheDocument();
  });

  test('shows sort indicators for sortable columns', () => {
    render(
      <TestWrapper>
        <DataTable {...defaultProps} />
      </TestWrapper>
    );
    
    // Name column should have ascending sort indicator
    expect(screen.getByText('Name ↑')).toBeInTheDocument();
    
    // Other columns should not have sort indicators
    expect(screen.getByText('Age')).toBeInTheDocument();
    expect(screen.queryByText('Age ↑')).not.toBeInTheDocument();
  });

  test('calls onSortChange when clicking sortable column header', () => {
    render(
      <TestWrapper>
        <DataTable {...defaultProps} />
      </TestWrapper>
    );
    
    // Click on Age column header (sortable)
    fireEvent.click(screen.getByText('Age'));
    
    expect(mockOnSortChange).toHaveBeenCalledWith({
      field: 'age',
      direction: 'asc',
    });
  });

  test('does not call onSortChange when clicking non-sortable column header', () => {
    render(
      <TestWrapper>
        <DataTable {...defaultProps} />
      </TestWrapper>
    );
    
    // Click on Email column header (not sortable)
    fireEvent.click(screen.getByText('Email'));
    
    expect(mockOnSortChange).not.toHaveBeenCalled();
  });

  test('calls onRowClick when clicking a row', () => {
    render(
      <TestWrapper>
        <DataTable {...defaultProps} />
      </TestWrapper>
    );
    
    // Click on first row
    fireEvent.click(screen.getByText('John Doe'));
    
    expect(mockOnRowClick).toHaveBeenCalledWith(mockData[0]);
  });

  test('renders actions column when actions are provided', () => {
    render(
      <TestWrapper>
        <DataTable {...defaultProps} />
      </TestWrapper>
    );
    
    expect(screen.getByText('Actions')).toBeInTheDocument();
    
    // Should have action buttons (MoreVertIcon)
    const actionButtons = screen.getAllByTestId('MoreVertIcon');
    expect(actionButtons.length).toBe(2); // One for each row
  });

  test('opens action menu when clicking action button', () => {
    render(
      <TestWrapper>
        <DataTable {...defaultProps} />
      </TestWrapper>
    );
    
    // Initially menu should be closed
    expect(screen.queryByText('Edit')).not.toBeInTheDocument();
    
    // Click on first row's action button
    const actionButtons = screen.getAllByTestId('MoreVertIcon');
    fireEvent.click(actionButtons[0]);
    
    // Menu should be open with action items
    expect(screen.getByText('Edit')).toBeInTheDocument();
    expect(screen.getByText('Delete')).toBeInTheDocument();
  });

  test('calls action onClick when menu item is clicked', () => {
    render(
      <TestWrapper>
        <DataTable {...defaultProps} />
      </TestWrapper>
    );
    
    // Click on first row's action button
    const actionButtons = screen.getAllByTestId('MoreVertIcon');
    fireEvent.click(actionButtons[0]);
    
    // Click on Edit menu item
    fireEvent.click(screen.getByText('Edit'));
    
    expect(mockActions[0].onClick).toHaveBeenCalledWith(mockData[0]);
  });

  test('renders pagination controls when pagination is provided', () => {
    render(
      <TestWrapper>
        <DataTable {...defaultProps} />
      </TestWrapper>
    );
    
    // PaginationControls component should be rendered
    expect(screen.getByTestId('pagination-controls')).toBeInTheDocument();
    expect(screen.getByText('Page 1 of 2')).toBeInTheDocument();
  });

  test('renders custom cell content when renderCell is provided', () => {
    const columnsWithRender = [
      ...mockColumns,
      {
        field: 'custom',
        headerName: 'Custom',
        renderCell: (item) => <span data-testid="custom-cell">{`Custom ${item.name}`}</span>,
      },
    ];
    
    render(
      <TestWrapper>
        <DataTable {...defaultProps} columns={columnsWithRender} />
      </TestWrapper>
    );
    
    expect(screen.getByText('Custom John Doe')).toBeInTheDocument();
    expect(screen.getByText('Custom Jane Smith')).toBeInTheDocument();
  });
});