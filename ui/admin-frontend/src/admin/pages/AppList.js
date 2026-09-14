import React, { useState, useEffect, useCallback, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import { deactivateCredential } from "../services/appService";
import { APP_STATUS, APP_STATUS_ORDER, getAppStatus } from "../../utils/appStatus";
import { Typography, Alert, Box, Chip } from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import WarningIcon from "@mui/icons-material/Warning";
import SecurityIcon from "@mui/icons-material/Security";
import DataTable from "../components/common/DataTable";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import ConfirmationDialog from "../components/common/ConfirmationDialog";
import BulkDeleteConfirmationDialog from "../components/common/BulkDeleteConfirmationDialog";
import BulkResultAlert from "../components/common/BulkResultAlert";
import FeedbackSnackbar, { useFeedbackSnackbar } from "../components/common/FeedbackSnackbar";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
} from "../styles/sharedStyles";
import useListQuery from "../hooks/useListQuery";
import useBulkActions, { standardBulkActions } from "../hooks/useBulkActions";
import Can from "../components/rbac/Can";
import { P } from "../rbac/permissions";

const AppList = () => {
  const navigate = useNavigate();
  const [apps, setApps] = useState([]);
  const [users, setUsers] = useState({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const { notify, snackbarProps } = useFeedbackSnackbar();
  const [confirmDialog, setConfirmDialog] = useState({
    open: false,
    appId: null,
    appName: "",
  });

  const { queryParams, sortConfig, updatePaginationData, searchTerm, tableProps } = useListQuery({
    initialSort: { field: "id", direction: "desc" },
  });

  const fetchApps = useCallback(async () => {
    try {
      setLoading(true);

      const params = { ...queryParams };
      // Approval status is calculated client-side, so the server sorts by id
      // and the page is re-ordered below.
      if (sortConfig?.field === "approval_status") {
        params.sort = sortConfig.direction === "desc" ? "-id" : "id";
      }

      const response = await apiClient.get("/apps", { params });

      let appsData = response.data.data || [];

      // If sorting by approval status, we need to sort client-side
      if (sortConfig?.field === "approval_status") {
        appsData = [...appsData].sort((a, b) => {
          const statusA = getApprovalStatus(a);
          const statusB = getApprovalStatus(b);

          // Order: Active > Awaiting approval > No credential > Disabled
          const comparison = APP_STATUS_ORDER[statusA] - APP_STATUS_ORDER[statusB];
          return sortConfig.direction === "asc" ? comparison : -comparison;
        });
      }

      setApps(appsData);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching apps", error);
      setError("Failed to load apps");
    } finally {
      setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queryParams, sortConfig, updatePaginationData]);

  useEffect(() => {
    fetchApps();
  }, [fetchApps]);

  useEffect(() => {
    fetchUsers();
  }, []);

  const fetchUsers = async () => {
    try {
      // Request all users by setting all=true
      const response = await apiClient.get("/users", {
        params: {
          all: true,
          // Add a large page_size as a fallback in case 'all' is not working
          page_size: 1000
        }
      });
      const userMap = {};
      response.data.data.forEach((user) => {
        userMap[user.id] = user.attributes.name;
      });
      setUsers(userMap);
    } catch (error) {
      console.error("Error fetching users", error);
    }
  };

  const bulk = useBulkActions({
    items: apps,
    resource: "apps",
    singular: "app",
    plural: "apps",
    notify,
    refresh: fetchApps,
  });

  // Same words as the portal (utils/appStatus.js): Active, Awaiting approval,
  // No credential, Disabled.
  // The list response carries credential_active (null when the app has no
  // credential), so no separate credentials fetch is needed.
  const getApprovalStatus = (app) => {
    const credentialActive = app.attributes.credential_active;
    return getAppStatus({
      isActive: app.attributes.is_active,
      hasCredential: credentialActive !== null && credentialActive !== undefined,
      credentialActive: Boolean(credentialActive),
    });
  };

  const getUserDisplay = (app) => {
    if (app.attributes.is_orphaned) {
      return (
        <Box display="flex" alignItems="center" gap={1}>
          <Chip
            icon={<WarningIcon />}
            label="Orphaned App"
            color="warning"
            size="small"
            variant="outlined"
          />
        </Box>
      );
    }

    return users[app.attributes.user_id] || "Unknown";
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/apps/${id}`);
      notify("App deleted successfully");
      fetchApps();
    } catch (error) {
      console.error("Error deleting app", error);
      notify("Failed to delete app", "error");
    }
  };

  // Approve = activate the app's credential, the same PATCH the app form
  // sends from its "active" switch.
  const handleApproveCredentials = async (app) => {
    const credentialId = app?.attributes?.credential_id;
    if (!credentialId) return;
    try {
      await apiClient.patch(`/credentials/${credentialId}`, {
        data: { type: "credentials", attributes: { active: true } },
      });
      notify("App credentials approved");
      fetchApps();
    } catch (error) {
      console.error("Error approving credentials", error);
      notify("Failed to approve credentials", "error");
    }
  };

  const handleDisableCredentials = (app) => {
    setConfirmDialog({
      open: true,
      appId: app.id,
      appName: app.attributes.name,
    });
  };

  const handleConfirmDisableCredentials = async () => {
    try {
      await deactivateCredential(confirmDialog.appId);
      notify("App credentials disabled successfully");
      // Refresh credentials and apps data
      fetchApps();
    } catch (error) {
      console.error("Error disabling credentials", error);
      notify("Failed to disable credentials", "error");
    }
    setConfirmDialog({ open: false, appId: null, appName: "" });
  };

  const handleCancelDisableCredentials = () => {
    setConfirmDialog({ open: false, appId: null, appName: "" });
  };

  const handleAppClick = (app) => {
    navigate(`/admin/apps/${app.id}`);
  };

  const handleAddApp = () => {
    navigate("/admin/apps/new");
  };

  const columns = [
    { field: "id", headerName: "ID", sortable: true },
    { field: "name", headerName: "Name", sortable: true, renderCell: (app) => app.attributes.name },
    { field: "description", headerName: "Description", sortable: true, renderCell: (app) => app.attributes.description },
    { field: "user_id", headerName: "User", sortable: true, renderCell: (app) => getUserDisplay(app) },
    { field: "approval_status", headerName: "Status", sortable: true, renderCell: (app) => getApprovalStatus(app) },
    {
      field: "monthly_budget",
      headerName: "Budget",
      sortable: true,
      renderCell: (app) =>
        app.attributes.monthly_budget
          ? `$${parseFloat(app.attributes.monthly_budget).toFixed(2)}`
          : "Not set",
    },
  ];

  const rowActions = [
    { key: "edit", label: "Edit app", onClick: (app) => navigate(`/admin/apps/edit/${app.id}`) },
    {
      key: "approve",
      label: "Approve credentials",
      icon: <SecurityIcon sx={{ mr: 1, fontSize: 20 }} />,
      hidden: (app) => !app || getApprovalStatus(app) !== APP_STATUS.AWAITING_APPROVAL,
      onClick: handleApproveCredentials,
    },
    {
      key: "disable",
      label: "Disable credentials",
      icon: <SecurityIcon sx={{ mr: 1, fontSize: 20 }} />,
      disabled: (app) => !app || getApprovalStatus(app) !== APP_STATUS.ACTIVE,
      onClick: handleDisableCredentials,
    },
    { key: "delete", label: "Delete app", onClick: (app) => handleDelete(app.id) },
  ];

  const bulkActions = useMemo(
    () => standardBulkActions({ run: bulk.run, requestDelete: bulk.requestDelete, canToggle: true }),
    [bulk.run, bulk.requestDelete],
  );

  if (error && apps.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">Apps</Typography>
          <Can permission={P.APPS_WRITE}>
            <PrimaryButton
              variant="contained"
              startIcon={<AddIcon />}
              onClick={handleAddApp}
            >
              Add app
            </PrimaryButton>
          </Can>
        </TitleBox>
        <Box sx={{ p: 3 }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Apps are used to grant developers direct access to LLMs and data sources in the AI Portal. With active credentials, an app can use the gateway API to work directly with LLMs or access the data source API to search through data. You can create apps for specific developers or set up catalogs so they can request access and customize their setup.</Typography>
        </Box>
        <ContentBox>
          <BulkResultAlert action={bulk.failures?.action} failures={bulk.failures?.failures} onClose={bulk.clearFailures} />
          <Can permission={P.APPS_WRITE}>
            {(canWrite) => (
              <DataTable
                {...tableProps}
                ariaLabel="Apps"
                searchPlaceholder="Search by name, description, or user..."
                columns={columns}
                data={apps}
                loading={loading}
                onRowClick={handleAppClick}
                actions={canWrite ? rowActions : undefined}
                {...(canWrite ? bulk.selectionProps : {})}
                bulkActions={canWrite ? bulkActions : undefined}
                emptyMessage={searchTerm ? `No apps found matching "${searchTerm}"` : "No apps found"}
                emptyState={
                  !searchTerm ? (
                    <EmptyStateWidget
                      title="No apps configured yet"
                      description="Apps are requests by users to access LLMs and data sources in the AI Portal. An app with an active credential can access the gateway API to work directly with LLMs, or use the portal data source API to search data sources. Click the button below to add a new app configuration."
                      buttonText="Add App"
                      buttonIcon={<AddIcon />}
                      onButtonClick={canWrite ? handleAddApp : undefined}
                    />
                  ) : undefined
                }
              />
            )}
          </Can>
        </ContentBox>
      </>

      <BulkDeleteConfirmationDialog
        open={bulk.deleteDialogOpen}
        resourcePath={null}
        objectLabel="app"
        objectLabelPlural="apps"
        items={bulk.deleteDialogItems}
        onConfirm={bulk.confirmDelete}
        onCancel={bulk.cancelDelete}
      />

      <FeedbackSnackbar {...snackbarProps} />

      <ConfirmationDialog
        open={confirmDialog.open}
        title="Disable App Credentials"
        message={`Are you sure you want to disable credentials for "${confirmDialog.appName}"? This will prevent the app from accessing the API.`}
        confirmText="The app will lose access immediately."
        buttonLabel="Disable Credentials"
        onConfirm={handleConfirmDisableCredentials}
        onCancel={handleCancelDisableCredentials}
        iconName="hexagon-exclamation"
        iconColor="background.buttonCritical"
        titleColor="text.criticalDefault"
        backgroundColor="background.surfaceCriticalDefault"
        borderColor="border.criticalDefaultSubdue"
        primaryButtonComponent="danger"
      />
    </>
  );
};

export default AppList;
