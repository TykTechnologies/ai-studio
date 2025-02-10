import React from 'react';
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

const SelectAPI = ({
  apis,
  selectedAPI,
  onSelect,
  loading,
  error
}) => {
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
        Select API
      </Typography>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      <List>
        {apis.map((api) => (
          <ListItem
            key={api.id}
            button
            selected={selectedAPI?.id === api.id}
            onClick={() => onSelect(api)}
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
            {selectedAPI?.id === api.id && (
              <ListItemIcon>
                <CheckCircleIcon color="primary" />
              </ListItemIcon>
            )}
            <ListItemText
              primary={api.name}
              secondary={api.description}
            />
          </ListItem>
        ))}
      </List>

      {apis.length === 0 && !loading && !error && (
        <Typography color="text.secondary" align="center">
          No APIs available. Make sure your provider configuration is correct.
        </Typography>
      )}
    </Box>
  );
};

export default SelectAPI;
