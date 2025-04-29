import React from "react";
import { Stack } from "@mui/material";
import AdvancedSettingsSection from "../AdvancedSettingsSection";
import {
  TwoColumnLayout,
  FieldGroup,
  LabeledField,
  BreakableFieldValue,
  FieldLabel,
} from "../styles";

const LDAPFields = ({ profileData }) => {
  return (
    <Stack spacing={2}>
      <TwoColumnLayout>
        <FieldGroup>
          <FieldLabel variant="bodyLargeBold" sx={{ width: '40%' }}>Server</FieldLabel>
          <BreakableFieldValue variant="bodyLargeDefault" ml={1}>
            {profileData.ProviderConfig?.LDAPServer || "-"}
          </BreakableFieldValue>
        </FieldGroup>
        <FieldGroup>
          <FieldLabel variant="bodyLargeBold" sx={{ width: '40%' }}>Port</FieldLabel>
          <BreakableFieldValue variant="bodyLargeDefault" ml={1}>
            {profileData.ProviderConfig?.LDAPPort || "-"}
          </BreakableFieldValue>
        </FieldGroup>
      </TwoColumnLayout>

      <LabeledField
        label="User DN"
        value={profileData.ProviderConfig?.LDAPUserDN}
      />

      <AdvancedSettingsSection>
        <LabeledField
          label="LDAP attributes"
          value={profileData.ProviderConfig?.LDAPAttributes?.join(", ")}
        />

        <LabeledField
          label="Use SSL"
          value={profileData.ProviderConfig?.LDAPUseSSL?.toString() || "false"}
        />
      </AdvancedSettingsSection>
    </Stack>
  );
};

export default LDAPFields;