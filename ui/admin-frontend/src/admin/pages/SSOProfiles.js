import React, { useState, useEffect, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Alert,
  Snackbar,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DataTable from "../components/common/DataTable";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import WarningDialog from "../components/common/WarningDialog";
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
  const [bannerTimeout, setBannerTimeout] = useState(null);
  const [sortField, setSortField] = useState("profile_id");
  const [sortOrder, setSortOrder] = useState("desc");
  const [warningDialogOpen, setWarningDialogOpen] = useState(false);
  const [profileToDelete, setProfileToDelete] = useState(null);

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
      console.error("Error fetching SSO profiles", error);
      setError("Failed to load SSO profiles");
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

            const timeout = setTimeout(() => {
              setSuccessBanner(current => ({ ...current, show: false }));
            }, 6000);
            
            setBannerTimeout(timeout);
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
    
    return () => {
      if (bannerTimeout) {
        clearTimeout(bannerTimeout);
      }
    };
  }, [bannerTimeout]);

  const handleDeleteClick = (profile) => {
    setProfileToDelete(profile);
    setWarningDialogOpen(true);
  };

  const handleCancelDelete = () => {
    setWarningDialogOpen(false);
    setProfileToDelete(null);
  };

  const handleConfirmDelete = async () => {
    if (!profileToDelete) return;
    
    try {
      await apiClient.delete(`/sso-profiles/${profileToDelete.attributes.profile_id}`);
      setSnackbar({
        open: true,
        message: "SSO profile deleted successfully",
        severity: "success",
      });
      fetchProfiles();
    } catch (error) {
      console.error("Error deleting SSO profile", error);
      setSnackbar({
        open: true,
        message: "Failed to delete SSO profile",
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
    
    if (bannerTimeout) {
      clearTimeout(bannerTimeout);
      setBannerTimeout(null);
    }
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
      renderCell: (item) => item.attributes.name || item.attributes.profile_id,
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
      renderCell: (item) => item.attributes.profile_type,
    },
    {
      field: "provider_type",
      headerName: "Provider Type",
      sortable: true,
      renderCell: (item) => item.attributes.provider_type,
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
      label: "Edit SSO profile",
      onClick: (profile) => navigate(`/admin/sso-profiles/edit/${profile.attributes.profile_id}`),
    },
    {
      label: "Delete SSO profile",
      onClick: handleDeleteClick,
    },
  ];

  const emptyState = {
    title: "No Single Sign-On profiles have been created yet.",
    description: "Single Sign-On (SSO) is an authentication process in which a user is provided access to the portal applications by using only a single set of login credentials e.g. username and password.\nGet started with Single Sign-On by adding a profile. Configure settings and, if needed, map user groups to assign developers to teams.",
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
        <Typography variant="headingXLarge">Single Sign-On Profiles</Typography>
        <PrimaryButton
          variant="contained"
          startIcon={<AddIcon />}
          onClick={handleAddProfile}
        >
          Add SSO Profile
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

      <WarningDialog
        open={warningDialogOpen}
        title="Delete SSO profile"
        message="This operation cannot be undone. If you remove this Single Sign-On profile, all users relying on it won't be able to sign in. Make sure they have another way to log in before proceeding."
        buttonLabel="Delete SSO profile"
        onConfirm={handleConfirmDelete}
        onCancel={handleCancelDelete}
      />
    </>
  );
};

export default SSOProfiles;