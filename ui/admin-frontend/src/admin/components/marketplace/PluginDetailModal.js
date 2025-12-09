import React, { useState, useEffect } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Typography,
  Box,
  Chip,
  Divider,
  List,
  ListItem,
  ListItemText,
  Link,
  Avatar,
  Tab,
  Tabs,
  Alert,
  CircularProgress,
} from '@mui/material';
import {
  OpenInNew as OpenInNewIcon,
  Security as SecurityIcon,
  Download as DownloadIcon,
  Star as StarIcon,
} from '@mui/icons-material';
import marketplaceService from '../../services/marketplaceService';
import { useEdition } from '../../context/EditionContext';

function TabPanel({ children, value, index }) {
  return (
    <div hidden={value !== index} style={{ paddingTop: 16 }}>
      {value === index && children}
    </div>
  );
}

const PluginDetailModal = ({ open, plugin, onClose, onInstall }) => {
  const { isEnterprise, loading: editionLoading } = useEdition();
  const [tabValue, setTabValue] = useState(0);
  const [versions, setVersions] = useState([]);
  const [loadingVersions, setLoadingVersions] = useState(false);

  // Disable install for enterprise-only plugins when not running enterprise binary
  const isInstallDisabled = editionLoading || (plugin?.enterprise_only && !isEnterprise);

  useEffect(() => {
    if (open && plugin) {
      loadVersions();
    }
  }, [open, plugin]);

  const loadVersions = async () => {
    setLoadingVersions(true);
    try {
      const data = await marketplaceService.getPluginVersions(plugin.plugin_id);
      setVersions(data.versions || []);
    } catch (err) {
      console.error('Failed to load versions:', err);
    } finally {
      setLoadingVersions(false);
    }
  };

  const handleTabChange = (event, newValue) => {
    setTabValue(newValue);
  };

  if (!plugin) return null;

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
      scroll="paper"
    >
      <DialogTitle>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <Avatar
            src={plugin.icon_url}
            alt={plugin.name}
            sx={{ width: 56, height: 56 }}
            variant="rounded"
          >
            {plugin.name.charAt(0)}
          </Avatar>
          <Box sx={{ flexGrow: 1 }}>
            <Typography variant="h5">{plugin.name}</Typography>
            <Box sx={{ display: 'flex', gap: 0.5, mt: 0.5, flexWrap: 'wrap' }}>
              <Chip label={plugin.publisher} size="small" color="primary" />
              <Chip label={plugin.maturity} size="small" />
              <Chip label={`v${plugin.version}`} size="small" variant="outlined" />
              {plugin.enterprise_only && (
                <Chip label="Enterprise" size="small" color="secondary" icon={<StarIcon />} />
              )}
            </Box>
          </Box>
        </Box>
      </DialogTitle>

      <Divider />

      <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
        <Tabs value={tabValue} onChange={handleTabChange}>
          <Tab label="Overview" />
          <Tab label="Versions" />
          <Tab label="Permissions" />
        </Tabs>
      </Box>

      <DialogContent>
        <TabPanel value={tabValue} index={0}>
          <Typography variant="body1" paragraph>
            {plugin.description}
          </Typography>

          {plugin.enterprise_only && isInstallDisabled && (
            <Alert severity="info" icon={<StarIcon />} sx={{ mb: 2 }}>
              <strong>Enterprise Only</strong>
              <Typography variant="body2">
                This plugin requires an Enterprise license to install and use.
              </Typography>
            </Alert>
          )}

          {plugin.deprecated && (
            <Alert severity="warning" sx={{ mb: 2 }}>
              <strong>This plugin is deprecated.</strong>
              {plugin.deprecated_message && (
                <Typography variant="body2">{plugin.deprecated_message}</Typography>
              )}
              {plugin.replacement_plugin && (
                <Typography variant="body2">
                  Recommended replacement: <strong>{plugin.replacement_plugin}</strong>
                </Typography>
              )}
            </Alert>
          )}

          <Box sx={{ my: 2 }}>
            <Typography variant="subtitle2" gutterBottom>
              Details
            </Typography>
            <List dense>
              <ListItem>
                <ListItemText
                  primary="Category"
                  secondary={plugin.category || 'N/A'}
                />
              </ListItem>
              <ListItem>
                <ListItemText
                  primary="License"
                  secondary={plugin.license || 'N/A'}
                />
              </ListItem>
              <ListItem>
                <ListItemText
                  primary="Hook Type"
                  secondary={plugin.primary_hook || 'N/A'}
                />
              </ListItem>
              {plugin.min_studio_version && (
                <ListItem>
                  <ListItemText
                    primary="Minimum AI Studio Version"
                    secondary={plugin.min_studio_version}
                  />
                </ListItem>
              )}
            </List>
          </Box>

          {(plugin.documentation_url ||
            plugin.repository_url ||
            plugin.support_url ||
            plugin.homepage_url) && (
            <>
              <Divider sx={{ my: 2 }} />
              <Typography variant="subtitle2" gutterBottom>
                Links
              </Typography>
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                {plugin.documentation_url && (
                  <Link
                    href={plugin.documentation_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
                  >
                    Documentation <OpenInNewIcon fontSize="small" />
                  </Link>
                )}
                {plugin.repository_url && (
                  <Link
                    href={plugin.repository_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
                  >
                    Repository <OpenInNewIcon fontSize="small" />
                  </Link>
                )}
                {plugin.support_url && (
                  <Link
                    href={plugin.support_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
                  >
                    Support <OpenInNewIcon fontSize="small" />
                  </Link>
                )}
                {plugin.homepage_url && (
                  <Link
                    href={plugin.homepage_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
                  >
                    Homepage <OpenInNewIcon fontSize="small" />
                  </Link>
                )}
              </Box>
            </>
          )}
        </TabPanel>

        <TabPanel value={tabValue} index={1}>
          {loadingVersions ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
              <CircularProgress />
            </Box>
          ) : (
            <List>
              {versions.map((version) => (
                <ListItem
                  key={version.version}
                  sx={{
                    border: 1,
                    borderColor: 'divider',
                    borderRadius: 1,
                    mb: 1,
                  }}
                >
                  <ListItemText
                    primary={
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Typography variant="subtitle1">
                          v{version.version}
                        </Typography>
                        {version.version === plugin.version && (
                          <Chip label="Current" size="small" color="primary" />
                        )}
                        {version.deprecated && (
                          <Chip label="Deprecated" size="small" color="error" />
                        )}
                      </Box>
                    }
                    secondary={
                      <>
                        <Typography variant="body2" component="span">
                          {version.description}
                        </Typography>
                        <br />
                        <Typography variant="caption" color="text.secondary">
                          Updated: {new Date(version.plugin_updated_at).toLocaleDateString()}
                        </Typography>
                      </>
                    }
                  />
                </ListItem>
              ))}
            </List>
          )}
        </TabPanel>

        <TabPanel value={tabValue} index={2}>
          <Alert severity="info" icon={<SecurityIcon />} sx={{ mb: 2 }}>
            This plugin requires the following permissions. You will be asked to approve these during installation.
          </Alert>

          {plugin.required_services && plugin.required_services.length > 0 && (
            <Box sx={{ mb: 2 }}>
              <Typography variant="subtitle2" gutterBottom>
                Service Access
              </Typography>
              <List dense>
                {plugin.required_services.map((scope) => (
                  <ListItem key={scope}>
                    <ListItemText
                      primary={scope}
                      secondary={getScopeDescription(scope)}
                    />
                  </ListItem>
                ))}
              </List>
            </Box>
          )}

          {plugin.oci_platforms && plugin.oci_platforms.length > 0 && (
            <Box>
              <Typography variant="subtitle2" gutterBottom>
                Supported Platforms
              </Typography>
              <Box sx={{ display: 'flex', gap: 0.5, flexWrap: 'wrap' }}>
                {plugin.oci_platforms.map((platform) => (
                  <Chip key={platform} label={platform} size="small" variant="outlined" />
                ))}
              </Box>
            </Box>
          )}
        </TabPanel>
      </DialogContent>

      <Divider />

      <DialogActions>
        <Button onClick={onClose}>Close</Button>
        {!plugin.deprecated && (
          <Button
            variant="contained"
            startIcon={<DownloadIcon />}
            onClick={() => {
              onInstall(plugin);
              onClose();
            }}
            disabled={isInstallDisabled}
          >
            {isInstallDisabled && plugin.enterprise_only ? 'Enterprise Required' : 'Install Plugin'}
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
};

// Helper function to provide descriptions for common scopes
function getScopeDescription(scope) {
  const descriptions = {
    'llms.proxy': 'Access to call LLM services via the proxy',
    'llms.read': 'Read LLM configurations',
    'llms.write': 'Modify LLM configurations',
    'tools.call': 'Execute tool operations',
    'tools.read': 'Read tool configurations',
    'datasources.query': 'Query datasources',
    'datasources.read': 'Read datasource configurations',
    'kv.readwrite': 'Read and write plugin key-value storage',
  };

  return descriptions[scope] || 'Permission to access this service';
}

export default PluginDetailModal;
