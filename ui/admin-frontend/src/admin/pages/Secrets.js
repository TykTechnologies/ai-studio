import React, { useState, useEffect, useCallback, useMemo } from "react";
import { useNavigate, Link as RouterLink } from "react-router-dom";
import apiClient from "../utils/apiClient";
import AddIcon from "@mui/icons-material/Add";
import WarningIcon from "@mui/icons-material/Warning";
import DataTable from "../components/common/DataTable";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import DeleteConfirmationDialog from "../components/common/DeleteConfirmationDialog";
import BulkDeleteConfirmationDialog from "../components/common/BulkDeleteConfirmationDialog";
import BulkResultAlert from "../components/common/BulkResultAlert";
import FeedbackSnackbar, { useFeedbackSnackbar } from "../components/common/FeedbackSnackbar";
import {
  Typography,
  Box,
  Paper,
  Chip,
  Link,
} from "@mui/material";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
} from "../styles/sharedStyles";
import useListQuery from "../hooks/useListQuery";
import useBulkActions, { standardBulkActions } from "../hooks/useBulkActions";
import Can from "../components/rbac/Can";
import { P } from "../rbac/permissions";

// Links each LLM that references the secret as $SECRET/<name>.
const renderReferencedBy = (referencedBy) => {
  const refs = Array.isArray(referencedBy) ? referencedBy : [];
  if (refs.length === 0) {
    return (
      <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
        Not referenced
      </Typography>
    );
  }
  return (
    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, alignItems: "center" }}>
      {refs.map((ref, index) => (
        <React.Fragment key={`${ref.type}-${ref.id}`}>
          {ref.type === "llm" ? (
            <Link component={RouterLink} to={`/admin/llms/${ref.id}`}>
              {ref.name}
            </Link>
          ) : (
            <span>{ref.name}</span>
          )}
          {index < refs.length - 1 ? "," : ""}
        </React.Fragment>
      ))}
    </Box>
  );
};

const secretName = (secret) => secret?.attributes?.var_name || String(secret?.id ?? "");

const Secrets = () => {
  const navigate = useNavigate();
  const [secrets, setSecrets] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deleteTarget, setDeleteTarget] = useState(null);
  const { notify, snackbarProps } = useFeedbackSnackbar();

  const { queryParams, updatePaginationData, searchTerm, tableProps } = useListQuery();

  const fetchSecrets = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/secrets", { params: queryParams });
      setSecrets(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching secrets", error);
      if (error.response?.status === 503) {
        setError(error.response.data.errors[0].detail);
      } else {
        setError("Failed to load secrets");
      }
    } finally {
      setLoading(false);
    }
  }, [queryParams, updatePaginationData]);

  useEffect(() => {
    fetchSecrets();
  }, [fetchSecrets]);

  const bulk = useBulkActions({
    items: secrets,
    resource: "secrets",
    singular: "secret",
    plural: "secrets",
    nameOf: secretName,
    notify,
    refresh: fetchSecrets,
  });

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/secrets/${id}`);
      notify("Secret deleted successfully");
      fetchSecrets();
    } catch (error) {
      console.error("Error deleting secret", error);
      notify("Failed to delete secret", "error");
    }
  };

  const handleSecretClick = (secret) => {
    navigate(`/admin/secrets/${secret.id}`);
  };

  const handleAddSecret = () => {
    navigate("/admin/secrets/new");
  };

  const columns = useMemo(() => [
    { field: "id", headerName: "ID", sortable: true },
    { field: "var_name", headerName: "Variable Name", sortable: true, renderCell: (secret) => secret.attributes.var_name },
    {
      field: "has_value",
      headerName: "Value",
      // The list never returns the value itself, only whether one is stored.
      renderCell: (secret) =>
        secret.attributes.has_value ? (
          <Chip label="Set" color="success" size="small" variant="outlined" />
        ) : (
          <Chip label="Empty" color="warning" size="small" variant="outlined" />
        ),
    },
    {
      field: "referenced_by",
      headerName: "Used by",
      cellProps: () => ({ onClick: (event) => event.stopPropagation() }),
      renderCell: (secret) => renderReferencedBy(secret.attributes.referenced_by),
    },
  ], []);

  const rowActions = useMemo(() => [
    { key: "edit", label: "Edit secret", onClick: (secret) => navigate(`/admin/secrets/edit/${secret.id}`) },
    { key: "delete", label: "Delete secret", onClick: (secret) => setDeleteTarget(secret) },
  ], [navigate]);

  const bulkActions = useMemo(
    () => standardBulkActions({ run: bulk.run, requestDelete: bulk.requestDelete }),
    [bulk.run, bulk.requestDelete],
  );

  if (error && secrets.length === 0) {
    return (
      <Box sx={{ p: 3 }}>
        <Paper
          elevation={0}
          sx={{
            p: 3,
            backgroundColor: '',
            color: 'warning.dark',
            display: 'flex',
            alignItems: 'flex-start',
            gap: 2,
            mb: 3
          }}
        >
          <WarningIcon color="error" sx={{ mt: 0.5 }} />
          <Box>
            <Typography variant="h6" color="warning.  " gutterBottom>
              Secrets Management Unavailable
            </Typography>
            <Typography variant="body1" color="warning.dark">
              {error}
            </Typography>
          </Box>
        </Paper>
        <Paper elevation={0} sx={{ p: 3 }}>
          <Typography variant="h6" gutterBottom>
            How to Fix This
          </Typography>
          <Typography variant="body1" color="text.secondary" paragraph>
            To enable secrets management, you need to:
          </Typography>
          <Box component="ol" sx={{ color: 'text.secondary', pl: 2 }}>
            <li>
              <Typography variant="body1">
                Set the TYK_AI_SECRET_KEY environment variable with any string value
              </Typography>
            </li>
            <li>
              <Typography variant="body1">
                Restart the server to apply the changes
              </Typography>
            </li>
          </Box>
          <Typography variant="body1" color="text.secondary" sx={{ mt: 2 }}>
            For more information, please refer to the documentation.
          </Typography>
        </Paper>
      </Box>
    );
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Secrets</Typography>
        <Can permission={P.SECRETS_WRITE}>
          <PrimaryButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddSecret}
          >
            Add secret
          </PrimaryButton>
        </Can>
      </TitleBox>
      <ContentBox>
        <BulkResultAlert action={bulk.failures?.action} failures={bulk.failures?.failures} onClose={bulk.clearFailures} />
        <Can permission={P.SECRETS_WRITE}>
          {(canWrite) => (
            <DataTable
              {...tableProps}
              ariaLabel="Secrets"
              searchPlaceholder="Search secrets by variable name..."
              columns={columns}
              data={secrets}
              loading={loading}
              onRowClick={handleSecretClick}
              getRowLabel={secretName}
              actions={canWrite ? rowActions : undefined}
              {...(canWrite ? bulk.selectionProps : {})}
              bulkActions={canWrite ? bulkActions : undefined}
              emptyState={
                !searchTerm ? (
                  <EmptyStateWidget
                    title="No secrets found"
                    description="Click the button below to add a new secret."
                    buttonText="Add Secret"
                    buttonIcon={<AddIcon />}
                    onButtonClick={canWrite ? handleAddSecret : undefined}
                  />
                ) : undefined
              }
            />
          )}
        </Can>
      </ContentBox>

      <DeleteConfirmationDialog
        open={Boolean(deleteTarget)}
        resourcePath="secrets"
        objectLabel="secret"
        item={deleteTarget ? { id: deleteTarget.id, name: deleteTarget.attributes?.var_name } : null}
        consequence="Deleting it removes the stored value; anything referencing it as $SECRET/name will stop authenticating."
        onConfirm={() => {
          const id = deleteTarget?.id;
          setDeleteTarget(null);
          handleDelete(id);
        }}
        onCancel={() => setDeleteTarget(null)}
      />

      <BulkDeleteConfirmationDialog
        open={bulk.deleteDialogOpen}
        resourcePath="secrets"
        objectLabel="secret"
        objectLabelPlural="secrets"
        items={bulk.deleteDialogItems}
        consequence="Deleting them removes the stored values; anything referencing them as $SECRET/name will stop authenticating."
        onConfirm={bulk.confirmDelete}
        onCancel={bulk.cancelDelete}
      />

      <FeedbackSnackbar {...snackbarProps} />
    </>
  );
};

export default Secrets;
