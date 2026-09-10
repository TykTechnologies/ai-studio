import React, { useEffect, useState } from "react";
import {
  Box,
  Button,
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
  Select,
  Switch,
  TextField,
} from "@mui/material";

export const FIELD_TYPES = [
  { value: "string", label: "Text (single line)" },
  { value: "text", label: "Text (multi line)" },
  { value: "number", label: "Number" },
  { value: "boolean", label: "Yes / No" },
  { value: "date", label: "Date" },
  { value: "email", label: "Email address" },
  { value: "url", label: "URL" },
  { value: "user", label: "User" },
  { value: "vocabulary", label: "Vocabulary (single value)" },
  { value: "multi_vocabulary", label: "Vocabulary (multiple values)" },
  { value: "string_list", label: "List of text values" },
];

const KEY_RE = /^[a-z][a-z0-9_]*$/;

export const emptyField = () => ({
  key: "",
  label: "",
  description: "",
  type: "string",
  required: false,
  severity: "error",
  vocabulary_slug: "",
  pattern: "",
  min: "",
  max: "",
  max_length: "",
  warn_if_past: false,
  portal_visible: false,
  gateway_visible: false,
});

const toForm = (field) => ({
  ...emptyField(),
  ...field,
  min: field?.min ?? "",
  max: field?.max ?? "",
  max_length: field?.max_length || "",
});

/** Converts the dialog form back into a MetadataFieldDef. */
export const fromForm = (form) => {
  const out = {
    key: form.key.trim(),
    label: form.label.trim() || form.key.trim(),
    description: form.description,
    type: form.type,
    required: Boolean(form.required),
    severity: form.severity || "error",
    portal_visible: Boolean(form.portal_visible),
    gateway_visible: Boolean(form.gateway_visible),
  };
  if (form.type === "vocabulary" || form.type === "multi_vocabulary") out.vocabulary_slug = form.vocabulary_slug;
  if (["string", "text", "email", "url"].includes(form.type)) {
    if (form.pattern) out.pattern = form.pattern;
    if (form.max_length !== "" && form.max_length !== null) out.max_length = Number(form.max_length);
  }
  if (form.type === "number") {
    if (form.min !== "" && form.min !== null) out.min = Number(form.min);
    if (form.max !== "" && form.max !== null) out.max = Number(form.max);
  }
  if (form.type === "date") out.warn_if_past = Boolean(form.warn_if_past);
  return out;
};

/**
 * Add / edit one field definition of a metadata schema.
 * Props: open, field (null = new), existingKeys (for uniqueness), vocabularies, onSave(fieldDef), onClose.
 */
const FieldDefinitionDialog = ({ open, field, existingKeys = [], vocabularies = [], onSave, onClose }) => {
  const [form, setForm] = useState(emptyField());
  const [errors, setErrors] = useState({});

  useEffect(() => {
    if (open) {
      setForm(toForm(field));
      setErrors({});
    }
  }, [open, field]);

  const set = (patch) => setForm((prev) => ({ ...prev, ...patch }));
  const needsVocab = form.type === "vocabulary" || form.type === "multi_vocabulary";
  const isText = ["string", "text", "email", "url"].includes(form.type);

  const validate = () => {
    const next = {};
    const key = form.key.trim();
    if (!KEY_RE.test(key)) next.key = "Use lowercase letters, digits and underscores, starting with a letter";
    else if (existingKeys.includes(key) && key !== field?.key) next.key = "A field with this key already exists";
    if (needsVocab && !form.vocabulary_slug) next.vocabulary_slug = "Choose a vocabulary";
    if (form.type === "number" && form.min !== "" && form.max !== "" && Number(form.min) > Number(form.max)) {
      next.min = "Min must not exceed max";
    }
    setErrors(next);
    return Object.keys(next).length === 0;
  };

  const handleSave = () => {
    if (!validate()) return;
    onSave(fromForm(form));
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{field ? "Edit Field" : "Add Field"}</DialogTitle>
      <DialogContent>
        <Grid container spacing={2} sx={{ mt: 0 }}>
          <Grid item xs={12} sm={6}>
            <TextField
              fullWidth
              label="Key"
              value={form.key}
              onChange={(e) => set({ key: e.target.value })}
              error={Boolean(errors.key)}
              helperText={errors.key || "Stored identifier, e.g. business_owner"}
              required
              disabled={Boolean(field)}
            />
          </Grid>
          <Grid item xs={12} sm={6}>
            <TextField fullWidth label="Label" value={form.label} onChange={(e) => set({ label: e.target.value })} />
          </Grid>
          <Grid item xs={12}>
            <TextField
              fullWidth
              label="Description"
              value={form.description}
              onChange={(e) => set({ description: e.target.value })}
              helperText="Shown as help text under the input"
            />
          </Grid>
          <Grid item xs={12} sm={6}>
            <FormControl fullWidth>
              <InputLabel id="field-type-label">Type</InputLabel>
              <Select
                labelId="field-type-label"
                label="Type"
                value={form.type}
                onChange={(e) => set({ type: e.target.value })}
              >
                {FIELD_TYPES.map((t) => (
                  <MenuItem key={t.value} value={t.value}>
                    {t.label}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Grid>
          <Grid item xs={12} sm={6}>
            <FormControl fullWidth>
              <InputLabel id="field-severity-label">When missing or invalid</InputLabel>
              <Select
                labelId="field-severity-label"
                label="When missing or invalid"
                value={form.severity}
                onChange={(e) => set({ severity: e.target.value })}
              >
                <MenuItem value="error">Error (blocks when enforced)</MenuItem>
                <MenuItem value="warning">Warning (never blocks)</MenuItem>
              </Select>
            </FormControl>
          </Grid>

          {needsVocab && (
            <Grid item xs={12}>
              <FormControl fullWidth error={Boolean(errors.vocabulary_slug)}>
                <InputLabel id="field-vocab-label">Vocabulary</InputLabel>
                <Select
                  labelId="field-vocab-label"
                  label="Vocabulary"
                  value={form.vocabulary_slug}
                  onChange={(e) => set({ vocabulary_slug: e.target.value })}
                >
                  {vocabularies.map((v) => (
                    <MenuItem key={v.slug} value={v.slug}>
                      {v.name} ({v.slug})
                    </MenuItem>
                  ))}
                </Select>
                {errors.vocabulary_slug && <FormHelperText>{errors.vocabulary_slug}</FormHelperText>}
              </FormControl>
            </Grid>
          )}

          {isText && (
            <>
              <Grid item xs={12} sm={8}>
                <TextField
                  fullWidth
                  label="Pattern (regular expression)"
                  value={form.pattern}
                  onChange={(e) => set({ pattern: e.target.value })}
                />
              </Grid>
              <Grid item xs={12} sm={4}>
                <TextField
                  fullWidth
                  type="number"
                  label="Max length"
                  value={form.max_length}
                  onChange={(e) => set({ max_length: e.target.value })}
                />
              </Grid>
            </>
          )}

          {form.type === "number" && (
            <>
              <Grid item xs={6}>
                <TextField
                  fullWidth
                  type="number"
                  label="Min"
                  value={form.min}
                  error={Boolean(errors.min)}
                  helperText={errors.min}
                  onChange={(e) => set({ min: e.target.value })}
                />
              </Grid>
              <Grid item xs={6}>
                <TextField fullWidth type="number" label="Max" value={form.max} onChange={(e) => set({ max: e.target.value })} />
              </Grid>
            </>
          )}

          <Grid item xs={12}>
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 2 }}>
              <FormControlLabel
                control={<Switch checked={form.required} onChange={(e) => set({ required: e.target.checked })} />}
                label="Required"
              />
              {form.type === "date" && (
                <FormControlLabel
                  control={<Switch checked={form.warn_if_past} onChange={(e) => set({ warn_if_past: e.target.checked })} />}
                  label="Warn when in the past"
                />
              )}
              <FormControlLabel
                control={<Switch checked={form.portal_visible} onChange={(e) => set({ portal_visible: e.target.checked })} />}
                label="Visible in portal"
              />
              <FormControlLabel
                control={<Switch checked={form.gateway_visible} onChange={(e) => set({ gateway_visible: e.target.checked })} />}
                label="Sent to gateways"
              />
            </Box>
          </Grid>
        </Grid>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button onClick={handleSave} variant="contained">
          {field ? "Update field" : "Add field"}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default FieldDefinitionDialog;
