import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import Groups from './Groups';
import { CACHE_KEYS } from '../../utils/constants';

jest.mock('./hooks/useGroups', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('./hooks/useGroupActions', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('react-router-dom', () => {
  const React = jest.requireActual('react');
  return {
    ...jest.requireActual('react-router-dom'),
    // eslint-disable-next-line react/display-name
    Link: React.forwardRef((props, ref) => (
      <a ref={ref} {...props}>
        {props.children}
      </a>
    )),
  };
});

const mockUseGroups = require('./hooks/useGroups').default;
const mockUseGroupActions = require('./hooks/useGroupActions').default;

const mockTheme = createTheme({
  palette: {
    primary: {
      main: '#1976d2',
    },
    secondary: {
      main: '#dc004e',
    },
    custom: {
      white: '#ffffff',
    },
    background: {
      buttonPrimaryDefault: '#007bff',
    },
    text: {
      defaultSubdued: '#6c757d',
      neutralDefault: '#495057',
      primary: '#212529',
    },
    border: {
      neutralDefault: '#cccccc',
    },
  },
  typography: {
    headingXLarge: { fontSize: '2rem' },
    bodyLargeDefault: { fontSize: '1rem' },
  },
});

const renderWithTheme = (component) => {
  return render(<ThemeProvider theme={mockTheme}>{component}</ThemeProvider>);
};

describe('Groups Component', () => {
  beforeEach(() => {
    mockUseGroups.mockReturnValue({
      groups: [],
      loading: false,
      error: null,
      page: 0,
      pageSize: 10,
      totalPages: 0,
      handlePageChange: jest.fn(),
      handlePageSizeChange: jest.fn(),
      handleSearch: jest.fn(),
      sortConfig: { key: 'name', direction: 'ascending' },
      handleSortChange: jest.fn(),
      refreshGroups: jest.fn(),
    });
    mockUseGroupActions.mockReturnValue({
      selectedGroup: null,
      warningDialogOpen: false,
      handleEdit: jest.fn(),
      handleDelete: jest.fn(),
      handleCancelDelete: jest.fn(),
      handleConfirmDelete: jest.fn(),
      handleGroupClick: jest.fn(),
    });
  });

  test('renders without crashing', () => {
    renderWithTheme(<Groups />);
    expect(screen.getByText('Teams')).toBeInTheDocument();
    expect(screen.getByText('Add team')).toBeInTheDocument();
  });

  test('displays loading spinner when loading and no groups', () => {
    mockUseGroups.mockReturnValueOnce({
      groups: [],
      loading: true,
      error: null,
      page: 0,
      pageSize: 10,
      totalPages: 0,
      handlePageChange: jest.fn(),
      handlePageSizeChange: jest.fn(),
      handleSearch: jest.fn(),
      sortConfig: { key: 'name', direction: 'ascending' },
      handleSortChange: jest.fn(),
      refreshGroups: jest.fn(),
    });
    renderWithTheme(<Groups />);
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });

  test('displays error message when error and no groups', () => {
    const errorMessage = 'Failed to fetch groups';
    mockUseGroups.mockReturnValueOnce({
      groups: [],
      loading: false,
      error: errorMessage,
      page: 0,
      pageSize: 10,
      totalPages: 0,
      handlePageChange: jest.fn(),
      handlePageSizeChange: jest.fn(),
      handleSearch: jest.fn(),
      sortConfig: { key: 'name', direction: 'ascending' },
      handleSortChange: jest.fn(),
      refreshGroups: jest.fn(),
    });
    renderWithTheme(<Groups />);
    expect(screen.getByText(errorMessage)).toBeInTheDocument();
  });

  test('displays groups table when groups are available', () => {
    mockUseGroups.mockReturnValueOnce({
      groups: [
        {
          id: '1',
          attributes: {
            name: 'Test Group',
            user_count: 2,
            catalogue_names: ['Catalog A'],
            data_catalogue_names: ['Data Catalog B'],
            tool_catalogue_names: ['Tool Catalog C'],
          },
        },
      ],
      loading: false,
      error: null,
      page: 0,
      pageSize: 10,
      totalPages: 1,
      handlePageChange: jest.fn(),
      handlePageSizeChange: jest.fn(),
      handleSearch: jest.fn(),
      sortConfig: { key: 'name', direction: 'ascending' },
      handleSortChange: jest.fn(),
      refreshGroups: jest.fn(),
    });
    renderWithTheme(<Groups />);
    expect(screen.getByText('Test Group')).toBeInTheDocument(); 
  });

  test('displays snackbar notification from localStorage', async () => {
    const notification = { message: 'Group created successfully', timestamp: Date.now() };
    localStorage.setItem(CACHE_KEYS.GROUP_NOTIFICATION, JSON.stringify(notification));

    renderWithTheme(<Groups />);

    expect(await screen.findByText('Group created successfully')).toBeInTheDocument();
    
    await waitFor(() => {
      expect(localStorage.getItem(CACHE_KEYS.GROUP_NOTIFICATION)).toBeNull();
    });
  });

}); 