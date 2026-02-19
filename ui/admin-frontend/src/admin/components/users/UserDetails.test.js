import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import { createTheme } from '@mui/material';
import UserDetails from './UserDetails';
import { EditionProvider } from '../../context/EditionContext';

// Mock apiClient
jest.mock('../../utils/apiClient', () => {
  const mockClient = {
    get: jest.fn(),
    post: jest.fn(),
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
    expect(screen.getByText('Admin:')).toBeInTheDocument();
    expect(screen.getByText('Yes')).toBeInTheDocument();

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
    expect(screen.getByText('Admin:')).toBeInTheDocument();
    expect(screen.getByText('No')).toBeInTheDocument();

    // Check that the AccessToSSOConfig field is NOT displayed for non-admin users
    expect(screen.queryByText('Access to IdP configuration:')).not.toBeInTheDocument();
  });
});