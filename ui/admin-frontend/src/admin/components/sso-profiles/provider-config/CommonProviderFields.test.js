import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import CommonProviderFields from './CommonProviderFields';
import { ThemeProvider, createTheme } from '@mui/material/styles';

// Mock theme for testing
const theme = createTheme({
  palette: {
    text: {
      primary: '#212121',
      defaultSubdued: '#757575',
    },
  },
});

// Mock ContentCopyIcon
jest.mock('@mui/icons-material/ContentCopy', () => {
  return function MockContentCopyIcon(props) {
    return <div data-testid="ContentCopyIcon" {...props} />;
  };
});

const TestWrapper = ({ children }) => (
  <ThemeProvider theme={theme}>{children}</ThemeProvider>
);

describe('CommonProviderFields', () => {
  const mockProfileData = {
    ProviderConfig: {
      CallbackBaseURL: 'https://example.com/callback-base',
      SAMLBaseURL: 'https://example.com/saml-base',
    }
  };
  
  const mockProfileMetadata = {
    loginUrl: 'https://example.com/login',
    callbackUrl: 'https://example.com/callback',
  };
  
  const mockHandleCopyToClipboard = jest.fn();
  
  const defaultProps = {
    profileData: mockProfileData,
    profileMetadata: mockProfileMetadata,
    handleCopyToClipboard: mockHandleCopyToClipboard,
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders all provider fields correctly', () => {
    render(
      <TestWrapper>
        <CommonProviderFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Check if all field labels are displayed
    expect(screen.getByText('Login URL')).toBeInTheDocument();
    expect(screen.getByText('Callback URL')).toBeInTheDocument();
    expect(screen.getByText('Access URL')).toBeInTheDocument();
    
    // Check if all field values are displayed
    expect(screen.getByText('https://example.com/login')).toBeInTheDocument();
    expect(screen.getByText('https://example.com/callback')).toBeInTheDocument();
    expect(screen.getByText('https://example.com/callback-base')).toBeInTheDocument();
  });

  test('displays dash when data is missing', () => {
    const incompleteProps = {
      profileData: {
        ProviderConfig: {}
      },
      profileMetadata: {},
      handleCopyToClipboard: mockHandleCopyToClipboard,
    };
    
    render(
      <TestWrapper>
        <CommonProviderFields {...incompleteProps} />
      </TestWrapper>
    );
    
    // Check if dashes are displayed for missing data
    expect(screen.getAllByText('-')).toHaveLength(3); // Login URL, Callback URL, Access URL
  });

  test('prefers CallbackBaseURL over SAMLBaseURL for Access URL', () => {
    render(
      <TestWrapper>
        <CommonProviderFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Should display CallbackBaseURL and not SAMLBaseURL
    expect(screen.getByText('https://example.com/callback-base')).toBeInTheDocument();
    expect(screen.queryByText('https://example.com/saml-base')).not.toBeInTheDocument();
  });

  test('falls back to SAMLBaseURL when CallbackBaseURL is missing', () => {
    const propsWithoutCallbackBaseURL = {
      ...defaultProps,
      profileData: {
        ProviderConfig: {
          SAMLBaseURL: 'https://example.com/saml-base',
        }
      }
    };
    
    render(
      <TestWrapper>
        <CommonProviderFields {...propsWithoutCallbackBaseURL} />
      </TestWrapper>
    );
    
    // Should display SAMLBaseURL
    expect(screen.getByText('https://example.com/saml-base')).toBeInTheDocument();
  });

  test('calls handleCopyToClipboard when Login URL copy button is clicked', () => {
    render(
      <TestWrapper>
        <CommonProviderFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Find all copy buttons
    const copyButtons = screen.getAllByRole('button');
    
    // Click the first button (Login URL)
    fireEvent.click(copyButtons[0]);
    
    // Check if handleCopyToClipboard was called with the correct arguments
    expect(mockHandleCopyToClipboard).toHaveBeenCalledWith(
      'https://example.com/login',
      'Login URL'
    );
  });

  test('calls handleCopyToClipboard when Callback URL copy button is clicked', () => {
    render(
      <TestWrapper>
        <CommonProviderFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Find all copy buttons
    const copyButtons = screen.getAllByRole('button');
    
    // Click the second button (Callback URL)
    fireEvent.click(copyButtons[1]);
    
    // Check if handleCopyToClipboard was called with the correct arguments
    expect(mockHandleCopyToClipboard).toHaveBeenCalledWith(
      'https://example.com/callback',
      'Callback URL'
    );
  });

  test('calls handleCopyToClipboard when Access URL copy button is clicked', () => {
    render(
      <TestWrapper>
        <CommonProviderFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Find all copy buttons
    const copyButtons = screen.getAllByRole('button');
    
    // Click the third button (Access URL)
    fireEvent.click(copyButtons[2]);
    
    // Check if handleCopyToClipboard was called with the correct arguments
    expect(mockHandleCopyToClipboard).toHaveBeenCalledWith(
      'https://example.com/callback-base',
      'Access URL'
    );
  });

  test('does not render copy buttons when URLs are missing', () => {
    const propsWithoutUrls = {
      profileData: {
        ProviderConfig: {}
      },
      profileMetadata: {},
      handleCopyToClipboard: mockHandleCopyToClipboard,
    };
    
    render(
      <TestWrapper>
        <CommonProviderFields {...propsWithoutUrls} />
      </TestWrapper>
    );
    
    // Check that no copy buttons are rendered
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  test('renders with responsive layout', () => {
    render(
      <TestWrapper>
        <CommonProviderFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Check that all sections are present
    expect(screen.getByText('Login URL')).toBeInTheDocument();
    expect(screen.getByText('Callback URL')).toBeInTheDocument();
    expect(screen.getByText('Access URL')).toBeInTheDocument();
    
    // Check that the values are displayed
    expect(screen.getByText('https://example.com/login')).toBeInTheDocument();
    expect(screen.getByText('https://example.com/callback')).toBeInTheDocument();
    expect(screen.getByText('https://example.com/callback-base')).toBeInTheDocument();
  });
});
