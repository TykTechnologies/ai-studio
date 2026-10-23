import React from 'react';
import { Alert, FormControlLabel, FormHelperText, Switch, Typography } from '@mui/material';
import { usePermissions } from '../../context/PermissionsContext';

/**
 * The admin-only "Allow internal host" switch of an LLM, embedder, tool,
 * datasource, guardrail or plugin (allow_internal_host). On, the asset's own
 * host may be an internal address (localhost, a private or in-cluster
 * address) while outbound blocking stays on for everything else. Only an
 * administrator may change it; anyone else sees its state, read-only, when
 * it is on. The environment settings (LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS,
 * LLM_UPSTREAM_BLOCK_INTERNAL) still apply on top.
 */
const AllowInternalHostSwitch = ({ checked, onChange, hostLabel = 'endpoint', disabled = false }) => {
  const { isAdmin } = usePermissions();
  if (!isAdmin) {
    return checked ? (
      <Typography variant="bodyMediumDefault" color="text.defaultSubdued" data-testid="allow-internal-host-readonly">
        Internal host allowed (set by an administrator)
      </Typography>
    ) : null;
  }
  return (
    <>
      <FormControlLabel
        control={
          <Switch
            checked={!!checked}
            onChange={(e) => onChange(e.target.checked)}
            name="allow_internal_host"
            color="primary"
            disabled={disabled}
            inputProps={{ 'data-testid': 'allow-internal-host-switch' }}
          />
        }
        label="Allow internal host"
      />
      <FormHelperText sx={{ mt: 0, ml: 0 }}>
        Lets this {hostLabel} be an internal address (localhost, a private or in-cluster host). Outbound
        calls to internal addresses stay blocked for everything else.
      </FormHelperText>
      {checked && (
        <Alert severity="warning" sx={{ mt: 1 }}>
          Only allow hosts you control: an internal address can reach services that are not meant to be
          exposed.
        </Alert>
      )}
    </>
  );
};

export default AllowInternalHostSwitch;
