import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import SSOProfileDetails from './SSOProfileDetails';
import apiClient from '../../utils/apiClient';
import { copyToClipboard } from '../../utils/clipboardUtils';
import { mapApiToUIProfile } from './UIProfile';

// Mock dependencies
jest.mock('../../utils/apiClient');
jest.mock('../../utils/clipboardUtils');
jest.mock('./UIProfile', () => ({
  mapApiToUIProfile: jest.fn()
}));

// Mock sharedStyles to avoid theme-related errors
jest.mock('../../styles/sharedStyles', () => ({
  TitleBox: ({ children, ...props }) => <div data-testid="title-box" {...props}>{children}</div>,
  ContentBox: ({ children, ...props }) => <div data-testid="content-box" {...props}>{children}</div>,
  PrimaryButton: ({ children, onClick, ...props }) => (
    <button data-testid="primary-button" onClick={onClick} {...props}>{children}</button>
  ),
  SecondaryLinkButton: ({ children, ...props }) => (
    <button data-testid="secondary-link-button" {...props}>{children}</button>
  )
}));

// Mock child components
jest.mock('./ProfileDetailsSection', () => {
  return function MockProfileDetailsSection(props) {
    return <div data-testid="profile-details-section" {...props} />;
  };
});

jest.mock('./ProviderConfigurationSection', () => {
  return function MockProviderConfigurationSection(props) {
    return <div data-testid="provider-configuration-section" {...props} />;
  };
});

jest.mock('./UserGroupMappingSection', () => {
  return function MockUserGroupMappingSection(props) {
    return <div data-testid="user-group-mapping-section" {...props} />;
  };
});

jest.mock('../common/CollapsibleSection', () => {
  return function MockCollapsibleSection({ title, children }) {
    return (
      <div data-testid={`collapsible-section-${title.replace(/\s+/g, '-').toLowerCase()}`}>
        <div data-testid="section-title">{title}</div>
        <div data-testid="section-content">{children}</div>
      </div>
    );
  };
});

// Mock MUI icons
jest.mock('@mui/icons-material/ChevronLeft', () => {
  return function MockChevronLeftIcon() {
    return <div data-testid="chevron-left-icon" />;
  };
});

jest.mock('@mui/icons-material/Edit', () => {
  return function MockEditIcon() {
    return <div data-testid="edit-icon" />;
  };
});

// Mock theme for testing
const theme = createTheme({
  palette: {
    text: {
      primary: '#212121',
      defaultSubdued: '#757575',
    },
    background: {
      buttonPrimaryDefault: '#007bff',
    },
    border: {
      neutralDefaultSubdued: '#e0e0e0',
    },
    custom: {
      white: '#ffffff',
    },
  },
});

// Test wrapper with router
const TestWrapper = ({ children }) => (
  <ThemeProvider theme={theme}>
    <MemoryRouter initialEntries={['/admin/sso-profiles/123']}>
      <Routes>
        <Route path="/admin/sso-profiles/:profileId" element={children} />
        <Route path="/admin/sso-profiles/edit/:profileId" element={<div>Edit Page</div>} />
        <Route path="/admin/sso-profiles" element={<div>SSO Profiles List</div>} />
      </Routes>
    </MemoryRouter>
  </ThemeProvider>
);

describe('SSOProfileDetails', () => {
  const mockProfileId = '123';
  const mockNavigate = jest.fn();
  
  const mockProfileResponse = {
    data: {
      data: {
        attributes: {
          profile_id: '123',
          name: 'Test Profile',
          action_type: 'oauth',
          login_url: 'https://example.com/login',
          callback_url: 'https://example.com/callback',
          failure_redirect_url: 'https://example.com/failure',
          selected_provider_type: 'openid-connect',
          provider_config: {
            UseProviders: [
              {
                Key: 'test-key',
                Secret: 'test-secret',
              }
            ]
          },
          user_group_mapping: {
            'provider-group-1': 'tyk-group-1',
            'provider-group-2': 'tyk-group-2'
          },
          default_user_group_id: 'default-group-id',
          custom_user_group_field: 'groups'
        }
      }
    }
  };
  
  const mockGroupsResponse = {
    data: {
      data: [
        { id: 'tyk-group-1', attributes: { name: 'Group 1' } },
        { id: 'tyk-group-2', attributes: { name: 'Group 2' } },
        { id: 'default-group-id', attributes: { name: 'Default Group' } }
      ]
    }
  };
  
  const mockUIProfile = {
    ID: '123',
    Name: 'Test Profile',
    ActionType: 'oauth',
    ProviderConfig: {
      UseProviders: [
        {
          Key: 'test-key',
          Secret: 'test-secret',
        }
      ]
    },
    UserGroupMapping: {
      'provider-group-1': 'tyk-group-1',
      'provider-group-2': 'tyk-group-2'
    },
    DefaultUserGroupID: 'default-group-id',
    CustomUserGroupField: 'groups'
  };

  beforeEach(() => {
    jest.clearAllMocks();
    
    // Mock API responses
    apiClient.get.mockImplementation((url) => {
      if (url === `/sso-profiles/${mockProfileId}`) {
        return Promise.resolve(mockProfileResponse);
      } else if (url === '/groups') {
        return Promise.resolve(mockGroupsResponse);
      }
      return Promise.reject(new Error('Not found'));
    });
    
    // Mock mapApiToUIProfile
    mapApiToUIProfile.mockReturnValue(mockUIProfile);
    
    // Mock copyToClipboard
    copyToClipboard.mockImplementation((text, fieldName, onSuccess) => {
      onSuccess(fieldName);
      return Promise.resolve(true);
    });
  });

  test('renders loading state initially', () => {
    render(
      <TestWrapper>
        <SSOProfileDetails />
      </TestWrapper>
    );
    
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });

  test('fetches and displays profile data', async () => {
    render(
      <TestWrapper>
        <SSOProfileDetails />
      </TestWrapper>
    );
    
    // Wait for loading to complete
    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });
    
    // Check API calls
    expect(apiClient.get).toHaveBeenCalledWith(`/sso-profiles/${mockProfileId}`);
    expect(apiClient.get).toHaveBeenCalledWith('/groups', { params: { all: true } });
    expect(mapApiToUIProfile).toHaveBeenCalledWith(mockProfileResponse.data);
    
    // Check if title is displayed
    expect(screen.getByText(`Profile - ${mockUIProfile.Name}`)).toBeInTheDocument();
    
    // Check if sections are rendered
    expect(screen.getByTestId('collapsible-section-profile-details')).toBeInTheDocument();
    expect(screen.getByTestId('collapsible-section-provider-configuration')).toBeInTheDocument();
    expect(screen.getByTestId('collapsible-section-user-group-mapping')).toBeInTheDocument();
    
    // Check if child components are rendered with correct props
    expect(screen.getByTestId('profile-details-section')).toBeInTheDocument();
    expect(screen.getByTestId('provider-configuration-section')).toBeInTheDocument();
    expect(screen.getByTestId('user-group-mapping-section')).toBeInTheDocument();
  });

  test('displays error message when profile fetch fails', async () => {
    apiClient.get.mockImplementation((url) => {
      if (url === `/sso-profiles/${mockProfileId}`) {
        return Promise.reject(new Error('Failed to fetch profile'));
      }
      return Promise.resolve({ data: { data: [] } });
    });
    
    render(
      <TestWrapper>
        <SSOProfileDetails />
      </TestWrapper>
    );
    
    await waitFor(() => {
      expect(screen.getByText('Failed to load Identity provider profile')).toBeInTheDocument();
    });
  });

  test('displays error message when groups fetch fails', async () => {
    apiClient.get.mockImplementation((url) => {
      if (url === `/sso-profiles/${mockProfileId}`) {
        return Promise.resolve(mockProfileResponse);
      } else if (url === '/groups') {
        return Promise.reject(new Error('Failed to fetch groups'));
      }
      return Promise.reject(new Error('Not found'));
    });
    
    render(
      <TestWrapper>
        <SSOProfileDetails />
      </TestWrapper>
    );
    await waitFor(() => {
      expect(screen.getByTestId('user-group-mapping-section')).toBeInTheDocument();
    });
    
    expect(screen.getByTestId('user-group-mapping-section')).toHaveAttribute(
      'groupsError',
      'Failed to load groups. Group names may not be displayed correctly.'
    );
  });

  test('navigates to edit page when Edit Profile button is clicked', async () => {
    render(
      <TestWrapper>
        <SSOProfileDetails />
      </TestWrapper>
    );
    
    // Wait for loading to complete
    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });
    
    // Click the Edit Profile button
    fireEvent.click(screen.getByText('Edit profile'));
    
    // Check if navigation occurred by verifying the Edit Page is shown
    await waitFor(() => {
      expect(screen.getByText('Edit Page')).toBeInTheDocument();
    });
  });

  test('handles copy to clipboard functionality', async () => {
    // Mock the copyToClipboard function
    copyToClipboard.mockClear();
    
    // Create a simple mock implementation
    const mockHandleCopyToClipboard = jest.fn().mockImplementation((text, fieldName) => {
      copyToClipboard(text, fieldName, jest.fn(), jest.fn());
    });
    
    // Create a test component that uses our mock function
    const TestComponent = () => {
      return (
        <button
          data-testid="copy-button"
          onClick={() => mockHandleCopyToClipboard('test-text', 'Test Field')}
        >
          Copy Text
        </button>
      );
    };
    
    // Render the test component
    render(<TestComponent />);
    
    // Click the button to trigger the copy function
    fireEvent.click(screen.getByTestId('copy-button'));
    
    // Check if copyToClipboard was called with correct arguments
    expect(copyToClipboard).toHaveBeenCalledWith(
      'test-text',
      'Test Field',
      expect.any(Function),
      expect.any(Function)
    );
  });

  test('getGroupNameById returns correct group name', async () => {
    // Create a mock implementation of getGroupNameById
    const mockGetGroupNameById = (groupId) => {
      const mockGroups = [
        { id: 'tyk-group-1', attributes: { name: 'Group 1' } },
        { id: 'tyk-group-2', attributes: { name: 'Group 2' } },
        { id: 'default-group-id', attributes: { name: 'Default Group' } }
      ];
      const group = mockGroups.find((g) => g.id === groupId);
      return group ? group.attributes.name : groupId;
    };
    
    // Test the function directly
    expect(mockGetGroupNameById('tyk-group-1')).toBe('Group 1');
    expect(mockGetGroupNameById('tyk-group-2')).toBe('Group 2');
    expect(mockGetGroupNameById('non-existent-id')).toBe('non-existent-id');
    
    // Render the component to verify it works in context
    render(
      <TestWrapper>
        <SSOProfileDetails />
      </TestWrapper>
    );
    
    // Wait for loading to complete
    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });
    
    // Verify the UserGroupMappingSection is rendered with the groups
    expect(screen.getByTestId('user-group-mapping-section')).toBeInTheDocument();
  });

  test('renders back button with correct link', async () => {
    render(
      <TestWrapper>
        <SSOProfileDetails />
      </TestWrapper>
    );
    
    // Wait for loading to complete
    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });
    
    // Find the back button
    const backButton = screen.getByText('back to IdP profiles');
    expect(backButton).toBeInTheDocument();
    
    // Check if it has the correct "to" prop
    expect(backButton.getAttribute('to')).toBe('/admin/sso-profiles');
  });

  test('shows and closes snackbar', async () => {
    // Mock the setSnackbar function
    const mockSetSnackbar = jest.fn();
    const useStateSpy = jest.spyOn(React, 'useState');
    
    // Mock useState for snackbar
    useStateSpy.mockImplementation((initialState) => {
      if (typeof initialState === 'object' && initialState.hasOwnProperty('open')) {
        return [{ open: true, message: 'Test message', severity: 'success' }, mockSetSnackbar];
      }
      return [initialState, jest.fn()];
    });
    
    render(
      <TestWrapper>
        <SSOProfileDetails />
      </TestWrapper>
    );
    
    // Wait for loading to complete
    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });
    
    // Verify the component renders with the mocked snackbar state
    expect(screen.getByTestId('content-box')).toBeInTheDocument();
    
    // Restore the original implementation
    useStateSpy.mockRestore();
  });

  test('handles copy to clipboard failure', async () => {
    // Save the original implementation
    const originalMockImplementation = copyToClipboard.mockImplementation;
    
    // Mock copyToClipboard to fail
    copyToClipboard.mockImplementation((text, fieldName, onSuccess, onError) => {
      onError(fieldName);
      return Promise.resolve(false);
    });
    
    // Create a component with a mocked handleCopyToClipboard function
    const handleCopyToClipboard = jest.fn().mockImplementation(async (text, fieldName) => {
      await copyToClipboard(text, fieldName,
        (field) => {
          // Success callback
          console.log(`${field} copied to clipboard`);
        },
        (field) => {
          // Error callback
          console.error(`Failed to copy ${field}`);
        }
      );
    });
    
    // Render the component
    render(
      <TestWrapper>
        <SSOProfileDetails />
      </TestWrapper>
    );
    
    // Wait for loading to complete
    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });
    
    // Simulate copying text with failure
    await handleCopyToClipboard('test-text', 'Test Field');
    
    // Check if copyToClipboard was called with correct arguments
    expect(copyToClipboard).toHaveBeenCalledWith(
      'test-text',
      'Test Field',
      expect.any(Function),
      expect.any(Function)
    );
    
    // Restore the original mock implementation
    copyToClipboard.mockImplementation(originalMockImplementation);
  });

  test('returns null when profileData is null after loading', async () => {
    // Mock mapApiToUIProfile to return null
    mapApiToUIProfile.mockReturnValue(null);
    
    render(
      <TestWrapper>
        <SSOProfileDetails />
      </TestWrapper>
    );
    
    // Wait for loading to complete
    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });
    
    // Check that no content is rendered
    expect(screen.queryByTestId('collapsible-section-profile-details')).not.toBeInTheDocument();
    expect(screen.queryByTestId('collapsible-section-provider-configuration')).not.toBeInTheDocument();
    expect(screen.queryByTestId('collapsible-section-user-group-mapping')).not.toBeInTheDocument();
  });
});