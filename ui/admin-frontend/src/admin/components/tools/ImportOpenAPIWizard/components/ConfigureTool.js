import React from 'react';
import { PROVIDER_TYPES } from '../constants';
import PrivacyLevelInput from '../../../common/privacy/PrivacyLevelInput';
import ToolAccessMethods from '../../ToolAccessMethods';
import {
  Box,
  Typography,
  TextField,
  Alert,
  CircularProgress
} from '@mui/material';

const ConfigureTool = ({
  toolConfig,
  onConfigChange,
  loading,
  error,
  selectedAPI,
  importMethod = PROVIDER_TYPES.DIRECT_IMPORT // default to direct import if not specified
}) => {
  const handleChange = (field) => (event) => {
    onConfigChange({
      ...toolConfig,
      [field]: event.target.value
    });
  };

  // The number arrives already clamped to 0–100 (or "" while cleared).
  const handlePrivacyChange = (score) => {
    onConfigChange({
      ...toolConfig,
      privacy_score: score
    });
  };

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" my={4}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box>
      <Typography variant="h6" gutterBottom>
        Configure Tool
      </Typography>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      <TextField
        fullWidth
        label="Name"
        value={toolConfig.name}
        onChange={handleChange('name')}
        margin="normal"
        required
      />

      <TextField
        fullWidth
        label="Description"
        value={toolConfig.description}
        onChange={handleChange('description')}
        margin="normal"
        multiline
        rows={4}
        required
      />

      <Box sx={{ mt: 2, mb: 1 }}>
        <PrivacyLevelInput
          value={toolConfig.privacy_score}
          onChange={handlePrivacyChange}
        />
      </Box>

      {/* Same block as the tool form: chat always, REST and MCP opt-in. */}
      <Box sx={{ mt: 2, mb: 2 }}>
        <Typography variant="subtitle2" gutterBottom>
          Access methods
        </Typography>
        <ToolAccessMethods
          restEnabled={Boolean(toolConfig.rest_access_enabled)}
          mcpEnabled={Boolean(toolConfig.mcp_access_enabled)}
          onChange={(name, value) => onConfigChange({ ...toolConfig, [name]: value })}
        />
      </Box>

      {(importMethod === PROVIDER_TYPES.TYK_DASHBOARD || importMethod === PROVIDER_TYPES.DIRECT_IMPORT) && (
        <>
          <Box sx={{ mb: 2 }}>
            <Typography variant="subtitle2" gutterBottom>
              Authentication Details
            </Typography>
            <Typography variant="body2" color="text.secondary" paragraph>
              Type: {selectedAPI?.security_details?.type || "None"}
              {selectedAPI?.security_details?.in && ` (in ${selectedAPI.security_details.in})`}
            </Typography>
          </Box>

          <Box sx={{ mb: 2 }}>
            <Typography variant="subtitle2" gutterBottom>
              Available Operations
            </Typography>
            {toolConfig.operations.map((operationId, index) => (
              <Typography key={index} variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                {operationId}
              </Typography>
            ))}
          </Box>

          <TextField
            fullWidth
            label="Auth Schema Name"
            value={toolConfig.auth_schema_name || ""}
            onChange={handleChange("auth_schema_name")}
            margin="normal"
            helperText={`Name of the security scheme in OpenAPI spec (${selectedAPI?.security_details?.type || "none"}${selectedAPI?.security_details?.in ? ` in ${selectedAPI.security_details.in}` : ""})`}
            autoComplete="off"
          />

          <TextField
            fullWidth
            label="Auth Key"
            type="password"
            value={toolConfig.auth_key}
            onChange={handleChange("auth_key")}
            margin="normal"
            helperText={toolConfig.auth_key_prefilled ? "API key auto-generated from provider" : "API key or token for authentication"}
            disabled={toolConfig.auth_key_prefilled}
            autoComplete="new-password"
            InputProps={{
              autoComplete: "new-password",
              form: {
                autoComplete: "off",
              },
            }}
          />
        </>
      )}
    </Box>
  );
};

export default ConfigureTool;
