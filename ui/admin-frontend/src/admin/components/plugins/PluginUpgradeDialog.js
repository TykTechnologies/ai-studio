import React, { useCallback, useEffect, useRef, useState } from 'react';
import {
  Alert,
  Box,
  Checkbox,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  FormControlLabel,
  InputLabel,
  MenuItem,
  Select,
  Typography,
} from '@mui/material';
import { ArrowForward as ArrowForwardIcon } from '@mui/icons-material';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import pluginService from '../../services/pluginService';
import pluginLoaderService from '../../services/pluginLoaderService';
import { useOptionalSyncStatus } from '../../context/SyncStatusContext';
import { SCOPE_DESCRIPTIONS } from './ScopeReviewSection';
import { PrimaryButton, SecondaryOutlineButton } from '../../styles/sharedStyles';

// The dialog has its own "Changelog" heading; drop the file's top-level one.
const stripChangelogTitle = (changelog) => changelog.replace(/^\s*#\s+changelog\s*\n/i, '');

// The changelog comes from the marketplace, not from this Studio: links always
// open outside the app. react-markdown does not render raw HTML.
const changelogComponents = {
  a: ({ node, children, ...props }) => (
    <a href={props.href} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  ),
};

const ScopeList = ({ scopes, color }) => (
  <Box component="ul" sx={{ m: 0, pl: 2.5 }}>
    {scopes.map((scope) => (
      <Box component="li" key={scope} sx={{ mb: 0.5 }}>
        <Chip label={scope} size="small" color={color} variant="outlined" sx={{ mr: 1 }} />
        <Typography variant="caption" color="textSecondary">
          {SCOPE_DESCRIPTIONS[scope] || 'Plugin-specific permission'}
        </Typography>
      </Box>
    ))}
  </Box>
);

/**
 * Reviews and applies a marketplace upgrade of an installed plugin. Opening it
 * previews the latest version; the picker re-previews any other published
 * version, including older ones.
 */
const PluginUpgradeDialog = ({ open, plugin, onClose, onUpgraded }) => {
  const syncStatus = useOptionalSyncStatus();

  const [preview, setPreview] = useState(null);
  const [version, setVersion] = useState('');
  const [loadingPreview, setLoadingPreview] = useState(false);
  const [upgrading, setUpgrading] = useState(false);
  const [error, setError] = useState(null);
  const [scopesApproved, setScopesApproved] = useState(false);
  const [downgradeConfirmed, setDowngradeConfirmed] = useState(false);

  const pluginId = plugin?.id;
  const previewRef = useRef(null);

  const loadPreview = useCallback(async (targetVersion) => {
    setLoadingPreview(true);
    setError(null);
    setScopesApproved(false);
    setDowngradeConfirmed(false);
    try {
      const result = await pluginService.previewPluginUpgrade(pluginId, targetVersion);
      previewRef.current = result;
      setPreview(result);
      setVersion(result?.target_version || targetVersion);
    } catch (err) {
      // Keep the last good preview (and its picker) when another version fails to load.
      setVersion(previewRef.current?.target_version || '');
      setError({ message: err.message });
    } finally {
      setLoadingPreview(false);
    }
  }, [pluginId]);

  useEffect(() => {
    if (open && pluginId) {
      previewRef.current = null;
      setPreview(null);
      setVersion('');
      loadPreview('');
    }
  }, [open, pluginId, loadPreview]);

  const handleVersionChange = (event) => {
    setVersion(event.target.value);
    loadPreview(event.target.value);
  };

  const addedScopes = preview?.scopes?.added || [];
  const removedScopes = preview?.scopes?.removed || [];
  const configIssues = preview?.config_issues || [];
  const warnings = preview?.warnings || [];
  const isDowngrade = Boolean(preview?.is_downgrade);
  const sameVersion = Boolean(preview?.same_version);

  const canConfirm =
    Boolean(preview) &&
    !sameVersion &&
    !loadingPreview &&
    !upgrading &&
    (addedScopes.length === 0 || scopesApproved) &&
    (!isDowngrade || downgradeConfirmed);

  const handleConfirm = async () => {
    setUpgrading(true);
    setError(null);
    try {
      const result = await pluginService.upgradePlugin(pluginId, {
        version: preview.target_version,
        approvedScopes: scopesApproved ? addedScopes : [],
        allowDowngrade: isDowngrade && downgradeConfirmed,
      });

      // A new version can bring new pages and menu entries.
      try {
        await pluginLoaderService.refresh();
      } catch (refreshErr) {
        console.warn('Failed to refresh plugin UI after upgrade:', refreshErr);
      }
      if (result?.affects_edges && syncStatus?.refreshSyncStatus) {
        syncStatus.refreshSyncStatus();
      }

      onUpgraded?.(result);
    } catch (err) {
      setError({ message: err.message, rolledBack: err.rolledBack });
      if (err.missingScopes?.length) {
        // The version changed under us: look at it again before approving.
        loadPreview(version);
      }
    } finally {
      setUpgrading(false);
    }
  };

  const actionLabel = isDowngrade ? 'Downgrade' : 'Upgrade';

  return (
    <Dialog open={open} onClose={upgrading ? undefined : onClose} maxWidth="md" fullWidth>
      <DialogTitle>
        {plugin?.name ? `Change version of ${plugin.name}` : 'Change plugin version'}
      </DialogTitle>

      <DialogContent dividers>
        {error && (
          <Alert severity={error.rolledBack ? 'warning' : 'error'} sx={{ mb: 2 }}>
            {error.rolledBack && (
              <Typography variant="body2" fontWeight="medium">
                The upgrade was rolled back. The plugin is running the version it had before.
              </Typography>
            )}
            <Typography variant="body2">{error.message}</Typography>
          </Alert>
        )}

        {loadingPreview && !preview && (
          <Box display="flex" alignItems="center" gap={2} py={4} justifyContent="center">
            <CircularProgress size={24} />
            <Typography variant="body2" color="textSecondary">
              Fetching and checking the new version…
            </Typography>
          </Box>
        )}

        {preview && (
          <Box sx={{ opacity: loadingPreview ? 0.5 : 1 }}>
            <Box display="flex" alignItems="center" gap={2} flexWrap="wrap" mb={2}>
              <Chip label={preview.installed_version ? `v${preview.installed_version}` : 'Version unknown'} variant="outlined" />
              <ArrowForwardIcon fontSize="small" color="action" />
              <FormControl size="small" sx={{ minWidth: 200 }}>
                <InputLabel id="plugin-upgrade-version-label">Target version</InputLabel>
                <Select
                  labelId="plugin-upgrade-version-label"
                  label="Target version"
                  value={version}
                  onChange={handleVersionChange}
                  disabled={loadingPreview || upgrading}
                >
                  {(preview.versions || []).map((v) => (
                    <MenuItem key={v.version} value={v.version}>
                      v{v.version}
                      {v.version === preview.latest_version ? ' (latest)' : ''}
                      {v.installed ? ' (installed)' : ''}
                      {v.deprecated ? ' (deprecated)' : ''}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
              {loadingPreview && <CircularProgress size={18} />}
            </Box>

            {sameVersion ? (
              <Alert severity="info" sx={{ mb: 2 }}>
                This plugin is already on v{preview.target_version}. Pick another version to change to it.
              </Alert>
            ) : (
              <Alert severity="success" sx={{ mb: 2 }}>
                Your configuration and all plugin data are kept. If v{preview.target_version} does not start
                with the current configuration, the plugin is put back on the version it has now.
              </Alert>
            )}

            {isDowngrade && (
              <Alert severity="warning" sx={{ mb: 2 }}>
                <Typography variant="body2">
                  v{preview.target_version} is older than the installed version. Data written by the newer
                  version may not be understood by the older one.
                </Typography>
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={downgradeConfirmed}
                      onChange={(e) => setDowngradeConfirmed(e.target.checked)}
                    />
                  }
                  label="I understand, downgrade anyway"
                />
              </Alert>
            )}

            {preview.deprecated && (
              <Alert severity="warning" sx={{ mb: 2 }}>
                This version is deprecated. {preview.deprecated_message}
              </Alert>
            )}

            {warnings.map((warning) => (
              <Alert severity="warning" sx={{ mb: 2 }} key={warning}>
                {warning}
              </Alert>
            ))}

            {addedScopes.length > 0 && (
              <Alert severity="warning" sx={{ mb: 2 }}>
                <Typography variant="body2" fontWeight="medium" gutterBottom>
                  This version requests new permissions
                </Typography>
                <ScopeList scopes={addedScopes} color="warning" />
                <FormControlLabel
                  sx={{ mt: 1 }}
                  control={
                    <Checkbox
                      checked={scopesApproved}
                      onChange={(e) => setScopesApproved(e.target.checked)}
                    />
                  }
                  label="Approve these permissions"
                />
              </Alert>
            )}

            {removedScopes.length > 0 && (
              <Alert severity="info" sx={{ mb: 2 }}>
                <Typography variant="body2" fontWeight="medium" gutterBottom>
                  Permissions this version no longer uses will be removed
                </Typography>
                <ScopeList scopes={removedScopes} color="default" />
              </Alert>
            )}

            {configIssues.length > 0 && (
              <Alert severity="warning" sx={{ mb: 2 }}>
                <Typography variant="body2" fontWeight="medium" gutterBottom>
                  The current configuration does not fully match this version&apos;s settings
                </Typography>
                <Box component="ul" sx={{ m: 0, pl: 2.5 }}>
                  {configIssues.map((issue) => (
                    <li key={issue}>
                      <Typography variant="body2">{issue}</Typography>
                    </li>
                  ))}
                </Box>
                <Typography variant="body2" sx={{ mt: 1 }}>
                  You can still upgrade and adjust the configuration afterwards.
                </Typography>
              </Alert>
            )}

            {preview.affects_edges && (
              <Alert severity="info" sx={{ mb: 2 }}>
                This plugin also runs on edge gateways. They pick up the new version with the next
                configuration push.
              </Alert>
            )}

            <Divider sx={{ my: 2 }} />
            <Typography variant="subtitle2" gutterBottom>
              Changelog
            </Typography>
            {preview.changelog ? (
              <Box
                sx={{
                  maxHeight: 260,
                  overflow: 'auto',
                  typography: 'body2',
                  '& h1, & h2, & h3': { typography: 'subtitle2', mt: 1.5, mb: 0.5 },
                  '& ul': { pl: 2.5, my: 0.5 },
                }}
              >
                <ReactMarkdown remarkPlugins={[remarkGfm]} components={changelogComponents}>
                  {stripChangelogTitle(preview.changelog)}
                </ReactMarkdown>
              </Box>
            ) : (
              <Typography variant="body2" color="textSecondary">
                No changelog was published for this version.
              </Typography>
            )}
          </Box>
        )}
      </DialogContent>

      <DialogActions>
        <SecondaryOutlineButton onClick={onClose} disabled={upgrading}>
          Cancel
        </SecondaryOutlineButton>
        <PrimaryButton
          variant="contained"
          onClick={handleConfirm}
          disabled={!canConfirm}
          startIcon={upgrading ? <CircularProgress size={16} color="inherit" /> : null}
        >
          {upgrading
            ? `${actionLabel === 'Upgrade' ? 'Upgrading' : 'Downgrading'}…`
            : preview
              ? `${actionLabel} to v${preview.target_version}`
              : actionLabel}
        </PrimaryButton>
      </DialogActions>
    </Dialog>
  );
};

export default PluginUpgradeDialog;
