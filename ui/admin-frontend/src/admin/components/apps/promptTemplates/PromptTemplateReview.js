import React, { useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Grid,
  Typography,
} from "@mui/material";
import { PrimaryButton, SecondaryOutlineButton, FieldLabel, FieldValue } from "../../../styles/sharedStyles";
import VisibleText, { VisibleRun, monoBlockSx } from "./VisibleText";
import PlacementDiagram from "./PlacementDiagram";
import PromptTemplateStatus from "./PromptTemplateStatus";
import { constBytes, expiresAt, isConst, placementLabel, previousApproved } from "./promptTemplateText";
import { diffParts, isSmallEdit, samePlacement } from "./templateDiff";
import { approvePromptTemplate, apiErrorMessage } from "../../../services/promptTemplateService";

// The approval view. Approving exempts the template's constants from
// request guardrail scoring for every request of the app, so the reviewer
// sees the exact bytes, where the template sits, and what changed since
// the last approved version, and must confirm.

export const SMALL_EDIT_WARNING = "A small edit is where an injection fits. Check every changed character.";

const formatTime = (s) => (s ? new Date(s).toLocaleString() : "Never");

const insSx = { bgcolor: "rgba(46, 125, 50, 0.18)", textDecoration: "underline" };
const delSx = { bgcolor: "rgba(211, 47, 47, 0.15)", textDecoration: "line-through" };

/** DiffView shows the change between two versions' parts. */
export const DiffView = ({ diff }) => (
  <Box component="pre" sx={monoBlockSx} data-testid="template-diff">
    {diff.hunks.map((h, hi) => {
      // Consecutive constant units are one run of text; a slot is a chip.
      const runs = [];
      for (const u of h.units) {
        const lastRun = runs[runs.length - 1];
        if (u.t === "c" && lastRun && lastRun.t === "c") lastRun.v += u.v;
        else runs.push({ ...u });
      }
      const sx = h.op === "ins" ? insSx : h.op === "del" ? delSx : {};
      return (
        <Box component="span" key={hi} sx={sx} data-op={h.op} data-testid={h.op === "eq" ? undefined : `diff-${h.op}`}>
          {runs.map((r, ri) =>
            r.t === "c" ? (
              <VisibleRun key={ri} text={r.v} atEnd={false} />
            ) : (
              <Box component="span" key={ri} sx={{ color: "warning.main", fontWeight: 600 }}>
                {`{slot: ${r.v}}`}
              </Box>
            )
          )}
        </Box>
      );
    })}
  </Box>
);

const Changes = ({ template, previous }) => {
  const diff = useMemo(
    () => (previous ? diffParts(previous.parts, template.parts) : null),
    [previous, template]
  );
  if (!previous) {
    return (
      <Alert severity="info">
        No earlier version of {template.name} was approved: this is new text. Review every character.
      </Alert>
    );
  }
  const placementChanged = !samePlacement(previous.placement, template.placement);
  return (
    <Box>
      <Typography variant="body2" sx={{ mb: 1 }}>
        Compared with {previous.ref || `${previous.name}@${previous.version}`} ({previous.status}).
      </Typography>
      {placementChanged && (
        <Alert severity="warning" sx={{ mb: 1 }}>
          Placement changed: {placementLabel(previous.placement)} to {placementLabel(template.placement)}.
        </Alert>
      )}
      {diff.tooLarge ? (
        <Alert severity="warning">The versions differ too much to compare. Review every character.</Alert>
      ) : diff.identical ? (
        <Typography variant="body2" color="text.secondary">
          The parts are identical to the earlier version.
        </Typography>
      ) : (
        <>
          {isSmallEdit(diff) && (
            <Alert severity="warning" sx={{ mb: 1 }} data-testid="small-edit-warning">
              {SMALL_EDIT_WARNING}
            </Alert>
          )}
          <Typography variant="caption" color="text.secondary" component="div" sx={{ mb: 0.5 }}>
            {diff.changedBytes} of {diff.totalBytes} constant bytes changed, in {diff.runs}{" "}
            {diff.runs === 1 ? "place" : "places"}. Inserted text is underlined; removed text is struck through.
          </Typography>
          <DiffView diff={diff} />
        </>
      )}
    </Box>
  );
};

const PromptTemplateReview = ({ open, appId, template, all, canApprove, userName, onClose, onApproved }) => {
  const [confirming, setConfirming] = useState(false);
  const [checked, setChecked] = useState(false);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);

  if (!template) return null;
  const previous = previousApproved(template, all);
  const consts = (template.parts || []).filter(isConst);
  const expiry = expiresAt(template);
  const name = (id) => (id ? (userName ? userName(id) : `User #${id}`) : "-");

  const close = () => {
    setConfirming(false);
    setChecked(false);
    setError(null);
    onClose();
  };

  const approve = async () => {
    setBusy(true);
    setError(null);
    try {
      const approved = await approvePromptTemplate(appId, template.id);
      setConfirming(false);
      setChecked(false);
      onApproved?.(approved);
    } catch (e) {
      setError(apiErrorMessage(e, "Could not approve the template"));
      setConfirming(false);
    } finally {
      setBusy(false);
    }
  };

  const facts = [
    ["Status", <PromptTemplateStatus status={template.status} />],
    ["Proposed", `${formatTime(template.created_at)} by ${name(template.created_by)}`],
    ["Approved", template.approved_at ? `${formatTime(template.approved_at)} by ${name(template.approved_by)}` : "Not approved"],
    ["Last matched", formatTime(template.last_matched_at)],
    [
      "Expires",
      template.status === "approved"
        ? `${expiry ? expiry.toLocaleString() : "-"} without a match (${template.expires_after_days} days)`
        : `${template.expires_after_days} days without a match, once approved`,
    ],
  ];

  return (
    <>
      <Dialog open={open} onClose={close} maxWidth="md" fullWidth aria-labelledby="prompt-template-review-title">
        <DialogTitle id="prompt-template-review-title">
          Review {template.name}@{template.version}
        </DialogTitle>
        <DialogContent dividers>
          {error && (
            <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
              {error}
            </Alert>
          )}
          <Grid container spacing={1} sx={{ mb: 2 }}>
            {facts.map(([label, value]) => (
              <React.Fragment key={label}>
                <Grid item xs={3}>
                  <FieldLabel>{label}:</FieldLabel>
                </Grid>
                <Grid item xs={9}>
                  <FieldValue component="div">{value}</FieldValue>
                </Grid>
              </React.Fragment>
            ))}
          </Grid>

          <Typography variant="headingSmall" component="h3" sx={{ mb: 1 }}>
            Where it sits
          </Typography>
          <Box sx={{ mb: 2 }}>
            <PlacementDiagram placement={template.placement} parts={template.parts} />
          </Box>

          <Typography variant="headingSmall" component="h3" sx={{ mb: 1 }}>
            Changes
          </Typography>
          <Box sx={{ mb: 2 }}>
            <Changes template={template} previous={previous} />
          </Box>

          <Typography variant="headingSmall" component="h3" sx={{ mb: 0.5 }}>
            Exact bytes ({constBytes(template.parts)} bytes of constant text)
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            Approved constants are not scored. Invisible and non-ASCII characters are shown as labelled markers;
            a faint return symbol marks each line break.
          </Typography>
          {consts.map((p, i) => (
            <Box key={i} sx={{ mb: 1.5 }}>
              <Typography variant="caption" color="text.secondary">
                Constant {i + 1}
              </Typography>
              <VisibleText text={p.const} label={`Constant ${i + 1}`} />
            </Box>
          ))}
        </DialogContent>
        <DialogActions>
          <SecondaryOutlineButton onClick={close}>Close</SecondaryOutlineButton>
          {canApprove && template.status === "proposed" && (
            <PrimaryButton variant="contained" onClick={() => setConfirming(true)} disabled={busy}>
              Approve
            </PrimaryButton>
          )}
        </DialogActions>
      </Dialog>

      <Dialog open={confirming} onClose={() => setConfirming(false)} aria-labelledby="prompt-template-confirm-title">
        <DialogTitle id="prompt-template-confirm-title">
          Approve {template.name}@{template.version}?
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2" sx={{ mb: 1 }}>
            Request guardrails will stop scoring these {constBytes(template.parts)} bytes of constant text in this app's
            requests.
            {previous && template.status === "proposed" && previous.status === "approved"
              ? ` ${previous.name}@${previous.version} will be retired.`
              : ""}
          </Typography>
          <FormControlLabel
            control={<Checkbox checked={checked} onChange={(e) => setChecked(e.target.checked)} />}
            label="I have checked every constant, character by character."
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirming(false)}>Cancel</Button>
          <PrimaryButton variant="contained" onClick={approve} disabled={!checked || busy}>
            Approve template
          </PrimaryButton>
        </DialogActions>
      </Dialog>
    </>
  );
};

export default PromptTemplateReview;
