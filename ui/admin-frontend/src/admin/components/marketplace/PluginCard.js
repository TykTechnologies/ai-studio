import React from 'react';
import {
  Card,
  CardContent,
  CardActions,
  Typography,
  Button,
  Chip,
  Box,
  Avatar,
} from '@mui/material';
import {
  Verified as VerifiedIcon,
  CheckCircle as CheckCircleIcon,
  Warning as WarningIcon,
} from '@mui/icons-material';

const PluginCard = ({ plugin, onViewDetails, onInstall }) => {
  const getPublisherColor = (publisher) => {
    switch (publisher) {
      case 'tyk-official':
        return 'primary';
      case 'tyk-verified':
        return 'secondary';
      default:
        return 'default';
    }
  };

  const getPublisherIcon = (publisher) => {
    switch (publisher) {
      case 'tyk-official':
        return <VerifiedIcon fontSize="small" />;
      case 'tyk-verified':
        return <CheckCircleIcon fontSize="small" />;
      default:
        return null;
    }
  };

  const getMaturityColor = (maturity) => {
    switch (maturity) {
      case 'stable':
        return 'success';
      case 'beta':
        return 'warning';
      case 'alpha':
        return 'error';
      default:
        return 'default';
    }
  };

  return (
    <Card
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        opacity: plugin.deprecated ? 0.6 : 1,
        position: 'relative',
      }}
    >
      {plugin.deprecated && (
        <Box
          sx={{
            position: 'absolute',
            top: 8,
            right: 8,
            zIndex: 1,
          }}
        >
          <Chip
            label="Deprecated"
            size="small"
            color="error"
            icon={<WarningIcon />}
          />
        </Box>
      )}

      <CardContent sx={{ flexGrow: 1 }}>
        <Box sx={{ display: 'flex', alignItems: 'flex-start', mb: 2 }}>
          <Avatar
            src={plugin.icon_url}
            alt={plugin.name}
            sx={{ width: 56, height: 56, mr: 2 }}
            variant="rounded"
          >
            {plugin.name.charAt(0)}
          </Avatar>
          <Box sx={{ flexGrow: 1 }}>
            <Typography variant="h6" component="div" gutterBottom>
              {plugin.name}
            </Typography>
            <Box sx={{ display: 'flex', gap: 0.5, flexWrap: 'wrap' }}>
              <Chip
                label={plugin.publisher}
                size="small"
                color={getPublisherColor(plugin.publisher)}
                icon={getPublisherIcon(plugin.publisher)}
              />
              {plugin.maturity && (
                <Chip
                  label={plugin.maturity}
                  size="small"
                  color={getMaturityColor(plugin.maturity)}
                />
              )}
              {plugin.category && (
                <Chip
                  label={plugin.category}
                  size="small"
                  variant="outlined"
                />
              )}
            </Box>
          </Box>
        </Box>

        <Typography
          variant="body2"
          color="text.secondary"
          sx={{
            mb: 2,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            display: '-webkit-box',
            WebkitLineClamp: 3,
            WebkitBoxOrient: 'vertical',
          }}
        >
          {plugin.description}
        </Typography>

        {plugin.synced_from_url && (
          <Box sx={{ mb: 1 }}>
            <Chip
              label={`Source: ${new URL(plugin.synced_from_url).hostname}`}
              size="small"
              variant="outlined"
              sx={{
                fontSize: '0.7rem',
                height: '20px',
                '& .MuiChip-label': {
                  px: 1,
                }
              }}
            />
          </Box>
        )}

        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Typography variant="caption" color="text.secondary">
            v{plugin.version}
          </Typography>
          {plugin.license && (
            <Typography variant="caption" color="text.secondary">
              {plugin.license}
            </Typography>
          )}
        </Box>
      </CardContent>

      <CardActions sx={{ px: 2, pb: 2 }}>
        <Button
          size="small"
          onClick={() => onViewDetails(plugin)}
          fullWidth
          variant="outlined"
        >
          View Details
        </Button>
        {!plugin.deprecated && (
          <Button
            size="small"
            onClick={() => onInstall(plugin)}
            fullWidth
            variant="contained"
          >
            Install
          </Button>
        )}
      </CardActions>
    </Card>
  );
};

export default PluginCard;
