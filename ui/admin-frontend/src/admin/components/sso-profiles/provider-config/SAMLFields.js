import React from "react";
import { Stack } from "@mui/material";
import AdvancedSettingsSection from "../AdvancedSettingsSection";
import {
  TwoColumnLayout,
  FieldGroup,
  LabeledField,
  LabeledCopyableField,
  BreakableFieldValue,
  FieldLabel,
} from "../styles";

const SAMLFields = ({ profileData, handleCopyToClipboard }) => {
  return (
    <Stack spacing={2}>
      <LabeledCopyableField
        label="Certificate path"
        value={profileData.ProviderConfig?.CertLocation}
        fieldName="Certificate path"
        handleCopyToClipboard={handleCopyToClipboard}
      />

      <LabeledCopyableField
        label="IDP metadata URL"
        value={profileData.ProviderConfig?.IDPMetaDataURL}
        fieldName="IDP metadata URL"
        handleCopyToClipboard={handleCopyToClipboard}
      />

      <AdvancedSettingsSection>
        <LabeledField
          label="SAML email claim"
          value={profileData.ProviderConfig?.SAMLEmailClaim}
        />

        <TwoColumnLayout>
          <FieldGroup>
            <FieldLabel variant="bodyLargeBold" sx={{ minWidth: '40%' }}>SAML forename</FieldLabel>
            <BreakableFieldValue variant="bodyLargeDefault" ml={1}>
              {profileData.ProviderConfig?.SAMLForenameClaim || "-"}
            </BreakableFieldValue>
          </FieldGroup>
          <FieldGroup>
            <FieldLabel variant="bodyLargeBold" sx={{ minWidth: '40%' }}>SAML surname</FieldLabel>
            <BreakableFieldValue variant="bodyLargeDefault" ml={1}>
              {profileData.ProviderConfig?.SAMLSurnameClaim || "-"}
            </BreakableFieldValue>
          </FieldGroup>
        </TwoColumnLayout>

        <LabeledField
          label="Force authentication"
          value={profileData.ProviderConfig?.ForceAuthentication?.toString() || "false"}
        />

        <TwoColumnLayout>
          <FieldGroup>
            <FieldLabel variant="bodyLargeBold" sx={{ minWidth: '40%' }}>Custom email</FieldLabel>
            <BreakableFieldValue variant="bodyLargeDefault" ml={1}>
              {profileData.CustomEmailField || "-"}
            </BreakableFieldValue>
          </FieldGroup>
          <FieldGroup>
            <FieldLabel variant="bodyLargeBold" sx={{ minWidth: '40%' }}>Custom ID</FieldLabel>
            <BreakableFieldValue variant="bodyLargeDefault" ml={1}>
              {profileData.CustomUserIDField || "-"}
            </BreakableFieldValue>
          </FieldGroup>
        </TwoColumnLayout>

        <LabeledField
          label="Provider domain"
          value={profileData.ProviderConstraints?.Domain}
        />
      </AdvancedSettingsSection>
    </Stack>
  );
};

export default SAMLFields;