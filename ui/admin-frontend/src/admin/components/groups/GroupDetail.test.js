import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import GroupDetail from './GroupDetail';
import useGroupDetail from './hooks/useGroupDetail';
import useTeamMembers from './hooks/useTeamMembers';

// Mock the custom hooks
jest.mock('./hooks/useGroupDetail');
jest.mock('./hooks/useTeamMembers');

// Mock react-router-dom's useNavigate
const mockedNavigate = jest.fn();
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockedNavigate,
}));

const mockTheme = createTheme({
  palette: {
    primary: { main: '#000' },
    secondary: { main: '#000' },
    error: { main: '#000' },
    warning: { main: '#000' },
    info: { main: '#000' },
    success: { main: '#000' },
    custom: {
      white: '#fff',
      black: '#000',
      // Add any other custom colors used by your components
    },
    background: {
      default: '#fff',
      paper: '#fff',
      buttonPrimaryDefault: '#000',
      // Add any other background colors
    },
    text: {
      primary: '#000',
      secondary: '#000',
      // Add any other text colors
    },
    border: {
      neutralDefault: '#000',
    },
    neutralDefault: '#000', // Added to address 'neutralDefault' error
  },
  // Add any other theme customizations your components might need
});

const renderWithProviders = (ui, { route = '/', initialEntries = [route] } = {}) => {
  return render(
    <ThemeProvider theme={mockTheme}>
      <MemoryRouter initialEntries={initialEntries}>
        {ui}
      </MemoryRouter>
    </ThemeProvider>
  );
};

const renderWithRouterAndProviders = (ui, { route = '/', initialEntries = [route] } = {}) => {
  return render(
    <ThemeProvider theme={mockTheme}>
      <MemoryRouter initialEntries={initialEntries}>
        <Routes>
          <Route path="/admin/groups/detail/:groupId" element={ui} />
          <Route path="/admin/groups" element={<div>Groups Page</div>} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );
};

describe('GroupDetail', () => {
  // Test 1: Loading state
  test('renders loading state', () => {
    useGroupDetail.mockReturnValue({
      group: null,
      users: [],
      catalogues: [],
      dataCatalogues: [],
      toolCatalogues: [],
      loading: true,
      error: null,
    });

    renderWithProviders(<GroupDetail />);

    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });

  // Test 2: Error state
  test('renders error state', () => {
    useGroupDetail.mockReturnValue({
      group: null,
      users: [],
      catalogues: [],
      dataCatalogues: [],
      toolCatalogues: [],
      loading: false,
      error: 'Failed to fetch group',
    });

    renderWithProviders(<GroupDetail />);

    expect(screen.getByText('Failed to fetch group')).toBeInTheDocument();
  });

  // Test 3: Group not found state
  test('renders group not found state', () => {
    useGroupDetail.mockReturnValue({
      group: null,
      users: [],
      catalogues: [],
      dataCatalogues: [],
      toolCatalogues: [],
      loading: false,
      error: null,
    });

    renderWithProviders(<GroupDetail />);

    expect(screen.getByText('Group not found')).toBeInTheDocument();
  });

  // Default mock for useTeamMembers for tests outside 'with group data'
  beforeEach(() => {
    useTeamMembers.mockReturnValue({
      users: [],
      error: null,
      loading: false,
      isLoadingMore: false,
      hasMore: false,
      handleLoadMore: jest.fn(),
      containerRef: { current: null },
    });
  });

  // Test 4: Successful data rendering
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
      useGroupDetail.mockReturnValue({
        group: mockGroup,
        catalogues: mockCatalogues,
        dataCatalogues: mockDataCatalogues,
        toolCatalogues: mockToolCatalogues,
        loading: false,
        error: null,
      });
      useTeamMembers.mockReturnValue({
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
      renderWithProviders(<GroupDetail />);

      // Check group name
      expect(screen.getByText('Team details')).toBeInTheDocument();
      expect(screen.getByText('Test Group')).toBeInTheDocument();

      // Check team members
      expect(screen.getByText('User One')).toBeInTheDocument();
      expect(screen.getByText('user1@example.com')).toBeInTheDocument();
      expect(screen.getByText('Admin')).toBeInTheDocument();
      expect(screen.getByText('User Two')).toBeInTheDocument();
      expect(screen.getByText('user2@example.com')).toBeInTheDocument();
      expect(screen.getByText('Chat user')).toBeInTheDocument();
      
      // Check catalogues
      expect(screen.getByText('Catalogue One')).toBeInTheDocument();
      expect(screen.getByText('Data Catalogue One')).toBeInTheDocument();
      expect(screen.getByText('Tool Catalogue One')).toBeInTheDocument();
    });

    // Test 5: "Edit team" button navigation
    test('navigates to edit page on "Edit team" button click', () => {
      renderWithProviders(<GroupDetail />);

      fireEvent.click(screen.getByRole('button', { name: /edit team/i }));
      expect(mockedNavigate).toHaveBeenCalledWith('/admin/groups/edit/group1');
    });

    // Test 6: "Back to teams" link navigation
    test('navigates to groups list on "back to teams" link click', () => {
       renderWithRouterAndProviders(<GroupDetail />, { initialEntries: ['/admin/groups/detail/group1'] });
      
      fireEvent.click(screen.getByRole('link', { name: /back to teams/i }));
      // Check if navigation occurred to the correct path, 
      // by verifying the content of the destination page or by checking the navigate function.
      // For this example, we'll assume navigation changes the content.
      expect(screen.getByText('Groups Page')).toBeInTheDocument();
    });
  });
}); 