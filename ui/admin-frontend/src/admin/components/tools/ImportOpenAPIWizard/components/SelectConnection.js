import React, { useEffect, useState } from "react";
import {
  Alert,
  Box,
  Chip,
  CircularProgress,
  FormControlLabel,
  Paper,
  Radio,
  RadioGroup,
  Typography,
  alpha,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import { SecondaryOutlineButton } from "../../../../styles/sharedStyles";
import { usePermissions } from "../../../../context/PermissionsContext";
import { P } from "../../../../rbac/permissions";
import { ConnectionStatusChip, TykUpsell, TykDisabledNotice } from "../../../../pages/tykShared";
import QuickConnectForm from "./QuickConnectForm";

/**
 * SelectConnection is the wizard step that picks the Tyk connection to
 * import from. It soft-gates like the admin pages: the enterprise prompt in
 * CE, the disabled notice when the integration is off, otherwise the saved
 * connections plus an inline way to add one.
 */
const SelectConnection = ({ status, connections, loading, error, selected, onSelect, reload, onCreated }) => {
  const { can } = usePermissions();
  const canAdd = can(P.TYK_CONNECTIONS_WRITE);
  const [adding, setAdding] = useState(false);

  useEffect(() => {
    reload().catch(() => {});
  }, [reload]);

  if (loading && !status) {
    return (
      <Box display="flex" justifyContent="center" my={4}>
        <CircularProgress />
      </Box>
    );
  }
  if (status && !status.available) return <TykUpsell />;
  if (status && !status.enabled) return <TykDisabledNotice status={status} />;

  const handleCreated = async (connection) => {
    setAdding(false);
    await reload().catch(() => {});
    onSelect({
      id: connection.id,
      name: connection.name,
      dashboard_url: connection.dashboard_url,
      status: connection.status,
      degraded: connection.degraded,
      effective_mode: connection.effective_mode,
      apis_read: connection.capabilities?.apis_read?.state || "",
    });
    if (onCreated) onCreated(connection);
  };

  return (
    <Box>
      <Typography variant="h6" gutterBottom>
        Choose a Tyk connection
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        The Dashboard whose OAS APIs you want to import. The same connections serve the MCP server catalogue.
      </Typography>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      {connections.length === 0 && !loading && (
        <Alert severity="info" sx={{ mb: 2 }} data-testid="no-connections">
          {canAdd
            ? "No Tyk connection yet. Add one below to import from a Dashboard."
            : "No Tyk connection is available. Ask an administrator to add one under Settings → Tyk Connections."}
        </Alert>
      )}

      <RadioGroup
        value={selected ? String(selected.id) : ""}
        onChange={(e) => {
          const c = connections.find((row) => String(row.id) === e.target.value);
          if (c) onSelect(c);
        }}
      >
        {connections.map((c) => {
          const isSelected = selected?.id === c.id;
          return (
            <Paper
              key={c.id}
              elevation={0}
              variant="outlined"
              sx={{
                mb: 1.5,
                p: 1.5,
                borderColor: (theme) => (isSelected ? theme.palette.primary.main : "inherit"),
                bgcolor: (theme) => (isSelected ? alpha(theme.palette.primary.main, 0.04) : "inherit"),
              }}
              data-testid={`connection-${c.id}`}
            >
              <FormControlLabel
                value={String(c.id)}
                control={<Radio />}
                sx={{ m: 0, width: "100%" }}
                label={
                  <Box sx={{ ml: 1 }}>
                    <Box display="flex" alignItems="center" gap={1} flexWrap="wrap">
                      <Typography variant="subtitle1" color="text.primary">
                        {c.name}
                      </Typography>
                      <ConnectionStatusChip status={c.status} degraded={c.degraded} />
                      {c.apis_read === "denied" && (
                        <Chip size="small" color="error" variant="outlined" label="Cannot read APIs" />
                      )}
                    </Box>
                    <Typography variant="body2" color="text.secondary">
                      {c.dashboard_url}
                      {c.degraded && c.degraded_reason ? ` · ${c.degraded_reason}` : ""}
                    </Typography>
                  </Box>
                }
              />
            </Paper>
          );
        })}
      </RadioGroup>

      {canAdd && !adding && (
        <SecondaryOutlineButton startIcon={<AddIcon />} onClick={() => setAdding(true)} data-testid="add-connection">
          Add connection
        </SecondaryOutlineButton>
      )}
      {canAdd && adding && <QuickConnectForm onCreated={handleCreated} onCancel={() => setAdding(false)} />}
    </Box>
  );
};

export default SelectConnection;
