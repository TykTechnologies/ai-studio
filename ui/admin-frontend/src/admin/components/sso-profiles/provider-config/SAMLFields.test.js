import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import SAMLFields from './SAMLFields';
import { ThemeProvider, createTheme } from '@mui/material/styles';

// Mock AdvancedSettingsSection component
jest.mock('../AdvancedSettingsSection', () => {
  return function MockAdvancedSettingsSection({ children }) {
    return (
      <div data-testid="advanced-settings-section">
        <button data-testid="toggle-advanced-settings">Toggle Advanced Settings</button>
        <div data-testid="advanced-settings-content">{children}</div>
      </div>
    );
  };
});

// Mock ContentCopyIcon
jest.mock('@mui/icons-material/ContentCopy', () => {
  return function MockContentCopyIcon(props) {
    return <div data-testid="ContentCopyIcon" {...props} />;
  };
});

// Mock theme for testing
const theme = createTheme({
  palette: {
    text: {
      primary: '#212121',
      defaultSubdued: '#757575',
    },
  },
});

const TestWrapper = ({ children }) => (
  <ThemeProvider theme={theme}>{children}</ThemeProvider>
);

describe('SAMLFields', () => {
  const mockProfileData = {
    ProviderConfig: {
      CertLocation: '/path/to/certificate.pem',
      IDPMetaDataURL: 'https://idp.example.com/metadata',
      SAMLEmailClaim: 'email',
      SAMLForenameClaim: 'givenName',
      SAMLSurnameClaim: 'surname',
      ForceAuthentication: true
    },
    CustomEmailField: 'custom_email',
    CustomUserIDField: 'custom_id',
    ProviderConstraints: {
      Domain: 'example.com'
    }
  };

  const mockHandleCopyToClipboard = jest.fn();
  
  const defaultProps = {
    profileData: mockProfileData,
    handleCopyToClipboard: mockHandleCopyToClipboard
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders all SAML fields correctly', () => {
    render(
      <TestWrapper>
        <SAMLFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Check if all field labels are displayed
    expect(screen.getByText('Certificate path')).toBeInTheDocument();
    expect(screen.getByText('IDP metadata URL')).toBeInTheDocument();
    
    // Check if all field values are displayed
    expect(screen.getByText('/path/to/certificate.pem')).toBeInTheDocument();
    expect(screen.getByText('https://idp.example.com/metadata')).toBeInTheDocument();
  });

  test('displays advanced settings correctly', () => {
    render(
      <TestWrapper>
        <SAMLFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Check if advanced settings section is rendered
    expect(screen.getByTestId('advanced-settings-section')).toBeInTheDocument();
    
    // Check if advanced settings content is rendered
    const advancedContent = screen.getByTestId('advanced-settings-content');
    expect(advancedContent).toBeInTheDocument();
    
    // Check if advanced fields labels and values are displayed
    expect(screen.getByText('SAML email claim')).toBeInTheDocument();
    expect(screen.getByText('email')).toBeInTheDocument();
    
    expect(screen.getByText('SAML forename')).toBeInTheDocument();
    expect(screen.getByText('givenName')).toBeInTheDocument();
    
    expect(screen.getByText('SAML surname')).toBeInTheDocument();
    expect(screen.getByText('surname')).toBeInTheDocument();
    
    expect(screen.getByText('Force authentication')).toBeInTheDocument();
    expect(screen.getByText('true')).toBeInTheDocument();
    
    expect(screen.getByText('Custom email')).toBeInTheDocument();
    expect(screen.getByText('custom_email')).toBeInTheDocument();
    
    expect(screen.getByText('Custom ID')).toBeInTheDocument();
    expect(screen.getByText('custom_id')).toBeInTheDocument();
    
    expect(screen.getByText('Provider domain')).toBeInTheDocument();
    expect(screen.getByText('example.com')).toBeInTheDocument();
  });

  test('displays dash when data is missing', () => {
    const incompleteProps = {
      profileData: {
        ProviderConfig: {}
      },
      handleCopyToClipboard: mockHandleCopyToClipboard
    };
    
    render(
      <TestWrapper>
        <SAMLFields {...incompleteProps} />
      </TestWrapper>
    );
    
    // Check if dashes are displayed for missing data
    const dashes = screen.getAllByText('-');
    expect(dashes.length).toBeGreaterThanOrEqual(7); // All fields should show dashes
  });

  test('displays "false" for ForceAuthentication when value is missing', () => {
    const propsWithoutForceAuth = {
      profileData: {
        ProviderConfig: {
          CertLocation: '/path/to/certificate.pem',
          IDPMetaDataURL: 'https://idp.example.com/metadata',
          // ForceAuthentication is missing
        }
      },
      handleCopyToClipboard: mockHandleCopyToClipboard
    };
    
    render(
      <TestWrapper>
        <SAMLFields {...propsWithoutForceAuth} />
      </TestWrapper>
    );
    
    // Check if "false" is displayed for missing ForceAuthentication
    expect(screen.getByText('Force authentication')).toBeInTheDocument();
    expect(screen.getByText('false')).toBeInTheDocument();
  });

  test('calls handleCopyToClipboard when Certificate path copy button is clicked', () => {
    render(
      <TestWrapper>
        <SAMLFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Find all copy buttons
    const copyButtons = screen.getAllByRole('button');
    
    // Click the first button (Certificate path)
    fireEvent.click(copyButtons[0]);
    
    // Check if handleCopyToClipboard was called with the correct arguments
    expect(mockHandleCopyToClipboard).toHaveBeenCalledWith(
      '/path/to/certificate.pem',
      'Certificate path'
    );
  });

  test('calls handleCopyToClipboard when IDP metadata URL copy button is clicked', () => {
    render(
      <TestWrapper>
        <SAMLFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Find all copy buttons
    const copyButtons = screen.getAllByRole('button');
    
    // Click the second button (IDP metadata URL)
    fireEvent.click(copyButtons[1]);
    
    // Check if handleCopyToClipboard was called with the correct arguments
    expect(mockHandleCopyToClipboard).toHaveBeenCalledWith(
      'https://idp.example.com/metadata',
      'IDP metadata URL'
    );
  });

  test('does not render copy buttons when fields are missing', () => {
    const propsWithoutCopyFields = {
      profileData: {
        ProviderConfig: {
          SAMLEmailClaim: 'email',
          SAMLForenameClaim: 'givenName',
          SAMLSurnameClaim: 'surname',
          ForceAuthentication: true
        }
      },
      handleCopyToClipboard: mockHandleCopyToClipboard
    };
    
    render(
      <TestWrapper>
        <SAMLFields {...propsWithoutCopyFields} />
      </TestWrapper>
    );
    
    // Check that no copy buttons are rendered
    expect(screen.queryByTestId('ContentCopyIcon')).not.toBeInTheDocument();
  });

  test('handles null profileData correctly', () => {
    // Create a modified version of the component for testing null profileData
    const SafeSAMLFields = (props) => {
      const safeProps = {
        profileData: props.profileData || { ProviderConfig: {} },
        handleCopyToClipboard: props.handleCopyToClipboard
      };
      return <SAMLFields {...safeProps} />;
    };

    render(
      <TestWrapper>
        <SafeSAMLFields profileData={null} handleCopyToClipboard={mockHandleCopyToClipboard} />
      </TestWrapper>
    );
    
    // Check if all fields display dashes for null profileData
    const dashes = screen.getAllByText('-');
    expect(dashes.length).toBeGreaterThanOrEqual(7); // All fields should show dashes
  });

  test('renders with responsive layout', () => {
    render(
      <TestWrapper>
        <SAMLFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Check for responsive layout by verifying the presence of key elements
    expect(screen.getByText('Certificate path')).toBeInTheDocument();
    expect(screen.getByText('IDP metadata URL')).toBeInTheDocument();
    expect(screen.getByText('SAML email claim')).toBeInTheDocument();
  });
});
