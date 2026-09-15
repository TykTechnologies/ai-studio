import React, { useEffect, useMemo, useRef, useState } from "react";
import { Alert, Box, Button, Chip, Grid, MenuItem, Stack, TextField, Typography } from "@mui/material";
import CodeIcon from "@mui/icons-material/Code";
import ViewListIcon from "@mui/icons-material/ViewList";
import AutoAwesomeIcon from "@mui/icons-material/AutoAwesome";
import FieldBuilder from "./FieldBuilder";
import ClientToolPreview from "./ClientToolPreview";
import { fieldsToSchema, parseSchemaText, schemaToFields } from "./schemaFields";
import { presetsForKind } from "./clientToolPresets";

const toJson = (schema) => JSON.stringify(schema, null, 2);

/**
 * Field state for one schema whose source of truth is the JSON text held by
 * the parent form. Fields are re-derived only when the text changes from
 * outside (a preset, or a hand edit in JSON mode); builder edits flow the
 * other way, so rows keep their identity while typing.
 */
const useSchemaFields = (text, setText) => {
  const derive = (t) => {
    const { schema, error } = parseSchemaText(t);
    if (error) return { fields: [], unsupported: [], invalid: true };
    const { fields, unsupported } = schemaToFields(schema || {});
    return { fields, unsupported, invalid: false };
  };
  const [state, setState] = useState(() => ({ text, ...derive(text) }));
  useEffect(() => {
    if (text !== state.text) setState({ text, ...derive(text) });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [text]);
  const setFields = (fields) => {
    const next = toJson(fieldsToSchema(fields));
    setState({ text: next, fields, unsupported: [], invalid: false });
    setText(next);
  };
  return { ...state, setFields };
};

const SchemaSection = ({ heading, help, text, setText, error, addLabel, emptyText, testIdPrefix, itemLabel }) => {
  const { fields, unsupported, invalid, setFields } = useSchemaFields(text, setText);
  const mustUseJson = unsupported.length > 0 || invalid;
  const [jsonMode, setJsonMode] = useState(false);
  const showJson = jsonMode || mustUseJson;
  return (
    <Box>
      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1, flexWrap: "wrap", gap: 1 }}>
        <Box>
          <Typography variant="subtitle2">{heading}</Typography>
          {help && (
            <Typography variant="caption" color="text.secondary">
              {help}
            </Typography>
          )}
        </Box>
        <Button
          size="small"
          startIcon={showJson ? <ViewListIcon /> : <CodeIcon />}
          onClick={() => setJsonMode((v) => !v)}
          disabled={mustUseJson}
          data-testid={`${testIdPrefix}-toggle-json`}
        >
          {showJson ? "Use the field builder" : "Edit as JSON"}
        </Button>
      </Box>
      {mustUseJson && !invalid && (
        <Alert severity="info" sx={{ mb: 1 }}>
          This schema uses features the field builder cannot show ({unsupported.join(", ")}), so it is shown as JSON.
        </Alert>
      )}
      {showJson ? (
        <TextField
          fullWidth
          value={text}
          onChange={(e) => setText(e.target.value)}
          error={!!error}
          helperText={error || "JSON Schema of type object"}
          multiline
          minRows={8}
          variant="outlined"
          autoComplete="off"
          inputProps={{ "data-testid": `${testIdPrefix}-json`, style: { fontFamily: "monospace", fontSize: 13 } }}
        />
      ) : (
        <FieldBuilder value={fields} onChange={setFields} addLabel={addLabel} emptyText={emptyText} testIdPrefix={testIdPrefix} itemLabel={itemLabel} />
      )}
    </Box>
  );
};

/**
 * Editor for a client (human-in-the-loop) tool: interaction kind, card
 * text, the fields involved, ready-made examples, and a live preview of
 * what the assistant and the person in the chat will see.
 *
 * `value` / `onChange` carry the definition the form saves
 * ({ kind, title, description, parameters, responseSchema }, schemas as
 * JSON text). `onApplyPreset` lets the parent also fill the tool's name and
 * description.
 */
const ClientToolEditor = ({ value, onChange, errors = {}, toolName, toolDescription, onApplyPreset }) => {
  const set = (patch) => onChange({ ...value, ...patch });
  const presets = useMemo(() => presetsForKind(value.kind), [value.kind]);
  const previewFieldsRef = useRef([]);

  const parameterFields = useMemo(() => {
    const { schema, error } = parseSchemaText(value.parameters);
    if (error) return previewFieldsRef.current;
    const { fields } = schemaToFields(schema || {});
    previewFieldsRef.current = fields;
    return fields;
  }, [value.parameters]);
  const responseSchema = useMemo(() => parseSchemaText(value.responseSchema).schema, [value.responseSchema]);

  const applyPreset = (preset) => {
    onChange({
      ...value,
      kind: preset.kind,
      title: preset.title,
      description: preset.instructions,
      parameters: toJson(fieldsToSchema(preset.parameters)),
      responseSchema: preset.kind === "form" ? toJson(fieldsToSchema(preset.response)) : "",
    });
    onApplyPreset?.(preset);
  };

  const isPresent = value.kind === "present";

  return (
    <Grid container spacing={3}>
      <Grid item xs={12} lg={7}>
        <Stack spacing={2.5}>
          <Grid container spacing={2}>
            <Grid item xs={12} md={5}>
              <TextField
                select
                fullWidth
                label="Interaction"
                value={value.kind}
                onChange={(e) => set({ kind: e.target.value })}
                helperText={
                  isPresent
                    ? "The assistant composes cards, tables, charts and forms from the built-in vocabulary; no input needed"
                    : value.kind === "form"
                      ? "The person fills in a form; the answers go back to the assistant"
                      : "The person approves or rejects what the assistant proposes"
                }
              >
                <MenuItem value="approval">Approval</MenuItem>
                <MenuItem value="form">Form</MenuItem>
                <MenuItem value="present">Generative UI (present)</MenuItem>
              </TextField>
            </Grid>
            {!isPresent && (
              <Grid item xs={12} md={7}>
                <TextField
                  fullWidth
                  label="Card title"
                  value={value.title}
                  onChange={(e) => set({ title: e.target.value })}
                  helperText="Heading of the card in the chat; defaults to the tool description"
                  autoComplete="off"
                />
              </Grid>
            )}
            {!isPresent && (
              <Grid item xs={12}>
                <TextField
                  fullWidth
                  label="Instructions for the person"
                  value={value.description}
                  onChange={(e) => set({ description: e.target.value })}
                  helperText="Shown under the title, e.g. 'Please fill in the delivery address.'"
                  autoComplete="off"
                />
              </Grid>
            )}
          </Grid>

          {isPresent && (
            <Typography variant="body2" color="text.secondary" data-testid="present-schema-note">
              The component vocabulary is built in and kept in step with the chat renderer, so there is nothing to author here.
              This tool is seeded on every installation as "Generative UI"; you only need another one if you want a different
              name or description.
            </Typography>
          )}

          {!isPresent && presets.length > 0 && (
            <Box data-testid="client-tool-presets">
              <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
                <AutoAwesomeIcon fontSize="small" color="action" />
                <Typography variant="subtitle2">Start from an example</Typography>
              </Box>
              <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap">
                {presets.map((preset) => (
                  <Chip key={preset.id} label={preset.name} variant="outlined" clickable onClick={() => applyPreset(preset)} />
                ))}
              </Stack>
              <Typography variant="caption" color="text.secondary">
                Fills in the name, description, card text and fields. Everything stays editable.
              </Typography>
            </Box>
          )}

          {!isPresent && (
            <SchemaSection
              heading={value.kind === "form" ? "Context the assistant provides" : "What the assistant must state"}
              help={
                value.kind === "form"
                  ? "Filled by the assistant when it calls the tool and shown above the form, e.g. why the details are needed."
                  : "Filled by the assistant when it asks; shown to the person so they know what they are approving."
              }
              text={value.parameters}
              setText={(t) => set({ parameters: t })}
              error={errors.parameters}
              addLabel="Add a detail"
              emptyText="No details yet. Add what the assistant should state, e.g. the action and the reason."
              testIdPrefix="client-parameters"
              itemLabel="Detail"
            />
          )}

          {value.kind === "form" && (
            <SchemaSection
              heading="Fields the person fills in"
              help="The form shown in the chat. Leave empty for a single free-text answer."
              text={value.responseSchema}
              setText={(t) => set({ responseSchema: t })}
              error={errors.responseSchema}
              addLabel="Add a field"
              emptyText="No fields yet: a single free-text box will be shown."
              testIdPrefix="client-response"
              itemLabel="Field"
            />
          )}
        </Stack>
      </Grid>

      {!isPresent && (
        <Grid item xs={12} lg={5}>
          <Box sx={{ position: { lg: "sticky" }, top: { lg: 16 } }}>
            <Typography variant="subtitle1" sx={{ mb: 1.5 }}>
              Preview
            </Typography>
            <ClientToolPreview
              kind={value.kind}
              toolName={toolName}
              toolDescription={toolDescription}
              title={value.title}
              instructions={value.description}
              parameterFields={parameterFields}
              responseSchema={responseSchema}
            />
          </Box>
        </Grid>
      )}
    </Grid>
  );
};

export default ClientToolEditor;
