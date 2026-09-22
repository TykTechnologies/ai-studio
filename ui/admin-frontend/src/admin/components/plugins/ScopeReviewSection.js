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
    label: 'Tools & data sources',
    description: 'Access to tools and data source configurations',
    color: 'warning',
  },
  'apps': {
    label: 'Application Management',
    description: 'Access to application configurations',
    color: 'secondary',
  },
  'kv': {
    label: 'Key-value storage',
    description: "The plugin's own key-value store in AI Studio, for state it keeps between calls",
    color: 'info',
  },
  'rbac': {
    label: 'Roles & permissions',
    description: 'Registering permission resources of its own, so roles can grant access to the plugin',
    color: 'error',
  },
  'resource-types': {
    label: 'Resource types',
    description: 'Registering custom resource types that Apps can be given access to (ResourceProvider plugins)',
    color: 'secondary',
  },
  // Object hook categories
  'llm': {
    label: 'LLM Object Hooks',
    description: 'Intercept and modify LLM create, update, and delete operations',
    color: 'success',
  },
  'datasource': {
    label: 'Data source object hooks',
    description: 'Intercept and modify datasource create, update, and delete operations',
    color: 'info',
  },
  'tool': {
    label: 'Tool Object Hooks',
    description: 'Intercept and modify tool create, update, and delete operations',
    color: 'warning',
  },
  'user': {
    label: 'User Object Hooks',
    description: 'Intercept and modify user create, update, and delete operations',
    color: 'error',
  },
};

export const SCOPE_DESCRIPTIONS = {
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

  // Plugin KV storage
  'kv.readwrite': "Read and write the plugin's own key-value storage",

  // Resource type management (ResourceProvider plugins)
  'resource-types.manage': 'Register and manage custom resource types that Apps can be granted access to',

  // RBAC
  'rbac.register': "Register the plugin's own permission resources with the role system",

  // LLM object hooks
  'llm.before_create': 'Hook called before creating a new LLM configuration',
  'llm.after_create': 'Hook called after creating a new LLM configuration',
  'llm.before_update': 'Hook called before updating an LLM configuration',
  'llm.after_update': 'Hook called after updating an LLM configuration',
  'llm.before_delete': 'Hook called before deleting an LLM configuration',
  'llm.after_delete': 'Hook called after deleting an LLM configuration',

  // Datasource object hooks
  'datasource.before_create': 'Hook called before creating a new data source',
  'datasource.after_create': 'Hook called after creating a new data source',
  'datasource.before_update': 'Hook called before updating a data source',
  'datasource.after_update': 'Hook called after updating a data source',
  'datasource.before_delete': 'Hook called before deleting a data source',
  'datasource.after_delete': 'Hook called after deleting a data source',

  // Tool object hooks
  'tool.before_create': 'Hook called before creating a new tool',
  'tool.after_create': 'Hook called after creating a new tool',
  'tool.before_update': 'Hook called before updating a tool',
  'tool.after_update': 'Hook called after updating a tool',
  'tool.before_delete': 'Hook called before deleting a tool',
  'tool.after_delete': 'Hook called after deleting a tool',

  // User object hooks
  'user.before_create': 'Hook called before creating a new user',
  'user.after_create': 'Hook called after creating a new user',
  'user.before_update': 'Hook called before updating a user',
  'user.after_update': 'Hook called after updating a user',
  'user.before_delete': 'Hook called before deleting a user',
  'user.after_delete': 'Hook called after deleting a user',
};

// "resource-types" -> "Resource Types": the fallback heading for a category
// that has no entry in SCOPE_CATEGORIES.
const formatCategoryLabel = (category) =>
  String(category || '')
    .split(/[-_]+/)
    .filter(Boolean)
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ');

const ScopeReviewSection = ({ scopes = [], onApprove, onDeny, loading = false, disabled = false }) => {
  if (!scopes || scopes.length === 0) {
    return (
      <Box>
        <Typography variant="h6" gutterBottom>
          Service Access Review
        </Typography>
        <Alert severity="info" sx={{ mb: 3 }}>
          This plugin does not request access to any AI Studio services.
        </Alert>

        {/* Action Buttons for plugins without scopes */}
        <Box display="flex" justifyContent="space-between" alignItems="center">
          <Typography variant="body2" color="textSecondary">
            No service permissions requested
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
    // Object hooks that modify data (before_* hooks) are high risk
    if (scope.includes('.before_create') || scope.includes('.before_update') || scope.includes('.before_delete')) {
      return { level: 'High', color: 'error' };
    }
    // Object hooks that observe data (after_* hooks) are medium risk
    if (scope.includes('.after_create') || scope.includes('.after_update') || scope.includes('.after_delete')) {
      return { level: 'Medium', color: 'warning' };
    }
    // Service scopes with write/operations access are high risk
    if (scope.endsWith('.write') || scope.endsWith('.operations')) {
      return { level: 'High', color: 'error' };
    }
    // Service scopes with config/detailed access are medium risk
    if (scope.endsWith('.config') || scope.endsWith('.detailed')) {
      return { level: 'Medium', color: 'warning' };
    }
    // Read-only scopes are low risk
    return { level: 'Low', color: 'success' };
  };

  return (
    <Box>
      <Typography variant="h6" gutterBottom>
        Permission Review
      </Typography>

      <Alert severity="warning" sx={{ mb: 3 }}>
        <Typography variant="body2" fontWeight="medium">
          This plugin is requesting the following permissions:
        </Typography>
        <Typography variant="body2" sx={{ mt: 1 }}>
          Please review these permissions carefully. Once approved, the plugin will have access to these capabilities within your AI Studio environment.
        </Typography>
      </Alert>

      <Box sx={{ mb: 3 }}>
        {Object.entries(groupedScopes).map(([category, categoryScopes]) => {
          const categoryInfo = SCOPE_CATEGORIES[category] || {
            label: formatCategoryLabel(category),
            description: `Access to ${formatCategoryLabel(category).toLowerCase()} related functionality`,
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