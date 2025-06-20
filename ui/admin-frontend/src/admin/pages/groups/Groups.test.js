import React from 'react';
import { screen, waitFor } from '@testing-library/react';
import Groups from './Groups';
import { CACHE_KEYS } from '../../utils/constants';
import { renderWithTheme } from '../../../test-utils/render-with-theme';

jest.mock('./hooks/useGroups', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('./hooks/useGroupActions', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('react-router-dom', () => {
  const ReactRouter = jest.requireActual('react-router-dom');
  const ActualReact = jest.requireActual('react');
  return {
    ...ReactRouter,
    Link: ActualReact.forwardRef((props, ref) => (
      <a ref={ref} {...props}>
        {props.children}
      </a>
    )),
  };
});

jest.mock('./components/ManageTeamMembersModal', () => ({
  __esModule: true,
  default: jest.fn(() => <div data-testid="mock-team-members-modal" />)
}));

jest.mock('./components/ManageGroupCatalogsModal', () => ({
  __esModule: true,
  default: jest.fn(() => <div data-testid="mock-catalogs-modal" />)
}));

jest.mock('../../hooks/useSystemFeatures', () => {
  const mockSystemFeaturesFn = jest.fn(() => {
    return {
      features: { feature_portal: false, feature_chat: false, feature_gateway: false },
      loading: false,
      error: null,
      fetchFeatures: jest.fn().mockResolvedValue({
        feature_portal: true, feature_chat: true, feature_gateway: true,
      }),
    };
  });
  return {
    __esModule: true,
    default: mockSystemFeaturesFn,
  };
});

jest.mock('../../hooks/useOverviewData', () => ({
  __esModule: true,
  default: jest.fn(),
}));

const mockUseGroups = require('./hooks/useGroups').default;
const mockUseGroupActions = require('./hooks/useGroupActions').default;
const mockUseOverviewData = require('../../hooks/useOverviewData').default;
const mockUseSystemFeatures = require('../../hooks/useSystemFeatures').default;

describe('Groups Component', () => {
  beforeEach(() => {
    mockUseSystemFeatures.mockReturnValue({
      features: {
        feature_portal: false,
        feature_chat: false,
        feature_gateway: false,
      },
      loading: false,
      error: null,
      fetchFeatures: jest.fn().mockResolvedValue({
        feature_portal: true,
        feature_chat: true,
        feature_gateway: true,
      }),
    });

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
      handleManageMembers: jest.fn(),
      handleManageCatalogs: jest.fn(),
    });
    mockUseOverviewData.mockReturnValue({
      getDocsLink: jest.fn().mockReturnValue('https://example.com'),
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

  test('renders team members management modal when closed', () => {
    const ManageTeamMembersModal = require('./components/ManageTeamMembersModal').default;

    renderWithTheme(<Groups />);
    
    expect(ManageTeamMembersModal).toHaveBeenCalledWith(
      expect.objectContaining({
        open: false,
        group: null
      }),
      {}
    );
  });

  test('renders catalogs management modal when closed', () => {
    const ManageGroupCatalogsModal = require('./components/ManageGroupCatalogsModal').default;

    renderWithTheme(<Groups />);
    
    expect(ManageGroupCatalogsModal).toHaveBeenCalledWith(
      expect.objectContaining({
        open: false,
        group: null,
        features: {
          feature_portal: false,
          feature_chat: false,
          feature_gateway: false,
        }
      }),
      {}
    );
  });
});