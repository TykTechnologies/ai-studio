import React, { useState } from "react";
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

const Groups = () => {
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

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
  } = useGroupActions(refreshGroups, setSnackbar);

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

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
    </>
  );
};

export default Groups;
