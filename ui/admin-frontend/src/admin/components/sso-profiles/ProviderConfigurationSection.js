import React from "react";
import { Stack } from "@mui/material";
import CommonProviderFields from "./provider-config/CommonProviderFields";
import OpenIDConnectFields from "./provider-config/OpenIDConnectFields";
import LDAPFields from "./provider-config/LDAPFields";
import SocialProviderFields from "./provider-config/SocialProviderFields";
import SAMLFields from "./provider-config/SAMLFields";

const NON_SOCIAL_PROVIDERS = ["openid-connect", "ldap", "saml"];

const isSocialProvider = (providerType) => {
  return !NON_SOCIAL_PROVIDERS.includes(providerType);
};

/**
 * Component for displaying the provider configuration section
 *
 * @param {Object} props - Component props
 * @param {Object} props.profileData - The profile data to display
 * @param {Object} props.profileMetadata - Additional profile metadata including URLs
 * @param {Function} props.handleCopyToClipboard - Function to handle copying to clipboard
 * @returns {React.ReactElement}
 */
const ProviderConfigurationSection = ({ profileData, profileMetadata, handleCopyToClipboard }) => {
  return (
    <Stack spacing={2}>
      {/* Common fields for all provider types */}
      <CommonProviderFields
        profileData={profileData}
        profileMetadata={profileMetadata}
        handleCopyToClipboard={handleCopyToClipboard}
      />

      {/* Provider-specific fields */}
      {profileMetadata.selectedProviderType === "openid-connect" && (
        <OpenIDConnectFields
          profileData={profileData}
          handleCopyToClipboard={handleCopyToClipboard}
        />
      )}

      {profileMetadata.selectedProviderType === "ldap" && (
        <LDAPFields
          profileData={profileData}
        />
      )}

      {profileMetadata.selectedProviderType === "saml" && (
        <SAMLFields
          profileData={profileData}
          handleCopyToClipboard={handleCopyToClipboard}
        />
      )}

      {isSocialProvider(profileMetadata.selectedProviderType) && (
        <SocialProviderFields
          profileData={profileData}
          handleCopyToClipboard={handleCopyToClipboard}
        />
      )}
    </Stack>
  );
};

export default ProviderConfigurationSection;