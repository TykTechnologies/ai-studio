import React, { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Radio,
  RadioGroup,
  Tooltip,
  Typography,
} from "@mui/material";
import { usePermissions } from "../../context/PermissionsContext";
import {
  CAPABILITY_OPTIONS,
  capabilityLabel,
  getGuardrailCapability,
  setGuardrailCapability,
} from "../../services/guardrailCapabilityService";

// The app's guardrail capability (features/GuardrailExceptions.md): what a
// full administrator attests the app can do. It bounds how far a guardrail
// exception may soften a verdict for the app; an app nobody has attested is
// treated as acting, the strictest. Studio suggests a value from the app's
// tools, MCP servers and datasources; the administrator attests it, since
// Studio cannot see everything the app can do.

const errorMessage = (err) =>
  err?.response?.data?.errors?.[0]?.detail || err?.message || "Could not save the guardrail capability";

const ReasonList = ({ reasons }) =>
  reasons && reasons.length > 0 ? (
    <Box component="ul" sx={{ m: 0, pl: 2.5 }} data-testid="capability-reasons">
      {reasons.map((r) => (
        <li key={r}>
          <Typography variant="body2">{r}</Typography>
        </li>
      ))}
    </Box>
  ) : null;

// onChange, when set, is told the capability each time it loads or changes
// (the exceptions table offers only what it allows).
const AppGuardrailCapability = ({ appId, onChange }) => {
  const { isFullAdmin } = usePermissions();
  const [cap, setCap] = useState(null);
  const [error, setError] = useState(null);
  const [open, setOpen] = useState(false);
  const [choice, setChoice] = useState("");
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    try {
      setCap(await getGuardrailCapability(appId));
      setError(null);
    } catch (err) {
      setError(errorMessage(err));
    }
  }, [appId]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (cap && onChange) onChange(cap);
  }, [cap, onChange]);

  const save = async (value) => {
    setSaving(true);
    try {
      setCap(await setGuardrailCapability(appId, value));
      setError(null);
      setOpen(false);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const openDialog = () => {
    setChoice(cap?.value || cap?.suggestion || "acting");
    setOpen(true);
  };

  if (!cap) {
    return error ? (
      <Typography variant="body2" color="error">
        {error}
      </Typography>
    ) : (
      <Typography variant="body2">Loading...</Typography>
    );
  }

  const attested = !!cap.value;
  return (
    <Box data-testid="app-guardrail-capability">
      <Box display="flex" alignItems="center" flexWrap="wrap" gap={1}>
        <Tooltip
          title={
            attested
              ? "Attested by an administrator. It bounds how far guardrail exceptions may soften a verdict for this app."
              : "Nobody has attested what this app can do, so guardrail exceptions treat it as acting (the strictest)."
          }
        >
          <Chip
            size="small"
            variant="outlined"
            color={attested ? "default" : "warning"}
            label={attested ? capabilityLabel(cap.value) : "Not attested (treated as acting)"}
            data-testid="capability-chip"
          />
        </Tooltip>
        {isFullAdmin && !attested && cap.suggestion && (
          <Button size="small" variant="outlined" disabled={saving} onClick={() => save(cap.suggestion)}>
            Accept suggestion: {capabilityLabel(cap.suggestion)}
          </Button>
        )}
        {isFullAdmin && (
          <Button size="small" onClick={openDialog}>
            Change
          </Button>
        )}
      </Box>
      {!attested && cap.reset_at && (
        <Typography variant="caption" color="warning.main" component="div" sx={{ mt: 0.5 }} data-testid="capability-reset">
          Cleared on {new Date(cap.reset_at).toLocaleString()} after a change to the app's tools or resources
          {cap.reset_by ? ` by user #${cap.reset_by}` : ""}; a full administrator needs to attest it again.
        </Typography>
      )}
      {!attested && cap.suggestion && (
        <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 0.5 }}>
          Suggested: {capabilityLabel(cap.suggestion)}
          {cap.reasons?.length ? ` (${cap.reasons[0]}${cap.reasons.length > 1 ? `, +${cap.reasons.length - 1} more` : ""})` : ""}
        </Typography>
      )}
      {error && (
        <Alert severity="error" sx={{ mt: 1 }}>
          {error}
        </Alert>
      )}

      <Dialog open={open} onClose={() => setOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>What can this app do?</DialogTitle>
        <DialogContent>
          <Typography variant="body2" sx={{ mb: 2 }}>
            This bounds how far a guardrail exception may soften a verdict for the app. Studio suggests{" "}
            <strong>{capabilityLabel(cap.suggestion)}</strong> from what it can see:
          </Typography>
          <ReasonList reasons={cap.reasons} />
          <Typography variant="body2" sx={{ mt: 2, mb: 1 }}>
            Studio cannot see everything the app can do (a CI job's own tokens, say), so confirm it:
          </Typography>
          <RadioGroup value={choice} onChange={(e) => setChoice(e.target.value)}>
            {CAPABILITY_OPTIONS.map((o) => (
              <FormControlLabel
                key={o.value}
                value={o.value}
                control={<Radio />}
                label={
                  <Box>
                    <Typography variant="body2">
                      {o.label}
                      {o.value === cap.suggestion ? " (suggested)" : ""}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      {o.help}
                    </Typography>
                  </Box>
                }
              />
            ))}
          </RadioGroup>
        </DialogContent>
        <DialogActions>
          {attested && (
            <Button color="warning" disabled={saving} onClick={() => save("")}>
              Clear attestation
            </Button>
          )}
          <Button onClick={() => setOpen(false)}>Cancel</Button>
          <Button variant="contained" disabled={saving || !choice} onClick={() => save(choice)}>
            Save
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default AppGuardrailCapability;
