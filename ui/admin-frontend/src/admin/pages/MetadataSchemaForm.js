import React, { useEffect, useState } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import {
  Alert,
  Box,
  Chip,
  CircularProgress,
  FormControl,
  FormControlLabel,
  Grid,
  IconButton,
  InputLabel,
  MenuItem,
  Select,
  Snackbar,
  Switch,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
  Button,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import AddIcon from "@mui/icons-material/Add";
import EditIcon from "@mui/icons-material/Edit";
import DeleteIcon from "@mui/icons-material/Delete";
import ArrowUpwardIcon from "@mui/icons-material/ArrowUpward";
import ArrowDownwardIcon from "@mui/icons-material/ArrowDownward";
import { generateSlug } from "../components/wizards/quick-start/utils";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  StyledPaper,
  StyledTableHeaderCell,
  StyledTableCell,
  StyledTableRow,
} from "../styles/sharedStyles";
import FieldDefinitionDialog, { FIELD_TYPES } from "../components/metadata/FieldDefinitionDialog";
import {
  getMetadataSchema,
  createMetadataSchema,
  updateMetadataSchema,
  getMetadataObjectTypes,
  getMetadataVocabularies,
} from "../services/governedMetadataService";
import { isPluginSourcedSchema } from "./MetadataSchemas";

const ALL_TYPES = "*";

const emptySchema = () => ({
  name: "",
  slug: "",
  description: "",
  applies_to: [ALL_TYPES],
  enforcement: "advisory",
  active: true,
  fields: [],
});

const typeLabel = (type) => FIELD_TYPES.find((t) => t.value === type)?.label || type;

const MetadataSchemaForm = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [schema, setSchema] = useState(emptySchema());
  const [slugTouched, setSlugTouched] = useState(Boolean(id));
  const [objectTypes, setObjectTypes] = useState([]);
  const [vocabularies, setVocabularies] = useState([]);
  const [loading, setLoading] = useState(Boolean(id));
  const [saving, setSaving] = useState(false);
  const [fieldDialog, setFieldDialog] = useState({ open: false, index: null });
  const [snackbar, setSnackbar] = useState({ open: false, message: "", severity: "success" });

  const readOnly = isPluginSourcedSchema(schema);

  useEffect(() => {
    (async () => {
      try {
        const [types, vocabs] = await Promise.all([getMetadataObjectTypes(), getMetadataVocabularies()]);
        setObjectTypes(types);
        setVocabularies(vocabs);
        if (id) {
          const loaded = await getMetadataSchema(id);
          setSchema({ ...emptySchema(), ...loaded, fields: [...(loaded.fields || [])].sort((a, b) => (a.order || 0) - (b.order || 0)) });
        }
      } catch (err) {
        setSnackbar({ open: true, message: err.message || "Failed to load schema", severity: "error" });
      } finally {
        setLoading(false);
      }
    })();
  }, [id]);

  const set = (patch) => setSchema((prev) => ({ ...prev, ...patch }));

  const handleNameChange = (name) => {
    set({ name, ...(slugTouched ? {} : { slug: generateSlug(name) }) });
  };

  const handleAppliesTo = (next) => {
    const values = Array.isArray(next) ? next : [next];
    const addedAll = values.includes(ALL_TYPES) && !schema.applies_to.includes(ALL_TYPES);
    set({ applies_to: addedAll ? [ALL_TYPES] : values.filter((v) => v !== ALL_TYPES) });
  };

  const saveField = (def) => {
    const fields = [...schema.fields];
    if (fieldDialog.index === null) {
      fields.push(def);
    } else {
      fields[fieldDialog.index] = { ...fields[fieldDialog.index], ...def };
    }
    set({ fields });
    setFieldDialog({ open: false, index: null });
  };

  const removeField = (index) => set({ fields: schema.fields.filter((_, i) => i !== index) });

  const moveField = (index, delta) => {
    const target = index + delta;
    if (target < 0 || target >= schema.fields.length) return;
    const fields = [...schema.fields];
    [fields[index], fields[target]] = [fields[target], fields[index]];
    set({ fields });
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!schema.name.trim()) {
      setSnackbar({ open: true, message: "Name is required", severity: "error" });
      return;
    }
    if (schema.applies_to.length === 0) {
      setSnackbar({ open: true, message: "Choose at least one object type", severity: "error" });
      return;
    }
    setSaving(true);
    try {
      const payload = {
        name: schema.name.trim(),
        slug: schema.slug.trim(),
        description: schema.description,
        applies_to: schema.applies_to,
        enforcement: schema.enforcement,
        active: Boolean(schema.active),
        fields: schema.fields.map((f, i) => ({ ...f, order: i + 1 })),
      };
      if (id) {
        await updateMetadataSchema(id, payload);
      } else {
        await createMetadataSchema(payload);
      }
      setSnackbar({ open: true, message: id ? "Schema updated" : "Schema created", severity: "success" });
      setTimeout(() => navigate("/admin/metadata/schemas"), 1500);
    } catch (err) {
      setSnackbar({ open: true, message: err.message || "Failed to save schema", severity: "error" });
    } finally {
      setSaving(false);
    }
  };

  const objectTypeLabel = (slug) =>
    slug === ALL_TYPES ? "All object types" : objectTypes.find((t) => t.slug === slug)?.label || slug;

  if (loading) {
    return (
      <ContentBox>
        <CircularProgress />
      </ContentBox>
    );
  }

  return (
    <>
      <TitleBox top="64px">
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <IconButton component={Link} to="/admin/metadata/schemas" aria-label="Back to schemas">
            <ArrowBackIcon />
          </IconButton>
          <Typography variant="headingXLarge">{id ? "Edit Metadata Schema" : "New Metadata Schema"}</Typography>
          <Chip label="Enterprise" size="small" color="primary" />
        </Box>
      </TitleBox>

      <ContentBox sx={{ pt: 0 }}>
        {readOnly && (
          <Alert severity="info" sx={{ mb: 2 }}>
            This schema is contributed by a plugin. You can activate it and set its enforcement level; its fields are
            managed by the plugin manifest.
          </Alert>
        )}
        <Box component="form" onSubmit={handleSubmit} noValidate>
          <StyledPaper sx={{ p: 3, mb: 3 }}>
            <Grid container spacing={3}>
              <Grid item xs={12} md={6}>
                <TextField
                  fullWidth
                  required
                  label="Name"
                  value={schema.name}
                  disabled={readOnly}
                  onChange={(e) => handleNameChange(e.target.value)}
                />
              </Grid>
              <Grid item xs={12} md={6}>
                <TextField
                  fullWidth
                  label="Slug"
                  value={schema.slug}
                  disabled={readOnly}
                  onChange={(e) => {
                    setSlugTouched(true);
                    set({ slug: e.target.value });
                  }}
                  helperText="Stable identifier; derived from the name until edited"
                />
              </Grid>
              <Grid item xs={12}>
                <TextField
                  fullWidth
                  label="Description"
                  value={schema.description || ""}
                  disabled={readOnly}
                  onChange={(e) => set({ description: e.target.value })}
                />
              </Grid>
              <Grid item xs={12} md={6}>
                <FormControl fullWidth disabled={readOnly}>
                  <InputLabel id="schema-applies-to-label">Applies to</InputLabel>
                  <Select
                    labelId="schema-applies-to-label"
                    label="Applies to"
                    multiple
                    value={schema.applies_to}
                    onChange={(e) => handleAppliesTo(e.target.value)}
                    renderValue={(vals) => (
                      <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                        {vals.map((v) => (
                          <Chip key={v} label={objectTypeLabel(v)} size="small" />
                        ))}
                      </Box>
                    )}
                  >
                    <MenuItem value={ALL_TYPES}>All object types</MenuItem>
                    {objectTypes.map((t) => (
                      <MenuItem key={t.slug} value={t.slug}>
                        {t.label}
                        {t.source !== "builtin" ? ` (${t.source})` : ""}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              </Grid>
              <Grid item xs={12} md={6}>
                <FormControl fullWidth>
                  <InputLabel id="schema-enforcement-label">Enforcement</InputLabel>
                  <Select
                    labelId="schema-enforcement-label"
                    label="Enforcement"
                    value={schema.enforcement}
                    onChange={(e) => set({ enforcement: e.target.value })}
                  >
                    <MenuItem value="advisory">Advisory: report issues, never block</MenuItem>
                    <MenuItem value="enforce">Enforce: block saving objects with hard errors</MenuItem>
                  </Select>
                </FormControl>
                {schema.enforcement === "enforce" && (
                  <Typography variant="caption" color="warning.main">
                    Creating or editing LLMs, tools and data sources will fail until required fields are valid. Check the
                    compliance report before switching this on.
                  </Typography>
                )}
              </Grid>
              <Grid item xs={12}>
                <FormControlLabel
                  control={<Switch checked={Boolean(schema.active)} onChange={(e) => set({ active: e.target.checked })} />}
                  label="Active"
                />
              </Grid>
            </Grid>
          </StyledPaper>

          <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1 }}>
            <Typography variant="h6">Fields</Typography>
            {!readOnly && (
              <Button startIcon={<AddIcon />} onClick={() => setFieldDialog({ open: true, index: null })}>
                Add field
              </Button>
            )}
          </Box>
          <TableContainer component={StyledPaper}>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell>#</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Key</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Label</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Type</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Required</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Vocabulary</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Visibility</StyledTableHeaderCell>
                  {!readOnly && <StyledTableHeaderCell>Actions</StyledTableHeaderCell>}
                </TableRow>
              </TableHead>
              <TableBody>
                {schema.fields.length === 0 ? (
                  <TableRow>
                    <StyledTableCell colSpan={8} align="center">
                      <Typography color="text.secondary" sx={{ py: 2 }}>
                        No fields yet. Add the governance attributes this schema should collect.
                      </Typography>
                    </StyledTableCell>
                  </TableRow>
                ) : (
                  schema.fields.map((f, index) => (
                    <StyledTableRow key={f.key}>
                      <StyledTableCell>{index + 1}</StyledTableCell>
                      <StyledTableCell>
                        <code>{f.key}</code>
                      </StyledTableCell>
                      <StyledTableCell>{f.label}</StyledTableCell>
                      <StyledTableCell>{typeLabel(f.type)}</StyledTableCell>
                      <StyledTableCell>
                        {f.required ? (
                          <Chip label={f.severity === "warning" ? "Required (warn)" : "Required"} size="small" color={f.severity === "warning" ? "warning" : "error"} />
                        ) : (
                          <Chip label="Optional" size="small" variant="outlined" />
                        )}
                      </StyledTableCell>
                      <StyledTableCell>{f.vocabulary_slug ? <code>{f.vocabulary_slug}</code> : "—"}</StyledTableCell>
                      <StyledTableCell>
                        <Box sx={{ display: "flex", gap: 0.5 }}>
                          {f.portal_visible && <Chip label="Portal" size="small" variant="outlined" />}
                          {f.gateway_visible && <Chip label="Gateway" size="small" variant="outlined" />}
                        </Box>
                      </StyledTableCell>
                      {!readOnly && (
                        <StyledTableCell>
                          <IconButton size="small" aria-label={`Move ${f.key} up`} onClick={() => moveField(index, -1)} disabled={index === 0}>
                            <ArrowUpwardIcon fontSize="small" />
                          </IconButton>
                          <IconButton
                            size="small"
                            aria-label={`Move ${f.key} down`}
                            onClick={() => moveField(index, 1)}
                            disabled={index === schema.fields.length - 1}
                          >
                            <ArrowDownwardIcon fontSize="small" />
                          </IconButton>
                          <IconButton size="small" aria-label={`Edit ${f.key}`} onClick={() => setFieldDialog({ open: true, index })}>
                            <EditIcon fontSize="small" />
                          </IconButton>
                          <IconButton size="small" color="error" aria-label={`Delete ${f.key}`} onClick={() => removeField(index)}>
                            <DeleteIcon fontSize="small" />
                          </IconButton>
                        </StyledTableCell>
                      )}
                    </StyledTableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </TableContainer>

          <Box mt={4} display="flex" justifyContent="flex-end" gap={2}>
            <Button component={Link} to="/admin/metadata/schemas">
              Cancel
            </Button>
            <PrimaryButton type="submit" variant="contained" disabled={saving}>
              {saving ? <CircularProgress size={20} /> : id ? "Save schema" : "Create schema"}
            </PrimaryButton>
          </Box>
        </Box>
      </ContentBox>

      <FieldDefinitionDialog
        open={fieldDialog.open}
        field={fieldDialog.index === null ? null : schema.fields[fieldDialog.index]}
        existingKeys={schema.fields.map((f) => f.key)}
        vocabularies={vocabularies}
        onSave={saveField}
        onClose={() => setFieldDialog({ open: false, index: null })}
      />

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={() => setSnackbar({ ...snackbar, open: false })}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert onClose={() => setSnackbar({ ...snackbar, open: false })} severity={snackbar.severity} sx={{ width: "100%" }}>
          {snackbar.message}
        </Alert>
      </Snackbar>
    </>
  );
};

export default MetadataSchemaForm;
