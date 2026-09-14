import React from 'react';
import { render, screen, fireEvent, within, act } from '@testing-library/react';
import '@testing-library/jest-dom';
import DataTable from '../DataTable';
import { ThemeProvider, createTheme } from '@mui/material/styles';

// setupTests stubs use-debounce as a pass-through; the search tests below
// exercise the real debounce, so restore the actual implementation here.
jest.mock('use-debounce', () => jest.requireActual('use-debounce'));

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
      neutralDefault: '#f5f5f5',
      secondaryExtraLight: '#fafafa',
    },
    custom: {
      white: '#ffffff',
      emptyStateBackground: '#f5f5f5',
    },
  },
  spacing: (factor) => `${factor * 8}px`,
});

// Mock PaginationControls component
jest.mock('../PaginationControls', () => {
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

const renderTable = (props) =>
  render(
    <TestWrapper>
      <DataTable {...props} />
    </TestWrapper>
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

  describe('rendering (legacy props)', () => {
    test('renders table with correct columns and data', () => {
      renderTable(defaultProps);
      expect(screen.getByRole('columnheader', { name: /Name/ })).toBeInTheDocument();
      expect(screen.getByRole('columnheader', { name: /Age/ })).toBeInTheDocument();
      expect(screen.getByRole('columnheader', { name: /Email/ })).toBeInTheDocument();
      expect(screen.getByText('John Doe')).toBeInTheDocument();
      expect(screen.getByText('30')).toBeInTheDocument();
      expect(screen.getByText('jane@example.com')).toBeInTheDocument();
    });

    test('renders a dash for missing values and custom cells via renderCell', () => {
      const columns = [
        ...mockColumns,
        { field: 'custom', headerName: 'Custom', renderCell: (item) => <span>{`Custom ${item.name}`}</span> },
        { field: 'missing', headerName: 'Missing' },
      ];
      renderTable({ ...defaultProps, columns });
      expect(screen.getByText('Custom John Doe')).toBeInTheDocument();
      expect(screen.getAllByText('-')).toHaveLength(2);
    });

    test('calls onRowClick when clicking a row', () => {
      renderTable(defaultProps);
      fireEvent.click(screen.getByText('John Doe'));
      expect(mockOnRowClick).toHaveBeenCalledWith(mockData[0]);
    });

    test('renders pagination controls when pagination is provided', () => {
      renderTable(defaultProps);
      expect(screen.getByText('Page 1 of 2')).toBeInTheDocument();
    });

    test('uses rowKey and rowProps', () => {
      renderTable({
        ...defaultProps,
        data: [{ key: 'a', name: 'Keyed' }],
        rowKey: 'key',
        rowProps: (item) => ({ 'data-testid': `row-${item.key}` }),
      });
      expect(screen.getByTestId('row-a')).toHaveTextContent('Keyed');
    });
  });

  describe('sorting', () => {
    test('marks the sorted column with aria-sort and a sort button', () => {
      renderTable(defaultProps);
      const nameHeader = screen.getByRole('columnheader', { name: /Name/ });
      expect(nameHeader).toHaveAttribute('aria-sort', 'ascending');
      expect(screen.getByRole('columnheader', { name: /Age/ })).not.toHaveAttribute('aria-sort');
      expect(screen.getByRole('button', { name: 'Sort by Name' })).toBeInTheDocument();
    });

    test('accepts the legacy { key, direction } sort config', () => {
      renderTable({ ...defaultProps, sortConfig: { key: 'age', direction: 'desc' } });
      expect(screen.getByRole('columnheader', { name: /Age/ })).toHaveAttribute('aria-sort', 'descending');
    });

    test('toggles direction on the active column and starts ascending on a new one', () => {
      renderTable(defaultProps);
      fireEvent.click(screen.getByRole('button', { name: 'Sort by Name' }));
      expect(mockOnSortChange).toHaveBeenCalledWith({ field: 'name', direction: 'desc' });
      fireEvent.click(screen.getByRole('button', { name: 'Sort by Age' }));
      expect(mockOnSortChange).toHaveBeenCalledWith({ field: 'age', direction: 'asc' });
    });

    test('does not offer sorting on non-sortable columns', () => {
      renderTable(defaultProps);
      expect(screen.queryByRole('button', { name: 'Sort by Email' })).not.toBeInTheDocument();
      fireEvent.click(screen.getByText('Email'));
      expect(mockOnSortChange).not.toHaveBeenCalled();
    });
  });

  describe('row actions', () => {
    test('renders a labelled actions button per row and opens the menu', () => {
      renderTable(defaultProps);
      expect(screen.getByText('Actions')).toBeInTheDocument();
      expect(screen.queryByText('Edit')).not.toBeInTheDocument();
      fireEvent.click(screen.getByRole('button', { name: 'Actions for John Doe' }));
      expect(screen.getByText('Edit')).toBeInTheDocument();
      expect(screen.getByText('Delete')).toBeInTheDocument();
    });

    test('calls the action with the row item and does not trigger the row click', () => {
      renderTable(defaultProps);
      fireEvent.click(screen.getByRole('button', { name: 'Actions for John Doe' }));
      fireEvent.click(screen.getByText('Edit'));
      expect(mockActions[0].onClick).toHaveBeenCalledWith(mockData[0]);
      expect(mockOnRowClick).not.toHaveBeenCalled();
    });

    test('supports per-row hidden, disabled and dynamic labels', () => {
      const actions = [
        { key: 'toggle', label: (item) => (item.age > 26 ? 'Retire' : 'Promote'), onClick: jest.fn() },
        { key: 'hide', label: 'Only for Jane', hidden: (item) => item.name !== 'Jane Smith', onClick: jest.fn() },
        { key: 'off', label: 'Locked', disabled: () => true, disabledReason: 'Nope', onClick: jest.fn() },
      ];
      renderTable({ ...defaultProps, actions });
      fireEvent.click(screen.getByRole('button', { name: 'Actions for John Doe' }));
      expect(screen.getByText('Retire')).toBeInTheDocument();
      expect(screen.queryByText('Only for Jane')).not.toBeInTheDocument();
      expect(screen.getByText('Locked')).toHaveAttribute('aria-disabled', 'true');
    });

    test('hides the actions column when no actions are given', () => {
      renderTable({ ...defaultProps, actions: undefined });
      expect(screen.queryByText('Actions')).not.toBeInTheDocument();
      expect(screen.queryByRole('button', { name: /Actions for/ })).not.toBeInTheDocument();
    });
  });

  describe('search', () => {
    beforeEach(() => {
      jest.useFakeTimers();
    });
    afterEach(() => {
      jest.useRealTimers();
    });

    test('debounces typing before calling onSearch', () => {
      const onSearch = jest.fn();
      renderTable({ ...defaultProps, enableSearch: true, onSearch, searchPlaceholder: 'Find things' });
      const input = screen.getByPlaceholderText('Find things');
      fireEvent.change(input, { target: { value: 'jo' } });
      fireEvent.change(input, { target: { value: 'joh' } });
      expect(onSearch).not.toHaveBeenCalled();
      act(() => {
        jest.advanceTimersByTime(450);
      });
      expect(onSearch).toHaveBeenCalledTimes(1);
      expect(onSearch).toHaveBeenCalledWith('joh');
    });

    test('an external reset clears the box without re-emitting', () => {
      const onSearch = jest.fn();
      const { rerender } = render(
        <TestWrapper>
          <DataTable {...defaultProps} enableSearch onSearch={onSearch} searchTerm="abc" />
        </TestWrapper>
      );
      expect(screen.getByPlaceholderText('Search...')).toHaveValue('abc');
      rerender(
        <TestWrapper>
          <DataTable {...defaultProps} enableSearch onSearch={onSearch} searchTerm="" />
        </TestWrapper>
      );
      act(() => {
        jest.advanceTimersByTime(450);
      });
      expect(screen.getByPlaceholderText('Search...')).toHaveValue('');
      expect(onSearch).not.toHaveBeenCalled();
    });

    test('the clear button empties the search', () => {
      const onSearch = jest.fn();
      renderTable({ ...defaultProps, enableSearch: true, onSearch, searchTerm: 'abc' });
      fireEvent.click(screen.getByRole('button', { name: 'Clear search' }));
      act(() => {
        jest.advanceTimersByTime(450);
      });
      expect(onSearch).toHaveBeenCalledWith('');
    });
  });

  describe('loading and empty states', () => {
    test('shows skeleton rows and aria-busy while loading with no data', () => {
      renderTable({ ...defaultProps, data: [], loading: true });
      expect(screen.getByRole('table')).toHaveAttribute('aria-busy', 'true');
      expect(screen.getAllByTestId('skeleton-row').length).toBeGreaterThan(0);
    });

    test('keeps rows visible while reloading', () => {
      renderTable({ ...defaultProps, loading: true });
      expect(screen.getByText('John Doe')).toBeInTheDocument();
      expect(screen.queryByTestId('skeleton-row')).not.toBeInTheDocument();
    });

    test('renders the emptyState node instead of the table when there is nothing at all', () => {
      renderTable({ ...defaultProps, data: [], emptyState: <div data-testid="empty-widget">Nothing yet</div>, enableSearch: true });
      expect(screen.getByTestId('empty-widget')).toBeInTheDocument();
      expect(screen.queryByRole('table')).not.toBeInTheDocument();
      expect(screen.queryByPlaceholderText('Search...')).not.toBeInTheDocument();
    });

    test('renders a no-matches row when a search finds nothing', () => {
      renderTable({ ...defaultProps, data: [], enableSearch: true, searchTerm: 'zzz' });
      expect(screen.getByTestId('data-table-empty')).toHaveTextContent('No matches for “zzz”');
      expect(screen.getByPlaceholderText('Search...')).toBeInTheDocument();
    });

    test('uses emptyMessage when given', () => {
      renderTable({ ...defaultProps, data: [], emptyMessage: 'No users found' });
      expect(screen.getByText('No users found')).toBeInTheDocument();
    });
  });

  describe('selection and bulk actions', () => {
    const bulkActions = [
      { key: 'activate', label: 'Activate', onClick: jest.fn() },
      { key: 'delete', label: 'Delete', danger: true, onClick: jest.fn(), disabled: (items) => items.length > 1, disabledReason: 'One at a time' },
    ];

    const renderSelectable = (selectedIds, onSelectionChange = jest.fn()) => {
      const utils = renderTable({
        ...defaultProps,
        selectable: true,
        selectedIds,
        onSelectionChange,
        bulkActions,
      });
      return { ...utils, onSelectionChange };
    };

    test('labels checkboxes and selects a row without triggering the row click', () => {
      const { onSelectionChange } = renderSelectable([]);
      expect(screen.getByRole('checkbox', { name: 'Select all on this page' })).toBeInTheDocument();
      fireEvent.click(screen.getByRole('checkbox', { name: 'Select John Doe' }));
      expect(onSelectionChange).toHaveBeenCalledWith(['1']);
      expect(mockOnRowClick).not.toHaveBeenCalled();
      expect(screen.queryByTestId('bulk-actions-toolbar')).not.toBeInTheDocument();
    });

    test('deselects a selected row', () => {
      const { onSelectionChange } = renderSelectable(['1', '2']);
      fireEvent.click(screen.getByRole('checkbox', { name: 'Select John Doe' }));
      expect(onSelectionChange).toHaveBeenCalledWith(['2']);
    });

    test('header checkbox is indeterminate with a partial selection and selects the page', () => {
      const { onSelectionChange } = renderSelectable(['1']);
      const header = screen.getByRole('checkbox', { name: 'Select all on this page' });
      expect(header).toHaveAttribute('data-indeterminate', 'true');
      expect(header).not.toBeChecked();
      fireEvent.click(header);
      expect(onSelectionChange).toHaveBeenCalledWith(['1', '2']);
    });

    test('header checkbox is checked when the whole page is selected and clears it', () => {
      const { onSelectionChange } = renderSelectable(['1', '2', '9']);
      const header = screen.getByRole('checkbox', { name: 'Select all on this page' });
      expect(header).toBeChecked();
      fireEvent.click(header);
      // Ids from other pages are kept.
      expect(onSelectionChange).toHaveBeenCalledWith(['9']);
    });

    test('shows the bulk toolbar with count, actions and Clear', () => {
      const { onSelectionChange } = renderSelectable(['1', '2']);
      const toolbar = screen.getByTestId('bulk-actions-toolbar');
      expect(within(toolbar).getByText('2 selected')).toBeInTheDocument();
      fireEvent.click(within(toolbar).getByRole('button', { name: 'Activate' }));
      expect(bulkActions[0].onClick).toHaveBeenCalledWith(mockData);
      expect(within(toolbar).getByRole('button', { name: 'Delete' })).toBeDisabled();
      fireEvent.click(within(toolbar).getByRole('button', { name: 'Clear' }));
      expect(onSelectionChange).toHaveBeenCalledWith([]);
    });

    test('marks selected rows', () => {
      renderSelectable(['2']);
      expect(screen.getByText('Jane Smith').closest('tr')).toHaveAttribute('aria-selected', 'true');
      expect(screen.getByText('John Doe').closest('tr')).toHaveAttribute('aria-selected', 'false');
    });
  });

  test('sets the table accessible name', () => {
    renderTable({ ...defaultProps, ariaLabel: 'People' });
    expect(screen.getByRole('table', { name: 'People' })).toBeInTheDocument();
  });
});
