import React from 'react';
import { screen, fireEvent } from '@testing-library/react';
import GroupDetail from './GroupDetail';
import { renderWithRouterAndTheme } from '../../../test-utils/render-with-theme';

jest.mock('./hooks/useGroupDetail', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('./hooks/useTeamMembers', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('../../hooks/useSystemFeatures', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('../../utils/featureUtils', () => ({
  getFeatureFlags: jest.fn(),
}));

jest.mock('react-router-dom', () => {
  const ReactRouter = jest.requireActual('react-router-dom');
  const ActualReact = jest.requireActual('react');
  return {
    ...ReactRouter,
    Link: ActualReact.forwardRef((props, ref) => (
      <a ref={ref} role="link" {...props}>
        {props.children}
      </a>
    )),
    useNavigate: () => mockedNavigate,
  };
});

const mockedNavigate = jest.fn();

const mockUseGroupDetail = require('./hooks/useGroupDetail').default;
const mockUseTeamMembers = require('./hooks/useTeamMembers').default;
const mockUseSystemFeatures = require('../../hooks/useSystemFeatures').default;
const { getFeatureFlags } = require('../../utils/featureUtils');

describe('GroupDetail', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    
    mockUseSystemFeatures.mockReturnValue({
      features: {
        feature_gateway: true,
        feature_portal: true,
        feature_chat: true,
      },
      loading: false,
      error: null,
      fetchFeatures: jest.fn(),
    });

    getFeatureFlags.mockReturnValue({
      isGatewayOnly: false,
      isPortalEnabled: true,
      isChatEnabled: true,
    });

    mockUseTeamMembers.mockReturnValue({
      users: [],
      error: null,
      loading: false,
      isLoadingMore: false,
      hasMore: false,
      handleLoadMore: jest.fn(),
      containerRef: { current: null },
    });
    
    mockedNavigate.mockClear();
  });

  test('renders loading state', () => {
    mockUseGroupDetail.mockReturnValue({
      group: null,
      users: [],
      catalogues: [],
      dataCatalogues: [],
      toolCatalogues: [],
      loading: true,
      error: null,
    });
    
    renderWithRouterAndTheme(<GroupDetail />);

    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });

  test('renders error state', () => {
    mockUseGroupDetail.mockReturnValue({
      group: null,
      users: [],
      catalogues: [],
      dataCatalogues: [],
      toolCatalogues: [],
      loading: false,
      error: 'Failed to fetch group',
    });
    
    renderWithRouterAndTheme(<GroupDetail />);

    expect(screen.getByText('Failed to fetch group')).toBeInTheDocument();
  });

  test('renders group not found state', () => {
    mockUseGroupDetail.mockReturnValue({
      group: null,
      users: [],
      catalogues: [],
      dataCatalogues: [],
      toolCatalogues: [],
      loading: false,
      error: null,
    });
    
    renderWithRouterAndTheme(<GroupDetail />);

    expect(screen.getByText('Team not found')).toBeInTheDocument();
  });

  describe('with group data', () => {
    const mockGroup = {
      id: 'group1',
      attributes: { name: 'Test Group' },
    };
    const mockUsers = [
      { id: 'user1', attributes: { name: 'User One', email: 'user1@example.com', role: 'Admin' } },
      { id: 'user2', attributes: { name: 'User Two', email: 'user2@example.com', role: 'Chat user' } },
    ];
    const mockCatalogues = [{ id: 'cat1', attributes: { name: 'Catalogue One' } }];
    const mockDataCatalogues = [{ id: 'dataCat1', attributes: { name: 'Data Catalogue One' } }];
    const mockToolCatalogues = [{ id: 'toolCat1', attributes: { name: 'Tool Catalogue One' } }];

    beforeEach(() => {
      mockUseGroupDetail.mockReturnValue({
        group: mockGroup,
        catalogues: mockCatalogues,
        dataCatalogues: mockDataCatalogues,
        toolCatalogues: mockToolCatalogues,
        loading: false,
        error: null,
      });
      mockUseTeamMembers.mockReturnValue({
        users: mockUsers,
        error: null,
        loading: false,
        isLoadingMore: false,
        hasMore: false,
        handleLoadMore: jest.fn(),
        containerRef: { current: null },
      });
    });

    test('renders group details correctly', () => {
      renderWithRouterAndTheme(<GroupDetail />);

      expect(screen.getByText('Team details')).toBeInTheDocument();
      expect(screen.getByText('Test Group')).toBeInTheDocument();

      expect(screen.getByText('User One')).toBeInTheDocument();
      expect(screen.getByText('user1@example.com')).toBeInTheDocument();
      expect(screen.getByText('Admin')).toBeInTheDocument();
      expect(screen.getByText('User Two')).toBeInTheDocument();
      expect(screen.getByText('user2@example.com')).toBeInTheDocument();
      expect(screen.getByText('Chat user')).toBeInTheDocument();
      
      expect(screen.getByText('Catalogue One')).toBeInTheDocument();
      expect(screen.getByText('Data Catalogue One')).toBeInTheDocument();
      expect(screen.getByText('Tool Catalogue One')).toBeInTheDocument();
    });

    test('renders group details without catalogs in gateway-only mode', () => {
      getFeatureFlags.mockReturnValue({
        isGatewayOnly: true,
        isPortalEnabled: false,
        isChatEnabled: false,
      });
      
      renderWithRouterAndTheme(<GroupDetail />);

      expect(screen.getByText('Team details')).toBeInTheDocument();
      expect(screen.getByText('Test Group')).toBeInTheDocument();
      expect(screen.getByText('User One')).toBeInTheDocument();
      expect(screen.getByText('user1@example.com')).toBeInTheDocument();
      
      expect(screen.queryByText('Catalogue One')).not.toBeInTheDocument();
      expect(screen.queryByText('Data Catalogue One')).not.toBeInTheDocument();
      expect(screen.queryByText('Tool Catalogue One')).not.toBeInTheDocument();
    });

    test('navigates to edit page on "Edit team" button click', () => {
      renderWithRouterAndTheme(<GroupDetail />);

      fireEvent.click(screen.getByRole('button', { name: /edit team/i }));
      expect(mockedNavigate).toHaveBeenCalledWith('/admin/groups/edit/group1');
    });

    test('navigates to groups list on "back to teams" link click', () => {
      renderWithRouterAndTheme(<GroupDetail />);
      
      const backLink = screen.getByRole('link', { name: /back to teams/i });
      expect(backLink).toBeInTheDocument();
      expect(backLink).toHaveAttribute('to', '/admin/groups');
    });
  });
}); 