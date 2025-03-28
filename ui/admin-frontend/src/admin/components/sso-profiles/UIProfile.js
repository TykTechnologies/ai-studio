import { getBaseUrl } from "../../utils/urlUtils";

// Default template for new SSO profile
export const createEmptyProfile = () => ({
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
    CallbackBaseURL: `${getBaseUrl()}/tib`,
    CertLocation: null,
    ExrtactUserNameFromBasicAuthHeader: false,
    FailureRedirect: `${getBaseUrl()}/?fail=true`,
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
  ReturnURL: `${getBaseUrl()}/sso`,
  DefaultUserGroupID: "1",
  CustomUserGroupField: "",
  UserGroupMapping: {},
  UserGroupSeparator: "",
  SSOOnlyForRegisteredUsers: false,
});

/**
 * Maps API response to UI Profile model
 * @param {Object} apiResponse 
 * @returns {Object}
 */
export const mapApiToUIProfile = (apiResponse) => {
  const attributes = apiResponse.data.attributes;
  
  return {
    ID: attributes.profile_id || "",
    Name: attributes.name || "",
    OrgID: attributes.org_id || "",
    ActionType: attributes.action_type || "",
    MatchedPolicyID: attributes.matched_policy_id || "",
    Type: attributes.type || "",
    ProviderName: attributes.provider_name || "",
    CustomEmailField: attributes.custom_email_field || "",
    CustomUserIDField: attributes.custom_user_id_field || "",
    ProviderConfig: attributes.provider_config || {},
    ProviderConstraintsDomain: attributes.provider_constraints_domain || "",
    ProviderConstraintsGroup: attributes.provider_constraints_group || "",
    ReturnURL: attributes.return_url || "",
    DefaultUserGroupID: attributes.default_user_group_id || "",
    CustomUserGroupField: attributes.custom_user_group_field || "",
    UserGroupMapping: attributes.user_group_mapping || {},
    UserGroupSeparator: attributes.user_group_separator || "",
    SSOOnlyForRegisteredUsers: attributes.sso_only_for_registered_users || false,
    ProviderConstraints: {
      Domain: attributes.provider_constraints_domain || "",
      Group: attributes.provider_constraints_group || ""    
    },
  };
};

/**
 * Maps UI Profile model to API request format
 * @param {Object} uiProfile
 * @returns {Object}
 */
export const mapUIProfileToApi = (uiProfile) => {
  return {
    data: {
      type: "sso-profiles",
      attributes: {
        profile_id: uiProfile.ID,
        name: uiProfile.Name,
        org_id: uiProfile.OrgID,
        action_type: uiProfile.ActionType,
        matched_policy_id: uiProfile.MatchedPolicyID,
        type: uiProfile.Type,
        provider_name: uiProfile.ProviderName,
        custom_email_field: uiProfile.CustomEmailField,
        custom_user_id_field: uiProfile.CustomUserIDField,
        provider_config: uiProfile.ProviderConfig,
        provider_constraints_domain: uiProfile.ProviderConstraints?.Domain,
        provider_constraints_group: uiProfile.ProviderConstraints?.Group,
        return_url: uiProfile.ReturnURL,
        default_user_group_id: uiProfile.DefaultUserGroupID,
        custom_user_group_field: uiProfile.CustomUserGroupField,
        user_group_mapping: uiProfile.UserGroupMapping,
        user_group_separator: uiProfile.UserGroupSeparator,
        sso_only_for_registered_users: uiProfile.SSOOnlyForRegisteredUsers
      }
    }
  };
};
