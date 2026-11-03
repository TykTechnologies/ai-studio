import React, { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormControlLabel,
  FormHelperText,
  Grid,
  InputLabel,
  MenuItem,
  Radio,
  RadioGroup,
  Select,
  TextField,
  Typography,
} from "@mui/material";
import { Link as RouterLink } from "react-router-dom";
import { PrimaryButton, SecondaryOutlineButton } from "../../styles/sharedStyles";
import { usePermissions } from "../../context/PermissionsContext";
import {
  CAPABILITY_OPTIONS,
  capabilityLabel,
  getGuardrailCapability,
  setGuardrailCapability,
} from "../../services/guardrailCapabilityService";
import {
  allowedActions,
  apiErrorMessage,
  channelLabel,
  createGuardrailException,
  exceptionActionLabel,
  listGuardrailExceptions,
} from "../../services/guardrailExceptionService";

const DEFAULT_EXPIRY_DAYS = 30;

// SoftenDialog fixes a false positive from the event that showed it: an
// exception for this app, filter, detector and channel, pre-filled from the
// event (target, softenTarget). It asks what the app can do only when nobody
// has attested it (Studio's suggestion pre-selected), and offers only the
// actions that capability allows, the softest first. A full administrator's
// exception applies at once; anyone else's is proposed.
const SoftenDialog = ({ open, target, onClose, onDone }) => {
  const { isFullAdmin } = usePermissions();
  const [capability, setCapability] = useState(null);
  const [meta, setMeta] = useState(null);
  const [loadError, setLoadError] = useState(null);
  const [attest, setAttest] = useState("");
  const [action, setAction] = useState("");
  const [days, setDays] = useState(DEFAULT_EXPIRY_DAYS);
  const [justification, setJustification] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
  const [result, setResult] = useState(null);

  useEffect(() => {
    if (!open || !target) return undefined;
    let cancelled = false;
    setCapability(null);
    setMeta(null);
    setLoadError(null);
    setError(null);
    setResult(null);
    setJustification("");
    setDays(DEFAULT_EXPIRY_DAYS);
    Promise.all([getGuardrailCapability(target.appId), listGuardrailExceptions(target.appId)])
      .then(([cap, list]) => {
        if (cancelled) return;
        setCapability(cap);
        setMeta(list.meta);
        setAttest(cap?.value || cap?.suggestion || "acting");
      })
      .catch((e) => {
        if (!cancelled) setLoadError(apiErrorMessage(e, "Could not load the app's guardrail settings"));
      });
    return () => {
      cancelled = true;
    };
  }, [open, target]);

  const attested = !!capability?.value;
  // The capability the floors are read for: the attested one; the one being
  // attested here by a full administrator; else acting (unattested).
  const effective = attested ? capability.value : isFullAdmin ? attest : "acting";
  const actions = useMemo(
    () => (target ? allowedActions(meta, effective, target.channel) : []),
    [meta, effective, target]
  );
  useEffect(() => {
    if (!actions.includes(action)) setAction(actions[0] || "");
  }, [actions, action]);

  if (!target) return null;

  const submit = async () => {
    setSaving(true);
    setError(null);
    try {
      if (!attested && isFullAdmin) {
        await setGuardrailCapability(target.appId, attest);
      }
      const saved = await createGuardrailException(target.appId, {
        filter_id: target.filterId,
        detector: target.detector,
        channel: target.channel,
        tool_name: target.toolName || "",
        action,
        justification: justification.trim(),
        expires_after_days: Number(days),
        source_event: target.eventId,
      });
      setResult(saved);
      if (onDone) onDone(saved);
    } catch (e) {
      setError(apiErrorMessage(e, "Could not create the exception"));
    } finally {
      setSaving(false);
    }
  };

  const loading = !capability && !loadError;
  const valid = action && justification.trim() && Number(days) >= 1;

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Not a threat? Soften for this app</DialogTitle>
      <DialogContent>
        {result ? (
          <Alert severity="success" data-testid="soften-result">
            {result.status === "approved"
              ? `Exception #${result.id} applies now: ${target.filterName} will ${exceptionActionLabel(result.action).toLowerCase()} instead of blocking this for ${target.appName}.`
              : `Exception #${result.id} is proposed. An administrator must approve it on the app's page before it has any effect.`}{" "}
            <RouterLink to={`/admin/apps/${target.appId}`}>Open the app</RouterLink>
          </Alert>
        ) : (
          <>
            <Box sx={{ mb: 2 }} data-testid="soften-scope">
              <Typography variant="body2">
                <strong>{target.filterName}</strong> flagged{" "}
                <Box component="span" sx={{ fontFamily: "monospace" }}>
                  {target.detector}
                </Box>{" "}
                in {channelLabel(target.channel).toLowerCase()}
                {target.toolName ? ` (${target.toolName})` : ""} of <strong>{target.appName}</strong>.
              </Typography>
              <Typography variant="caption" color="text.secondary">
                The exception covers only this app, filter, detector and place. The filter still detects it and every
                hit is recorded.
              </Typography>
            </Box>
            {loadError && (
              <Alert severity="error" sx={{ mb: 2 }}>
                {loadError}
              </Alert>
            )}
            {error && (
              <Alert severity="error" sx={{ mb: 2 }} data-testid="soften-error">
                {error}
              </Alert>
            )}
            {loading ? (
              <Box display="flex" justifyContent="center" p={2}>
                <CircularProgress size={24} />
              </Box>
            ) : (
              capability && (
                <Grid container spacing={2}>
                  {!attested && (
                    <Grid item xs={12} data-testid="soften-capability">
                      <Typography variant="subtitle2">What can this app do?</Typography>
                      {isFullAdmin ? (
                        <>
                          <Typography variant="caption" color="text.secondary" component="div">
                            Studio suggests {capabilityLabel(capability.suggestion)}
                            {capability.reasons?.length ? `: ${capability.reasons.join("; ")}` : ""}.
                          </Typography>
                          <RadioGroup value={attest} onChange={(e) => setAttest(e.target.value)}>
                            {CAPABILITY_OPTIONS.map((o) => (
                              <FormControlLabel
                                key={o.value}
                                value={o.value}
                                control={<Radio size="small" />}
                                label={`${o.label}${o.value === capability.suggestion ? " (suggested)" : ""}: ${o.help}`}
                              />
                            ))}
                          </RadioGroup>
                        </>
                      ) : (
                        <Typography variant="body2" color="text.secondary">
                          Nobody has attested this yet, so the app is treated as acting (the strictest). A full
                          administrator can attest it on the app's page; Studio suggests{" "}
                          {capabilityLabel(capability.suggestion)}.
                        </Typography>
                      )}
                    </Grid>
                  )}
                  <Grid item xs={12} md={7}>
                    <FormControl fullWidth error={actions.length === 0}>
                      <InputLabel id="soften-action-label">Instead of blocking</InputLabel>
                      <Select
                        labelId="soften-action-label"
                        label="Instead of blocking"
                        value={actions.includes(action) ? action : ""}
                        onChange={(e) => setAction(e.target.value)}
                        disabled={actions.length === 0}
                      >
                        {actions.map((a) => (
                          <MenuItem key={a} value={a}>
                            {exceptionActionLabel(a)}
                          </MenuItem>
                        ))}
                      </Select>
                      <FormHelperText data-testid="soften-action-help">
                        {actions.length === 0
                          ? `An app that is ${capabilityLabel(effective).toLowerCase()} cannot be softened here.`
                          : `What an app that is ${capabilityLabel(effective).toLowerCase()} allows.`}
                      </FormHelperText>
                    </FormControl>
                  </Grid>
                  <Grid item xs={12} md={5}>
                    <TextField
                      fullWidth
                      type="number"
                      label="Expires after (days)"
                      value={days}
                      onChange={(e) => setDays(e.target.value)}
                      helperText="Since it last applied."
                      inputProps={{ min: 1, max: 3650, "data-testid": "soften-days" }}
                    />
                  </Grid>
                  <Grid item xs={12}>
                    <TextField
                      fullWidth
                      required
                      label="Justification"
                      value={justification}
                      onChange={(e) => setJustification(e.target.value)}
                      placeholder="Why this traffic is legitimate"
                      inputProps={{ maxLength: 500, "data-testid": "soften-justification" }}
                    />
                  </Grid>
                  {!isFullAdmin && (
                    <Grid item xs={12}>
                      <Typography variant="caption" color="text.secondary">
                        Your exception is proposed: an administrator approves it before it has any effect.
                      </Typography>
                    </Grid>
                  )}
                </Grid>
              )
            )}
          </>
        )}
      </DialogContent>
      <DialogActions>
        <SecondaryOutlineButton onClick={onClose}>{result ? "Close" : "Cancel"}</SecondaryOutlineButton>
        {!result && (
          <PrimaryButton onClick={submit} disabled={!valid || saving || loading} data-testid="soften-submit">
            {isFullAdmin ? "Soften" : "Propose"}
          </PrimaryButton>
        )}
      </DialogActions>
    </Dialog>
  );
};

export default SoftenDialog;
