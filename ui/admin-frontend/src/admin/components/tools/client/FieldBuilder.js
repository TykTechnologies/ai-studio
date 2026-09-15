import React from "react";
import {
  Box,
  Button,
  Checkbox,
  FormControlLabel,
  IconButton,
  MenuItem,
  Paper,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/Delete";
import ArrowUpwardIcon from "@mui/icons-material/ArrowUpward";
import ArrowDownwardIcon from "@mui/icons-material/ArrowDownward";
import { FIELD_TYPES, nameFromLabel, newField } from "./schemaFields";

const FieldRow = ({ field, index, count, onChange, onRemove, onMove, testId, itemLabel }) => {
  const update = (patch) => onChange({ ...field, ...patch });
  const onLabel = (label) => {
    // Keep the identifier in step with the label until it is edited by hand.
    const autoName = field.name === "" || field.name === nameFromLabel(field.label);
    update({ label, ...(autoName ? { name: nameFromLabel(label) } : {}) });
  };
  return (
    <Paper variant="outlined" sx={{ p: 1.5 }} data-testid={testId}>
      <Stack direction={{ xs: "column", md: "row" }} spacing={1.5} alignItems={{ md: "flex-start" }} flexWrap="wrap" useFlexGap>
        <TextField
          label="Label"
          value={field.label}
          onChange={(e) => onLabel(e.target.value)}
          size="small"
          sx={{ flex: "2 1 140px", minWidth: 140 }}
          autoComplete="off"
          inputProps={{ "aria-label": `${itemLabel} ${index + 1} label` }}
        />
        <TextField
          label="Identifier"
          value={field.name}
          onChange={(e) => update({ name: nameFromLabel(e.target.value) || e.target.value })}
          size="small"
          sx={{ flex: "1.5 1 120px", minWidth: 120 }}
          autoComplete="off"
          helperText={!field.name && !field.label ? "Required" : undefined}
          inputProps={{ "aria-label": `${itemLabel} ${index + 1} identifier`, style: { fontFamily: "monospace", fontSize: 13 } }}
        />
        <TextField
          select
          label="Type"
          value={field.type}
          onChange={(e) => update({ type: e.target.value })}
          size="small"
          sx={{ flex: "1.2 1 140px", minWidth: 140 }}
          inputProps={{ "aria-label": `${itemLabel} ${index + 1} type` }}
        >
          {FIELD_TYPES.map((t) => (
            <MenuItem key={t.value} value={t.value}>
              {t.label}
            </MenuItem>
          ))}
        </TextField>
        <FormControlLabel
          sx={{ mt: 0.5, whiteSpace: "nowrap" }}
          control={<Checkbox size="small" checked={!!field.required} onChange={(e) => update({ required: e.target.checked })} />}
          label="Required"
        />
        <Box sx={{ display: "flex", alignItems: "center", mt: 0.25, ml: "auto" }}>
          <Tooltip title="Move up">
            <span>
              <IconButton size="small" disabled={index === 0} onClick={() => onMove(-1)} aria-label={`Move ${itemLabel.toLowerCase()} ${index + 1} up`}>
                <ArrowUpwardIcon fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
          <Tooltip title="Move down">
            <span>
              <IconButton size="small" disabled={index === count - 1} onClick={() => onMove(1)} aria-label={`Move ${itemLabel.toLowerCase()} ${index + 1} down`}>
                <ArrowDownwardIcon fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
          <Tooltip title="Remove field">
            <IconButton size="small" onClick={onRemove} aria-label={`Remove ${itemLabel.toLowerCase()} ${index + 1}`}>
              <DeleteIcon fontSize="small" />
            </IconButton>
          </Tooltip>
        </Box>
      </Stack>
      <Stack direction={{ xs: "column", md: "row" }} spacing={1.5} sx={{ mt: 1.5 }}>
        <TextField
          label="Help text"
          value={field.description}
          onChange={(e) => update({ description: e.target.value })}
          size="small"
          fullWidth
          autoComplete="off"
          placeholder="Shown under the field; also tells the assistant what belongs here"
        />
        {field.type === "select" && (
          <TextField
            label="Choices (comma separated)"
            value={(field.options || []).join(", ")}
            onChange={(e) => update({ options: e.target.value.split(",").map((s) => s.trim()).filter(Boolean) })}
            size="small"
            fullWidth
            autoComplete="off"
            inputProps={{ "aria-label": `${itemLabel} ${index + 1} choices` }}
          />
        )}
      </Stack>
    </Paper>
  );
};

/**
 * Editable list of form fields. `value` is an array of fields (see
 * schemaFields.js); `onChange` receives the whole new array.
 */
const FieldBuilder = ({ value, onChange, addLabel = "Add field", emptyText, testIdPrefix = "field", itemLabel = "Field" }) => {
  const fields = value || [];
  const setAt = (i, field) => onChange(fields.map((f, idx) => (idx === i ? field : f)));
  const removeAt = (i) => onChange(fields.filter((_, idx) => idx !== i));
  const move = (i, delta) => {
    const j = i + delta;
    if (j < 0 || j >= fields.length) return;
    const next = [...fields];
    [next[i], next[j]] = [next[j], next[i]];
    onChange(next);
  };
  return (
    <Stack spacing={1.5} data-testid={`${testIdPrefix}-builder`}>
      {fields.length === 0 && emptyText && (
        <Typography variant="body2" color="text.secondary">
          {emptyText}
        </Typography>
      )}
      {fields.map((field, i) => (
        <FieldRow
          key={field.key || i}
          field={field}
          index={i}
          count={fields.length}
          onChange={(f) => setAt(i, f)}
          onRemove={() => removeAt(i)}
          onMove={(d) => move(i, d)}
          testId={`${testIdPrefix}-row`}
          itemLabel={itemLabel}
        />
      ))}
      <Box>
        <Button size="small" startIcon={<AddIcon />} onClick={() => onChange([...fields, newField()])} data-testid={`${testIdPrefix}-add`}>
          {addLabel}
        </Button>
      </Box>
    </Stack>
  );
};

export default FieldBuilder;
