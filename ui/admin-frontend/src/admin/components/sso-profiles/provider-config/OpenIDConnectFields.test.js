import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import OpenIDConnectFields from './OpenIDConnectFields';
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

// Mock AdvancedSettingsSection
jest.mock('../AdvancedSettingsSection', () => {
  return function MockAdvancedSettingsSection({ children }) {
    return <div data-testid="advanced-settings-section">{children}</div>;
  };
});

const TestWrapper = ({ children }) => (
  <ThemeProvider theme={theme}>{children}</ThemeProvider>
);

describe('OpenIDConnectFields', () => {
  const mockHandleCopyToClipboard = jest.fn();
  
  // Complete profile data with all fields
  const completeProfileData = {
    ProviderConfig: {
      UseProviders: [
        {
          Key: 'test-client-id',
          Secret: 'test-secret',
          DiscoverURL: 'https://example.com/.well-known',
          SkipUserInfoRequest: true,
          Scopes: ['openid', 'profile', 'email']
        }
      ]
    },
    CustomEmailField: 'custom-email',
    CustomUserIDField: 'custom-id'
  };

  // Partial profile data with some fields missing
  const partialProfileData = {
    ProviderConfig: {
      UseProviders: [
        {
          Key: 'test-client-id',
          Secret: 'test-secret'
        }
      ]
    }
  };

  // Minimal profile data with empty provider config
  const minimalProfileData = {
    ProviderConfig: {
      UseProviders: []
    }
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders with complete profile data', () => {
    render(
      <TestWrapper>
        <OpenIDConnectFields 
          profileData={completeProfileData}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // Check that all fields are rendered with correct values
    expect(screen.getByText('Client ID/Key')).toBeInTheDocument();
    expect(screen.getByText('test-client-id')).toBeInTheDocument();
    
    expect(screen.getByText('Secret')).toBeInTheDocument();
    expect(screen.getByText('********')).toBeInTheDocument();
    
    expect(screen.getByText('Discover URL (well known endpoint)')).toBeInTheDocument();
    expect(screen.getByText('https://example.com/.well-known')).toBeInTheDocument();
    
    expect(screen.getByText('Custom email')).toBeInTheDocument();
    expect(screen.getByText('custom-email')).toBeInTheDocument();
    
    expect(screen.getByText('Custom ID')).toBeInTheDocument();
    expect(screen.getByText('custom-id')).toBeInTheDocument();
    
    expect(screen.getByText('Skip user info request')).toBeInTheDocument();
    expect(screen.getByText('true')).toBeInTheDocument();
    
    expect(screen.getByText('Scopes')).toBeInTheDocument();
    expect(screen.getByText('openid, profile, email')).toBeInTheDocument();
  });

  test('renders with partial profile data', () => {
    render(
      <TestWrapper>
        <OpenIDConnectFields 
          profileData={partialProfileData}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // Check that fields with values are rendered correctly
    expect(screen.getByText('Client ID/Key')).toBeInTheDocument();
    expect(screen.getByText('test-client-id')).toBeInTheDocument();
    
    expect(screen.getByText('Secret')).toBeInTheDocument();
    expect(screen.getByText('********')).toBeInTheDocument();
    
    // Check that fields without values show fallback
    expect(screen.getByText('Discover URL (well known endpoint)')).toBeInTheDocument();
    
    // Check that dash characters are shown for missing values
    const dashElements = screen.getAllByText('-');
    expect(dashElements.length).toBeGreaterThan(0);
    
    expect(screen.getByText('Custom email')).toBeInTheDocument();
    
    expect(screen.getByText('Custom ID')).toBeInTheDocument();
    
    expect(screen.getByText('Skip user info request')).toBeInTheDocument();
    expect(screen.getByText('false')).toBeInTheDocument();
    
    expect(screen.getByText('Scopes')).toBeInTheDocument();
  });

  test('renders with minimal profile data', () => {
    render(
      <TestWrapper>
        <OpenIDConnectFields 
          profileData={minimalProfileData}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // Check that all fields are rendered with fallback values
    expect(screen.getByText('Client ID/Key')).toBeInTheDocument();
    expect(screen.getAllByText('-').length).toBeGreaterThan(0);
    
    expect(screen.getByText('Secret')).toBeInTheDocument();
    expect(screen.getByText('********')).toBeInTheDocument();
    
    expect(screen.getByText('Discover URL (well known endpoint)')).toBeInTheDocument();
    
    expect(screen.getByText('Custom email')).toBeInTheDocument();
    
    expect(screen.getByText('Custom ID')).toBeInTheDocument();
    
    expect(screen.getByText('Skip user info request')).toBeInTheDocument();
    expect(screen.getByText('false')).toBeInTheDocument();
    
    expect(screen.getByText('Scopes')).toBeInTheDocument();
  });

  test('calls handleCopyToClipboard when copy button is clicked for Client ID', () => {
    render(
      <TestWrapper>
        <OpenIDConnectFields
          profileData={completeProfileData}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // Find the Client ID copy button by its position near the text
    const clientIdText = screen.getByText('test-client-id');
    const clientIdCopyButton = screen.getAllByRole('button')[0];
    
    // Click the button
    fireEvent.click(clientIdCopyButton);
    
    // Check that handleCopyToClipboard was called with correct arguments
    expect(mockHandleCopyToClipboard).toHaveBeenCalledWith('test-client-id', 'Client ID/Key');
  });

  test('calls handleCopyToClipboard when copy button is clicked for Discover URL', () => {
    render(
      <TestWrapper>
        <OpenIDConnectFields
          profileData={completeProfileData}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // Find the Discover URL copy button by its position near the text
    const discoverUrlText = screen.getByText('https://example.com/.well-known');
    const discoverUrlCopyButton = screen.getAllByRole('button')[1];
    
    // Click the button
    fireEvent.click(discoverUrlCopyButton);
    
    // Check that handleCopyToClipboard was called with correct arguments
    expect(mockHandleCopyToClipboard).toHaveBeenCalledWith('https://example.com/.well-known', 'Discover URL');
  });

  test('does not render copy buttons when values are missing', () => {
    render(
      <TestWrapper>
        <OpenIDConnectFields 
          profileData={minimalProfileData}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // No copy buttons should be rendered
    expect(screen.queryAllByTestId('ContentCopyIcon').length).toBe(0);
  });

  test('renders AdvancedSettingsSection with correct content', () => {
    render(
      <TestWrapper>
        <OpenIDConnectFields 
          profileData={completeProfileData}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // Check that AdvancedSettingsSection is rendered
    expect(screen.getByTestId('advanced-settings-section')).toBeInTheDocument();
    
    // Check that advanced settings content is within the section
    const advancedSection = screen.getByTestId('advanced-settings-section');
    expect(advancedSection).toContainElement(screen.getByText('Custom email'));
    expect(advancedSection).toContainElement(screen.getByText('Custom ID'));
    expect(advancedSection).toContainElement(screen.getByText('Skip user info request'));
    expect(advancedSection).toContainElement(screen.getByText('Scopes'));
  });

  test('applies correct styling to components', () => {
    render(
      <TestWrapper>
        <OpenIDConnectFields 
          profileData={completeProfileData}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // Check that labels have correct typography
    const labels = screen.getAllByText(/Client ID\/Key|Secret|Discover URL|Custom email|Custom ID|Skip user info request|Scopes/);
    labels.forEach(label => {
      expect(label).toHaveClass('MuiTypography-bodyLargeBold');
    });
    
    // Check that values have correct typography
    const values = screen.getAllByText(/test-client-id|https:\/\/example\.com\/\.well-known|custom-email|custom-id|true|openid, profile, email/);
    values.forEach(value => {
      expect(value).toHaveClass('MuiTypography-bodyLargeDefault');
    });
  });
});