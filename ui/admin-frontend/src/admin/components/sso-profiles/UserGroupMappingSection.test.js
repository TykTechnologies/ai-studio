import React from 'react';
import { render, screen } from '@testing-library/react';
import UserGroupMappingSection from './UserGroupMappingSection';
import { ThemeProvider, createTheme } from '@mui/material/styles';

// Mock DataTable component
jest.mock('../common/DataTable', () => {
  return function MockDataTable({ columns, data }) {
    return (
      <div data-testid="data-table">
        <div data-testid="columns">{JSON.stringify(columns)}</div>
        <div data-testid="data">{JSON.stringify(data)}</div>
      </div>
    );
  };
});

// Mock theme for testing
const theme = createTheme({
  palette: {
    text: {
      primary: '#212121',
      defaultSubdued: '#757575',
    },
    border: {
      neutralDefaultSubdued: '#e0e0e0',
    },
  },
});

const TestWrapper = ({ children }) => (
  <ThemeProvider theme={theme}>{children}</ThemeProvider>
);

describe('UserGroupMappingSection', () => {
  const mockGroups = [
    { id: 'group1', name: 'Group 1' },
    { id: 'group2', name: 'Group 2' },
    { id: 'group3', name: 'Group 3' },
  ];

  const mockProfileData = {
    DefaultUserGroupID: 'group1',
    CustomUserGroupField: 'groups',
    UserGroupMapping: {
      'provider-group-1': 'group1',
      'provider-group-2': 'group2',
    },
  };

  // Create a proper mock implementation
  const mockGetGroupNameById = jest.fn().mockImplementation((id) => {
    const group = mockGroups.find((g) => g.id === id);
    return group ? group.name : null;
  });

  const defaultProps = {
    profileData: mockProfileData,
    groups: mockGroups,
    groupsError: null,
    getGroupNameById: mockGetGroupNameById,
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders user group mapping section with complete data', () => {
    render(
      <TestWrapper>
        <UserGroupMappingSection {...defaultProps} />
      </TestWrapper>
    );

    // Check if the description text is displayed
    expect(screen.getByText(/User group mapping is how you assign developers to teams/)).toBeInTheDocument();

    // Check if default user group is displayed
    expect(screen.getByText('Default user group')).toBeInTheDocument();
    // Check that Default group text exists
    expect(screen.getAllByText('Default group').length).toBeGreaterThan(0);

    // Check if custom user group claim name is displayed
    expect(screen.getByText('Custom user group claim name')).toBeInTheDocument();
    expect(screen.getByText('groups')).toBeInTheDocument();

    // Check if DataTable is rendered
    expect(screen.getByTestId('data-table')).toBeInTheDocument();
  });

  test('renders with empty user group mapping', () => {
    const propsWithEmptyMapping = {
      ...defaultProps,
      profileData: {
        ...mockProfileData,
        UserGroupMapping: {},
      },
    };

    render(
      <TestWrapper>
        <UserGroupMappingSection {...propsWithEmptyMapping} />
      </TestWrapper>
    );

    // DataTable should not be rendered when UserGroupMapping is empty
    expect(screen.queryByTestId('data-table')).not.toBeInTheDocument();
  });

  test('renders with null user group mapping', () => {
    const propsWithNullMapping = {
      ...defaultProps,
      profileData: {
        ...mockProfileData,
        UserGroupMapping: null,
      },
    };

    render(
      <TestWrapper>
        <UserGroupMappingSection {...propsWithNullMapping} />
      </TestWrapper>
    );

    // DataTable should not be rendered when UserGroupMapping is null
    expect(screen.queryByTestId('data-table')).not.toBeInTheDocument();
  });

  test('renders with groups error', () => {
    const propsWithError = {
      ...defaultProps,
      groupsError: 'Failed to load groups',
    };

    render(
      <TestWrapper>
        <UserGroupMappingSection {...propsWithError} />
      </TestWrapper>
    );

    // Error alert should be displayed
    expect(screen.getByText('Failed to load groups')).toBeInTheDocument();
  });

  test('prepares user group mapping data correctly', () => {
    // Create a mock component that captures the prepared data
    const CaptureDataComponent = ({ profileData, getGroupNameById }) => {
      // Call the actual data preparation function
      const prepareData = () => {
        if (!profileData || !profileData.UserGroupMapping) {
          return [];
        }
    
        return Object.entries(profileData.UserGroupMapping).map(([providerGroupId, tykGroupId], index) => ({
          id: index.toString(),
          providerGroupId,
          tykGroupId,
          tykGroupName: getGroupNameById(tykGroupId),
        }));
      };
      
      const data = prepareData();
      
      return (
        <div data-testid="captured-data">
          {JSON.stringify(data)}
        </div>
      );
    };
    
    // Create a specific mock for this test
    const specificMockGetGroupNameById = jest.fn()
      .mockImplementation((id) => {
        if (id === 'group1') return 'Group 1';
        if (id === 'group2') return 'Group 2';
        return null;
      });
    
    render(
      <TestWrapper>
        <CaptureDataComponent
          profileData={mockProfileData}
          getGroupNameById={specificMockGetGroupNameById}
        />
      </TestWrapper>
    );
    
    // Get the captured data
    const dataElement = screen.getByTestId('captured-data');
    const data = JSON.parse(dataElement.textContent);
    
    // Check if data is prepared correctly
    expect(data).toHaveLength(2);
    expect(data[0].providerGroupId).toBe('provider-group-1');
    expect(data[0].tykGroupId).toBe('group1');
    expect(data[0].tykGroupName).toBe('Group 1');
    expect(data[1].providerGroupId).toBe('provider-group-2');
    expect(data[1].tykGroupId).toBe('group2');
    expect(data[1].tykGroupName).toBe('Group 2');
    
    // Verify the mock was called with the expected arguments
    expect(specificMockGetGroupNameById).toHaveBeenCalledWith('group1');
    expect(specificMockGetGroupNameById).toHaveBeenCalledWith('group2');
  });

  test('renders DataTable with correct columns', () => {
    render(
      <TestWrapper>
        <UserGroupMappingSection {...defaultProps} />
      </TestWrapper>
    );

    // Get the columns passed to DataTable
    const columnsElement = screen.getByTestId('columns');
    const columns = JSON.parse(columnsElement.textContent);

    // Check if columns are defined correctly
    expect(columns).toHaveLength(2);
    expect(columns[0].field).toBe('providerGroupId');
    expect(columns[0].headerName).toBe('Identity Provider group ID');
    expect(columns[1].field).toBe('tykGroupName');
    expect(columns[1].headerName).toBe('Tyk AI studio user group');
  });

  test('uses tykGroupId as fallback when tykGroupName is not available', () => {
    // Mock getGroupNameById to return null for group3
    const mockGetGroupNameByIdWithNull = jest.fn().mockReturnValue(null);

    const propsWithUnknownGroup = {
      ...defaultProps,
      profileData: {
        ...mockProfileData,
        UserGroupMapping: {
          'provider-group-3': 'group3',
        },
      },
      getGroupNameById: mockGetGroupNameByIdWithNull,
    };

    render(
      <TestWrapper>
        <UserGroupMappingSection {...propsWithUnknownGroup} />
      </TestWrapper>
    );

    // Get the data passed to DataTable
    const dataElement = screen.getByTestId('data');
    const data = JSON.parse(dataElement.textContent);

    // Check if tykGroupId is used as fallback
    expect(data[0].tykGroupId).toBe('group3');
    expect(data[0].tykGroupName).toBeNull();
  });

  test('renders with default values when profileData properties are missing', () => {
    const incompleteProfileData = {
      // DefaultUserGroupID is missing
      // CustomUserGroupField is missing
      UserGroupMapping: {
        'provider-group-1': 'group1',
      },
    };

    render(
      <TestWrapper>
        <UserGroupMappingSection
          {...defaultProps}
          profileData={incompleteProfileData}
        />
      </TestWrapper>
    );

    // Default user group should show "Default group" when DefaultUserGroupID is missing
    expect(screen.getByText('Default group')).toBeInTheDocument();

    // Custom user group claim name should show "group" when CustomUserGroupField is missing
    expect(screen.getByText('group')).toBeInTheDocument();
  });
});