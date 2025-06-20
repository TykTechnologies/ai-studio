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
  FieldValue,
  CopyableFieldContainer,
  CopyButton,
  CopyIcon,
} from "../styles";

const OpenIDConnectFields = ({ profileData, handleCopyToClipboard }) => {
  const clientKey = profileData.ProviderConfig?.UseProviders?.[0]?.Key;
  const discoverUrl = profileData.ProviderConfig?.UseProviders?.[0]?.DiscoverURL;
  const skipUserInfoRequest = profileData.ProviderConfig?.UseProviders?.[0]?.SkipUserInfoRequest?.toString() || "false";
  const scopes = profileData.ProviderConfig?.UseProviders?.[0]?.Scopes?.join(", ");
  
  return (
    <Stack spacing={2}>
      <TwoColumnLayout>
        <FieldGroup>
          <FieldLabel variant="bodyLargeBold" sx={{ minWidth: '40%' }}>Client ID/Key</FieldLabel>
          <CopyableFieldContainer>
            <FieldValue variant="bodyLargeDefault" ml={1}>{clientKey || "-"}</FieldValue>
            {clientKey && (
              <CopyButton size="small" onClick={() => handleCopyToClipboard(clientKey, "Client ID/Key")}>
                <CopyIcon />
              </CopyButton>
            )}
          </CopyableFieldContainer>
        </FieldGroup>
        <FieldGroup>
          <FieldLabel variant="bodyLargeBold" sx={{ width: '40%' }}>Secret</FieldLabel>
          <FieldValue variant="bodyLargeDefault" ml={1}>{"*".repeat(8)}</FieldValue>
        </FieldGroup>
      </TwoColumnLayout>

      <LabeledCopyableField
        label="Discover URL (well known endpoint)"
        value={discoverUrl}
        fieldName="Discover URL"
        handleCopyToClipboard={handleCopyToClipboard}
      />

      <AdvancedSettingsSection>
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
          label="Skip user info request"
          value={skipUserInfoRequest}
        />
        
        <LabeledField
          label="Scopes"
          value={scopes}
        />
      </AdvancedSettingsSection>
    </Stack>
  );
};

export default OpenIDConnectFields;