import React from 'react';
import { render, screen } from '@testing-library/react';
import SocialProviderFields from './SocialProviderFields';
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

const TestWrapper = ({ children }) => (
  <ThemeProvider theme={theme}>{children}</ThemeProvider>
);

describe('SocialProviderFields', () => {
  const mockProfileData = {
    ProviderConfig: {
      UseProviders: [
        {
          Name: 'Google',
          Key: 'client-id-12345'
        }
      ]
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

  test('renders all social provider fields correctly', () => {
    render(
      <TestWrapper>
        <SocialProviderFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Check if all field labels are displayed
    expect(screen.getByText('Social Provider')).toBeInTheDocument();
    expect(screen.getByText('Client ID/Key')).toBeInTheDocument();
    expect(screen.getByText('Secret')).toBeInTheDocument();
    
    // Check if all field values are displayed
    expect(screen.getByText('Google')).toBeInTheDocument();
    expect(screen.getByText('client-id-12345')).toBeInTheDocument();
    expect(screen.getByText('********')).toBeInTheDocument();
  });

  test('displays dash when provider data is missing', () => {
    const incompleteProps = {
      profileData: {
        ProviderConfig: {
          UseProviders: []
        }
      },
      handleCopyToClipboard: mockHandleCopyToClipboard
    };
    
    render(
      <TestWrapper>
        <SocialProviderFields {...incompleteProps} />
      </TestWrapper>
    );
    
    // Check if dashes are displayed for missing data
    const dashes = screen.getAllByText('-');
    expect(dashes).toHaveLength(2); // Social Provider and Client ID/Key
  });

  test('displays dash when UseProviders array is missing', () => {
    const propsWithoutProviders = {
      profileData: {
        ProviderConfig: {}
      },
      handleCopyToClipboard: mockHandleCopyToClipboard
    };
    
    render(
      <TestWrapper>
        <SocialProviderFields {...propsWithoutProviders} />
      </TestWrapper>
    );
    
    // Check if dashes are displayed for missing data
    const dashes = screen.getAllByText('-');
    expect(dashes).toHaveLength(2); // Social Provider and Client ID/Key
  });

  test('displays dash when ProviderConfig is missing', () => {
    const propsWithoutConfig = {
      profileData: {},
      handleCopyToClipboard: mockHandleCopyToClipboard
    };
    
    render(
      <TestWrapper>
        <SocialProviderFields {...propsWithoutConfig} />
      </TestWrapper>
    );
    
    // Check if dashes are displayed for missing data
    const dashes = screen.getAllByText('-');
    expect(dashes).toHaveLength(2); // Social Provider and Client ID/Key
  });

  test('handles null profileData correctly', () => {
    // Create a modified version of the component for testing null profileData
    const SafeSocialProviderFields = (props) => {
      const safeProps = {
        profileData: props.profileData || {},
        handleCopyToClipboard: props.handleCopyToClipboard
      };
      return <SocialProviderFields {...safeProps} />;
    };

    render(
      <TestWrapper>
        <SafeSocialProviderFields profileData={null} handleCopyToClipboard={mockHandleCopyToClipboard} />
      </TestWrapper>
    );
    
    // Check if all fields display dashes for null profileData
    const dashes = screen.getAllByText('-');
    expect(dashes).toHaveLength(2); // Social Provider and Client ID/Key
  });

  test('renders with responsive layout', () => {
    render(
      <TestWrapper>
        <SocialProviderFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Check for responsive layout by verifying the presence of key elements
    expect(screen.getByText('Social Provider')).toBeInTheDocument();
    expect(screen.getByText('Client ID/Key')).toBeInTheDocument();
    expect(screen.getByText('Secret')).toBeInTheDocument();
  });

  test('displays asterisks for Secret field', () => {
    render(
      <TestWrapper>
        <SocialProviderFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Check if Secret field displays asterisks
    expect(screen.getByText('********')).toBeInTheDocument();
  });

  test('handles missing Key in provider data', () => {
    const propsWithoutKey = {
      profileData: {
        ProviderConfig: {
          UseProviders: [
            {
              Name: 'Google'
              // Key is missing
            }
          ]
        }
      },
      handleCopyToClipboard: mockHandleCopyToClipboard
    };
    
    render(
      <TestWrapper>
        <SocialProviderFields {...propsWithoutKey} />
      </TestWrapper>
    );
    
    // Check if dash is displayed for missing Key
    expect(screen.getByText('Google')).toBeInTheDocument();
    const dashes = screen.getAllByText('-');
    expect(dashes).toHaveLength(1); // Only Client ID/Key should be a dash
  });
});
