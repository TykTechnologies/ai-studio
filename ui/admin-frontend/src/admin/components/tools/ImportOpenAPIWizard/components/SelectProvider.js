import React, { useEffect } from 'react';
import {
  Box,
  Typography,
  List,
  ListItem,
  ListItemText,
  Alert,
  CircularProgress,
  ListItemIcon,
  alpha
} from '@mui/material';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';

const SelectProvider = ({
  providers,
  selectedProvider,
  onSelect,
  loading,
  error,
  onFetchProviders
}) => {
  useEffect(() => {
    onFetchProviders();
  }, [onFetchProviders]);

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
        Select API Provider
      </Typography>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      <List>
        {providers.map((provider) => (
          <ListItem
            key={provider.id}
            button
            selected={selectedProvider?.id === provider.id}
            onClick={() => onSelect(provider)}
            sx={{
              '&.Mui-selected': {
                backgroundColor: (theme) => alpha(theme.palette.primary.main, 0.1),
                '&:hover': {
                  backgroundColor: (theme) => alpha(theme.palette.primary.main, 0.15),
                },
              },
              borderRadius: 1,
              mb: 1,
            }}
          >
            {selectedProvider?.id === provider.id && (
              <ListItemIcon>
                <CheckCircleIcon color="primary" />
              </ListItemIcon>
            )}
            <ListItemText
              primary={provider.name}
              secondary={provider.description}
            />
          </ListItem>
        ))}
      </List>

      {providers.length === 0 && !loading && !error && (
        <Typography color="text.secondary" align="center">
          No providers available
        </Typography>
      )}
    </Box>
  );
};

export default SelectProvider;
