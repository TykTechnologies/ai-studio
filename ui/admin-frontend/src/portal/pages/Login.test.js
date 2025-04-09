import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import { createTheme } from '@mui/material';
import Login from './Login';

// Mock pubClient
jest.mock('../../admin/utils/pubClient', () => {
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
    default: mockClient,
    reinitializePubClient: jest.fn()
  };
});

// Mock window.location
const originalLocation = window.location;
beforeAll(() => {
  delete window.location;
  window.location = {
    href: '',
    pathname: '/login'
  };
});

afterAll(() => {
  window.location = originalLocation;
});

// Mock console.error to prevent error output in tests
const originalConsoleError = console.error;
beforeAll(() => {
  console.error = jest.fn();
});

afterAll(() => {
  console.error = originalConsoleError;
});

describe('Login Component', () => {
  // Create a custom theme for testing
  const theme = createTheme({
    palette: {
      text: {
        primary: '#ffffff',
        defaultSubdued: 'rgba(255, 255, 255, 0.6)',
      },
      background: {
        buttonPrimaryOutlineHover: '#e0e0e0',
      },
      custom: {
        white: '#ffffff',
      },
    },
  });

  // Wrapper component with theme provider and router
  const Wrapper = ({ children }) => (
    <ThemeProvider theme={theme}>
      <MemoryRouter initialEntries={['/login']}>
        <Routes>
          <Route path="/login" element={children} />
          <Route path="/register" element={<div>Register Page</div>} />
          <Route path="/forgot-password" element={<div>Forgot Password Page</div>} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

  let pubClient;

  beforeEach(() => {
    jest.clearAllMocks();
    window.location.href = '';
    pubClient = require('../../admin/utils/pubClient').default;
    pubClient.get.mockReset();
    pubClient.post.mockReset();
  });

  test('renders login form correctly', async () => {
    // Mock SSO config response
    pubClient.get.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ data: { tibEnabled: false } });
      }
      return Promise.resolve({ data: {} });
    });

    render(<Login />, { wrapper: Wrapper });

    // Check that the component renders with the correct content
    expect(screen.getByText('Log in to your account')).toBeInTheDocument();
    expect(screen.getByLabelText('Email address')).toBeInTheDocument();
    expect(screen.getByLabelText('Password')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Log in' })).toBeInTheDocument();
    expect(screen.getByText("Don't have an account?")).toBeInTheDocument();
    expect(screen.getByText('Sign up')).toBeInTheDocument();
    expect(screen.getByText('Forgot password?')).toBeInTheDocument();

    // SSO button should not be visible when SSO is disabled
    expect(screen.queryByText('Log in with SSO')).not.toBeInTheDocument();
  });

  test('renders SSO button when SSO is enabled', async () => {
    // Mock SSO config and profile responses
    pubClient.get.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ data: { tibEnabled: true } });
      }
      if (url === '/login-sso-profile') {
        return Promise.resolve({
          data: {
            data: {
              attributes: {
                login_url: 'https://sso.example.com/login'
              }
            }
          }
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<Login />, { wrapper: Wrapper });

    // Wait for SSO button to appear
    await waitFor(() => {
      expect(screen.getByText('OR')).toBeInTheDocument();
    });
    
    // Check that the SSO button is in the document
    expect(screen.getByText('Log in with SSO')).toBeInTheDocument();

    // Check that the SSO button has the correct href
    const ssoButton = screen.getByRole('link', { name: 'Log in with SSO' });
    expect(ssoButton).toHaveAttribute('href', 'https://sso.example.com/login');
  });

  test('handles form submission with valid credentials', async () => {
    // Mock SSO config response
    pubClient.get.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ data: { tibEnabled: false } });
      }
      if (url === '/common/me') {
        return Promise.resolve({
          data: {
            attributes: {
              ui_options: {
                show_portal: true
              }
            }
          }
        });
      }
      return Promise.resolve({ data: {} });
    });

    // Mock successful login response
    pubClient.post.mockImplementation((url) => {
      if (url === '/auth/login') {
        return Promise.resolve({
          data: {
            message: 'Login successful'
          }
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<Login />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'test@example.com' }
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'password123' }
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Log in' }));

    // Wait for the login process to complete
    await waitFor(() => {
      expect(pubClient.post).toHaveBeenCalledWith('/auth/login', {
        data: {
          type: 'login',
          attributes: { email: 'test@example.com', password: 'password123' }
        }
      });
    });

    // Check that the user is redirected to the portal dashboard
    expect(window.location.href).toBe('/portal/dashboard');
  });

  test('redirects to chat dashboard when user has chat access only', async () => {
    // Mock SSO config response
    pubClient.get.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ data: { tibEnabled: false } });
      }
      if (url === '/common/me') {
        return Promise.resolve({
          data: {
            attributes: {
              ui_options: {
                show_portal: false,
                show_chat: true
              }
            }
          }
        });
      }
      return Promise.resolve({ data: {} });
    });

    // Mock successful login response
    pubClient.post.mockImplementation((url) => {
      if (url === '/auth/login') {
        return Promise.resolve({
          data: {
            message: 'Login successful'
          }
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<Login />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'test@example.com' }
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'password123' }
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Log in' }));

    // Wait for the login process to complete
    await waitFor(() => {
      expect(pubClient.post).toHaveBeenCalledWith('/auth/login', {
        data: {
          type: 'login',
          attributes: { email: 'test@example.com', password: 'password123' }
        }
      });
    });

    // Check that the user is redirected to the chat dashboard
    expect(window.location.href).toBe('/chat/dashboard');
  });

  test('displays error when user has no access to any features', async () => {
    // Mock SSO config response
    pubClient.get.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ data: { tibEnabled: false } });
      }
      if (url === '/common/me') {
        return Promise.resolve({
          data: {
            attributes: {
              ui_options: {
                show_portal: false,
                show_chat: false
              }
            }
          }
        });
      }
      return Promise.resolve({ data: {} });
    });

    // Mock successful login response
    pubClient.post.mockImplementation((url) => {
      if (url === '/auth/login') {
        return Promise.resolve({
          data: {
            message: 'Login successful'
          }
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<Login />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'test@example.com' }
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'password123' }
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Log in' }));

    // Wait for the error message to appear
    await waitFor(() => {
      expect(screen.getByText("Your account doesn't have access to any features.")).toBeInTheDocument();
    });

    // Check that the user is not redirected
    expect(window.location.href).toBe('');
  });

  test('handles login error with error response', async () => {
    // Mock SSO config response
    pubClient.get.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ data: { tibEnabled: false } });
      }
      return Promise.resolve({ data: {} });
    });

    // Mock failed login response with error message
    pubClient.post.mockImplementation((url) => {
      if (url === '/auth/login') {
        return Promise.reject({
          response: {
            data: {
              error: 'Invalid credentials'
            }
          }
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<Login />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'test@example.com' }
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'wrongpassword' }
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Log in' }));

    // Wait for the error message to appear
    await waitFor(() => {
      expect(screen.getByText('Invalid credentials')).toBeInTheDocument();
    });

    // Check that the user is not redirected
    expect(window.location.href).toBe('');
  });

  test('handles login error with errors array', async () => {
    // Mock SSO config response
    pubClient.get.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ data: { tibEnabled: false } });
      }
      return Promise.resolve({ data: {} });
    });

    // Mock failed login response with errors array
    pubClient.post.mockImplementation((url) => {
      if (url === '/auth/login') {
        return Promise.reject({
          response: {
            data: {
              errors: [
                { detail: 'Account locked' }
              ]
            }
          }
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<Login />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'test@example.com' }
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'password123' }
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Log in' }));

    // Wait for the error message to appear
    await waitFor(() => {
      expect(screen.getByText('Account locked')).toBeInTheDocument();
    });

    // Check that the user is not redirected
    expect(window.location.href).toBe('');
  });

  test('handles unexpected login error', async () => {
    // Mock SSO config response
    pubClient.get.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ data: { tibEnabled: false } });
      }
      return Promise.resolve({ data: {} });
    });

    // Mock failed login response with no specific error
    pubClient.post.mockImplementation((url) => {
      if (url === '/auth/login') {
        return Promise.reject({
          response: {
            data: {}
          }
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<Login />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'test@example.com' }
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'password123' }
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Log in' }));

    // Wait for the error message to appear
    await waitFor(() => {
      expect(screen.getByText('An unexpected error occurred. Please try again.')).toBeInTheDocument();
    });

    // Check that the user is not redirected
    expect(window.location.href).toBe('');
  });

  test('handles network error during login', async () => {
    // Mock SSO config response
    pubClient.get.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ data: { tibEnabled: false } });
      }
      return Promise.resolve({ data: {} });
    });

    // Mock network error during login
    pubClient.post.mockImplementation((url) => {
      if (url === '/auth/login') {
        return Promise.reject(new Error('Network error'));
      }
      return Promise.resolve({ data: {} });
    });

    render(<Login />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'test@example.com' }
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'password123' }
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Log in' }));

    // Wait for the error message to appear
    await waitFor(() => {
      expect(screen.getByText('An unexpected error occurred. Please try again.')).toBeInTheDocument();
    });

    // Check that the user is not redirected
    expect(window.location.href).toBe('');
  });

  test('navigates to register page when sign up link is clicked', async () => {
    // Mock SSO config response
    pubClient.get.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ data: { tibEnabled: false } });
      }
      return Promise.resolve({ data: {} });
    });

    render(<Login />, { wrapper: Wrapper });

    // Click the sign up link
    fireEvent.click(screen.getByText('Sign up'));

    // Check that we navigate to the register page
    expect(screen.getByText('Register Page')).toBeInTheDocument();
  });

  test('navigates to forgot password page when forgot password link is clicked', async () => {
    // Mock SSO config response
    pubClient.get.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ data: { tibEnabled: false } });
      }
      return Promise.resolve({ data: {} });
    });

    render(<Login />, { wrapper: Wrapper });

    // Click the forgot password link
    fireEvent.click(screen.getByText('Forgot password?'));

    // Check that we navigate to the forgot password page
    expect(screen.getByText('Forgot Password Page')).toBeInTheDocument();
  });

  test('handles error when fetching SSO config', async () => {
    // Mock error when fetching SSO config
    pubClient.get.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.reject(new Error('Failed to fetch SSO config'));
      }
      return Promise.resolve({ data: {} });
    });

    render(<Login />, { wrapper: Wrapper });

    // Component should still render without SSO button
    expect(screen.getByText('Log in to your account')).toBeInTheDocument();
    expect(screen.getByLabelText('Email address')).toBeInTheDocument();
    expect(screen.getByLabelText('Password')).toBeInTheDocument();
    
    // Wait to ensure SSO button doesn't appear
    await waitFor(() => {
      expect(screen.queryByText('Log in with SSO')).not.toBeInTheDocument();
    });
    
    // Check that console.error was called
    expect(console.error).toHaveBeenCalled();
  });

  test('submits form with empty fields', async () => {
    // Mock SSO config response
    pubClient.get.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ data: { tibEnabled: false } });
      }
      return Promise.resolve({ data: {} });
    });

    // Mock failed login response for empty fields
    pubClient.post.mockImplementation((url) => {
      if (url === '/auth/login') {
        return Promise.reject({
          response: {
            data: {
              error: 'Email and password are required'
            }
          }
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<Login />, { wrapper: Wrapper });

    // Submit the form without filling in any fields
    fireEvent.click(screen.getByRole('button', { name: 'Log in' }));

    // Wait for the error message to appear
    await waitFor(() => {
      expect(screen.getByText('Email and password are required')).toBeInTheDocument();
    });

    // Verify the API call was made with empty fields
    expect(pubClient.post).toHaveBeenCalledWith('/auth/login', {
      data: {
        type: 'login',
        attributes: { email: '', password: '' }
      }
    });
  });
});