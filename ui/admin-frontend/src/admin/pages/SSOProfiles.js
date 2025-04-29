import React, { useState, useEffect, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Alert,
  Snackbar,
  Box,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DataTable from "../components/common/DataTable";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import ConfirmationDialog from "../components/common/ConfirmationDialog";
import SuccessBanner from "../components/common/SuccessBanner";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
} from "../styles/sharedStyles";
import usePagination from "../hooks/usePagination";
import { format } from "date-fns";

// Constant for localStorage key
const SSO_NOTIFICATION_KEY = 'tyk_ai_studio_admin_sso_notification';

const SSOProfiles = () => {
  const navigate = useNavigate();
  const [profiles, setProfiles] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [successBanner, setSuccessBanner] = useState({
    show: false,
    title: "",
    message: ""
  });
  const [sortField, setSortField] = useState("profile_id");
  const [sortOrder, setSortOrder] = useState("desc");
  const [warningDialogOpen, setWarningDialogOpen] = useState(false);
  const [profileToDelete, setProfileToDelete] = useState(null);
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false);
  const [profileToSetDefault, setProfileToSetDefault] = useState(null);

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchProfiles = useCallback(async () => {
    try {
      setLoading(true);
      const sortParam = `${sortOrder === "desc" ? "-" : ""}${sortField}`;
      
      const response = await apiClient.get("/sso-profiles", {
        params: {
          page,
          page_size: pageSize,
          sort: sortParam,
        },
      });
      
      setProfiles(response.data.data || []);
      const totalCount = response.data.meta.total_count || 0;
      const totalPages = response.data.meta.total_pages || 0;
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching Identity provider profiles", error);
      setError("Failed to load Identity provider profiles");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, updatePaginationData, sortField, sortOrder]);

  useEffect(() => {
    fetchProfiles();
  }, [fetchProfiles]);

  useEffect(() => {
    const notificationData = localStorage.getItem(SSO_NOTIFICATION_KEY);
    
    if (notificationData) {
      try {
        const notification = JSON.parse(notificationData);
        const isStillRelevant = Date.now() - notification.timestamp < 5 * 60 * 1000;
        
        if (isStillRelevant) {
          if (notification.operation === "create") {
            setSuccessBanner({
              show: true,
              title: notification.title,
              message: notification.message
            });
          } else {
            setSnackbar({
              open: true,
              message: notification.message,
              severity: "success",
            });
          }
        }
        
        localStorage.removeItem(SSO_NOTIFICATION_KEY);
      } catch (error) {
        console.error('Error parsing notification data', error);
        localStorage.removeItem(SSO_NOTIFICATION_KEY);
      }
    }
    
  }, []);

  const handleDeleteClick = (profile) => {
    setProfileToDelete(profile);
    setWarningDialogOpen(true);
  };

  const handleCancelDelete = () => {
    setWarningDialogOpen(false);
    setProfileToDelete(null);
  };

  const handleSetDefaultClick = (profile) => {
    setProfileToSetDefault(profile);
    setConfirmDialogOpen(true);
  };

  const handleConfirmSetDefault = async () => {
    if (!profileToSetDefault) return;
    
    try {
      await apiClient.post(`/sso-profiles/${profileToSetDefault.attributes.profile_id}/use-in-login-page`);
      setSnackbar({
        open: true,
        message: "Default IdP profile for SSO login set successfully",
        severity: "success",
      });
      fetchProfiles();
    } catch (error) {
      console.error("Error setting default IdP profile for SSO login", error);
      setSnackbar({
        open: true,
        message: "Failed to set default IdP profile for SSO login",
        severity: "error",
      });
    } finally {
      setConfirmDialogOpen(false);
      setProfileToSetDefault(null);
    }
  };

  const handleCancelSetDefault = () => {
    setConfirmDialogOpen(false);
    setProfileToSetDefault(null);
  };

  const handleConfirmDelete = async () => {
    if (!profileToDelete) return;
    
    try {
      await apiClient.delete(`/sso-profiles/${profileToDelete.attributes.profile_id}`);
      setSnackbar({
        open: true,
        message: "Identity provider profile deleted successfully",
        severity: "success",
      });
      fetchProfiles();
    } catch (error) {
      console.error("Error deleting Identity provider profile", error);
      setSnackbar({
        open: true,
        message: "Failed to delete Identity provider profile",
        severity: "error",
      });
    } finally {
      setWarningDialogOpen(false);
      setProfileToDelete(null);
    }
  };

  const handleProfileClick = (profile) => {
    navigate(`/admin/sso-profiles/${profile.attributes.profile_id}`);
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };
  
  const handleCloseBanner = () => {
    setSuccessBanner(current => ({ ...current, show: false }));
  };

  const handleAddProfile = () => {
    navigate("/admin/sso-profiles/new");
  };

  const formatDate = (dateString) => {
    try {
      return format(new Date(dateString), "MMM d, yyyy h:mm a");
    } catch (error) {
      return dateString;
    }
  };

  const handleSortChange = (newSortConfig) => {
    setSortField(newSortConfig.field);
    setSortOrder(newSortConfig.direction);
  };

  const columns = [
    {
      field: "name",
      headerName: "Profile Name",
      sortable: true,
      renderCell: (item) => (
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          {item.attributes.name || item.attributes.profile_id}
          {item.attributes.use_in_login_page && (
            <Box
              sx={{
                backgroundColor: "background.neutralDefault",
                borderRadius: "6px",
                padding: "2px 6px",
              }}
            >
              <Typography variant="bodySmallDefault" color="text.primary">
                Default
              </Typography>
            </Box>
          )}
        </Box>
      ),
    },
    {
      field: "profile_id",
      headerName: "Profile ID",
      sortable: true,
      renderCell: (item) => item.attributes.profile_id,
    },
    {
      field: "profile_type",
      headerName: "Profile Type",
      sortable: true,
      renderCell: (item) => item.attributes.profile_type || "-",
    },
    {
      field: "provider_type",
      headerName: "Provider Type",
      sortable: true,
      renderCell: (item) => item.attributes.provider_type || "-",
    },
    {
      field: "updated_by",
      headerName: "Updated By",
      sortable: true,
      renderCell: (item) => item.attributes.updated_by,
    },
    {
      field: "updated_at",
      headerName: "Updated At",
      sortable: true,
      renderCell: (item) => formatDate(item.attributes.updated_at),
    },
  ];

  const actions = [
    {
      label: "Edit IdP profile",
      onClick: (profile) => navigate(`/admin/sso-profiles/edit/${profile.attributes.profile_id}`),
    },
    {
      label: "Delete IdP profile",
      onClick: handleDeleteClick,
    },
    {
      label: "Default IdP for SSO login",
      onClick: handleSetDefaultClick,
    }
  ];

  const emptyState = {
    title: "No Identity provider profiles have been created yet.",
    description: "Single Sign-On (SSO) is an authentication process in which a user is provided access to the AI studio applications by using only a single set of login credentials e.g. username and password.\nGet started with Single Sign-On by adding a profile. Configure settings and, if needed, map user groups to assign users to teams.",
    learnMoreLink: "",
  };

  if (loading && profiles.length === 0) {
    return <CircularProgress />;
  }

  if (error && profiles.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Identity provider profiles</Typography>
        <PrimaryButton
          variant="contained"
          startIcon={<AddIcon />}
          onClick={handleAddProfile}
        >
          Create profile
        </PrimaryButton>
      </TitleBox>
      <ContentBox>
        {/* Success Banner for Create Operations */}
        {successBanner.show && (
          <SuccessBanner
            title={successBanner.title}
            message={successBanner.message}
            onClose={handleCloseBanner}
          />
        )}
        
        {profiles.length === 0 ? (
          <EmptyStateWidget
            title={emptyState.title}
            description={emptyState.description}
            learnMoreLink={emptyState.learnMoreLink}
          />
        ) : (
          <DataTable
            columns={columns}
            data={profiles}
            actions={actions}
            pagination={{
              page,
              pageSize,
              totalPages,
              onPageChange: handlePageChange,
              onPageSizeChange: handlePageSizeChange,
            }}
            onRowClick={handleProfileClick}
            sortConfig={{ field: sortField, direction: sortOrder }}
            onSortChange={handleSortChange}
          />
        )}
      </ContentBox>

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={handleCloseSnackbar}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={handleCloseSnackbar}
          severity={snackbar.severity}
          sx={{ width: "100%" }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>

      <ConfirmationDialog
        open={warningDialogOpen}
        title="Delete Identity provider profile"
        message="This operation cannot be undone. If you remove this Identity provider profile, all users relying on it won't be able to sign in. Make sure they have another way to log in before proceeding."
        buttonLabel="Delete profile"
        onConfirm={handleConfirmDelete}
        onCancel={handleCancelDelete}
        iconName="hexagon-exclamation"
        iconColor="background.buttonCritical"
        titleColor="text.criticalDefault"
        backgroundColor="background.surfaceCriticalDefault"
        borderColor="border.criticalDefaultSubdue"
        primaryButtonComponent="danger"
      />

      <ConfirmationDialog
        open={confirmDialogOpen}
        title="Set as the default IdP profile for SSO login"
        message="Only one profile can be set as default on the login page. Selecting this will replace the current default."
        buttonLabel="Confirm"
        onConfirm={handleConfirmSetDefault}
        onCancel={handleCancelSetDefault}
        iconName="triangle-exclamation"
        iconColor="background.iconWarningDefault"
        titleColor="text.warningDefault"
        backgroundColor="background.surfaceWarningDefault"
        borderColor="border.warningDefaultSubdued"
        primaryButtonComponent="primary"
      />
    </>
  );
};

export default SSOProfiles;