import React, { useCallback, useEffect, useState } from "react";
import { Alert, Box, Grid, MenuItem, TextField, Typography } from "@mui/material";
import apiClient from "../../utils/apiClient";
import Section from "../common/Section";
import ConfirmationDialog from "../common/ConfirmationDialog";
import Can from "../rbac/Can";
import { P } from "../../rbac/permissions";
import { FieldLabel, FieldValue, SecondaryOutlineButton } from "../../styles/sharedStyles";

// Vendors whose native API extensions hand out signed resource IDs (Anthropic
// Message Batches, OpenAI Responses; vendors/*/nativeapi.go). Every LLM has a
// signing key, but only these use it.
export const NATIVE_RESOURCE_VENDORS = ["anthropic", "openai"];

const SOURCE_LABELS = {
  secret: "Hub-generated secret",
  // The LLM has no secret yet; it gets one at the next Studio start or rotation.
  vendor_credential: "Derived from the API key (not upgraded yet)",
  none: "None (no secret and no API key)",
};

// A previous vendor_credential key is the one stored at the upgrade, so its
// kid stays put when the API key changes.
const PREVIOUS_SOURCE_LABELS = {
  ...SOURCE_LABELS,
  vendor_credential: "Pre-upgrade key (IDs minted before the upgrade)",
};

// grace_period_hours choices; the API default is 720.
const GRACE_OPTIONS = [
  { hours: 720, label: "30 days (default)" },
  { hours: 168, label: "7 days" },
  { hours: 24, label: "1 day" },
  { hours: 0, label: "Revoke immediately" },
];

const formatTime = (iso) => (iso ? new Date(iso).toLocaleString() : "");

const KID = ({ children }) => (
  <FieldValue component="span" sx={{ fontFamily: "monospace" }}>
    {children}
  </FieldValue>
);

/**
 * Status and rotation of the key that signs an LLM's native API resource IDs
 * (batch and response IDs, `{vendor_id}.t1k{kid}.{mac}`). Shows key IDs only:
 * the API never returns the secret.
 */
const ResourceSigningKeySection = ({ llmId }) => {
  const [status, setStatus] = useState(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [graceHours, setGraceHours] = useState(720);
  const [rotating, setRotating] = useState(false);
  const [message, setMessage] = useState(null);

  const load = useCallback(async () => {
    try {
      const res = await apiClient.get(`/llms/${llmId}/resource-signing-key`);
      setStatus(res.data?.data || null);
    } catch (err) {
      setStatus(null);
    }
  }, [llmId]);

  useEffect(() => {
    load();
  }, [load]);

  const openDialog = () => {
    setGraceHours(720);
    setDialogOpen(true);
  };

  const rotate = async () => {
    setDialogOpen(false);
    setRotating(true);
    setMessage(null);
    try {
      const res = await apiClient.post(`/llms/${llmId}/resource-signing-key/rotate`, {
        grace_period_hours: graceHours,
      });
      setStatus(res.data?.data || null);
      setMessage({ severity: "success", text: "Signing key rotated. Edges get the new key on their next configuration push." });
    } catch (err) {
      const conflict = err?.response?.status === 409;
      setMessage({
        severity: "error",
        text: conflict
          ? "The key was rotated by someone else at the same time. The status below has been reloaded."
          : err?.response?.data?.errors?.[0]?.detail || "Failed to rotate the signing key.",
      });
      await load();
    } finally {
      setRotating(false);
    }
  };

  if (!status) return null;

  const previous = status.previous;
  const previousStillActive = previous && previous.active;

  return (
    <Section
      title="Native API resource IDs"
      description="The key that signs the resource IDs this LLM's native API extensions hand out (for example batch or response IDs). The kid is the 8 characters after .t1k in an ID."
      actions={
        <Can permission={P.LLMS_WRITE}>
          <SecondaryOutlineButton onClick={openDialog} disabled={rotating}>
            Rotate signing key
          </SecondaryOutlineButton>
        </Can>
      }
    >
      {message && (
        <Alert severity={message.severity} sx={{ mb: 2 }} onClose={() => setMessage(null)}>
          {message.text}
        </Alert>
      )}
      <Grid container spacing={2}>
        <Grid item xs={3}>
          <FieldLabel>Current key:</FieldLabel>
        </Grid>
        <Grid item xs={9}>
          {status.kid ? <KID>{status.kid}</KID> : <FieldValue component="span">-</FieldValue>}
          <Typography variant="body2" color="text.secondary">
            {SOURCE_LABELS[status.source] || status.source}
            {status.rotated_at && ` · rotated ${formatTime(status.rotated_at)}`}
          </Typography>
        </Grid>
        {previous && (
          <>
            <Grid item xs={3}>
              <FieldLabel>Previous key:</FieldLabel>
            </Grid>
            <Grid item xs={9}>
              {previous.kid ? <KID>{previous.kid}</KID> : <FieldValue component="span">-</FieldValue>}
              <Typography variant="body2" color="text.secondary">
                {PREVIOUS_SOURCE_LABELS[previous.source] || previous.source}
                {" · "}
                {previous.active
                  ? `Accepted until ${formatTime(previous.expires_at)}`
                  : `Expired ${formatTime(previous.expires_at)}: IDs it signed no longer verify`}
              </Typography>
            </Grid>
          </>
        )}
      </Grid>

      <ConfirmationDialog
        open={dialogOpen}
        data-testid="rotate-signing-key-dialog"
        title="Rotate signing key"
        message={
          <Box component="span" sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
            <span>
              New resource IDs will be signed with a new key. IDs signed with the current key
              {status.kid ? ` (${status.kid})` : ""} keep working for the grace period you choose.
              Edges get the new key on their next configuration push.
            </span>
            {previousStillActive && (
              <span>
                Only the key being replaced is kept: the previous key ({previous.kid}) stops verifying
                now, before its grace period ends.
              </span>
            )}
            <TextField
              select
              size="small"
              id="signing-key-grace"
              label="Keep the current key valid for"
              value={graceHours}
              onChange={(e) => setGraceHours(Number(e.target.value))}
              sx={{ mt: 1, maxWidth: 320 }}
            >
              {GRACE_OPTIONS.map((o) => (
                <MenuItem key={o.hours} value={o.hours}>
                  {o.label}
                </MenuItem>
              ))}
            </TextField>
          </Box>
        }
        confirmText=""
        buttonLabel="Rotate"
        onConfirm={rotate}
        onCancel={() => setDialogOpen(false)}
        iconName="hexagon-exclamation"
        iconColor="background.buttonCritical"
        titleColor="text.criticalDefault"
        backgroundColor="background.surfaceCriticalDefault"
        borderColor="border.criticalDefaultSubdue"
        primaryButtonComponent="danger"
      />
    </Section>
  );
};

export default ResourceSigningKeySection;
