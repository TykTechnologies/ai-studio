import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter, Routes, Route, useNavigate } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import { createTheme } from '@mui/material';
import Register from './Register';

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

// Mock config
jest.mock('../../config', () => ({
  getConfig: jest.fn().mockReturnValue({
    DEFAULT_SIGNUP_MODE: 'both'
  }),
  loadConfig: jest.fn()
}));

// Mock useNavigate
const mockNavigate = jest.fn();
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
}));

// Mock window.location
const originalLocation = window.location;
beforeAll(() => {
  delete window.location;
  window.location = {
    href: '',
    pathname: '/register'
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

describe('Register Component', () => {
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
        purpleExtraDark: '#4b0082',
      },
      primary: {
        main: '#7b68ee',
      },
      success: {
        main: '#28a745',
      },
      error: {
        main: '#dc3545',
      },
    },
    typography: {
      bodyLargeBold: {
        fontWeight: 'bold',
      },
      bodyMediumSemiBold: {
        fontWeight: 600,
      },
      bodyMediumDefault: {
        fontWeight: 'normal',
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
      <MemoryRouter initialEntries={['/register']}>
        <Routes>
          <Route path="/register" element={children} />
          <Route path="/login" element={<div>Login Page</div>} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

  let pubClient;
  let getConfig;

  beforeEach(() => {
    jest.clearAllMocks();
    window.location.href = '';
    pubClient = require('../../admin/utils/pubClient').default;
    pubClient.get.mockReset();
    pubClient.post.mockReset();
    getConfig = require('../../config').getConfig;
    mockNavigate.mockReset();
  });

  test('renders registration form correctly', () => {
    render(<Register />, { wrapper: Wrapper });

    // Check that the component renders with the correct content
    expect(screen.getByText('Create an account')).toBeInTheDocument();
    expect(screen.getByLabelText('Name')).toBeInTheDocument();
    expect(screen.getByLabelText('Email address')).toBeInTheDocument();
    expect(screen.getByLabelText('Password')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sign up' })).toBeInTheDocument();
    expect(screen.getByText('Already a member?')).toBeInTheDocument();
    expect(screen.getByText('Log in')).toBeInTheDocument();
    
    // Check that the signup options are rendered in 'both' mode
    expect(screen.getByLabelText('Sign up for AI Portal')).toBeInTheDocument();
    expect(screen.getByLabelText('Sign up for AI Chats')).toBeInTheDocument();
  });

  test('renders form with portal mode only', () => {
    getConfig.mockReturnValue({
      DEFAULT_SIGNUP_MODE: 'portal'
    });

    render(<Register />, { wrapper: Wrapper });

    // Signup options should not be visible in portal-only mode
    expect(screen.queryByLabelText('Sign up for AI Portal')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Sign up for AI Chats')).not.toBeInTheDocument();
  });

  test('renders form with chat mode only', () => {
    getConfig.mockReturnValue({
      DEFAULT_SIGNUP_MODE: 'chat'
    });

    render(<Register />, { wrapper: Wrapper });

    // Signup options should not be visible in chat-only mode
    expect(screen.queryByLabelText('Sign up for AI Portal')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Sign up for AI Chats')).not.toBeInTheDocument();
  });

  test('displays password criteria when password field is focused', () => {
    render(<Register />, { wrapper: Wrapper });

    // Focus the password field
    fireEvent.focus(screen.getByLabelText('Password'));

    // Check that password criteria are displayed
    expect(screen.getByText('At least 8 characters')).toBeInTheDocument();
    expect(screen.getByText('Contains a number')).toBeInTheDocument();
    expect(screen.getByText('Contains a special character')).toBeInTheDocument();
    expect(screen.getByText('Contains an uppercase letter')).toBeInTheDocument();
  });

  test('validates password criteria correctly', async () => {
    render(<Register />, { wrapper: Wrapper });

    // Focus the password field
    const passwordField = screen.getByLabelText('Password');
    fireEvent.focus(passwordField);

    // Test each password criteria
    
    // Empty password - all criteria should fail
    fireEvent.change(passwordField, { target: { value: '' } });
    expect(screen.getByText('At least 8 characters')).toBeInTheDocument();
    
    // Short password - length criteria should fail
    fireEvent.change(passwordField, { target: { value: 'Abc1!' } });
    
    // Password with all criteria met
    fireEvent.change(passwordField, { target: { value: 'Password123!' } });
    
    // Wait for the criteria to update
    await waitFor(() => {
      // All criteria should be met
      expect(screen.getByText('At least 8 characters')).toBeInTheDocument();
    });
    
    // Check each criteria separately
    expect(screen.getByText('At least 8 characters')).toHaveClass('MuiFormHelperText-root');
    expect(screen.getByText('Contains a number')).toHaveClass('MuiFormHelperText-root');
    expect(screen.getByText('Contains a special character')).toHaveClass('MuiFormHelperText-root');
    expect(screen.getByText('Contains an uppercase letter')).toHaveClass('MuiFormHelperText-root');
  });

  test('handles form submission with valid data', async () => {
    // Mock successful registration response
    pubClient.post.mockResolvedValue({
      data: {
        message: 'User registered successfully'
      }
    });

    render(<Register />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'Test User' }
    });
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'test@example.com' }
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'Password123!' }
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Sign up' }));

    // Wait for the registration process to complete
    await waitFor(() => {
      expect(pubClient.post).toHaveBeenCalledWith('/auth/register', {
        data: {
          type: 'register',
          attributes: {
            name: 'Test User',
            email: 'test@example.com',
            password: 'Password123!',
            with_portal: true,
            with_chat: true,
          }
        }
      });
    });

    // Check that the navigate function was called with the correct path
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/login');
    });
  });

  test('prevents submission when password criteria are not met', async () => {
    render(<Register />, { wrapper: Wrapper });

    // Fill in the form with invalid password
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'Test User' }
    });
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'test@example.com' }
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'password' } // Missing uppercase, number, and special character
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Sign up' }));

    // Check that an error message is displayed
    await waitFor(() => {
      expect(screen.getByText('Please ensure all password criteria are met.')).toBeInTheDocument();
    });

    // Check that the API was not called
    expect(pubClient.post).not.toHaveBeenCalled();
  });

  test('prevents submission when no signup option is selected in "both" mode', async () => {
    render(<Register />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'Test User' }
    });
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'test@example.com' }
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'Password123!' }
    });

    // Uncheck both options
    fireEvent.click(screen.getByLabelText('Sign up for AI Portal'));
    fireEvent.click(screen.getByLabelText('Sign up for AI Chats'));

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Sign up' }));

    // Check that an error message is displayed
    await waitFor(() => {
      expect(screen.getByText('Please select at least one option (Portal or Chat)')).toBeInTheDocument();
    });

    // Check that the API was not called
    expect(pubClient.post).not.toHaveBeenCalled();
  });

  test('handles registration error with 400 status', async () => {
    // Mock failed registration response with 400 status
    pubClient.post.mockRejectedValue({
      response: {
        status: 400,
        data: {
          errors: [
            { detail: 'Invalid email format' }
          ]
        }
      }
    });

    render(<Register />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'Test User' }
    });
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'invalid-email' }
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'Password123!' }
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Sign up' }));

    // Wait for the error message to appear
    await waitFor(() => {
      expect(screen.getByText('Invalid email format')).toBeInTheDocument();
    });

    // Check that the user is not redirected
    expect(window.location.href).toBe('');
  });

  test('handles registration error with 409 status (email already exists)', async () => {
    // Mock failed registration response with 409 status
    pubClient.post.mockRejectedValue({
      response: {
        status: 409
      }
    });

    render(<Register />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'Test User' }
    });
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'existing@example.com' }
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'Password123!' }
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Sign up' }));

    // Wait for the error message to appear
    await waitFor(() => {
      expect(screen.getByText('An account with this email already exists.')).toBeInTheDocument();
    });

    // Check that the user is not redirected
    expect(window.location.href).toBe('');
  });

  test('handles unexpected registration error', async () => {
    // Mock failed registration response with unexpected error
    pubClient.post.mockRejectedValue({
      response: {
        status: 500
      }
    });

    render(<Register />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'Test User' }
    });
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'test@example.com' }
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'Password123!' }
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Sign up' }));

    // Wait for the error message to appear
    await waitFor(() => {
      expect(screen.getByText('An unexpected error occurred. Please try again.')).toBeInTheDocument();
    });

    // Check that the user is not redirected
    expect(window.location.href).toBe('');
  });

  test('handles network error during registration', async () => {
    // Mock network error during registration
    pubClient.post.mockRejectedValue(new Error('Network error'));

    render(<Register />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'Test User' }
    });
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'test@example.com' }
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'Password123!' }
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Sign up' }));

    // Wait for the error message to appear
    await waitFor(() => {
      expect(screen.getByText('An unexpected error occurred. Please try again.')).toBeInTheDocument();
    });

    // Check that the user is not redirected
    expect(window.location.href).toBe('');
  });

  test('navigates to login page when login link is clicked', () => {
    render(<Register />, { wrapper: Wrapper });

    // Click the login link
    fireEvent.click(screen.getByText('Log in'));

    // Check that the user is navigated to the login page
    expect(screen.getByText('Login Page')).toBeInTheDocument();
  });
});