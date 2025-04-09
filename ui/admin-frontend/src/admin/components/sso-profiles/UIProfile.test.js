import { createEmptyProfile, mapApiToUIProfile, mapUIProfileToApi } from './UIProfile';
import { getBaseUrl } from '../../utils/urlUtils';

// Mock the getBaseUrl function
jest.mock('../../utils/urlUtils', () => ({
  getBaseUrl: jest.fn()
}));

describe('UIProfile', () => {
  beforeEach(() => {
    // Reset all mocks before each test
    jest.clearAllMocks();
    // Set a default mock return value for getBaseUrl
    getBaseUrl.mockReturnValue('https://example.com');
  });

  describe('createEmptyProfile', () => {
    test('returns a default empty profile with correct structure', () => {
      const emptyProfile = createEmptyProfile();
      
      // Check that the profile has the expected structure
      expect(emptyProfile).toEqual({
        ID: "",
        Name: "",
        OrgID: "",
        ActionType: "",
        MatchedPolicyID: "",
        Type: "",
        ProviderName: "",
        CustomEmailField: "",
        CustomUserIDField: "",
        ProviderConfig: {
          AccessTokenField: null,
          CallbackBaseURL: "https://example.com/tib",
          CertLocation: null,
          ExrtactUserNameFromBasicAuthHeader: false,
          FailureRedirect: "https://example.com/?fail=true",
          ForceAuthentication: false,
          IDPMetaDataURL: null,
          LDAPAttributes: null,
          LDAPPort: null,
          LDAPServer: null,
          LDAPUseSSL: false,
          LDAPUserDN: null,
          OKCode: null,
          OKRegex: null,
          OKResponse: null,
          ResponseIsJson: false,
          SAMLBaseURL: null,
          SAMLEmailClaim: null,
          SAMLForenameClaim: null,
          SAMLSurnameClaim: null,
          TargetHost: null,
          UseProviders: [
            {
              DiscoverURL: null,
              Key: null,
              Name: null,
              Scopes: null,
              Secret: null,
              SkipUserInfoRequest: false
            }
          ],
          UsernameField: null
        },
        ProviderConstraints: {
          Domain: null,
          Group: null 
        },
        ReturnURL: "https://example.com/sso",
        DefaultUserGroupID: "1",
        CustomUserGroupField: "",
        UserGroupMapping: {},
        UserGroupSeparator: "",
        SSOOnlyForRegisteredUsers: false,
      });
      
      // Verify that getBaseUrl was called the expected number of times
      expect(getBaseUrl).toHaveBeenCalledTimes(3);
    });

    test('uses the correct base URL from getBaseUrl', () => {
      // Set a different mock return value
      getBaseUrl.mockReturnValue('https://test-domain.com');
      
      const emptyProfile = createEmptyProfile();
      
      // Check that URLs use the mocked base URL
      expect(emptyProfile.ProviderConfig.CallbackBaseURL).toBe('https://test-domain.com/tib');
      expect(emptyProfile.ProviderConfig.FailureRedirect).toBe('https://test-domain.com/?fail=true');
      expect(emptyProfile.ReturnURL).toBe('https://test-domain.com/sso');
      
      expect(getBaseUrl).toHaveBeenCalledTimes(3);
    });
  });

  describe('mapApiToUIProfile', () => {
    test('correctly maps API response to UI profile', () => {
      const mockApiResponse = {
        data: {
          attributes: {
            profile_id: '123',
            name: 'Test Profile',
            org_id: 'org123',
            action_type: 'auth',
            matched_policy_id: 'policy123',
            type: 'oauth',
            provider_name: 'Google',
            custom_email_field: 'email',
            custom_user_id_field: 'sub',
            provider_config: {
              AccessTokenField: 'access_token',
              CallbackBaseURL: 'https://example.com/tib',
              UsernameField: 'username'
            },
            provider_constraints_domain: 'example.com',
            provider_constraints_group: 'admin',
            return_url: 'https://example.com/sso',
            default_user_group_id: '2',
            custom_user_group_field: 'groups',
            user_group_mapping: { admin: '1', user: '2' },
            user_group_separator: ',',
            sso_only_for_registered_users: true
          }
        }
      };

      const uiProfile = mapApiToUIProfile(mockApiResponse);
      
      // Check that the mapping is correct
      expect(uiProfile).toEqual({
        ID: '123',
        Name: 'Test Profile',
        OrgID: 'org123',
        ActionType: 'auth',
        MatchedPolicyID: 'policy123',
        Type: 'oauth',
        ProviderName: 'Google',
        CustomEmailField: 'email',
        CustomUserIDField: 'sub',
        ProviderConfig: {
          AccessTokenField: 'access_token',
          CallbackBaseURL: 'https://example.com/tib',
          UsernameField: 'username'
        },
        ReturnURL: 'https://example.com/sso',
        DefaultUserGroupID: '2',
        CustomUserGroupField: 'groups',
        UserGroupMapping: { admin: '1', user: '2' },
        UserGroupSeparator: ',',
        SSOOnlyForRegisteredUsers: true,
        ProviderConstraints: {
          Domain: 'example.com',
          Group: 'admin'
        }
      });
    });

    test('handles missing or empty values in API response', () => {
      const mockApiResponse = {
        data: {
          attributes: {
            // Only providing minimal fields
            profile_id: '123',
            // Other fields are missing
            sso_only_for_registered_users: false
          }
        }
      };

      const uiProfile = mapApiToUIProfile(mockApiResponse);
      
      // Check that only non-empty fields are included
      expect(uiProfile).toEqual({
        ID: '123',
        SSOOnlyForRegisteredUsers: false
      });
    });
  });

  describe('mapUIProfileToApi', () => {
    test('correctly maps UI profile to API request format', () => {
      const mockUIProfile = {
        ID: '123',
        Name: 'Test Profile',
        OrgID: 'org123',
        ActionType: 'auth',
        MatchedPolicyID: 'policy123',
        Type: 'oauth',
        ProviderName: 'Google',
        CustomEmailField: 'email',
        CustomUserIDField: 'sub',
        ProviderConfig: {
          AccessTokenField: 'access_token',
          CallbackBaseURL: 'https://example.com/tib',
          UsernameField: 'username'
        },
        ProviderConstraints: {
          Domain: 'example.com',
          Group: 'admin'
        },
        ReturnURL: 'https://example.com/sso',
        DefaultUserGroupID: '2',
        CustomUserGroupField: 'groups',
        UserGroupMapping: { admin: '1', user: '2' },
        UserGroupSeparator: ',',
        SSOOnlyForRegisteredUsers: true
      };

      const apiRequest = mapUIProfileToApi(mockUIProfile);
      
      // Check that the mapping is correct
      expect(apiRequest).toEqual({
        data: {
          type: 'sso-profiles',
          attributes: {
            profile_id: '123',
            name: 'Test Profile',
            org_id: 'org123',
            action_type: 'auth',
            matched_policy_id: 'policy123',
            type: 'oauth',
            provider_name: 'Google',
            custom_email_field: 'email',
            custom_user_id_field: 'sub',
            provider_config: {
              AccessTokenField: 'access_token',
              CallbackBaseURL: 'https://example.com/tib',
              UsernameField: 'username'
            },
            provider_constraints_domain: 'example.com',
            provider_constraints_group: 'admin',
            return_url: 'https://example.com/sso',
            default_user_group_id: '2',
            custom_user_group_field: 'groups',
            user_group_mapping: { admin: '1', user: '2' },
            user_group_separator: ',',
            sso_only_for_registered_users: true
          }
        }
      });
    });

    test('handles null ProviderConstraints', () => {
      const mockUIProfile = {
        ID: '123',
        Name: 'Test Profile',
        // ProviderConstraints is null
        ProviderConstraints: null
      };

      const apiRequest = mapUIProfileToApi(mockUIProfile);
      
      // Check that provider_constraints_domain and provider_constraints_group are undefined
      expect(apiRequest.data.attributes.provider_constraints_domain).toBeUndefined();
      expect(apiRequest.data.attributes.provider_constraints_group).toBeUndefined();
    });

    test('handles undefined ProviderConstraints fields', () => {
      const mockUIProfile = {
        ID: '123',
        Name: 'Test Profile',
        ProviderConstraints: {
          // Domain is undefined
          // Group is undefined
        }
      };

      const apiRequest = mapUIProfileToApi(mockUIProfile);
      
      // Check that provider_constraints_domain and provider_constraints_group are undefined
      expect(apiRequest.data.attributes.provider_constraints_domain).toBeUndefined();
      expect(apiRequest.data.attributes.provider_constraints_group).toBeUndefined();
    });
  });
});