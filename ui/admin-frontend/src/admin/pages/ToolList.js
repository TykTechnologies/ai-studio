import React, { useState, useEffect, useCallback, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import { Typography, Alert, Box, Stack } from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DownloadIcon from "@mui/icons-material/Download";
import DataTable from "../components/common/DataTable";
import ActiveStatusDot from "../components/common/ActiveStatusDot";
import PrivacyLevelChip from "../components/common/privacy/PrivacyLevelChip";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import DeleteConfirmationDialog from "../components/common/DeleteConfirmationDialog";
import BulkDeleteConfirmationDialog from "../components/common/BulkDeleteConfirmationDialog";
import BulkResultAlert from "../components/common/BulkResultAlert";
import FeedbackSnackbar, { useFeedbackSnackbar } from "../components/common/FeedbackSnackbar";
import ImportOpenAPIWizard from "../components/tools/ImportOpenAPIWizard";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  PrimaryOutlineButton,
} from "../styles/sharedStyles";
import useListQuery from "../hooks/useListQuery";
import useBulkActions, { standardBulkActions } from "../hooks/useBulkActions";
import useConfig from "../hooks/useConfig";
import Can from "../components/rbac/Can";
import { P } from "../rbac/permissions";

const ToolList = () => {
  const navigate = useNavigate();
  const { getDocsLink } = useConfig();
  const [tools, setTools] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deleteTarget, setDeleteTarget] = useState(null);
  const [importWizardOpen, setImportWizardOpen] = useState(false);
  const { notify, snackbarProps } = useFeedbackSnackbar();

  const { queryParams, updatePaginationData, searchTerm, tableProps } = useListQuery();

  const fetchTools = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/tools", { params: queryParams });
      setTools(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching tools", error);
      setError("Failed to load tools");
    } finally {
      setLoading(false);
    }
  }, [queryParams, updatePaginationData]);

  useEffect(() => {
    fetchTools();
  }, [fetchTools]);

  const bulk = useBulkActions({
    items: tools,
    resource: "tools",
    singular: "tool",
    plural: "tools",
    notify,
    refresh: fetchTools,
  });

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/tools/${id}`);
      notify("Tool deleted successfully");
      fetchTools();
    } catch (error) {
      console.error("Error deleting tool", error);
      notify("Failed to delete tool", "error");
    }
  };

  const handleToolClick = (tool) => {
    navigate(`/admin/tools/${tool.id}`);
  };

  const handleAddTool = () => {
    navigate("/admin/tools/new");
  };

  const handleImportTool = async (toolData) => {
    try {
      notify("Tool imported successfully");
      // Navigate to the tool details page
      navigate(`/admin/tools/${toolData.id}`);
    } catch (error) {
      console.error("Error importing tool", error);
      notify("Failed to import tool", "error");
    } finally {
      setLoading(false);
    }
  };

  const columns = useMemo(() => [
    { field: "name", headerName: "Name", sortable: true, renderCell: (tool) => tool.attributes.name },
    { field: "description", headerName: "Description", renderCell: (tool) => tool.attributes.description },
    {
      field: "privacy_score",
      headerName: "Privacy Level",
      sortable: true,
      renderCell: (tool) => <PrivacyLevelChip score={tool.attributes.privacy_score} />,
    },
    {
      field: "active",
      headerName: "Active",
      sortable: true,
      renderCell: (tool) => <ActiveStatusDot active={tool.attributes.active !== false} />,
    },
  ], []);

  // The row toggle goes through the same bulk endpoint as the toolbar so the
  // list and the tool form (which has the switch) agree on what "active" is.
  const runBulk = bulk.run;
  const rowActions = useMemo(() => [
    { key: "edit", label: "Edit tool", onClick: (tool) => navigate(`/admin/tools/edit/${tool.id}`) },
    { key: "delete", label: "Delete tool", onClick: (tool) => setDeleteTarget(tool) },
    {
      key: "toggle",
      label: (tool) => `${tool?.attributes?.active !== false ? "Deactivate" : "Activate"} tool`,
      onClick: (tool) => runBulk(tool.attributes.active !== false ? "deactivate" : "activate", [tool]),
    },
  ], [navigate, runBulk]);

  const bulkActions = useMemo(
    () => standardBulkActions({ run: bulk.run, requestDelete: bulk.requestDelete, canToggle: true }),
    [bulk.run, bulk.requestDelete],
  );

  if (error && tools.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Tools</Typography>
        <Stack direction="row" spacing={2}>
          <PrimaryOutlineButton
            variant="contained"
            startIcon={<DownloadIcon />}
            onClick={() => setImportWizardOpen(true)}
          >
            Import OpenAPI
          </PrimaryOutlineButton>
          <Can permission={P.TOOLS_WRITE}>
            <PrimaryButton
              variant="contained"
              startIcon={<AddIcon />}
              onClick={handleAddTool}
            >
              Add tool
            </PrimaryButton>
          </Can>
        </Stack>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Tools are external services that enhance the AI's capabilities by providing access to additional data and functions within chat rooms. Defined by the OpenAPI specification, you can specify which operations the LLM can use to fulfill user requests effectively.</Typography>
      </Box>
      <ContentBox>
        <BulkResultAlert action={bulk.failures?.action} failures={bulk.failures?.failures} onClose={bulk.clearFailures} />
        <Can permission={P.TOOLS_WRITE}>
          {(canWrite) => (
            <DataTable
              {...tableProps}
              ariaLabel="Tools"
              searchPlaceholder="Search tools by name..."
              columns={columns}
              data={tools}
              loading={loading}
              onRowClick={handleToolClick}
              actions={canWrite ? rowActions : undefined}
              {...(canWrite ? bulk.selectionProps : {})}
              bulkActions={canWrite ? bulkActions : undefined}
              emptyState={
                !searchTerm ? (
                  <EmptyStateWidget
                    title="No tools added yet"
                    description="Tools are external services that can be used in chat rooms to enhance or provide additional data access and capabilities to the AI that the user is interacting with. Tools are defined by an OpenAPI specification, and you can define which operations are available to the LLM to use from the spec as functions it can call to fulfil the user request."
                    learnMoreLink={getDocsLink("tools")}
                    actions={
                      <>
                        <PrimaryOutlineButton
                          variant="contained"
                          startIcon={<DownloadIcon />}
                          onClick={() => setImportWizardOpen(true)}
                        >
                          Import OpenAPI
                        </PrimaryOutlineButton>
                        {canWrite && (
                          <PrimaryButton
                            variant="contained"
                            startIcon={<AddIcon />}
                            onClick={handleAddTool}
                          >
                            Add tool
                          </PrimaryButton>
                        )}
                      </>
                    }
                  />
                ) : undefined
              }
            />
          )}
        </Can>
      </ContentBox>

      <ImportOpenAPIWizard
        open={importWizardOpen}
        onClose={() => setImportWizardOpen(false)}
        onImport={handleImportTool}
      />

      <DeleteConfirmationDialog
        open={Boolean(deleteTarget)}
        resourcePath="tools"
        objectLabel="tool"
        item={deleteTarget ? { id: deleteTarget.id, name: deleteTarget.attributes?.name } : null}
        consequence="Deleting it removes it from all of them; chats and agents that call it lose the tool."
        onConfirm={() => {
          const id = deleteTarget?.id;
          setDeleteTarget(null);
          handleDelete(id);
        }}
        onCancel={() => setDeleteTarget(null)}
      />

      <BulkDeleteConfirmationDialog
        open={bulk.deleteDialogOpen}
        resourcePath="tools"
        objectLabel="tool"
        objectLabelPlural="tools"
        items={bulk.deleteDialogItems}
        consequence="Deleting them removes them from all of those; chats and agents that call them lose the tools."
        onConfirm={bulk.confirmDelete}
        onCancel={bulk.cancelDelete}
      />

      <FeedbackSnackbar {...snackbarProps} />
    </>
  );
};

export default ToolList;
