import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import { createTheme } from '@mui/material';
import UserForm from './UserForm';

// Mock apiClient
jest.mock('../../utils/apiClient', () => {
  const mockClient = {
    get: jest.fn(),
    post: jest.fn(),
    patch: jest.fn(),
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

// Mock useNavigate
const mockNavigate = jest.fn();
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
  useParams: () => ({ id: undefined }) // Default to create mode
}));

describe('UserForm Component', () => {
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

  // Wrapper component with theme provider and router
  const Wrapper = ({ children }) => (
    <ThemeProvider theme={theme}>
      <MemoryRouter>
        <Routes>
          <Route path="*" element={children} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

  let apiClient;

  beforeEach(() => {
    jest.clearAllMocks();
    apiClient = require('../../utils/apiClient').default;
    apiClient.get.mockReset();
    apiClient.post.mockReset();
    apiClient.patch.mockReset();

    // Mock successful API responses
    apiClient.get.mockImplementation((url) => {
      if (url === '/groups') {
        return Promise.resolve({
          data: {
            data: [
              { id: '1', attributes: { name: 'Group 1' } },
              { id: '2', attributes: { name: 'Group 2' } }
            ]
          }
        });
      }
      return Promise.resolve({ data: {} });
    });

    apiClient.post.mockResolvedValue({
      data: {
        data: {
          id: '123',
          attributes: {
            email: 'test@example.com',
            name: 'Test User',
            is_admin: true
          }
        }
      }
    });
  });

  test('renders form correctly in create mode', async () => {
    render(<UserForm />, { wrapper: Wrapper });

    // Wait for the component to load
    await waitFor(() => {
      expect(screen.getAllByText('Add user')[0]).toBeInTheDocument();
    });

    // Check that all form fields are present
    expect(screen.getByRole('textbox', { name: /name/i })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: /email/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
    expect(screen.getByText('Admin User')).toBeInTheDocument();
    expect(screen.getByText('Show Portal')).toBeInTheDocument();
    expect(screen.getByText('Show Chat')).toBeInTheDocument();
    expect(screen.getByText('Email Verified')).toBeInTheDocument();

    // The "Enable access to IdP configuration" switch should not be visible initially
    // because isAdmin is false by default
    expect(screen.queryByText('Enable access to IdP configuration')).not.toBeInTheDocument();
  });

  test('shows AccessToSSOConfig switch when Admin is selected', async () => {
    render(<UserForm />, { wrapper: Wrapper });

    // Wait for the component to load
    await waitFor(() => {
      expect(screen.getAllByText('Add user')[0]).toBeInTheDocument();
    });

    // Initially, the AccessToSSOConfig switch should not be visible
    expect(screen.queryByText('Enable access to IdP configuration')).not.toBeInTheDocument();

    // Toggle the Admin switch
    const adminSwitch = screen.getByLabelText('Admin User');
    fireEvent.click(adminSwitch);

    // Now the AccessToSSOConfig switch should be visible
    expect(screen.getByText('Enable access to IdP configuration')).toBeInTheDocument();

    // Toggle the Admin switch back to off
    fireEvent.click(adminSwitch);

    // The AccessToSSOConfig switch should disappear again
    expect(screen.queryByText('Enable access to IdP configuration')).not.toBeInTheDocument();
  });

  test('submits form with AccessToSSOConfig when Admin is selected', async () => {
    render(<UserForm />, { wrapper: Wrapper });

    // Wait for the component to load
    await waitFor(() => {
      expect(screen.getAllByText('Add user')[0]).toBeInTheDocument();
    });

    // Fill in the form
    // Find the input fields by their label text in the DOM
    const nameInput = screen.getByRole('textbox', { name: /name/i });
    const emailInput = screen.getByRole('textbox', { name: /email/i });
    const passwordInput = screen.getByLabelText(/password/i);
    
    // Fill in the form
    fireEvent.change(nameInput, { target: { value: 'Test Admin' } });
    fireEvent.change(emailInput, { target: { value: 'admin@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    // Toggle the Admin switch
    const adminSwitch = screen.getByLabelText('Admin User');
    fireEvent.click(adminSwitch);

    // Now the AccessToSSOConfig switch should be visible
    const ssoConfigSwitch = screen.getByLabelText('Enable access to IdP configuration');
    expect(ssoConfigSwitch).toBeInTheDocument();

    // Toggle the AccessToSSOConfig switch
    fireEvent.click(ssoConfigSwitch);

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: /add user/i }));

    // Wait for the API call to complete
    await waitFor(() => {
      expect(apiClient.post).toHaveBeenCalledWith('/users', {
        data: {
          type: 'User',
          attributes: {
            name: 'Test Admin',
            email: 'admin@example.com',
            password: 'password123',
            is_admin: true,
            show_portal: true,
            show_chat: true,
            email_verified: false,
            notifications_enabled: false,
            access_to_sso_config: true,
          },
        },
      });
    });
  });

  test('does not allow AccessToSSOConfig when not Admin', async () => {
    render(<UserForm />, { wrapper: Wrapper });

    // Wait for the component to load
    await waitFor(() => {
      expect(screen.getAllByText('Add user')[0]).toBeInTheDocument();
    });

    // Fill in the form
    // Find the input fields by their label text in the DOM
    const nameInput = screen.getByRole('textbox', { name: /name/i });
    const emailInput = screen.getByRole('textbox', { name: /email/i });
    const passwordInput = screen.getByLabelText(/password/i);
    
    // Fill in the form
    fireEvent.change(nameInput, { target: { value: 'Regular User' } });
    fireEvent.change(emailInput, { target: { value: 'user@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    // Submit the form (without toggling Admin)
    fireEvent.click(screen.getByRole('button', { name: /add user/i }));

    // Wait for the API call to complete
    await waitFor(() => {
      expect(apiClient.post).toHaveBeenCalledWith('/users', {
        data: {
          type: 'User',
          attributes: {
            name: 'Regular User',
            email: 'user@example.com',
            password: 'password123',
            is_admin: false,
            show_portal: true,
            show_chat: true,
            email_verified: false,
            notifications_enabled: false,
            access_to_sso_config: false,
          },
        },
      });
    });

    // Verify that access_to_sso_config is false in the API call
    const apiCall = apiClient.post.mock.calls[0][1];
    expect(apiCall.data.attributes.access_to_sso_config).toBe(false);
  });
});