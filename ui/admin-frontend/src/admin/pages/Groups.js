import React, { useState, useEffect, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Table,
  TableBody,
  TableHead,
  TableRow,
  Typography,
  IconButton,
  CircularProgress,
  Alert,
  Menu,
  MenuItem,
  DialogActions,
  FormControl,
  InputLabel,
  Select,
  Snackbar,
  Button,
  Box,
} from "@mui/material";
import { Link } from "react-router-dom";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import {
  StyledPaper,
  TitleBox,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  PrimaryButton,
  StyledDialogContent,
  StyledDialogTitle,
  StyledDialog,
} from "../styles/sharedStyles";
import AddIcon from "@mui/icons-material/Add";
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";
import useSystemFeatures from "../hooks/useSystemFeatures";

const Groups = () => {
  const navigate = useNavigate();
  const [groups, setGroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedGroup, setSelectedGroup] = useState(null);
  const [openAddCatalogueModal, setOpenAddCatalogueModal] = useState(false);
  const [openAddDataCatalogueModal, setOpenAddDataCatalogueModal] =
    useState(false);
  const [openAddToolCatalogueModal, setOpenAddToolCatalogueModal] =
    useState(false);
  const [openAddUserModal, setOpenAddUserModal] = useState(false);
  const [catalogues, setCatalogues] = useState([]);
  const [dataCatalogues, setDataCatalogues] = useState([]);
  const [toolCatalogues, setToolCatalogues] = useState([]);
  const [users, setUsers] = useState([]);
  const [selectedCatalogue, setSelectedCatalogue] = useState("");
  const [selectedDataCatalogue, setSelectedDataCatalogue] = useState("");
  const [selectedToolCatalogue, setSelectedToolCatalogue] = useState("");
  const [selectedUser, setSelectedUser] = useState("");
  const [openConfirmDialog, setOpenConfirmDialog] = useState(false);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const { features, loading: featuresLoading } = useSystemFeatures();

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchGroups = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/groups", {
        params: {
          page,
          page_size: pageSize,
        },
      });
      setGroups(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching groups", error);
      setError("Failed to load groups");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, updatePaginationData]);

  useEffect(() => {
    fetchGroups();
  }, [fetchGroups]);

  const handleMenuOpen = (event, group) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedGroup(group);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleAddCatalogue = async () => {
    setOpenAddCatalogueModal(true);
    handleMenuClose();
    try {
      const response = await apiClient.get("/catalogues");
      setCatalogues(response.data.data || []);
    } catch (error) {
      console.error("Error fetching catalogues", error);
    }
  };

  const handleAddDataCatalogue = async () => {
    setOpenAddDataCatalogueModal(true);
    handleMenuClose();
    try {
      const response = await apiClient.get("/data-catalogues");
      setDataCatalogues(response.data.data || []);
    } catch (error) {
      console.error("Error fetching data catalogues", error);
    }
  };

  const handleAddToolCatalogue = async () => {
    setOpenAddToolCatalogueModal(true);
    handleMenuClose();
    try {
      const response = await apiClient.get("/tool-catalogues");
      setToolCatalogues(response.data || []);
    } catch (error) {
      console.error("Error fetching tool catalogues", error);
    }
  };

  const handleAddUser = async () => {
    setOpenAddUserModal(true);
    handleMenuClose();
    try {
      const response = await apiClient.get("/users");
      setUsers(response.data.data || []);
    } catch (error) {
      console.error("Error fetching users", error);
    }
  };

  const handleEdit = () => {
    navigate(`/admin/groups/edit/${selectedGroup.id}`);
    handleMenuClose();
  };

  const handleDelete = () => {
    setOpenConfirmDialog(true);
    handleMenuClose();
  };

  const handleConfirmDelete = async () => {
    try {
      const usersResponse = await apiClient.get(
        `/groups/${selectedGroup.id}/users`,
      );
      const groupUsers = usersResponse.data.data || [];
      for (const user of groupUsers) {
        await apiClient.delete(`/groups/${selectedGroup.id}/users/${user.id}`);
      }
      await apiClient.delete(`/groups/${selectedGroup.id}`);
      setGroups(groups.filter((group) => group.id !== selectedGroup.id));
      setOpenConfirmDialog(false);
      setSnackbar({
        open: true,
        message: "Group deleted successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error deleting group", error);
      setSnackbar({
        open: true,
        message: "Failed to delete group",
        severity: "error",
      });
    }
  };

  const handleAddCatalogueConfirm = async () => {
    if (!selectedCatalogue) {
      setSnackbar({
        open: true,
        message: "Please select a catalogue",
        severity: "warning",
      });
      return;
    }

    try {
      await apiClient.post(`/groups/${selectedGroup.id}/catalogues`, {
        data: {
          id: selectedCatalogue,
          type: "catalogues",
        },
      });
      setOpenAddCatalogueModal(false);
      setSelectedCatalogue("");
      fetchGroups();
      setSnackbar({
        open: true,
        message: "Catalogue added to group successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error adding catalogue to group", error);
      setSnackbar({
        open: true,
        message: "Failed to add catalogue to group",
        severity: "error",
      });
    }
  };

  const handleAddDataCatalogueConfirm = async () => {
    if (!selectedDataCatalogue) {
      setSnackbar({
        open: true,
        message: "Please select a data catalogue",
        severity: "warning",
      });
      return;
    }

    try {
      await apiClient.post(`/groups/${selectedGroup.id}/data-catalogues`, {
        data: {
          id: selectedDataCatalogue,
          type: "data-catalogues",
        },
      });
      setOpenAddDataCatalogueModal(false);
      setSelectedDataCatalogue("");
      fetchGroups();
      setSnackbar({
        open: true,
        message: "Data Catalogue added to group successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error adding data catalogue to group", error);
      setSnackbar({
        open: true,
        message: "Failed to add data catalogue to group",
        severity: "error",
      });
    }
  };

  const handleAddToolCatalogueConfirm = async () => {
    if (!selectedToolCatalogue) {
      setSnackbar({
        open: true,
        message: "Please select a tool catalogue",
        severity: "warning",
      });
      return;
    }

    try {
      await apiClient.post(`/groups/${selectedGroup.id}/tool-catalogues`, {
        data: {
          id: selectedToolCatalogue,
          type: "tool-catalogues",
        },
      });
      setOpenAddToolCatalogueModal(false);
      setSelectedToolCatalogue("");
      fetchGroups();
      setSnackbar({
        open: true,
        message: "Tool Catalogue added to group successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error adding tool catalogue to group", error);
      setSnackbar({
        open: true,
        message: "Failed to add tool catalogue to group",
        severity: "error",
      });
    }
  };

  const handleAddUserConfirm = async () => {
    if (!selectedUser) {
      setSnackbar({
        open: true,
        message: "Please select a user",
        severity: "warning",
      });
      return;
    }

    try {
      await apiClient.post(`/groups/${selectedGroup.id}/users`, {
        data: {
          id: selectedUser,
          type: "users",
        },
      });
      setOpenAddUserModal(false);
      setSelectedUser("");
      fetchGroups();
      setSnackbar({
        open: true,
        message: "User added to group successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error adding user to group", error);
      setSnackbar({
        open: true,
        message: "Failed to add user to group",
        severity: "error",
      });
    }
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  const handleGroupClick = (group) => {
    navigate(`/admin/groups/${group.id}`);
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
        <Typography variant="headingXLarge">User groups</Typography>
        <PrimaryButton
          variant="contained"
          startIcon={<AddIcon />}
          component={Link}
          to="/admin/groups/new"
        >
          Add group
        </PrimaryButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">User groups help you organize users and easily manage their access to LLM providers, data sources, and tools through catalogs. Linking user groups to specific catalogs ensures each team can only see and access the LLM provider and or data relevant to them.</Typography>  
      </Box>
      <Box sx={{ p: 3 }}>
        <StyledPaper>
          <Table>
            <TableHead>
              <TableRow>
                <StyledTableHeaderCell>ID</StyledTableHeaderCell>
                <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                <StyledTableHeaderCell align="right">
                  Actions
                </StyledTableHeaderCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {groups.length > 0 ? (
                groups.map((group) => (
                  <StyledTableRow
                    key={group.id}
                    onClick={() => handleGroupClick(group)}
                    sx={{ cursor: 'pointer' }}
                  >
                    <StyledTableCell>{group.id}</StyledTableCell>
                    <StyledTableCell>{group.attributes.name}</StyledTableCell>
                    <StyledTableCell align="right">
                      <IconButton
                        onClick={(event) => handleMenuOpen(event, group)}
                      >
                        <MoreVertIcon />
                      </IconButton>
                    </StyledTableCell>
                  </StyledTableRow>
                ))
              ) : (
                <TableRow>
                  <StyledTableCell colSpan={3}>No groups found</StyledTableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
          <PaginationControls
            page={page}
            pageSize={pageSize}
            totalPages={totalPages}
            onPageChange={handlePageChange}
            onPageSizeChange={handlePageSizeChange}
          />
        </StyledPaper>
      </Box>

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        {features.feature_portal && (
          <MenuItem onClick={handleAddCatalogue}>
            Add LLM catalogue to group
          </MenuItem>
        )}
        <MenuItem onClick={handleAddDataCatalogue}>
          Add data catalogue to group
        </MenuItem>
        <MenuItem onClick={handleAddToolCatalogue}>
          Add tool catalogue to group
        </MenuItem>
        <MenuItem onClick={handleAddUser}>Add user to group</MenuItem>
        <MenuItem onClick={handleEdit}>Edit group</MenuItem>
        <MenuItem onClick={handleDelete}>Delete group</MenuItem>
      </Menu>

      <StyledDialog
        open={openAddCatalogueModal}
        onClose={() => setOpenAddCatalogueModal(false)}
      >
        <StyledDialogTitle>Add Catalogue to Group</StyledDialogTitle>
        <StyledDialogContent>
          <Typography
            gutterBottom
            sx={(theme) => ({ padding: theme.spacing(2) })}
          >
            Catalogues are baskets of LLMs, Tools, and Data sources that you can
            make available to a group.
          </Typography>
          <FormControl fullWidth>
            <InputLabel>Catalogue</InputLabel>
            <Select
              value={selectedCatalogue}
              onChange={(e) => setSelectedCatalogue(e.target.value)}
            >
              {catalogues.map((catalogue) => (
                <MenuItem key={catalogue.id} value={catalogue.id}>
                  {catalogue.attributes.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </StyledDialogContent>
        <DialogActions>
          <Button onClick={() => setOpenAddCatalogueModal(false)}>
            Cancel
          </Button>
          <PrimaryButton onClick={handleAddCatalogueConfirm} color="primary">
            Add
          </PrimaryButton>
        </DialogActions>
      </StyledDialog>

      <StyledDialog
        open={openAddDataCatalogueModal}
        onClose={() => setOpenAddDataCatalogueModal(false)}
      >
        <StyledDialogTitle>Add Data Catalogue to Group</StyledDialogTitle>
        <StyledDialogContent>
          <Typography
            gutterBottom
            sx={(theme) => ({ padding: theme.spacing(2) })}
          >
            Data Catalogues are collections of data sources that you can make
            available to a group.
          </Typography>
          <FormControl fullWidth>
            <InputLabel>Data Catalogue</InputLabel>
            <Select
              value={selectedDataCatalogue}
              onChange={(e) => setSelectedDataCatalogue(e.target.value)}
            >
              {dataCatalogues.map((dataCatalogue) => (
                <MenuItem key={dataCatalogue.id} value={dataCatalogue.id}>
                  {dataCatalogue.attributes.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </StyledDialogContent>
        <DialogActions>
          <Button onClick={() => setOpenAddDataCatalogueModal(false)}>
            Cancel
          </Button>
          <PrimaryButton onClick={handleAddDataCatalogueConfirm} color="primary">
            Add
          </PrimaryButton>
        </DialogActions>
      </StyledDialog>

      <StyledDialog
        open={openAddToolCatalogueModal}
        onClose={() => setOpenAddToolCatalogueModal(false)}
      >
        <StyledDialogTitle>Add Tool Catalogue to Group</StyledDialogTitle>
        <StyledDialogContent>
          <Typography
            gutterBottom
            sx={(theme) => ({ padding: theme.spacing(2) })}
          >
            Tool Catalogues are collections of tools that you can make available
            to a group.
          </Typography>
          <FormControl fullWidth>
            <InputLabel>Tool Catalogue</InputLabel>
            <Select
              value={selectedToolCatalogue}
              onChange={(e) => setSelectedToolCatalogue(e.target.value)}
            >
              {toolCatalogues.map((toolCatalogue) => (
                <MenuItem key={toolCatalogue.id} value={toolCatalogue.id}>
                  {toolCatalogue.attributes.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </StyledDialogContent>
        <DialogActions>
          <Button onClick={() => setOpenAddToolCatalogueModal(false)}>
            Cancel
          </Button>
          <PrimaryButton onClick={handleAddToolCatalogueConfirm} color="primary">
            Add
          </PrimaryButton>
        </DialogActions>
      </StyledDialog>

      <StyledDialog
        open={openAddUserModal}
        onClose={() => setOpenAddUserModal(false)}
      >
        <StyledDialogTitle>Add User to Group</StyledDialogTitle>
        <StyledDialogContent>
          <Typography
            gutterBottom
            sx={(theme) => ({ padding: theme.spacing(2) })}
          >
            Add a user to this group, users can be a member of multiple groups
            and benefit from access to multiple catalogues.
          </Typography>
          <FormControl fullWidth>
            <InputLabel>User</InputLabel>
            <Select
              value={selectedUser}
              onChange={(e) => setSelectedUser(e.target.value)}
            >
              {users.map((user) => (
                <MenuItem key={user.id} value={user.id}>
                  {user.attributes.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </StyledDialogContent>
        <DialogActions>
          <Button onClick={() => setOpenAddUserModal(false)}>Cancel</Button>
          <PrimaryButton onClick={handleAddUserConfirm} color="primary">
            Add
          </PrimaryButton>
        </DialogActions>
      </StyledDialog>

      <StyledDialog
        open={openConfirmDialog}
        onClose={() => setOpenConfirmDialog(false)}
      >
        <StyledDialogTitle>Confirm Delete</StyledDialogTitle>
        <StyledDialogContent>
          <Typography
            gutterBottom
            sx={(theme) => ({ padding: theme.spacing(2) })}
          >
            Are you sure you want to delete this group? All users will be
            removed from the group before deletion.
          </Typography>
        </StyledDialogContent>
        <DialogActions>
          <Button onClick={() => setOpenConfirmDialog(false)}>Cancel</Button>
          <Button onClick={handleConfirmDelete} color="primary">
            Delete
          </Button>
        </DialogActions>
      </StyledDialog>

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
    </>
  );
};

export default Groups;
