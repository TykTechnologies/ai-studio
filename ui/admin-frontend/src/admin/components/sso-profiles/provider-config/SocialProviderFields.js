import React from "react";
import { Stack } from "@mui/material";
import {
  TwoColumnLayout,
  FieldGroup,
  LabeledField,
  BreakableFieldValue,
  FieldLabel,
  FieldValue,
} from "../styles";

const SocialProviderFields = ({ profileData }) => {
  return (
    <Stack spacing={2}>
      <LabeledField
        label="Social Provider"
        value={profileData.ProviderConfig?.UseProviders?.[0]?.Name}
      />

      <TwoColumnLayout>
        <FieldGroup>
          <FieldLabel variant="bodyLargeBold" sx={{ minWidth: '40%' }}>Client ID/Key</FieldLabel>
          <BreakableFieldValue variant="bodyLargeDefault" ml={1}>
            {profileData.ProviderConfig?.UseProviders?.[0]?.Key || "-"}
          </BreakableFieldValue>
        </FieldGroup>
        <FieldGroup>
          <FieldLabel variant="bodyLargeBold" sx={{ width: '40%' }}>Secret</FieldLabel>
          <FieldValue variant="bodyLargeDefault" ml={1}>{"*".repeat(8)}</FieldValue>
        </FieldGroup>
      </TwoColumnLayout>
    </Stack>
  );
};

export default SocialProviderFields;