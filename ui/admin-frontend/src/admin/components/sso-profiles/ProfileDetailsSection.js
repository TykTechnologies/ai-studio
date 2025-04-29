import React from "react";
import { Stack } from "@mui/material";
import {
  LabeledField,
  LabeledCopyableField,
} from "./styles";

const ProfileDetailsSection = ({ profileData, profileMetadata, handleCopyToClipboard }) => {
  return (
    <Stack spacing={2}>
      <LabeledField
        label="Profile name"
        value={profileData.Name}
      />

      <LabeledField
        label="Profile type"
        value={profileData.ActionType}
      />

      <LabeledField
        label="Provider type"
        value={profileMetadata.selectedProviderType}
      />

      <LabeledCopyableField
        label="Redirect URL on failure"
        value={profileMetadata.failureRedirectUrl}
        fieldName="Redirect URL on failure"
        handleCopyToClipboard={handleCopyToClipboard}
      />

      <LabeledField
        label="Default profile for SSO at Login page"
        value={profileMetadata.useInLoginPage ? "Yes" : "No"}
      />
    </Stack>
  );
};

export default ProfileDetailsSection;