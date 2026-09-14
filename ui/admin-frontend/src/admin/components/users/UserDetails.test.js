import React from 'react';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import { createTheme } from '@mui/material';
import UserDetails from './UserDetails';
import { EditionProvider } from '../../context/EditionContext';

// Permission gates: the caller manages users, so every action renders.
jest.mock('../../context/PermissionsContext', () => ({
  usePermissions: () => ({ rbacEnabled: false, can: () => true, canAny: () => true, canAll: () => true }),
}));

// Frontend config: the SSO key policy flag is read through useConfig.
const mockConfig = { allowSSOUserAPIKeys: false };
jest.mock('../../hooks/useConfig', () => ({
  __esModule: true,
  default: () => ({ config: mockConfig, loading: false, error: null }),
}));

// Mock apiClient
jest.mock('../../utils/apiClient', () => {
  const mockClient = {
    get: jest.fn(),
    post: jest.fn(),
    delete: jest.fn(),
    interceptors: {
      request: { use: jest.fn() },
      response: { use: jest.fn() }
    }
  };
  return {
    __esModule: true,
    default: mockClient
  };
});

// Mock axios for EditionContext
jest.mock('axios', () => ({
  get: jest.fn().mockResolvedValue({
    data: { edition: 'community', version: '1.0.0' }
  })
}));

// Mock useNavigate
const mockNavigate = jest.fn();
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
  useParams: () => ({ id: '123' })
}));

describe('UserDetails Component', () => {
  // Create a custom theme for testing
  const theme = createTheme({
    palette: {
      text: {
        primary: '#ffffff',
        defaultSubdued: 'rgba(255, 255, 255, 0.6)',
      },
      background: {
        buttonPrimaryDefault: '#007bff',
        buttonPrimaryDefaultHover: '#0069d9',
      },
      custom: {
        white: '#ffffff',
      },
      primary: {
        main: '#7b68ee',
      },
      error: {
        main: '#dc3545',
      },
      border: {
        neutralDefault: '#e0e0e0',
      },
    },
    typography: {
      bodyLargeMedium: {
        fontSize: '1rem',
        fontWeight: 500,
      },
      headingXLarge: {
        fontSize: '2rem',
        fontWeight: 'bold',
      },
    },
  });

  // Wrapper component with theme provider, edition provider, and router
  const Wrapper = ({ children }) => (
    <ThemeProvider theme={theme}>
      <EditionProvider>
        <MemoryRouter initialEntries={['/admin/users/123']}>
          <Routes>
            <Route path="/admin/users/:id" element={children} />
          </Routes>
        </MemoryRouter>
      </EditionProvider>
    </ThemeProvider>
  );

  let apiClient;

  beforeEach(() => {
    jest.clearAllMocks();
    apiClient = require('../../utils/apiClient').default;
    apiClient.get.mockReset();
    apiClient.post.mockReset();
  });

  test('renders admin user details with AccessToSSOConfig enabled', async () => {
    // Mock API responses
    apiClient.get.mockImplementation((url) => {
      if (url === '/users/123') {
        return Promise.resolve({
          data: {
            data: {
              id: '123',
              attributes: {
                name: 'Admin User',
                email: 'admin@example.com',
                is_admin: true,
                role: 'Admin',
                show_chat: true,
                show_portal: true,
                email_verified: true,
                api_key: 'api_key_123456789',
                notifications_enabled: true,
                access_to_sso_config: true
              }
            }
          }
        });
      } else if (url === '/users/123/groups') {
        return Promise.resolve({
          data: {
            data: [
              { id: '1', attributes: { name: 'Group 1' } }
            ]
          }
        });
      } else if (url === '/chat-history-records') {
        return Promise.resolve({
          data: {
            data: []
          },
          headers: {
            'x-total-count': '0',
            'x-total-pages': '0'
          }
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<UserDetails />, { wrapper: Wrapper });

    // Wait for the component to load
    await waitFor(() => {
      expect(screen.getAllByText('User details')[0]).toBeInTheDocument();
    });

    // Check that the user details are displayed
    expect(screen.getByText('Name:')).toBeInTheDocument();
    expect(screen.getByText('Admin User')).toBeInTheDocument();
    expect(screen.getByText('Email:')).toBeInTheDocument();
    expect(screen.getByText('admin@example.com')).toBeInTheDocument();
    expect(screen.getByText('Account type:')).toBeInTheDocument();
    expect(screen.getByText('Admin')).toBeInTheDocument();

    // Check that the AccessToSSOConfig field is displayed for admin users
    // Check that the AccessToSSOConfig field is displayed for admin users
    expect(screen.getByText('Access to IdP configuration:')).toBeInTheDocument();
    
    // Since we know the structure of the component, we can check that the text "Enabled"
    // appears somewhere in the document after the "Access to IdP configuration:" label
    expect(screen.getAllByText(/Enabled/)[0]).toBeInTheDocument();
  });

  test('renders admin user details with AccessToSSOConfig disabled', async () => {
    // Mock API responses
    apiClient.get.mockImplementation((url) => {
      if (url === '/users/123') {
        return Promise.resolve({
          data: {
            data: {
              id: '123',
              attributes: {
                name: 'Admin User',
                email: 'admin@example.com',
                is_admin: true,
                role: 'Admin',
                show_chat: true,
                show_portal: true,
                email_verified: true,
                api_key: 'api_key_123456789',
                notifications_enabled: true,
                access_to_sso_config: false
              }
            }
          }
        });
      } else if (url === '/users/123/groups') {
        return Promise.resolve({
          data: {
            data: [
              { id: '1', attributes: { name: 'Group 1' } }
            ]
          }
        });
      } else if (url === '/chat-history-records') {
        return Promise.resolve({
          data: {
            data: []
          },
          headers: {
            'x-total-count': '0',
            'x-total-pages': '0'
          }
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<UserDetails />, { wrapper: Wrapper });

    // Wait for the component to load
    await waitFor(() => {
      expect(screen.getAllByText('User details')[0]).toBeInTheDocument();
    });

    // Check that the AccessToSSOConfig field is displayed for admin users
    // Check that the AccessToSSOConfig field is displayed for admin users
    expect(screen.getByText('Access to IdP configuration:')).toBeInTheDocument();
    
    // Since we know the structure of the component, we can check that the text "Disabled"
    // appears somewhere in the document after the "Access to IdP configuration:" label
    expect(screen.getAllByText(/Disabled/)[0]).toBeInTheDocument();
  });

  test('does not show AccessToSSOConfig for non-admin users', async () => {
    // Mock API responses
    apiClient.get.mockImplementation((url) => {
      if (url === '/users/123') {
        return Promise.resolve({
          data: {
            data: {
              id: '123',
              attributes: {
                name: 'Regular User',
                email: 'user@example.com',
                is_admin: false,
                role: 'Developer',
                show_chat: true,
                show_portal: true,
                email_verified: true,
                api_key: 'api_key_123456789',
                notifications_enabled: false,
                access_to_sso_config: false
              }
            }
          }
        });
      } else if (url === '/users/123/groups') {
        return Promise.resolve({
          data: {
            data: [
              { id: '1', attributes: { name: 'Group 1' } }
            ]
          }
        });
      } else if (url === '/chat-history-records') {
        return Promise.resolve({
          data: {
            data: []
          },
          headers: {
            'x-total-count': '0',
            'x-total-pages': '0'
          }
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<UserDetails />, { wrapper: Wrapper });

    // Wait for the component to load
    await waitFor(() => {
      expect(screen.getAllByText('User details')[0]).toBeInTheDocument();
    });

    // Check that the user details are displayed
    expect(screen.getByText('Name:')).toBeInTheDocument();
    expect(screen.getByText('Regular User')).toBeInTheDocument();
    expect(screen.getByText('Email:')).toBeInTheDocument();
    expect(screen.getByText('user@example.com')).toBeInTheDocument();
    expect(screen.getByText('Account type:')).toBeInTheDocument();
    expect(screen.getByText('Developer')).toBeInTheDocument();

    // Check that the AccessToSSOConfig field is NOT displayed for non-admin users
    expect(screen.queryByText('Access to IdP configuration:')).not.toBeInTheDocument();
  });

  // Provenance, the API key states and the disable switch.
  const mockUserFetch = (attributes) => {
    apiClient.get.mockImplementation((url) => {
      if (url === '/users/123') {
        return Promise.resolve({ data: { data: { id: '123', attributes } } });
      }
      if (url === '/users/123/groups') {
        return Promise.resolve({ data: { data: [] } });
      }
      if (url === '/chat-history-records') {
        return Promise.resolve({ data: { data: [] }, headers: { 'x-total-count': '0', 'x-total-pages': '0' } });
      }
      return Promise.resolve({ data: {} });
    });
  };

  const baseAttributes = {
    name: 'Plain User',
    email: 'plain@example.com',
    is_admin: false,
    show_chat: true,
    show_portal: true,
    email_verified: true,
    notifications_enabled: false,
    access_to_sso_config: false,
    disabled: false,
  };

  test('a user with no API key shows "No API key issued" and an issue button, never a fake mask', async () => {
    mockUserFetch({ ...baseAttributes, has_api_key: false, auth_source: 'admin' });
    render(<UserDetails />, { wrapper: Wrapper });
    await waitFor(() => expect(screen.getByTestId('user-no-api-key')).toBeInTheDocument());

    expect(screen.queryByText(/\*{8}/)).not.toBeInTheDocument();
    expect(screen.getByTestId('user-issue-api-key')).toBeInTheDocument();
    expect(screen.queryByTestId('user-revoke-api-key')).not.toBeInTheDocument();
    expect(screen.getByTestId('user-origin')).toHaveTextContent('Admin-created');
    expect(screen.getByTestId('user-last-login')).toHaveTextContent('Never');
    expect(screen.getByTestId('user-status-chip')).toHaveTextContent('Active');
  });

  test('an SSO-provisioned user cannot be issued a key while the policy is off', async () => {
    mockUserFetch({
      ...baseAttributes,
      has_api_key: false,
      auth_source: 'sso',
      sso_profile_id: 'onelogin',
      last_login_at: '2026-09-14T09:00:00Z',
      last_login_method: 'sso',
    });
    render(<UserDetails />, { wrapper: Wrapper });
    await waitFor(() => expect(screen.getByTestId('user-no-api-key')).toBeInTheDocument());

    expect(screen.queryByTestId('user-issue-api-key')).not.toBeInTheDocument();
    expect(screen.getByTestId('user-sso-no-api-key')).toBeInTheDocument();
    expect(screen.getByTestId('user-origin')).toHaveTextContent('SSO (profile onelogin)');
    expect(screen.getByTestId('user-last-login')).toHaveTextContent('(SSO)');
  });

  test('an issued key offers regenerate and revoke, and revoking clears it', async () => {
    mockUserFetch({ ...baseAttributes, has_api_key: true, api_key: 'abcd1234567890wxyz', auth_source: 'local', api_key_last_used_at: '2026-09-14T09:00:00Z' });
    apiClient.delete.mockResolvedValue({
      data: { data: { id: '123', attributes: { ...baseAttributes, has_api_key: false, auth_source: 'local' } } },
    });
    render(<UserDetails />, { wrapper: Wrapper });
    await waitFor(() => expect(screen.getByTestId('user-revoke-api-key')).toBeInTheDocument());

    expect(screen.getByText('abcd********************wxyz')).toBeInTheDocument();
    expect(screen.getByTestId('user-roll-api-key')).toBeInTheDocument();
    expect(screen.getByTestId('user-api-key-last-used')).not.toHaveTextContent('never');

    fireEvent.click(screen.getByTestId('user-revoke-api-key'));
    fireEvent.click(await screen.findByTestId('user-revoke-api-key-confirm'));

    await waitFor(() => expect(apiClient.delete).toHaveBeenCalledWith('/users/123/api-key'));
    await waitFor(() => expect(screen.getByTestId('user-no-api-key')).toBeInTheDocument());
  });

  test('a disabled user shows the Disabled chip and the toggle re-enables them', async () => {
    mockUserFetch({ ...baseAttributes, has_api_key: false, auth_source: 'local', disabled: true, disabled_at: '2026-09-14T09:00:00Z' });
    apiClient.post.mockResolvedValue({
      data: { data: { id: '123', attributes: { ...baseAttributes, has_api_key: false, auth_source: 'local', disabled: false } } },
    });
    render(<UserDetails />, { wrapper: Wrapper });
    await waitFor(() => expect(screen.getByTestId('user-status-chip')).toHaveTextContent('Disabled'));

    fireEvent.click(screen.getByTestId('user-toggle-disabled'));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith('/users/123/enable'));
    await waitFor(() => expect(screen.getByTestId('user-status-chip')).toHaveTextContent('Active'));
  });
});