import React from 'react';
import { render, screen, cleanup } from '@testing-library/react';
import ProviderConfigurationSection from './ProviderConfigurationSection';
import { ThemeProvider, createTheme } from '@mui/material/styles';

// Mock components with prop tracking
const mockCommonProviderFields = jest.fn().mockImplementation(() =>
  <div data-testid="common-provider-fields" />
);
const mockOpenIDConnectFields = jest.fn().mockImplementation(() =>
  <div data-testid="openid-connect-fields" />
);
const mockLDAPFields = jest.fn().mockImplementation(() =>
  <div data-testid="ldap-fields" />
);
const mockSocialProviderFields = jest.fn().mockImplementation(() =>
  <div data-testid="social-provider-fields" />
);
const mockSAMLFields = jest.fn().mockImplementation(() =>
  <div data-testid="saml-fields" />
);

// Mock the imported components
jest.mock('./provider-config/CommonProviderFields', () => ({
  __esModule: true,
  default: (props) => {
    mockCommonProviderFields(props);
    return <div data-testid="common-provider-fields" />;
  }
}));

jest.mock('./provider-config/OpenIDConnectFields', () => ({
  __esModule: true,
  default: (props) => {
    mockOpenIDConnectFields(props);
    return <div data-testid="openid-connect-fields" />;
  }
}));

jest.mock('./provider-config/LDAPFields', () => ({
  __esModule: true,
  default: (props) => {
    mockLDAPFields(props);
    return <div data-testid="ldap-fields" />;
  }
}));

jest.mock('./provider-config/SocialProviderFields', () => ({
  __esModule: true,
  default: (props) => {
    mockSocialProviderFields(props);
    return <div data-testid="social-provider-fields" />;
  }
}));

jest.mock('./provider-config/SAMLFields', () => ({
  __esModule: true,
  default: (props) => {
    mockSAMLFields(props);
    return <div data-testid="saml-fields" />;
  }
}));

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

describe('ProviderConfigurationSection', () => {
  const mockProfileData = {
    Name: 'Test Profile',
    ActionType: 'oauth',
    ProviderConfig: {
      CallbackBaseURL: 'https://example.com/callback',
      UseProviders: [
        {
          Key: 'test-key',
          Secret: 'test-secret',
          DiscoverURL: 'https://example.com/.well-known',
        },
      ],
    },
  };
  
  const mockHandleCopyToClipboard = jest.fn();
  
  beforeEach(() => {
    jest.clearAllMocks();
    cleanup(); // Clean up after each test
  });

  test('renders CommonProviderFields for all provider types', () => {
    // Test with OpenID Connect provider
    const openidProfileMetadata = {
      selectedProviderType: 'openid-connect',
      loginUrl: 'https://example.com/login',
      callbackUrl: 'https://example.com/callback',
    };
    
    render(
      <TestWrapper>
        <ProviderConfigurationSection 
          profileData={mockProfileData}
          profileMetadata={openidProfileMetadata}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // CommonProviderFields should always be rendered
    expect(screen.getByTestId('common-provider-fields')).toBeInTheDocument();
  });

  test('renders OpenIDConnectFields when provider type is openid-connect', () => {
    const openidProfileMetadata = {
      selectedProviderType: 'openid-connect',
      loginUrl: 'https://example.com/login',
      callbackUrl: 'https://example.com/callback',
    };
    
    render(
      <TestWrapper>
        <ProviderConfigurationSection 
          profileData={mockProfileData}
          profileMetadata={openidProfileMetadata}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // OpenIDConnectFields should be rendered
    expect(screen.getByTestId('openid-connect-fields')).toBeInTheDocument();
    
    // Other provider fields should not be rendered
    expect(screen.queryByTestId('ldap-fields')).not.toBeInTheDocument();
    expect(screen.queryByTestId('saml-fields')).not.toBeInTheDocument();
    expect(screen.queryByTestId('social-provider-fields')).not.toBeInTheDocument();
  });

  test('renders LDAPFields when provider type is ldap', () => {
    const ldapProfileMetadata = {
      selectedProviderType: 'ldap',
      loginUrl: 'https://example.com/login',
      callbackUrl: 'https://example.com/callback',
    };
    
    render(
      <TestWrapper>
        <ProviderConfigurationSection 
          profileData={mockProfileData}
          profileMetadata={ldapProfileMetadata}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // LDAPFields should be rendered
    expect(screen.getByTestId('ldap-fields')).toBeInTheDocument();
    
    // Other provider fields should not be rendered
    expect(screen.queryByTestId('openid-connect-fields')).not.toBeInTheDocument();
    expect(screen.queryByTestId('saml-fields')).not.toBeInTheDocument();
    expect(screen.queryByTestId('social-provider-fields')).not.toBeInTheDocument();
  });

  test('renders SAMLFields when provider type is saml', () => {
    const samlProfileMetadata = {
      selectedProviderType: 'saml',
      loginUrl: 'https://example.com/login',
      callbackUrl: 'https://example.com/callback',
    };
    
    render(
      <TestWrapper>
        <ProviderConfigurationSection 
          profileData={mockProfileData}
          profileMetadata={samlProfileMetadata}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // SAMLFields should be rendered
    expect(screen.getByTestId('saml-fields')).toBeInTheDocument();
    
    // Other provider fields should not be rendered
    expect(screen.queryByTestId('openid-connect-fields')).not.toBeInTheDocument();
    expect(screen.queryByTestId('ldap-fields')).not.toBeInTheDocument();
    expect(screen.queryByTestId('social-provider-fields')).not.toBeInTheDocument();
  });

  test('renders SocialProviderFields when provider type is a social provider', () => {
    const socialProfileMetadata = {
      selectedProviderType: 'google',
      loginUrl: 'https://example.com/login',
      callbackUrl: 'https://example.com/callback',
    };
    
    render(
      <TestWrapper>
        <ProviderConfigurationSection 
          profileData={mockProfileData}
          profileMetadata={socialProfileMetadata}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // SocialProviderFields should be rendered
    expect(screen.getByTestId('social-provider-fields')).toBeInTheDocument();
    
    // Other provider fields should not be rendered
    expect(screen.queryByTestId('openid-connect-fields')).not.toBeInTheDocument();
    expect(screen.queryByTestId('ldap-fields')).not.toBeInTheDocument();
    expect(screen.queryByTestId('saml-fields')).not.toBeInTheDocument();
  });

  test('passes correct props to child components', () => {
    const openidProfileMetadata = {
      selectedProviderType: 'openid-connect',
      loginUrl: 'https://example.com/login',
      callbackUrl: 'https://example.com/callback',
    };
    
    render(
      <TestWrapper>
        <ProviderConfigurationSection
          profileData={mockProfileData}
          profileMetadata={openidProfileMetadata}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // Check that CommonProviderFields was called with the correct props
    expect(mockCommonProviderFields).toHaveBeenCalledWith(
      expect.objectContaining({
        profileData: mockProfileData,
        profileMetadata: openidProfileMetadata,
        handleCopyToClipboard: mockHandleCopyToClipboard
      })
    );
    
    // Check that OpenIDConnectFields was called with the correct props
    expect(mockOpenIDConnectFields).toHaveBeenCalledWith(
      expect.objectContaining({
        profileData: mockProfileData,
        handleCopyToClipboard: mockHandleCopyToClipboard
      })
    );
  });

  test('isSocialProvider correctly identifies social providers', () => {
    // Test with a social provider
    const socialProfileMetadata = {
      selectedProviderType: 'google',
      loginUrl: 'https://example.com/login',
      callbackUrl: 'https://example.com/callback',
    };
    
    render(
      <TestWrapper>
        <ProviderConfigurationSection
          profileData={mockProfileData}
          profileMetadata={socialProfileMetadata}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // SocialProviderFields should be rendered for social providers
    expect(screen.getByTestId('social-provider-fields')).toBeInTheDocument();
    expect(mockSocialProviderFields).toHaveBeenCalled();
    
    cleanup(); // Clean up before the next render
    
    // Test with a non-social provider
    const nonSocialProfileMetadata = {
      selectedProviderType: 'openid-connect',
      loginUrl: 'https://example.com/login',
      callbackUrl: 'https://example.com/callback',
    };
    
    render(
      <TestWrapper>
        <ProviderConfigurationSection
          profileData={mockProfileData}
          profileMetadata={nonSocialProfileMetadata}
          handleCopyToClipboard={mockHandleCopyToClipboard}
        />
      </TestWrapper>
    );
    
    // OpenIDConnectFields should be rendered for openid-connect provider
    expect(screen.getByTestId('openid-connect-fields')).toBeInTheDocument();
    // SocialProviderFields should not be rendered for non-social providers
    expect(screen.queryByTestId('social-provider-fields')).not.toBeInTheDocument();
  });
});