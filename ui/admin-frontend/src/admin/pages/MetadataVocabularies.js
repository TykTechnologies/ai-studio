import React, { useState, useEffect, useCallback } from "react";
import {
  Typography,
  Box,
  Table,
  TableBody,
  TableHead,
  TableRow,
  TableContainer,
  CircularProgress,
  Alert,
  Snackbar,
  IconButton,
  Chip,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  FormControlLabel,
  Switch,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import EditIcon from "@mui/icons-material/Edit";
import DeleteIcon from "@mui/icons-material/Delete";
import LabelOutlinedIcon from "@mui/icons-material/LabelOutlined";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  StyledPaper,
  StyledTableHeaderCell,
  StyledTableCell,
  StyledTableRow,
} from "../styles/sharedStyles";
import EnterpriseFeatureBadge from "../components/common/EnterpriseFeatureBadge";
import ConfirmationDialog from "../components/common/ConfirmationDialog";
import {
  isGovernedMetadataAvailable,
  getMetadataVocabularies,
  createMetadataVocabulary,
  updateMetadataVocabulary,
  deleteMetadataVocabulary,
} from "../services/governedMetadataService";

const emptyTerm = () => ({ value: "", label: "", description: "", deprecated: false });
const emptyVocabulary = () => ({ name: "", slug: "", description: "", terms: [emptyTerm()] });

const isPluginSourced = (source) => typeof source === "string" && source.startsWith("plugin:");

const MetadataVocabularies = () => {
  const [available, setAvailable] = useState(null);
  const [vocabularies, setVocabularies] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyVocabulary());
  const [saving, setSaving] = useState(false);
  const [toDelete, setToDelete] = useState(null);
  const [snackbar, setSnackbar] = useState({ open: false, message: "", severity: "success" });

  const notify = (message, severity = "success") => setSnackbar({ open: true, message, severity });

  const fetchAll = useCallback(async () => {
    try {
      setLoading(true);
      setVocabularies(await getMetadataVocabularies());
      setError("");
    } catch (err) {
      setError("Failed to load vocabularies");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    (async () => {
      const ok = await isGovernedMetadataAvailable();
      setAvailable(ok);
      if (ok) {
        fetchAll();
      } else {
        setLoading(false);
      }
    })();
  }, [fetchAll]);

  const openCreate = () => {
    setEditing(null);
    setForm(emptyVocabulary());
    setDialogOpen(true);
  };

  const openEdit = (vocab) => {
    setEditing(vocab);
    setForm({
      name: vocab.name,
      slug: vocab.slug,
      description: vocab.description || "",
      terms: (vocab.terms || []).map((t) => ({ ...emptyTerm(), ...t })),
    });
    setDialogOpen(true);
  };

  const updateTerm = (index, patch) =>
    setForm((prev) => ({
      ...prev,
      terms: prev.terms.map((t, i) => (i === index ? { ...t, ...patch } : t)),
    }));

  const removeTerm = (index) =>
    setForm((prev) => ({ ...prev, terms: prev.terms.filter((_, i) => i !== index) }));

  const handleSave = async () => {
    const terms = form.terms.filter((t) => t.value.trim() !== "");
    if (!form.name.trim()) {
      notify("Name is required", "error");
      return;
    }
    if (terms.length === 0) {
      notify("At least one term is required", "error");
      return;
    }
    setSaving(true);
    try {
      const payload = {
        name: form.name.trim(),
        slug: form.slug.trim(),
        description: form.description,
        terms: terms.map((t) => ({
          value: t.value.trim(),
          label: t.label.trim() || t.value.trim(),
          description: t.description,
          deprecated: Boolean(t.deprecated),
        })),
      };
      if (editing) {
        await updateMetadataVocabulary(editing.id, payload);
        notify("Vocabulary updated");
      } else {
        await createMetadataVocabulary(payload);
        notify("Vocabulary created");
      }
      setDialogOpen(false);
      fetchAll();
    } catch (err) {
      notify(err.message || "Failed to save vocabulary", "error");
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!toDelete) return;
    try {
      await deleteMetadataVocabulary(toDelete.id);
      notify("Vocabulary deleted");
      setToDelete(null);
      fetchAll();
    } catch (err) {
      notify(err.message || "Failed to delete vocabulary", "error");
      setToDelete(null);
    }
  };

  return (
    <>
      <TitleBox top="64px">
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <LabelOutlinedIcon />
          <Typography variant="headingXLarge">Metadata Vocabularies</Typography>
          <Chip label="Enterprise" size="small" color="primary" />
        </Box>
        {available && (
          <PrimaryButton startIcon={<AddIcon />} onClick={openCreate}>
            Add Vocabulary
          </PrimaryButton>
        )}
      </TitleBox>

      <ContentBox sx={{ pt: 0 }}>
        {available === false && (
          <EnterpriseFeatureBadge
            feature="Governed Metadata"
            description="Define controlled vocabularies and metadata schemas for LLMs, tools and data sources with the Enterprise Edition."
          />
        )}
        {loading && available !== false && <CircularProgress />}
        {error && <Alert severity="error">{error}</Alert>}
        {!loading && !error && available && (
          <TableContainer component={StyledPaper}>
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Slug</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Terms</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Source</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Actions</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {vocabularies.length === 0 ? (
                  <TableRow>
                    <StyledTableCell colSpan={5} align="center">
                      <Typography color="text.secondary" sx={{ py: 3 }}>
                        No vocabularies yet. Create one to control the allowed values of a schema field.
                      </Typography>
                    </StyledTableCell>
                  </TableRow>
                ) : (
                  vocabularies.map((vocab) => (
                    <StyledTableRow key={vocab.id}>
                      <StyledTableCell>
                        <Typography variant="body2" fontWeight="medium">
                          {vocab.name}
                        </Typography>
                        {vocab.description && (
                          <Typography variant="caption" color="text.secondary">
                            {vocab.description}
                          </Typography>
                        )}
                      </StyledTableCell>
                      <StyledTableCell>
                        <code>{vocab.slug}</code>
                      </StyledTableCell>
                      <StyledTableCell>
                        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                          {(vocab.terms || []).slice(0, 6).map((t) => (
                            <Chip
                              key={t.value}
                              label={t.label || t.value}
                              size="small"
                              variant="outlined"
                              color={t.deprecated ? "default" : "primary"}
                            />
                          ))}
                          {(vocab.terms || []).length > 6 && (
                            <Chip label={`+${vocab.terms.length - 6}`} size="small" />
                          )}
                        </Box>
                      </StyledTableCell>
                      <StyledTableCell>
                        <Chip
                          label={isPluginSourced(vocab.source) ? `Plugin ${vocab.source.slice(7)}` : vocab.source || "admin"}
                          size="small"
                          variant="outlined"
                        />
                      </StyledTableCell>
                      <StyledTableCell>
                        <IconButton size="small" aria-label={`Edit ${vocab.name}`} onClick={() => openEdit(vocab)}>
                          <EditIcon fontSize="small" />
                        </IconButton>
                        <IconButton
                          size="small"
                          color="error"
                          aria-label={`Delete ${vocab.name}`}
                          onClick={() => setToDelete(vocab)}
                        >
                          <DeleteIcon fontSize="small" />
                        </IconButton>
                      </StyledTableCell>
                    </StyledTableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </ContentBox>

      <Dialog open={dialogOpen} onClose={() => setDialogOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>{editing ? "Edit Vocabulary" : "Create Vocabulary"}</DialogTitle>
        <DialogContent>
          <TextField
            fullWidth
            label="Name"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            required
            sx={{ mt: 1 }}
          />
          <TextField
            fullWidth
            label="Slug"
            value={form.slug}
            onChange={(e) => setForm({ ...form, slug: e.target.value })}
            helperText="Lowercase identifier referenced by schema fields. Derived from the name when left blank."
            sx={{ mt: 2 }}
          />
          <TextField
            fullWidth
            label="Description"
            value={form.description}
            onChange={(e) => setForm({ ...form, description: e.target.value })}
            sx={{ mt: 2 }}
          />

          <Typography variant="subtitle1" sx={{ mt: 3, mb: 1 }}>
            Terms
          </Typography>
          <Table size="small">
            <TableHead>
              <TableRow>
                <StyledTableHeaderCell>Value</StyledTableHeaderCell>
                <StyledTableHeaderCell>Label</StyledTableHeaderCell>
                <StyledTableHeaderCell>Description</StyledTableHeaderCell>
                <StyledTableHeaderCell>Deprecated</StyledTableHeaderCell>
                <StyledTableHeaderCell />
              </TableRow>
            </TableHead>
            <TableBody>
              {form.terms.map((term, index) => (
                <TableRow key={index}>
                  <StyledTableCell>
                    <TextField
                      size="small"
                      value={term.value}
                      inputProps={{ "aria-label": `Term ${index + 1} value` }}
                      onChange={(e) => updateTerm(index, { value: e.target.value })}
                    />
                  </StyledTableCell>
                  <StyledTableCell>
                    <TextField
                      size="small"
                      value={term.label}
                      inputProps={{ "aria-label": `Term ${index + 1} label` }}
                      onChange={(e) => updateTerm(index, { label: e.target.value })}
                    />
                  </StyledTableCell>
                  <StyledTableCell>
                    <TextField
                      size="small"
                      value={term.description}
                      inputProps={{ "aria-label": `Term ${index + 1} description` }}
                      onChange={(e) => updateTerm(index, { description: e.target.value })}
                    />
                  </StyledTableCell>
                  <StyledTableCell>
                    <FormControlLabel
                      control={
                        <Switch
                          size="small"
                          checked={Boolean(term.deprecated)}
                          onChange={(e) => updateTerm(index, { deprecated: e.target.checked })}
                        />
                      }
                      label=""
                    />
                  </StyledTableCell>
                  <StyledTableCell>
                    <IconButton
                      size="small"
                      aria-label={`Remove term ${index + 1}`}
                      onClick={() => removeTerm(index)}
                      disabled={form.terms.length === 1}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </StyledTableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <Button
            startIcon={<AddIcon />}
            sx={{ mt: 1 }}
            onClick={() => setForm({ ...form, terms: [...form.terms, emptyTerm()] })}
          >
            Add term
          </Button>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDialogOpen(false)}>Cancel</Button>
          <Button onClick={handleSave} variant="contained" disabled={saving}>
            {saving ? <CircularProgress size={20} /> : editing ? "Update" : "Create"}
          </Button>
        </DialogActions>
      </Dialog>

      <ConfirmationDialog
        title="Delete Vocabulary"
        message={`Delete "${toDelete?.name}"? Schema fields that reference it must be changed first.`}
        buttonLabel="Delete"
        open={Boolean(toDelete)}
        onConfirm={handleDelete}
        onCancel={() => setToDelete(null)}
        onClose={() => setToDelete(null)}
        iconName="triangle-exclamation"
        primaryButtonComponent="danger"
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

export default MetadataVocabularies;
