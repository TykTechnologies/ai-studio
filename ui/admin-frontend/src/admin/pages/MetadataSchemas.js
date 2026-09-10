import React, { useState, useEffect, useCallback } from "react";
import { useNavigate } from "react-router-dom";
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
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import EditIcon from "@mui/icons-material/Edit";
import DeleteIcon from "@mui/icons-material/Delete";
import FactCheckOutlinedIcon from "@mui/icons-material/FactCheckOutlined";
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
  getMetadataSchemas,
  deleteMetadataSchema,
  getMetadataObjectTypes,
} from "../services/governedMetadataService";

export const isPluginSourcedSchema = (schema) =>
  typeof schema?.source === "string" && schema.source.startsWith("plugin:");

const MetadataSchemas = () => {
  const navigate = useNavigate();
  const [available, setAvailable] = useState(null);
  const [schemas, setSchemas] = useState([]);
  const [objectTypes, setObjectTypes] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [toDelete, setToDelete] = useState(null);
  const [snackbar, setSnackbar] = useState({ open: false, message: "", severity: "success" });

  const fetchAll = useCallback(async () => {
    try {
      setLoading(true);
      const [list, types] = await Promise.all([getMetadataSchemas(), getMetadataObjectTypes()]);
      setSchemas(list);
      setObjectTypes(types);
      setError("");
    } catch (err) {
      setError("Failed to load metadata schemas");
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

  const typeLabel = (slug) =>
    slug === "*" ? "All object types" : objectTypes.find((t) => t.slug === slug)?.label || slug;

  const handleDelete = async () => {
    if (!toDelete) return;
    try {
      await deleteMetadataSchema(toDelete.id);
      setSnackbar({ open: true, message: "Schema deleted", severity: "success" });
      setToDelete(null);
      fetchAll();
    } catch (err) {
      setSnackbar({ open: true, message: err.message || "Failed to delete schema", severity: "error" });
      setToDelete(null);
    }
  };

  return (
    <>
      <TitleBox top="64px">
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <FactCheckOutlinedIcon />
          <Typography variant="headingXLarge">Metadata Schemas</Typography>
          <Chip label="Enterprise" size="small" color="primary" />
        </Box>
        {available && (
          <PrimaryButton startIcon={<AddIcon />} onClick={() => navigate("/admin/metadata/schemas/new")}>
            Add Schema
          </PrimaryButton>
        )}
      </TitleBox>

      <ContentBox sx={{ pt: 0 }}>
        {available === false && (
          <EnterpriseFeatureBadge
            feature="Governed Metadata"
            description="Define required ownership, lifecycle, risk and classification metadata for LLMs, tools and data sources with the Enterprise Edition."
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
                  <StyledTableHeaderCell>Applies To</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Fields</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Enforcement</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Active</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Source</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Actions</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {schemas.length === 0 ? (
                  <TableRow>
                    <StyledTableCell colSpan={7} align="center">
                      <Typography color="text.secondary" sx={{ py: 3 }}>
                        No metadata schemas yet. Create one to define which governance fields your objects must carry.
                      </Typography>
                    </StyledTableCell>
                  </TableRow>
                ) : (
                  schemas.map((schema) => (
                    <StyledTableRow key={schema.id}>
                      <StyledTableCell>
                        <Typography variant="body2" fontWeight="medium">
                          {schema.name}
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          <code>{schema.slug}</code>
                        </Typography>
                      </StyledTableCell>
                      <StyledTableCell>
                        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                          {(schema.applies_to || []).map((slug) => (
                            <Chip key={slug} label={typeLabel(slug)} size="small" variant="outlined" />
                          ))}
                        </Box>
                      </StyledTableCell>
                      <StyledTableCell>{(schema.fields || []).length}</StyledTableCell>
                      <StyledTableCell>
                        {schema.enforcement === "enforce" ? (
                          <Chip label="Enforced" size="small" color="error" />
                        ) : (
                          <Chip label="Advisory" size="small" variant="outlined" />
                        )}
                      </StyledTableCell>
                      <StyledTableCell>
                        <Chip
                          label={schema.active ? "Active" : "Inactive"}
                          size="small"
                          color={schema.active ? "success" : "default"}
                        />
                      </StyledTableCell>
                      <StyledTableCell>
                        <Chip
                          label={isPluginSourcedSchema(schema) ? `Plugin ${schema.source.slice(7)}` : "Admin"}
                          size="small"
                          variant="outlined"
                        />
                      </StyledTableCell>
                      <StyledTableCell>
                        <IconButton
                          size="small"
                          aria-label={`Edit ${schema.name}`}
                          onClick={() => navigate(`/admin/metadata/schemas/edit/${schema.id}`)}
                        >
                          <EditIcon fontSize="small" />
                        </IconButton>
                        <IconButton
                          size="small"
                          color="error"
                          aria-label={`Delete ${schema.name}`}
                          disabled={isPluginSourcedSchema(schema)}
                          onClick={() => setToDelete(schema)}
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

      <ConfirmationDialog
        title="Delete Schema"
        message={`Delete "${toDelete?.name}"? Values already stored on objects are kept but no longer validated.`}
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

export default MetadataSchemas;
