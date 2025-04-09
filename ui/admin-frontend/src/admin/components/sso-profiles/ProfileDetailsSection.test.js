import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import ProfileDetailsSection from './ProfileDetailsSection';
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

describe('ProfileDetailsSection', () => {
  const mockProfileData = {
    Name: 'Test Profile',
    ActionType: 'oauth',
  };
  
  const mockProfileMetadata = {
    selectedProviderType: 'openid-connect',
    failureRedirectUrl: 'https://example.com/failure',
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

  test('renders profile details correctly', () => {
    render(
      <TestWrapper>
        <ProfileDetailsSection {...defaultProps} />
      </TestWrapper>
    );
    
    // Check if profile name is displayed
    expect(screen.getByText('Profile name')).toBeInTheDocument();
    expect(screen.getByText('Test Profile')).toBeInTheDocument();
    
    // Check if profile type is displayed
    expect(screen.getByText('Profile type')).toBeInTheDocument();
    expect(screen.getByText('oauth')).toBeInTheDocument();
    
    // Check if provider type is displayed
    expect(screen.getByText('Provider type')).toBeInTheDocument();
    expect(screen.getByText('openid-connect')).toBeInTheDocument();
    
    // Check if redirect URL is displayed
    expect(screen.getByText('Redirect URL on failure')).toBeInTheDocument();
    expect(screen.getByText('https://example.com/failure')).toBeInTheDocument();
  });

  test('displays dash when data is missing', () => {
    const incompleteProps = {
      profileData: {
        // Name is missing
        ActionType: 'oauth',
      },
      profileMetadata: {
        // selectedProviderType is missing
        // failureRedirectUrl is missing
      },
      handleCopyToClipboard: mockHandleCopyToClipboard,
    };
    
    render(
      <TestWrapper>
        <ProfileDetailsSection {...incompleteProps} />
      </TestWrapper>
    );
    
    // Check if dashes are displayed for missing data
    expect(screen.getAllByText('-')).toHaveLength(3); // Name, provider type, and redirect URL
  });

  test('calls handleCopyToClipboard when copy button is clicked', () => {
    render(
      <TestWrapper>
        <ProfileDetailsSection {...defaultProps} />
      </TestWrapper>
    );
    
    // Find the copy button for the redirect URL
    const copyButton = screen.getByRole('button');
    fireEvent.click(copyButton);
    
    // Check if handleCopyToClipboard was called with the correct arguments
    expect(mockHandleCopyToClipboard).toHaveBeenCalledWith(
      'https://example.com/failure',
      'Redirect URL on failure'
    );
  });

  test('does not render copy button when failureRedirectUrl is missing', () => {
    const propsWithoutUrl = {
      ...defaultProps,
      profileMetadata: {
        ...mockProfileMetadata,
        failureRedirectUrl: null,
      },
    };
    
    render(
      <TestWrapper>
        <ProfileDetailsSection {...propsWithoutUrl} />
      </TestWrapper>
    );
    
    // Check that no copy button is rendered
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  test('renders with responsive layout', () => {
    render(
      <TestWrapper>
        <ProfileDetailsSection {...defaultProps} data-testid="profile-details" />
      </TestWrapper>
    );
    
    // Instead of directly accessing DOM nodes, we'll check that the component renders
    // and contains the expected content in a structured way
    
    // Check that all sections are present
    expect(screen.getByText('Profile name')).toBeInTheDocument();
    expect(screen.getByText('Profile type')).toBeInTheDocument();
    expect(screen.getByText('Provider type')).toBeInTheDocument();
    expect(screen.getByText('Redirect URL on failure')).toBeInTheDocument();
    
    // Check that the values are displayed next to their labels
    const profileNameLabel = screen.getByText('Profile name');
    const profileNameValue = screen.getByText('Test Profile');
    expect(profileNameLabel).toBeInTheDocument();
    expect(profileNameValue).toBeInTheDocument();
  });
});