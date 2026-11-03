import React, { useEffect, useState } from "react";
import {
  Alert,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Grid,
  Typography,
} from "@mui/material";
import { FieldLabel, FieldValue, PrimaryButton, SecondaryOutlineButton } from "../../../styles/sharedStyles";
import PromptTemplateStatus from "../promptTemplates/PromptTemplateStatus";
import {
  apiErrorMessage,
  approveGuardrailException,
  channelLabel,
  exceptionActionLabel,
  exceptionExpiresAt,
  isGuardrailExceptionChanged,
} from "../../../services/guardrailExceptionService";

const formatTime = (s) => (s ? new Date(s).toLocaleString() : "Never");

// GuardrailExceptionReview shows an exception as it will apply. A full
// administrator approves exactly the version shown (its content hash); one
// that changed meanwhile is reloaded instead of approved.
const GuardrailExceptionReview = ({ open, appId, exception, filterName, canApprove, userName, onClose, onReload, onApproved }) => {
  const [confirmed, setConfirmed] = useState(false);
  const [error, setError] = useState(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setConfirmed(false);
    setError(null);
  }, [open, exception]);

  if (!exception) return null;
  const approvable = canApprove && exception.status === "proposed";
  const expiry = exceptionExpiresAt(exception);

  const approve = async () => {
    setSaving(true);
    setError(null);
    try {
      onApproved(await approveGuardrailException(appId, exception.id, exception.content_hash));
    } catch (e) {
      if (isGuardrailExceptionChanged(e)) {
        setError("The exception changed since you opened it. It has been reloaded: review it again.");
        onReload();
      } else {
        setError(apiErrorMessage(e, "Could not approve the exception"));
      }
    } finally {
      setSaving(false);
    }
  };

  const row = (label, value) => (
    <>
      <Grid item xs={4}>
        <FieldLabel>{label}</FieldLabel>
      </Grid>
      <Grid item xs={8}>
        <FieldValue>{value}</FieldValue>
      </Grid>
    </>
  );

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Guardrail exception #{exception.id}</DialogTitle>
      <DialogContent>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        <Grid container spacing={1} data-testid="gx-review">
          {row("Status", <PromptTemplateStatus status={exception.status} />)}
          {row("Filter", filterName(exception.filter_id))}
          {row("Detector", exception.detector || "Any detector")}
          {row("Where", channelLabel(exception.channel))}
          {exception.tool_name && row("Tool", exception.tool_name)}
          {row("Instead of blocking", exceptionActionLabel(exception.action))}
          {exception.action === "alert" && row("Raises event", `guardrail.alert.${exception.alert_event || ""}`)}
          {row("Justification", exception.justification)}
          {row("Proposed by", userName(exception.proposed_by))}
          {exception.approved_at && row("Approved", `${userName(exception.approved_by)}, ${formatTime(exception.approved_at)}`)}
          {row("Hits", `${exception.hit_count || 0} (last ${formatTime(exception.last_hit_at)})`)}
          {row(
            "Expires",
            expiry ? formatTime(expiry) : `${exception.expires_after_days} days without a hit, once approved`
          )}
          {exception.source_event && row("From compliance event", `#${exception.source_event}`)}
        </Grid>
        {approvable && (
          <>
            <Typography variant="body2" sx={{ mt: 2 }}>
              Approving lets this app's matching traffic through the filter with the action above instead of the
              filter's own, for as long as the exception keeps applying.
            </Typography>
            <FormControlLabel
              sx={{ mt: 1 }}
              control={
                <Checkbox
                  checked={confirmed}
                  onChange={(e) => setConfirmed(e.target.checked)}
                  inputProps={{ "data-testid": "gx-confirm" }}
                />
              }
              label="I have checked the filter, detector, channel and action"
            />
          </>
        )}
      </DialogContent>
      <DialogActions>
        <SecondaryOutlineButton onClick={onClose}>Close</SecondaryOutlineButton>
        {approvable && (
          <PrimaryButton onClick={approve} disabled={!confirmed || saving} data-testid="gx-approve">
            Approve
          </PrimaryButton>
        )}
      </DialogActions>
    </Dialog>
  );
};

export default GuardrailExceptionReview;
