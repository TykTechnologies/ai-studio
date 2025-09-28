import React from 'react';
import {
  Box,
  Typography,
  Alert,
} from '@mui/material';
import { SecondaryOutlineButton } from '../../../styles/sharedStyles';
import ScopeReviewSection from '../ScopeReviewSection';

const Step2ScopeApproval = ({ scopes, manifest, pluginData, onApprove, onBack, loading, disabled }) => {
  const handleApprove = () => {
    onApprove(true);
  };

  const handleDeny = () => {
    onApprove(false);
  };

  return (
    <Box>
      <Typography variant="h6" gutterBottom>
        Plugin Security Review
      </Typography>

      <Typography variant="body2" color="textSecondary" sx={{ mb: 3 }}>
        The AI Studio plugin <strong>{pluginData.name}</strong> is requesting access to specific services within your environment.
        Please review and approve these permissions before proceeding.
      </Typography>

      {/* Plugin Info */}
      <Alert severity="info" sx={{ mb: 3 }}>
        <Typography variant="body2" fontWeight="medium">
          Plugin Details:
        </Typography>
        <Typography variant="body2">
          • Name: {pluginData.name}
        </Typography>
        <Typography variant="body2">
          • Command: {pluginData.command}
        </Typography>
        {manifest && (
          <>
            <Typography variant="body2">
              • Manifest ID: {manifest.id}
            </Typography>
            <Typography variant="body2">
              • Version: {manifest.version}
            </Typography>
          </>
        )}
      </Alert>

      {/* Scope Review */}
      <ScopeReviewSection
        scopes={scopes}
        onApprove={handleApprove}
        onDeny={handleDeny}
        loading={loading}
        disabled={disabled}
      />

      {/* Back Button */}
      <Box sx={{ mt: 4, display: 'flex', justifyContent: 'flex-start' }}>
        <SecondaryOutlineButton onClick={onBack} disabled={disabled || loading}>
          Back
        </SecondaryOutlineButton>
      </Box>
    </Box>
  );
};

export default Step2ScopeApproval;