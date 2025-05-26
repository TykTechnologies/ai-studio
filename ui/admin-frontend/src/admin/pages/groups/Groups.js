import React, { useState, useEffect } from "react";
import { Link } from "react-router-dom";
import {
  Typography,
  CircularProgress,
  Alert,
  Snackbar,
  Box,
} from "@mui/material";
import {
  TitleBox,
  PrimaryButton,
} from "../../styles/sharedStyles";
import AddIcon from "@mui/icons-material/Add";
import useGroups from "./hooks/useGroups";
import useGroupActions from "./hooks/useGroupActions";
import GroupsTable from "./components/GroupsTable";
import GroupDeleteDialog from "./components/GroupDeleteDialog";
import ManageTeamMembersModal from "./components/ManageTeamMembersModal";
import ManageGroupCatalogsModal from "./components/ManageGroupCatalogsModal";
import { CACHE_KEYS } from "../../utils/constants";

const Groups = () => {
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  
  const [manageTeamMembersOpen, setManageTeamMembersOpen] = useState(false);
  const [manageCatalogsOpen, setManageCatalogsOpen] = useState(false);

  const {
    groups,
    loading,
    error,
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    handleSearch,
    sortConfig,
    handleSortChange,
    refreshGroups,
  } = useGroups();

  const {
    selectedGroup,
    warningDialogOpen,
    handleEdit,
    handleDelete,
    handleCancelDelete,
    handleConfirmDelete,
    handleGroupClick,
    handleManageMembers,
    handleManageCatalogs,
  } = useGroupActions(refreshGroups, setSnackbar);

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  useEffect(() => {
    const notificationData = localStorage.getItem(CACHE_KEYS.GROUP_NOTIFICATION);
    if (notificationData) {
      try {
        const notification = JSON.parse(notificationData);
        const isStillRelevant = Date.now() - notification.timestamp < 5 * 60 * 1000; 

        if (isStillRelevant) {
          setSnackbar({
            open: true,
            message: notification.message,
            severity: "success", 
          });
        }
      } catch (error) {
        console.error('Error parsing group notification data', error);
      }

      localStorage.removeItem(CACHE_KEYS.GROUP_NOTIFICATION);
    }
  }, []);

  if (loading && groups.length === 0) {
    return <CircularProgress />;
  }

  if (error && groups.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Teams</Typography>
        <PrimaryButton
          variant="contained"
          startIcon={<AddIcon />}
          component={Link}
          to="/admin/groups/new"
        >
          Add team
        </PrimaryButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Teams help you organize users and easily manage their access to LLM providers, data sources, and tools through catalogs. Linking teams to specific catalogs ensures they access only AI and data relevant to them.</Typography>
      </Box>
      <Box sx={{ p: 3 }}>
        <GroupsTable
          groups={groups}
          page={page}
          pageSize={pageSize}
          totalPages={totalPages}
          handlePageChange={handlePageChange}
          handlePageSizeChange={handlePageSizeChange}
          handleSearch={handleSearch}
          sortConfig={sortConfig}
          handleSortChange={handleSortChange}
          handleGroupClick={handleGroupClick}
          handleEdit={handleEdit}
          handleDelete={handleDelete}
          handleManageMembers={(group) => {
            handleManageMembers(group);
            setManageTeamMembersOpen(true);
          }}
          handleManageCatalogs={(group) => {
            handleManageCatalogs(group);
            setManageCatalogsOpen(true);
          }}
        />
      </Box>

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

      <GroupDeleteDialog
        open={warningDialogOpen}
        selectedGroup={selectedGroup}
        onConfirm={handleConfirmDelete}
        onCancel={handleCancelDelete}
      />

      <ManageTeamMembersModal
        open={manageTeamMembersOpen}
        onClose={() => setManageTeamMembersOpen(false)}
        group={selectedGroup}
        onSuccess={(message) => {
          setSnackbar({
            open: true,
            message,
            severity: "success",
          });
          refreshGroups();
        }}
        onError={(message) => {
          setSnackbar({
            open: true,
            message,
            severity: "error",
          });
        }}
      />

      <ManageGroupCatalogsModal
        open={manageCatalogsOpen}
        onClose={() => setManageCatalogsOpen(false)}
        group={selectedGroup}
        onSuccess={(message) => {
          setSnackbar({
            open: true,
            message,
            severity: "success",
          });
          refreshGroups();
        }}
        onError={(message) => {
          setSnackbar({
            open: true,
            message,
            severity: "error",
          });
        }}
      />
    </>
  );
};

export default Groups;
