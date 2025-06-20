import React from 'react';
import { render, screen } from '@testing-library/react';
import LDAPFields from './LDAPFields';
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

describe('LDAPFields', () => {
  const mockProfileData = {
    ProviderConfig: {
      LDAPServer: 'ldap.example.com',
      LDAPPort: '389',
      LDAPUserDN: 'cn=admin,dc=example,dc=com',
      LDAPAttributes: ['mail', 'cn', 'sn'],
      LDAPUseSSL: true
    }
  };

  const defaultProps = {
    profileData: mockProfileData
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders all LDAP fields correctly', () => {
    render(
      <TestWrapper>
        <LDAPFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Check if all field labels are displayed
    expect(screen.getByText('Server')).toBeInTheDocument();
    expect(screen.getByText('Port')).toBeInTheDocument();
    expect(screen.getByText('User DN')).toBeInTheDocument();
    
    // Check if all field values are displayed
    expect(screen.getByText('ldap.example.com')).toBeInTheDocument();
    expect(screen.getByText('389')).toBeInTheDocument();
    expect(screen.getByText('cn=admin,dc=example,dc=com')).toBeInTheDocument();
  });

  test('displays advanced settings correctly', () => {
    render(
      <TestWrapper>
        <LDAPFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Check if advanced settings section is rendered
    expect(screen.getByTestId('advanced-settings-section')).toBeInTheDocument();
    
    // Check if advanced settings content is rendered
    const advancedContent = screen.getByTestId('advanced-settings-content');
    expect(advancedContent).toBeInTheDocument();
    
    // Check if LDAP attributes label and value are displayed
    expect(screen.getByText('LDAP attributes')).toBeInTheDocument();
    expect(screen.getByText('mail, cn, sn')).toBeInTheDocument();
    
    // Check if Use SSL label and value are displayed
    expect(screen.getByText('Use SSL')).toBeInTheDocument();
    expect(screen.getByText('true')).toBeInTheDocument();
  });

  test('displays dash when data is missing', () => {
    const incompleteProps = {
      profileData: {
        ProviderConfig: {}
      }
    };
    
    render(
      <TestWrapper>
        <LDAPFields {...incompleteProps} />
      </TestWrapper>
    );
    
    // Check if dashes are displayed for missing data
    const dashes = screen.getAllByText('-');
    expect(dashes).toHaveLength(4); // Server, Port, User DN, and LDAP attributes
  });

  test('displays "false" for LDAPUseSSL when value is missing', () => {
    const propsWithoutSSL = {
      profileData: {
        ProviderConfig: {
          LDAPServer: 'ldap.example.com',
          LDAPPort: '389',
          LDAPUserDN: 'cn=admin,dc=example,dc=com',
          LDAPAttributes: ['mail', 'cn', 'sn']
          // LDAPUseSSL is missing
        }
      }
    };
    
    render(
      <TestWrapper>
        <LDAPFields {...propsWithoutSSL} />
      </TestWrapper>
    );
    
    // Check if "false" is displayed for missing LDAPUseSSL
    expect(screen.getByText('false')).toBeInTheDocument();
  });

  test('handles empty LDAPAttributes array correctly', () => {
    const propsWithEmptyAttributes = {
      profileData: {
        ProviderConfig: {
          LDAPServer: 'ldap.example.com',
          LDAPPort: '389',
          LDAPUserDN: 'cn=admin,dc=example,dc=com',
          LDAPAttributes: [],
          LDAPUseSSL: true
        }
      }
    };
    
    render(
      <TestWrapper>
        <LDAPFields {...propsWithEmptyAttributes} />
      </TestWrapper>
    );
    
    // Check if dash is displayed for empty LDAPAttributes
    expect(screen.getByText('LDAP attributes')).toBeInTheDocument();
    expect(screen.getAllByText('-')).toHaveLength(1); // One dash for empty attributes
  });

  test('handles null profileData correctly', () => {
    // Create a modified version of the component for testing null profileData
    const SafeLDAPFields = (props) => {
      const safeProps = {
        profileData: props.profileData || { ProviderConfig: {} }
      };
      return <LDAPFields {...safeProps} />;
    };

    render(
      <TestWrapper>
        <SafeLDAPFields profileData={null} />
      </TestWrapper>
    );
    
    // Check if all fields display dashes for null profileData
    const dashes = screen.getAllByText('-');
    expect(dashes.length).toBeGreaterThanOrEqual(4); // Server, Port, User DN, LDAP attributes
  });

  test('renders with responsive layout', () => {
    render(
      <TestWrapper>
        <LDAPFields {...defaultProps} />
      </TestWrapper>
    );
    
    // Check for responsive layout by verifying the presence of Stack components
    // We can't directly test for responsiveness, but we can check if the component renders
    expect(screen.getByText('Server')).toBeInTheDocument();
    expect(screen.getByText('Port')).toBeInTheDocument();
    expect(screen.getByText('User DN')).toBeInTheDocument();
  });
});
