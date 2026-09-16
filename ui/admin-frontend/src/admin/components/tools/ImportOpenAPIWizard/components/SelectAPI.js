import React, { useState } from 'react';
import {
  Box,
  Typography,
  List,
  ListItemButton,
  ListItemText,
  Alert,
  Chip,
  CircularProgress,
  ListItemIcon,
  TextField,
  alpha
} from '@mui/material';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';

// A Dashboard can hold thousands of APIs. The list scrolls in its own box so
// the search stays in view, and only the first MAX_VISIBLE matches are
// rendered; the hint asks for a narrower search beyond that.
const MAX_VISIBLE = 200;

// SelectAPI lists the Tyk OAS APIs of the chosen connection. The whole list
// is fetched at once (metadata only), so the search is client-side.
const SelectAPI = ({
  apis,
  selectedAPI,
  onSelect,
  loading,
  error
}) => {
  const [query, setQuery] = useState('');

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" my={4}>
        <CircularProgress />
      </Box>
    );
  }

  const q = query.trim().toLowerCase();
  const matches = q
    ? apis.filter((api) =>
        (api.name || '').toLowerCase().includes(q) || (api.listen_path || '').toLowerCase().includes(q)
      )
    : apis;
  const visible = matches.slice(0, MAX_VISIBLE);

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

      {apis.length > 0 && (
        <TextField
          fullWidth
          size="small"
          placeholder={`Search ${apis.length} APIs by name or listen path`}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          sx={{ mb: 1 }}
          inputProps={{ 'data-testid': 'api-search' }}
        />
      )}

      <List sx={{ maxHeight: 360, overflowY: 'auto', pt: 0 }} data-testid="api-list">
        {visible.map((api) => (
          <ListItemButton
            key={api.api_id}
            selected={selectedAPI?.api_id === api.api_id}
            onClick={() => onSelect(api)}
            data-testid={`api-${api.api_id}`}
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
            {selectedAPI?.api_id === api.api_id && (
              <ListItemIcon>
                <CheckCircleIcon color="primary" />
              </ListItemIcon>
            )}
            <ListItemText
              primary={
                <Box display="flex" alignItems="center" gap={1}>
                  <span>{api.name}</span>
                  {api.active === false && <Chip size="small" label="inactive" />}
                </Box>
              }
              secondary={api.listen_path}
            />
          </ListItemButton>
        ))}
      </List>

      {apis.length === 0 && !loading && !error && (
        <Typography color="text.secondary" align="center">
          No OAS APIs found on this Dashboard. Classic API definitions cannot be imported as tools.
        </Typography>
      )}
      {apis.length > 0 && matches.length === 0 && (
        <Typography color="text.secondary" align="center">
          No API matches "{query}".
        </Typography>
      )}
      {matches.length > MAX_VISIBLE && (
        <Typography color="text.secondary" variant="body2" align="center" data-testid="api-overflow">
          Showing the first {MAX_VISIBLE} of {matches.length} matches. Narrow the search to find the rest.
        </Typography>
      )}
    </Box>
  );
};

export default SelectAPI;
