import React from "react";
import {
  Box,
  Typography,
  Alert,
  alpha,
  FormControlLabel,
  Radio,
  RadioGroup,
  Paper,
  CircularProgress,
} from "@mui/material";
import StorageIcon from "@mui/icons-material/Storage";
import UploadFileIcon from "@mui/icons-material/UploadFile";

import { PROVIDER_TYPES } from "../constants";

const getProviderIcon = (provider) => {
  if (!provider) return null;
  const type = provider.attributes?.type || provider.type;
  switch (type) {
    case PROVIDER_TYPES.TYK_DASHBOARD:
      return StorageIcon;
    case PROVIDER_TYPES.DIRECT_IMPORT:
      return UploadFileIcon;
    default:
      return null;
  }
};

const SelectProvider = ({
  providers,
  selectedProvider,
  onSelect,
  loading,
  error,
}) => {

  if (loading) {
    return (
      <Box>
        <Typography variant="h6" gutterBottom>
          Select Import Method
        </Typography>
        <Box display="flex" justifyContent="center" my={4}>
          <CircularProgress />
        </Box>
      </Box>
    );
  }

  return (
    <Box>
      <Typography variant="h6" gutterBottom>
        Select Import Method
      </Typography>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      {providers.length === 0 && (
        <Typography color="text.secondary" align="center" sx={{ my: 2 }}>
          No import methods available
        </Typography>
      )}

      <RadioGroup
        value={selectedProvider?.id || ""}
        onChange={(e) => {
          const provider = providers.find((p) => p.id === e.target.value);
          if (provider) {
            console.log('Selected provider:', provider);
            onSelect(provider);
          }
        }}
      >
        {providers.map((provider) => {
          const Icon = getProviderIcon(provider);
          return (
            <Paper
              key={provider.id}
              elevation={0}
              variant="outlined"
              sx={{
                mb: 2,
                p: 2,
                borderColor: (theme) =>
                  selectedProvider?.id === provider.id
                    ? theme.palette.primary.main
                    : "inherit",
                bgcolor: (theme) =>
                  selectedProvider?.id === provider.id
                    ? alpha(theme.palette.primary.main, 0.04)
                    : "inherit",
              }}
            >
              <FormControlLabel
                value={provider.id}
                control={<Radio />}
                label={
                  <Box sx={{ ml: 1 }}>
                    <Box display="flex" alignItems="center" gap={1}>
                      <Icon color={selectedProvider?.id === provider.id ? "primary" : "action"} />
                      <Typography variant="subtitle1" color="text.primary">
                        {provider.attributes?.name || provider.name}
                      </Typography>
                    </Box>
                    <Typography variant="body2" color="text.secondary">
                        {provider.attributes?.description || provider.description}
                    </Typography>
                  </Box>
                }
                sx={{ m: 0, width: "100%" }}
              />
            </Paper>
          );
        })}
      </RadioGroup>
    </Box>
  );
};

export default SelectProvider;
