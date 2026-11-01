import React, { useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  IconButton,
  InputLabel,
  MenuItem,
  Paper,
  Select,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import ArrowUpwardIcon from "@mui/icons-material/ArrowUpward";
import ArrowDownwardIcon from "@mui/icons-material/ArrowDownward";
import DeleteIcon from "@mui/icons-material/Delete";
import AddIcon from "@mui/icons-material/Add";
import { PrimaryButton, SecondaryOutlineButton } from "../../../styles/sharedStyles";
import { MarkerSummary } from "./VisibleText";
import {
  ANCHORS,
  DEFAULT_EXPIRES_AFTER_DAYS,
  MESSAGES,
  MIN_CONST_BYTES,
  ROLES,
  describePlacement,
  fromEditorParts,
  toEditorParts,
  utf8Length,
  validateTemplate,
} from "./promptTemplateText";
import { createPromptTemplate, updatePromptTemplate, apiErrorMessage } from "../../../services/promptTemplateService";

// Proposes a template, or changes one. Changing the parts or placement of
// an approved or retired version creates the next version, proposed, which
// needs approving again (the API decides; the dialog says so up front).

const MESSAGE_LABELS = { first: "First message of the role", last: "Last message of the role", any: "Any message of the role" };
const ANCHOR_LABELS = {
  both: "Whole message (starts and ends it)",
  start: "Start of the message (text may follow)",
  end: "End of the message (text may precede)",
};

const emptyForm = () => ({
  name: "",
  placement: { role: "user", message: "any", anchor: "both" },
  parts: [{ kind: "const", const: "" }, { kind: "slot", slot: "" }, { kind: "const", const: "" }],
  expiresAfterDays: DEFAULT_EXPIRES_AFTER_DAYS,
});

const formFrom = (t) => ({
  name: t.name,
  placement: { role: t.placement?.role || "user", message: t.placement?.message || "any", anchor: t.placement?.anchor || "both" },
  parts: toEditorParts(t.parts),
  expiresAfterDays: t.expires_after_days || DEFAULT_EXPIRES_AFTER_DAYS,
});

/** PartsEditor edits an ordered list of constants and slots. */
export const PartsEditor = ({ parts, errors, onChange, showErrors }) => {
  const set = (i, next) => onChange(parts.map((p, j) => (j === i ? next : p)));
  const move = (i, d) => {
    const next = [...parts];
    [next[i], next[i + d]] = [next[i + d], next[i]];
    onChange(next);
  };
  const remove = (i) => onChange(parts.filter((_, j) => j !== i));
  let constNo = 0;
  let slotNo = 0;
  return (
    <Box>
      {parts.map((p, i) => {
        const label = p.kind === "const" ? `Constant ${++constNo}` : `Slot ${++slotNo}`;
        const err = showErrors ? errors?.[i] : null;
        return (
          <Paper key={i} variant="outlined" sx={{ p: 1.5, mb: 1 }} data-testid={`part-${i}`}>
            <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
              <Typography variant="subtitle2" sx={{ flexGrow: 1 }}>
                {label}
              </Typography>
              <Tooltip title="Move up">
                <span>
                  <IconButton size="small" aria-label={`Move ${label} up`} disabled={i === 0} onClick={() => move(i, -1)}>
                    <ArrowUpwardIcon fontSize="small" />
                  </IconButton>
                </span>
              </Tooltip>
              <Tooltip title="Move down">
                <span>
                  <IconButton
                    size="small"
                    aria-label={`Move ${label} down`}
                    disabled={i === parts.length - 1}
                    onClick={() => move(i, 1)}
                  >
                    <ArrowDownwardIcon fontSize="small" />
                  </IconButton>
                </span>
              </Tooltip>
              <Tooltip title="Remove">
                <IconButton size="small" aria-label={`Remove ${label}`} onClick={() => remove(i)}>
                  <DeleteIcon fontSize="small" />
                </IconButton>
              </Tooltip>
            </Box>
            {p.kind === "const" ? (
              <>
                <TextField
                  fullWidth
                  multiline
                  minRows={3}
                  maxRows={14}
                  value={p.const}
                  onChange={(e) => set(i, { ...p, const: e.target.value })}
                  inputProps={{ "aria-label": label, style: { fontFamily: "monospace", fontSize: "0.85rem" } }}
                  error={Boolean(err)}
                  helperText={err || `${utf8Length(p.const)} bytes (at least ${MIN_CONST_BYTES}). Exact text: no trimming or normalisation.`}
                />
                {p.const && <MarkerSummary text={p.const} />}
              </>
            ) : (
              <TextField
                fullWidth
                size="small"
                label="Slot name"
                value={p.slot}
                onChange={(e) => set(i, { ...p, slot: e.target.value })}
                inputProps={{ "aria-label": `${label} name` }}
                error={Boolean(err)}
                helperText={err || "The text here varies per request and is always scored."}
              />
            )}
          </Paper>
        );
      })}
      <Box sx={{ display: "flex", gap: 1 }}>
        <Button size="small" startIcon={<AddIcon />} onClick={() => onChange([...parts, { kind: "const", const: "" }])}>
          Add constant
        </Button>
        <Button size="small" startIcon={<AddIcon />} onClick={() => onChange([...parts, { kind: "slot", slot: "" }])}>
          Add slot
        </Button>
      </Box>
    </Box>
  );
};

const PromptTemplateEditor = ({ open, appId, template, onClose, onSaved }) => {
  const [form, setForm] = useState(emptyForm);
  const [showErrors, setShowErrors] = useState(false);
  const [apiError, setApiError] = useState(null);
  const [saving, setSaving] = useState(false);
  const editing = Boolean(template);

  useEffect(() => {
    if (open) {
      setForm(template ? formFrom(template) : emptyForm());
      setShowErrors(false);
      setApiError(null);
    }
  }, [open, template]);

  const validation = validateTemplate(form, { checkName: !editing });
  const invalid = validation.form.length > 0 || Object.keys(validation.parts).length > 0;
  const setPlacement = (k, v) => setForm({ ...form, placement: { ...form.placement, [k]: v } });

  const save = async () => {
    setShowErrors(true);
    setApiError(null);
    if (invalid) return;
    const body = {
      placement: form.placement,
      parts: fromEditorParts(form.parts),
      expires_after_days: Number(form.expiresAfterDays),
    };
    setSaving(true);
    try {
      const saved = editing
        ? await updatePromptTemplate(appId, template.id, body)
        : await createPromptTemplate(appId, { ...body, name: form.name });
      onSaved(saved);
    } catch (e) {
      setApiError(apiErrorMessage(e, "Could not save the template"));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth aria-labelledby="prompt-template-editor-title">
      <DialogTitle id="prompt-template-editor-title">
        {editing ? `Edit ${template.name}@${template.version}` : "Propose a prompt template"}
      </DialogTitle>
      <DialogContent dividers>
        {editing && template.status !== "proposed" && (
          <Alert severity="info" sx={{ mb: 2 }}>
            This version is {template.status}. Changing its text or placement creates a new version, proposed, which an
            administrator must approve again. This version stays as it is.
          </Alert>
        )}
        {apiError && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setApiError(null)} data-testid="api-error">
            {apiError}
          </Alert>
        )}
        {showErrors && validation.form.length > 0 && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {validation.form.map((m) => (
              <div key={m}>{m}</div>
            ))}
          </Alert>
        )}
        <Box sx={{ display: "flex", gap: 2, mb: 2, flexWrap: "wrap" }}>
          <TextField
            label="Name"
            size="small"
            value={form.name}
            disabled={editing}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            helperText={editing ? "A template cannot be renamed." : "e.g. pr-review. Letters, digits, '.', '_' and '-'."}
            sx={{ minWidth: 240 }}
          />
          <TextField
            label="Expires after (days)"
            size="small"
            type="number"
            value={form.expiresAfterDays}
            onChange={(e) => setForm({ ...form, expiresAfterDays: e.target.value })}
            helperText="Without a match, it drops back to proposed."
            inputProps={{ min: 1, max: 3650 }}
            sx={{ width: 220 }}
          />
        </Box>

        <Typography variant="headingSmall" component="h3" sx={{ mb: 1 }}>
          Placement
        </Typography>
        <Box sx={{ display: "flex", gap: 2, mb: 1, flexWrap: "wrap" }}>
          <FormControl size="small" sx={{ minWidth: 140 }}>
            <InputLabel id="pt-role">Role</InputLabel>
            <Select labelId="pt-role" label="Role" value={form.placement.role} onChange={(e) => setPlacement("role", e.target.value)}>
              {ROLES.map((r) => (
                <MenuItem key={r} value={r}>
                  {r}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <FormControl size="small" sx={{ minWidth: 240 }}>
            <InputLabel id="pt-message">Message</InputLabel>
            <Select
              labelId="pt-message"
              label="Message"
              value={form.placement.message}
              onChange={(e) => setPlacement("message", e.target.value)}
            >
              {MESSAGES.map((m) => (
                <MenuItem key={m} value={m}>
                  {MESSAGE_LABELS[m]}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <FormControl size="small" sx={{ minWidth: 280 }}>
            <InputLabel id="pt-anchor">Anchor</InputLabel>
            <Select
              labelId="pt-anchor"
              label="Anchor"
              value={form.placement.anchor}
              onChange={(e) => setPlacement("anchor", e.target.value)}
            >
              {ANCHORS.map((a) => (
                <MenuItem key={a} value={a}>
                  {ANCHOR_LABELS[a]}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Box>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          {describePlacement(form.placement)}
        </Typography>

        <Typography variant="headingSmall" component="h3" sx={{ mb: 0.5 }}>
          Parts
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
          The app's own text as constants, byte for byte, and the parts that vary as named slots, in order. Each
          constant is at least {MIN_CONST_BYTES} bytes, with no invisible characters.
        </Typography>
        <PartsEditor
          parts={form.parts}
          errors={validation.parts}
          showErrors={showErrors}
          onChange={(parts) => setForm({ ...form, parts })}
        />
      </DialogContent>
      <DialogActions>
        <SecondaryOutlineButton onClick={onClose}>Cancel</SecondaryOutlineButton>
        <PrimaryButton variant="contained" onClick={save} disabled={saving}>
          {editing ? "Save" : "Propose"}
        </PrimaryButton>
      </DialogActions>
    </Dialog>
  );
};

export default PromptTemplateEditor;
