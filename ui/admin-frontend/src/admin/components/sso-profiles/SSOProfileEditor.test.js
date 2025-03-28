import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import SSOProfileEditor from './SSOProfileEditor';
import apiClient from '../../utils/apiClient';
import { createEmptyProfile, mapApiToUIProfile, mapUIProfileToApi } from './UIProfile';

// Mock dependencies
jest.mock('../../utils/apiClient');
jest.mock('./UIProfile', () => ({
  createEmptyProfile: jest.fn(),
  mapApiToUIProfile: jest.fn(),
  mapUIProfileToApi: jest.fn()
}));

// Mock react-router-dom hooks
const mockNavigate = jest.fn();
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
  useParams: jest.fn(),
}));

// Mock localStorage
const mockLocalStorage = (() => {
  let store = {};
  return {
    getItem: jest.fn(key => store[key] || null),
    setItem: jest.fn((key, value) => {
      store[key] = value.toString();
    }),
    clear: jest.fn(() => {
      store = {};
    })
  };
})();
Object.defineProperty(window, 'localStorage', {
  value: mockLocalStorage
});

// Mock Monaco Editor
jest.mock('@monaco-editor/react', () => {
  return function MockEditor({ value, onChange, height }) {
    return (
      <div data-testid="monaco-editor" style={{ height }}>
        <textarea
          data-testid="monaco-editor-textarea"
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      </div>
    );
  };
});

// Mock sharedStyles to avoid theme-related errors
jest.mock('../../styles/sharedStyles', () => ({
  TitleBox: ({ children, ...props }) => <div data-testid="title-box" {...props}>{children}</div>,
  ContentBox: ({ children, ...props }) => <div data-testid="content-box" {...props}>{children}</div>,
  PrimaryButton: ({ children, onClick, ...props }) => (
    <button data-testid="primary-button" onClick={onClick} {...props}>{children}</button>
  ),
  SecondaryLinkButton: ({ children, ...props }) => (
    <button data-testid="secondary-link-button" {...props}>{children}</button>
  ),
  DangerOutlineButton: ({ children, onClick, ...props }) => (
    <button data-testid="danger-outline-button" onClick={onClick} {...props}>{children}</button>
  )
}));

// Mock WarningDialog
jest.mock('../../components/common/WarningDialog', () => {
  return function MockWarningDialog({ open, onConfirm, onCancel, title, message, buttonLabel }) {
    if (!open) return null;
    return (
      <div data-testid="warning-dialog">
        <div data-testid="warning-dialog-title">{title}</div>
        <div data-testid="warning-dialog-message">{message}</div>
        <button data-testid="warning-dialog-confirm" onClick={onConfirm}>
          {buttonLabel}
        </button>
        <button data-testid="warning-dialog-cancel" onClick={onCancel}>
          Cancel
        </button>
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
const TestWrapper = ({ children, initialEntries = ['/admin/sso-profiles/new'] }) => (
  <ThemeProvider theme={theme}>
    <MemoryRouter initialEntries={initialEntries}>
      <Routes>
        <Route path="/admin/sso-profiles/new" element={children} />
        <Route path="/admin/sso-profiles/:profileId" element={children} />
        <Route path="/admin/sso-profiles" element={<div>SSO Profiles List</div>} />
      </Routes>
    </MemoryRouter>
  </ThemeProvider>
);

describe('SSOProfileEditor', () => {
  const mockProfileId = '123';
  const mockEmptyProfile = {
    ID: '',
    Name: 'Test Profile',
    ActionType: 'oauth',
    ProviderConfig: {
      UseProviders: [
        {
          Key: '',
          Secret: '',
        }
      ]
    },
    DefaultUserGroupID: '1',
    UserGroupMapping: {}
  };
  
  const mockExistingProfile = {
    ID: '123',
    Name: 'Existing Profile',
    ActionType: 'oauth',
    ProviderConfig: {
      UseProviders: [
        {
          Key: 'test-key',
          Secret: 'test-secret',
        }
      ]
    },
    DefaultUserGroupID: '1',
    UserGroupMapping: {
      'provider-group-1': 'tyk-group-1'
    }
  };

  const mockProfileResponse = {
    data: {
      data: {
        attributes: {
          profile_id: '123',
          name: 'Existing Profile',
          action_type: 'oauth',
          provider_config: {
            UseProviders: [
              {
                Key: 'test-key',
                Secret: 'test-secret',
              }
            ]
          },
          default_user_group_id: '1',
          user_group_mapping: {
            'provider-group-1': 'tyk-group-1'
          }
        }
      }
    }
  };

  beforeEach(() => {
    jest.clearAllMocks();
    
    // Mock createEmptyProfile
    createEmptyProfile.mockReturnValue(mockEmptyProfile);
    
    // Mock mapApiToUIProfile
    mapApiToUIProfile.mockReturnValue(mockExistingProfile);
    
    // Mock mapUIProfileToApi
    mapUIProfileToApi.mockImplementation((profile) => ({
      data: {
        type: 'sso-profiles',
        attributes: {
          profile_id: profile.ID,
          name: profile.Name,
          action_type: profile.ActionType,
          provider_config: profile.ProviderConfig,
          default_user_group_id: profile.DefaultUserGroupID,
          user_group_mapping: profile.UserGroupMapping
        }
      }
    }));
    
    // Mock API responses
    apiClient.get.mockImplementation((url) => {
      if (url === `/sso-profiles/${mockProfileId}`) {
        return Promise.resolve(mockProfileResponse);
      }
      return Promise.reject(new Error('Not found'));
    });
    
    apiClient.post.mockResolvedValue({ data: { data: { id: 'new-id' } } });
    apiClient.put.mockResolvedValue({ data: { data: { id: mockProfileId } } });
    apiClient.delete.mockResolvedValue({});
    
    // Mock useParams for different test cases
    require('react-router-dom').useParams.mockReturnValue({ profileId: 'new' });
  });

  test('renders create mode correctly', () => {
    render(
      <TestWrapper>
        <SSOProfileEditor />
      </TestWrapper>
    );
    
    // Check title
    expect(screen.getByText('Create SSO Profile')).toBeInTheDocument();
    
    // Check buttons
    expect(screen.getByText('Save Profile')).toBeInTheDocument();
    expect(screen.queryByText('Delete Profile')).not.toBeInTheDocument(); // Delete button should not be present in create mode
    
    // Check editor
    expect(screen.getByTestId('monaco-editor')).toBeInTheDocument();
    expect(screen.getByTestId('monaco-editor-textarea')).toHaveValue(JSON.stringify(mockEmptyProfile, null, 2));
  });

  test('renders edit mode correctly and fetches profile data', async () => {
    // Mock useParams for edit mode
    require('react-router-dom').useParams.mockReturnValue({ profileId: mockProfileId });
    
    render(
      <TestWrapper initialEntries={[`/admin/sso-profiles/${mockProfileId}`]}>
        <SSOProfileEditor />
      </TestWrapper>
    );
    
    // Should show loading initially
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
    
    // Wait for loading to complete
    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });
    
    // Check API call
    expect(apiClient.get).toHaveBeenCalledWith(`/sso-profiles/${mockProfileId}`);
    
    // Check title
    expect(screen.getByText('Edit SSO Profile')).toBeInTheDocument();
    
    // Check buttons
    expect(screen.getByText('Save Profile')).toBeInTheDocument();
    expect(screen.getByText('Delete Profile')).toBeInTheDocument(); // Delete button should be present in edit mode
    
    // Check editor content
    expect(screen.getByTestId('monaco-editor')).toBeInTheDocument();
    expect(screen.getByTestId('monaco-editor-textarea')).toHaveValue(JSON.stringify(mockExistingProfile, null, 2));
  });

  test('handles editor content changes', () => {
    render(
      <TestWrapper>
        <SSOProfileEditor />
      </TestWrapper>
    );
    
    const editorTextarea = screen.getByTestId('monaco-editor-textarea');
    const updatedContent = JSON.stringify({ ...mockEmptyProfile, Name: 'Updated Name' }, null, 2);
    
    // Simulate editor content change
    fireEvent.change(editorTextarea, { target: { value: updatedContent } });
    
    // Check if editor content is updated
    expect(editorTextarea).toHaveValue(updatedContent);
  });

  test('handles save in create mode', async () => {
    render(
      <TestWrapper>
        <SSOProfileEditor />
      </TestWrapper>
    );
    
    // Click save button
    fireEvent.click(screen.getByText('Save Profile'));
    // Check if API was called correctly
    await waitFor(() => {
      expect(apiClient.post).toHaveBeenCalledWith('/sso-profiles', expect.any(Object));
    });
    
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/admin/sso-profiles');
    });
    
    // Check if localStorage notification was set
    expect(localStorage.setItem).toHaveBeenCalledWith(
      'tyk_ai_studio_admin_sso_notification',
      expect.stringContaining('create')
    );
  });

  test('handles save in edit mode', async () => {
    // Mock useParams for edit mode
    require('react-router-dom').useParams.mockReturnValue({ profileId: mockProfileId });
    
    render(
      <TestWrapper initialEntries={[`/admin/sso-profiles/${mockProfileId}`]}>
        <SSOProfileEditor />
      </TestWrapper>
    );
    
    // Wait for loading to complete
    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });
    
    // Click save button
    fireEvent.click(screen.getByText('Save Profile'));
    
    // Check if API was called correctly
    await waitFor(() => {
      expect(apiClient.put).toHaveBeenCalledWith(`/sso-profiles/${mockProfileId}`, expect.any(Object));
    });
    
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/admin/sso-profiles');
    });
    
    // Check if localStorage notification was set
    expect(localStorage.setItem).toHaveBeenCalledWith(
      'tyk_ai_studio_admin_sso_notification',
      expect.stringContaining('update')
    );
  });

  test('handles save error', async () => {
    // Mock API error
    apiClient.post.mockRejectedValue({
      response: {
        data: {
          errors: [{ detail: 'Invalid profile data' }]
        }
      }
    });
    
    render(
      <TestWrapper>
        <SSOProfileEditor />
      </TestWrapper>
    );
    
    // Click save button
    fireEvent.click(screen.getByText('Save Profile'));
    
    // Check if error snackbar is shown
    await waitFor(() => {
      expect(screen.getByText('Invalid profile data')).toBeInTheDocument();
    });
  });

  test('handles delete button click and shows warning dialog', async () => {
    // Mock useParams for edit mode
    require('react-router-dom').useParams.mockReturnValue({ profileId: mockProfileId });
    
    render(
      <TestWrapper initialEntries={[`/admin/sso-profiles/${mockProfileId}`]}>
        <SSOProfileEditor />
      </TestWrapper>
    );
    
    // Wait for loading to complete
    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });
    
    // Click delete button
    fireEvent.click(screen.getByText('Delete Profile'));
    
    // Check if warning dialog is shown
    expect(screen.getByTestId('warning-dialog')).toBeInTheDocument();
    expect(screen.getByTestId('warning-dialog-title')).toHaveTextContent('Delete SSO profile');
  });

  test('handles delete confirmation', async () => {
    // Mock useParams for edit mode
    require('react-router-dom').useParams.mockReturnValue({ profileId: mockProfileId });
    
    render(
      <TestWrapper initialEntries={[`/admin/sso-profiles/${mockProfileId}`]}>
        <SSOProfileEditor />
      </TestWrapper>
    );
    
    // Wait for loading to complete
    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });
    
    // Click delete button
    fireEvent.click(screen.getByText('Delete Profile'));
    
    // Click confirm in warning dialog
    fireEvent.click(screen.getByTestId('warning-dialog-confirm'));
    
    // Check if API was called correctly
    await waitFor(() => {
      expect(apiClient.delete).toHaveBeenCalledWith(`/sso-profiles/${mockProfileId}`);
    });
    
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/admin/sso-profiles');
    });
    
    // Check if localStorage notification was set
    expect(localStorage.setItem).toHaveBeenCalledWith(
      'tyk_ai_studio_admin_sso_notification',
      expect.stringContaining('delete')
    );
  });

  test('handles delete cancel', async () => {
    // Mock useParams for edit mode
    require('react-router-dom').useParams.mockReturnValue({ profileId: mockProfileId });
    
    render(
      <TestWrapper initialEntries={[`/admin/sso-profiles/${mockProfileId}`]}>
        <SSOProfileEditor />
      </TestWrapper>
    );
    
    // Wait for loading to complete
    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });
    
    // Click delete button
    fireEvent.click(screen.getByText('Delete Profile'));
    
    // Click cancel in warning dialog
    fireEvent.click(screen.getByTestId('warning-dialog-cancel'));
    
    // Check if warning dialog is closed
    expect(screen.queryByTestId('warning-dialog')).not.toBeInTheDocument();
    
    // Check that API was not called
    expect(apiClient.delete).not.toHaveBeenCalled();
  });

  test('handles delete error', async () => {
    // Mock API error
    apiClient.delete.mockRejectedValue(new Error('Failed to delete profile'));
    
    // Mock useParams for edit mode
    require('react-router-dom').useParams.mockReturnValue({ profileId: mockProfileId });
    
    render(
      <TestWrapper initialEntries={[`/admin/sso-profiles/${mockProfileId}`]}>
        <SSOProfileEditor />
      </TestWrapper>
    );
    
    // Wait for loading to complete
    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });
    
    // Click delete button
    fireEvent.click(screen.getByText('Delete Profile'));
    
    // Click confirm in warning dialog
    fireEvent.click(screen.getByTestId('warning-dialog-confirm'));
    
    // Check if error snackbar is shown
    await waitFor(() => {
      expect(screen.getByText('Failed to delete SSO profile')).toBeInTheDocument();
    });
  });

  test('handles fetch error', async () => {
    // Mock API error
    apiClient.get.mockRejectedValue(new Error('Failed to fetch profile'));
    
    // Mock useParams for edit mode
    require('react-router-dom').useParams.mockReturnValue({ profileId: mockProfileId });
    
    render(
      <TestWrapper initialEntries={[`/admin/sso-profiles/${mockProfileId}`]}>
        <SSOProfileEditor />
      </TestWrapper>
    );
    
    // Wait for loading to complete
    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });
    
    // Check if error message is shown
    expect(screen.getByText('Failed to load SSO profile')).toBeInTheDocument();
  });

  test('handles snackbar close', async () => {
    // Mock API error to trigger snackbar
    apiClient.post.mockRejectedValue({
      response: {
        data: {
          errors: [{ detail: 'Invalid profile data' }]
        }
      }
    });
    
    render(
      <TestWrapper>
        <SSOProfileEditor />
      </TestWrapper>
    );
    
    // Click save button to trigger error
    fireEvent.click(screen.getByText('Save Profile'));
    
    // Wait for snackbar to appear
    await waitFor(() => {
      expect(screen.getByText('Invalid profile data')).toBeInTheDocument();
    });
    
    // Close snackbar
    fireEvent.click(screen.getByRole('button', { name: /close/i }));
    
    // Check if snackbar is closed
    await waitFor(() => {
      expect(screen.queryByText('Invalid profile data')).not.toBeInTheDocument();
    });
  });

  test('handles invalid JSON in editor when saving', async () => {
    render(
      <TestWrapper>
        <SSOProfileEditor />
      </TestWrapper>
    );
    
    const editorTextarea = screen.getByTestId('monaco-editor-textarea');
    
    // Set invalid JSON in editor
    fireEvent.change(editorTextarea, { target: { value: '{ invalid json' } });
    
    // Click save button
    fireEvent.click(screen.getByText('Save Profile'));
    
    // Check if error snackbar is shown
    await waitFor(() => {
      expect(screen.getByText(/Failed to save SSO profile/i)).toBeInTheDocument();
    });
    
    // API should not be called with invalid JSON
    expect(apiClient.post).not.toHaveBeenCalled();
  });

  test('back button has correct link to SSO profiles list', () => {
    render(
      <TestWrapper>
        <SSOProfileEditor />
      </TestWrapper>
    );
    
    // Find the back button
    const backButton = screen.getByText('back to SSO Profiles');
    
    // Check if it has the correct "to" prop
    expect(backButton).toBeInTheDocument();
    expect(backButton).toHaveAttribute('to', '/admin/sso-profiles');
  });
});