import React, { useState, useEffect, useCallback } from "react";
import apiClient from "../utils/apiClient";
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
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  FormControlLabel,
  Switch,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import EditIcon from "@mui/icons-material/Edit";
import DeleteIcon from "@mui/icons-material/Delete";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  StyledPaper,
  StyledTableHeaderCell,
  StyledTableCell,
  StyledTableRow,
} from "../styles/sharedStyles";

const emptyTemplate = {
  name: "",
  text: "",
  applies_to_type: "all",
  required: true,
  active: true,
  sort_order: 0,
};

const AttestationTemplates = () => {
  const [templates, setTemplates] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingTemplate, setEditingTemplate] = useState(null);
  const [formData, setFormData] = useState({ ...emptyTemplate });
  const [saving, setSaving] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [templateToDelete, setTemplateToDelete] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

  const fetchTemplates = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/attestation-templates");
      setTemplates(response.data.data || []);
      setError("");
    } catch (err) {
      setError("Failed to load attestation templates");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchTemplates();
  }, [fetchTemplates]);

  const handleOpenCreate = () => {
    setEditingTemplate(null);
    setFormData({ ...emptyTemplate });
    setDialogOpen(true);
  };

  const handleOpenEdit = (template) => {
    setEditingTemplate(template);
    setFormData({
      name: template.name,
      text: template.text,
      applies_to_type: template.applies_to_type,
      required: template.required,
      active: template.active,
      sort_order: template.sort_order,
    });
    setDialogOpen(true);
  };

  const handleSave = async () => {
    if (!formData.name.trim() || !formData.text.trim()) {
      setSnackbar({
        open: true,
        message: "Name and text are required",
        severity: "error",
      });
      return;
    }

    setSaving(true);
    try {
      const payload = {
        data: {
          attributes: {
            name: formData.name,
            text: formData.text,
            applies_to_type: formData.applies_to_type,
            required: formData.required,
            active: formData.active,
            sort_order: parseInt(formData.sort_order) || 0,
          },
        },
      };

      if (editingTemplate) {
        await apiClient.patch(
          `/attestation-templates/${editingTemplate.id}`,
          payload
        );
        setSnackbar({
          open: true,
          message: "Template updated",
          severity: "success",
        });
      } else {
        await apiClient.post("/attestation-templates", payload);
        setSnackbar({
          open: true,
          message: "Template created",
          severity: "success",
        });
      }
      setDialogOpen(false);
      fetchTemplates();
    } catch (err) {
      setSnackbar({
        open: true,
        message:
          err.response?.data?.errors?.[0]?.detail || "Failed to save template",
        severity: "error",
      });
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!templateToDelete) return;
    try {
      await apiClient.delete(
        `/attestation-templates/${templateToDelete.id}`
      );
      setSnackbar({
        open: true,
        message: "Template deleted",
        severity: "success",
      });
      setDeleteDialogOpen(false);
      setTemplateToDelete(null);
      fetchTemplates();
    } catch (err) {
      setSnackbar({
        open: true,
        message: "Failed to delete template",
        severity: "error",
      });
    }
  };

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Attestation Templates</Typography>
        <PrimaryButton startIcon={<AddIcon />} onClick={handleOpenCreate}>
          Add Template
        </PrimaryButton>
      </TitleBox>

      <ContentBox sx={{ pt: 0 }}>
        {loading && <CircularProgress />}
        {error && <Alert severity="error">{error}</Alert>}
        {!loading && !error && (
          <TableContainer component={StyledPaper}>
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Text</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Applies To</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Required</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Active</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Order</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Actions</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {templates.length === 0 ? (
                  <TableRow>
                    <StyledTableCell colSpan={7} align="center">
                      <Typography color="text.secondary" sx={{ py: 3 }}>
                        No attestation templates yet. Create one to require
                        submitters to agree to terms before submitting.
                      </Typography>
                    </StyledTableCell>
                  </TableRow>
                ) : (
                  templates.map((template) => (
                    <StyledTableRow key={template.id}>
                      <StyledTableCell>
                        <Typography variant="body2" fontWeight="medium">
                          {template.name}
                        </Typography>
                      </StyledTableCell>
                      <StyledTableCell>
                        <Typography
                          variant="body2"
                          sx={{
                            maxWidth: 300,
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                          }}
                        >
                          {template.text}
                        </Typography>
                      </StyledTableCell>
                      <StyledTableCell>
                        <Chip
                          label={template.applies_to_type}
                          size="small"
                          variant="outlined"
                        />
                      </StyledTableCell>
                      <StyledTableCell>
                        {template.required ? (
                          <Chip
                            label="Required"
                            size="small"
                            color="error"
                          />
                        ) : (
                          <Chip
                            label="Optional"
                            size="small"
                            variant="outlined"
                          />
                        )}
                      </StyledTableCell>
                      <StyledTableCell>
                        <Chip
                          label={template.active ? "Active" : "Inactive"}
                          size="small"
                          color={template.active ? "success" : "default"}
                        />
                      </StyledTableCell>
                      <StyledTableCell>{template.sort_order}</StyledTableCell>
                      <StyledTableCell>
                        <IconButton
                          size="small"
                          onClick={() => handleOpenEdit(template)}
                        >
                          <EditIcon fontSize="small" />
                        </IconButton>
                        <IconButton
                          size="small"
                          color="error"
                          onClick={() => {
                            setTemplateToDelete(template);
                            setDeleteDialogOpen(true);
                          }}
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

      {/* Create/Edit Dialog */}
      <Dialog
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>
          {editingTemplate ? "Edit Template" : "Create Template"}
        </DialogTitle>
        <DialogContent>
          <TextField
            fullWidth
            label="Name"
            value={formData.name}
            onChange={(e) =>
              setFormData({ ...formData, name: e.target.value })
            }
            required
            sx={{ mt: 1 }}
          />
          <TextField
            fullWidth
            label="Attestation Text"
            value={formData.text}
            onChange={(e) =>
              setFormData({ ...formData, text: e.target.value })
            }
            multiline
            rows={3}
            required
            sx={{ mt: 2 }}
            helperText="Supports Markdown — use [link text](https://url) for links"
          />
          <FormControl fullWidth sx={{ mt: 2 }}>
            <InputLabel>Applies To</InputLabel>
            <Select
              value={formData.applies_to_type}
              label="Applies To"
              onChange={(e) =>
                setFormData({ ...formData, applies_to_type: e.target.value })
              }
            >
              <MenuItem value="all">All resource types</MenuItem>
              <MenuItem value="datasource">Data sources only</MenuItem>
              <MenuItem value="tool">Tools only</MenuItem>
            </Select>
          </FormControl>
          <TextField
            fullWidth
            label="Sort Order"
            type="number"
            value={formData.sort_order}
            onChange={(e) =>
              setFormData({ ...formData, sort_order: e.target.value })
            }
            sx={{ mt: 2 }}
          />
          <Box sx={{ mt: 2, display: "flex", gap: 3 }}>
            <FormControlLabel
              control={
                <Switch
                  checked={formData.required}
                  onChange={(e) =>
                    setFormData({ ...formData, required: e.target.checked })
                  }
                />
              }
              label="Required"
            />
            <FormControlLabel
              control={
                <Switch
                  checked={formData.active}
                  onChange={(e) =>
                    setFormData({ ...formData, active: e.target.checked })
                  }
                />
              }
              label="Active"
            />
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDialogOpen(false)}>Cancel</Button>
          <Button
            onClick={handleSave}
            variant="contained"
            disabled={saving}
          >
            {saving ? (
              <CircularProgress size={20} />
            ) : editingTemplate ? (
              "Update"
            ) : (
              "Create"
            )}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Delete Dialog */}
      <Dialog
        open={deleteDialogOpen}
        onClose={() => setDeleteDialogOpen(false)}
      >
        <DialogTitle>Delete Template</DialogTitle>
        <DialogContent>
          <Typography>
            Are you sure you want to delete "{templateToDelete?.name}"?
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteDialogOpen(false)}>Cancel</Button>
          <Button onClick={handleDelete} color="error">
            Delete
          </Button>
        </DialogActions>
      </Dialog>

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={() => setSnackbar({ ...snackbar, open: false })}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={() => setSnackbar({ ...snackbar, open: false })}
          severity={snackbar.severity}
          sx={{ width: "100%" }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </>
  );
};

export default AttestationTemplates;
