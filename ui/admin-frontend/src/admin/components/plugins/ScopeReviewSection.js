import React from 'react';
import {
  Box,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Chip,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  Alert,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';

const SCOPE_CATEGORIES = {
  'analytics': {
    label: 'Analytics',
    description: 'Access to analytics data and metrics',
    color: 'info',
  },
  'plugins': {
    label: 'Plugin Management',
    description: 'Access to plugin information and operations',
    color: 'primary',
  },
  'llms': {
    label: 'LLM Management',
    description: 'Access to LLM configurations and operations',
    color: 'success',
  },
  'tools': {
    label: 'Tools & Data Sources',
    description: 'Access to tools and data source configurations',
    color: 'warning',
  },
  'apps': {
    label: 'Application Management',
    description: 'Access to application configurations',
    color: 'secondary',
  },
};

const SCOPE_DESCRIPTIONS = {
  // Analytics scopes
  'analytics.read': 'View analytics data and metrics',
  'analytics.detailed': 'Access detailed analytics reports',
  'analytics.reports': 'Generate and export analytics reports',

  // Plugin scopes
  'plugins.read': 'View plugin information and configurations',
  'plugins.write': 'Create, update, and delete plugins',
  'plugins.config': 'Modify plugin configurations',

  // LLM scopes
  'llms.read': 'View LLM configurations and status',
  'llms.write': 'Create, update, and delete LLM configurations',

  // Tools scopes
  'tools.read': 'View tool and data source configurations',
  'tools.write': 'Create, update, and delete tools and data sources',
  'tools.operations': 'Execute tool operations and queries',

  // App scopes
  'apps.read': 'View application configurations',
  'apps.write': 'Create, update, and delete applications',
};

const ScopeReviewSection = ({ scopes = [], onApprove, onDeny, loading = false, disabled = false }) => {
  if (!scopes || scopes.length === 0) {
    return (
      <Box>
        <Typography variant="h6" gutterBottom>
          Service Access Review
        </Typography>
        <Alert severity="info">
          This plugin does not request access to any AI Studio services.
        </Alert>
      </Box>
    );
  }

  // Group scopes by category
  const groupedScopes = scopes.reduce((groups, scope) => {
    const category = scope.split('.')[0];
    if (!groups[category]) {
      groups[category] = [];
    }
    groups[category].push(scope);
    return groups;
  }, {});

  const getScopeLevel = (scope) => {
    if (scope.endsWith('.write') || scope.endsWith('.operations')) {
      return { level: 'High', color: 'error' };
    }
    if (scope.endsWith('.config') || scope.endsWith('.detailed')) {
      return { level: 'Medium', color: 'warning' };
    }
    return { level: 'Low', color: 'success' };
  };

  return (
    <Box>
      <Typography variant="h6" gutterBottom>
        Service Access Review
      </Typography>

      <Alert severity="warning" sx={{ mb: 3 }}>
        <Typography variant="body2" fontWeight="medium">
          This AI Studio plugin is requesting access to the following services:
        </Typography>
        <Typography variant="body2" sx={{ mt: 1 }}>
          Please review these permissions carefully. Once approved, the plugin will have access to these services within your AI Studio environment.
        </Typography>
      </Alert>

      <Box sx={{ mb: 3 }}>
        {Object.entries(groupedScopes).map(([category, categoryScopes]) => {
          const categoryInfo = SCOPE_CATEGORIES[category] || {
            label: category.charAt(0).toUpperCase() + category.slice(1),
            description: `Access to ${category} related functionality`,
            color: 'default',
          };

          return (
            <Accordion key={category} defaultExpanded>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Box display="flex" alignItems="center" gap={2}>
                  <Chip
                    label={categoryInfo.label}
                    color={categoryInfo.color}
                    size="small"
                  />
                  <Typography variant="subtitle2">
                    {categoryScopes.length} permission{categoryScopes.length !== 1 ? 's' : ''}
                  </Typography>
                </Box>
              </AccordionSummary>
              <AccordionDetails>
                <Typography variant="body2" color="textSecondary" sx={{ mb: 2 }}>
                  {categoryInfo.description}
                </Typography>

                <TableContainer component={Paper} variant="outlined">
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        <TableCell>Permission</TableCell>
                        <TableCell>Description</TableCell>
                        <TableCell align="center">Risk Level</TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {categoryScopes.map((scope) => {
                        const riskInfo = getScopeLevel(scope);
                        return (
                          <TableRow key={scope}>
                            <TableCell>
                              <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                                {scope}
                              </Typography>
                            </TableCell>
                            <TableCell>
                              <Typography variant="body2">
                                {SCOPE_DESCRIPTIONS[scope] || 'Standard access permission'}
                              </Typography>
                            </TableCell>
                            <TableCell align="center">
                              <Chip
                                label={riskInfo.level}
                                color={riskInfo.color}
                                size="small"
                                variant="outlined"
                              />
                            </TableCell>
                          </TableRow>
                        );
                      })}
                    </TableBody>
                  </Table>
                </TableContainer>
              </AccordionDetails>
            </Accordion>
          );
        })}
      </Box>

      {/* Action Buttons */}
      <Box display="flex" justifyContent="space-between" alignItems="center">
        <Typography variant="body2" color="textSecondary">
          Total permissions requested: {scopes.length}
        </Typography>

        <Box display="flex" gap={2}>
          <Box
            component="button"
            onClick={onDeny}
            disabled={disabled || loading}
            sx={{
              px: 3,
              py: 1,
              border: '1px solid',
              borderColor: 'error.main',
              backgroundColor: 'transparent',
              color: 'error.main',
              borderRadius: 1,
              cursor: 'pointer',
              '&:hover': {
                backgroundColor: 'error.main',
                color: 'white',
              },
              '&:disabled': {
                opacity: 0.5,
                cursor: 'not-allowed',
              },
            }}
          >
            {loading ? 'Processing...' : 'Decline'}
          </Box>

          <Box
            component="button"
            onClick={onApprove}
            disabled={disabled || loading}
            sx={{
              px: 3,
              py: 1,
              border: '1px solid',
              borderColor: 'success.main',
              backgroundColor: 'success.main',
              color: 'white',
              borderRadius: 1,
              cursor: 'pointer',
              '&:hover': {
                backgroundColor: 'success.dark',
              },
              '&:disabled': {
                opacity: 0.5,
                cursor: 'not-allowed',
              },
            }}
          >
            {loading ? 'Processing...' : 'Accept'}
          </Box>
        </Box>
      </Box>
    </Box>
  );
};

export default ScopeReviewSection;