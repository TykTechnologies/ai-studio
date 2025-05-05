import React from "react";
import { Stack } from "@mui/material";
import { LabeledCopyableField } from "../styles";

const CommonProviderFields = ({ profileData, profileMetadata, handleCopyToClipboard }) => {
  const accessUrl = profileData.ProviderConfig?.CallbackBaseURL || profileData.ProviderConfig?.SAMLBaseURL;
  
  return (
    <Stack spacing={2}>
      <LabeledCopyableField
        label="Login URL"
        value={profileMetadata.loginUrl}
        fieldName="Login URL"
        handleCopyToClipboard={handleCopyToClipboard}
      />
      
      <LabeledCopyableField
        label="Callback URL"
        value={profileMetadata.callbackUrl}
        fieldName="Callback URL"
        handleCopyToClipboard={handleCopyToClipboard}
      />
      
      <LabeledCopyableField
        label="Access URL"
        value={accessUrl}
        fieldName="Access URL"
        handleCopyToClipboard={handleCopyToClipboard}
      />
    </Stack>
  );
};

export default CommonProviderFields;