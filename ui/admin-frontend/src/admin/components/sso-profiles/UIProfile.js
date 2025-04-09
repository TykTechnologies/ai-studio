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
  const profile = {};
  
  if (attributes.profile_id) profile.ID = attributes.profile_id;
  if (attributes.name) profile.Name = attributes.name;
  if (attributes.org_id) profile.OrgID = attributes.org_id;
  if (attributes.action_type) profile.ActionType = attributes.action_type;
  if (attributes.matched_policy_id) profile.MatchedPolicyID = attributes.matched_policy_id;
  if (attributes.type) profile.Type = attributes.type;
  if (attributes.provider_name) profile.ProviderName = attributes.provider_name;
  if (attributes.custom_email_field) profile.CustomEmailField = attributes.custom_email_field;
  if (attributes.custom_user_id_field) profile.CustomUserIDField = attributes.custom_user_id_field;
  
  if (attributes.provider_config && Object.keys(attributes.provider_config).length > 0) {
    profile.ProviderConfig = attributes.provider_config;
  }
  
  if (attributes.return_url) profile.ReturnURL = attributes.return_url;
  if (attributes.default_user_group_id) profile.DefaultUserGroupID = attributes.default_user_group_id;
  if (attributes.custom_user_group_field) profile.CustomUserGroupField = attributes.custom_user_group_field;
  
  if (attributes.user_group_mapping && Object.keys(attributes.user_group_mapping).length > 0) {
    profile.UserGroupMapping = attributes.user_group_mapping;
  }
  
  if (attributes.user_group_separator) profile.UserGroupSeparator = attributes.user_group_separator;
  
  profile.SSOOnlyForRegisteredUsers = attributes.sso_only_for_registered_users;
  
  const hasDomain = attributes.provider_constraints_domain;
  const hasGroup = attributes.provider_constraints_group;
  
  if (hasDomain || hasGroup) {
    profile.ProviderConstraints = {};
    if (hasDomain) profile.ProviderConstraints.Domain = attributes.provider_constraints_domain;
    if (hasGroup) profile.ProviderConstraints.Group = attributes.provider_constraints_group;
  }
  
  return profile;
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
