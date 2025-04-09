import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import { createTheme } from '@mui/material';
import ForgotPassword from './ForgotPassword';

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
  window.location = {
    href: '',
    pathname: '/forgot-password'
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

describe('ForgotPassword Component', () => {
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
      <MemoryRouter initialEntries={['/forgot-password']}>
        <Routes>
          <Route path="/forgot-password" element={children} />
          <Route path="/login" element={<div>Login Page</div>} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

  let pubClient;

  beforeEach(() => {
    jest.clearAllMocks();
    window.location.href = '';
    pubClient = require('../../admin/utils/pubClient').default;
    pubClient.post.mockReset();
  });

  test('renders forgot password form correctly', () => {
    render(<ForgotPassword />, { wrapper: Wrapper });

    // Check that the component renders with the correct content
    expect(screen.getByText('Password reset')).toBeInTheDocument();
    expect(screen.getByText('We will send you a link you can use to reset your password securely')).toBeInTheDocument();
    expect(screen.getByLabelText('Email address')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Reset password' })).toBeInTheDocument();
    expect(screen.getByText('Return to Login')).toBeInTheDocument();
  });

  test('handles form submission with valid email', async () => {
    // Mock successful API response
    pubClient.post.mockResolvedValue({
      data: {
        message: 'Password reset email sent'
      }
    });

    render(<ForgotPassword />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'test@example.com' }
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Reset password' }));

    // Wait for the API call to complete
    await waitFor(() => {
      expect(pubClient.post).toHaveBeenCalledWith('/auth/forgot-password', {
        data: {
          type: 'forgot-password',
          attributes: { email: 'test@example.com' },
        },
      });
    });

    // Wait for the component to update and show success state
    await waitFor(() => {
      expect(screen.getByText(/Password reset request sent/i)).toBeInTheDocument();
    });
    
    // Check that the success message is displayed
    expect(screen.getByText(/Success! If we can find an account/i)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Go to Login' })).toBeInTheDocument();
  });

  test('displays error message when API call fails', async () => {
    // Mock failed API response
    pubClient.post.mockRejectedValue(new Error('API error'));

    render(<ForgotPassword />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'test@example.com' }
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Reset password' }));

    // Wait for the error message to appear
    await waitFor(() => {
      expect(screen.getByText('Failed to send reset password email. Please try again.')).toBeInTheDocument();
    });

    // Check that we're still on the forgot password page (not showing success state)
    expect(screen.getByRole('button', { name: 'Reset password' })).toBeInTheDocument();
  });

  test('navigates to login page when "Return to Login" link is clicked', () => {
    render(<ForgotPassword />, { wrapper: Wrapper });

    // Click the "Return to Login" link
    fireEvent.click(screen.getByText('Return to Login'));

    // Check that we're navigated to the login page
    expect(screen.getByText('Login Page')).toBeInTheDocument();
  });

  test('navigates to login page from success state when "Go to Login" button is clicked', async () => {
    // Mock successful API response
    pubClient.post.mockResolvedValue({
      data: {
        message: 'Password reset email sent'
      }
    });

    render(<ForgotPassword />, { wrapper: Wrapper });

    // Fill in the form
    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: 'test@example.com' }
    });

    // Submit the form
    fireEvent.click(screen.getByRole('button', { name: 'Reset password' }));

    // Wait for the success state to appear
    await waitFor(() => {
      expect(screen.getByText('Password reset request sent')).toBeInTheDocument();
    });

    // Click the "Go to Login" button
    fireEvent.click(screen.getByRole('link', { name: 'Go to Login' }));

    // Check that we're navigated to the login page
    expect(screen.getByText('Login Page')).toBeInTheDocument();
  });

  test('email field has required attribute', () => {
    render(<ForgotPassword />, { wrapper: Wrapper });
    
    // Check that the email input has the required attribute
    const emailInput = screen.getByLabelText('Email address');
    expect(emailInput).toHaveAttribute('required');
  });

  test('allows submitting the form with Enter key', async () => {
    // Mock successful API response
    pubClient.post.mockResolvedValue({
      data: {
        message: 'Password reset email sent'
      }
    });

    render(<ForgotPassword />, { wrapper: Wrapper });

    // Fill in the form
    const emailInput = screen.getByLabelText('Email address');
    fireEvent.change(emailInput, {
      target: { value: 'test@example.com' }
    });

    // Find the form by querying for a form that contains the email input
    const form = screen.getByRole('button', { name: 'Reset password' }).form;
    
    // Submit the form directly
    fireEvent.submit(form);

    // Wait for the API call to complete
    await waitFor(() => {
      expect(pubClient.post).toHaveBeenCalledWith('/auth/forgot-password', {
        data: {
          type: 'forgot-password',
          attributes: { email: 'test@example.com' },
        },
      });
    });

    // Wait for the component to update and show success state
    await waitFor(() => {
      expect(screen.getByText(/Password reset request sent/i)).toBeInTheDocument();
    });
  });
});