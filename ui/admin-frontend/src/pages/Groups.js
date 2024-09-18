import React, { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Table,
  TableBody,
  TableCell,
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
} from "@mui/material";
import { Link } from "react-router-dom";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableRow,
  StyledButton,
  StyledDialogContent,
  StyledDialogTitle,
  StyledDialog,
} from "../styles/sharedStyles";
import AddIcon from "@mui/icons-material/Add";

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
  const [openAddUserModal, setOpenAddUserModal] = useState(false);
  const [catalogues, setCatalogues] = useState([]);
  const [dataCatalogues, setDataCatalogues] = useState([]);
  const [users, setUsers] = useState([]);
  const [selectedCatalogue, setSelectedCatalogue] = useState("");
  const [selectedDataCatalogue, setSelectedDataCatalogue] = useState("");
  const [selectedUser, setSelectedUser] = useState("");
  const [openConfirmDialog, setOpenConfirmDialog] = useState(false);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

  useEffect(() => {
    fetchGroups();
  }, []);

  const fetchGroups = async () => {
    try {
      const response = await apiClient.get("/groups");
      setGroups(response.data.data || []);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching groups", error);
      setError("Failed to load groups");
      setLoading(false);
    }
  };

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
    // Fetch catalogues
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
    // Fetch data catalogues
    try {
      const response = await apiClient.get("/data-catalogues");
      setDataCatalogues(response.data.data || []);
    } catch (error) {
      console.error("Error fetching data catalogues", error);
    }
  };

  const handleAddUser = async () => {
    setOpenAddUserModal(true);
    handleMenuClose();
    // Fetch users
    try {
      const response = await apiClient.get("/users");
      setUsers(response.data.data || []);
    } catch (error) {
      console.error("Error fetching users", error);
    }
  };

  const handleEdit = () => {
    navigate(`/groups/edit/${selectedGroup.id}`);
    handleMenuClose();
  };

  const handleDelete = () => {
    setOpenConfirmDialog(true);
    handleMenuClose();
  };

  const handleConfirmDelete = async () => {
    try {
      // Remove all users from the group
      const usersResponse = await apiClient.get(
        `/groups/${selectedGroup.id}/users`,
      );
      const groupUsers = usersResponse.data.data || [];
      for (const user of groupUsers) {
        await apiClient.delete(`/groups/${selectedGroup.id}/users/${user.id}`);
      }

      // Delete the group
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
    navigate(`/groups/${group.id}`);
  };

  if (loading) {
    return <CircularProgress />;
  }

  if (error) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">Groups</Typography>
        <StyledButton
          variant="contained"
          startIcon={<AddIcon />}
          component={Link}
          to="/groups/new"
        >
          Add Group
        </StyledButton>
      </TitleBox>
      <ContentBox>
        <Table>
          <TableHead>
            <TableRow>
              <StyledTableCell>ID</StyledTableCell>
              <StyledTableCell>Name</StyledTableCell>
              <StyledTableCell align="right">Actions</StyledTableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {groups.map((group) => (
              <StyledTableRow
                key={group.id}
                onClick={() => handleGroupClick(group)}
                sx={{ cursor: "pointer" }}
              >
                <TableCell>{group.id}</TableCell>
                <TableCell>{group.attributes.name}</TableCell>
                <TableCell align="right">
                  <IconButton onClick={(event) => handleMenuOpen(event, group)}>
                    <MoreVertIcon />
                  </IconButton>
                </TableCell>
              </StyledTableRow>
            ))}
          </TableBody>
        </Table>
      </ContentBox>

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleAddCatalogue}>Add Catalogue to Group</MenuItem>
        <MenuItem onClick={handleAddDataCatalogue}>
          Add Data Catalogue to Group
        </MenuItem>
        <MenuItem onClick={handleAddUser}>Add User to Group</MenuItem>
        <MenuItem onClick={handleEdit}>Edit Group</MenuItem>
        <MenuItem onClick={handleDelete}>Delete Group</MenuItem>
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
          <Button onClick={handleAddCatalogueConfirm} color="primary">
            Add
          </Button>
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
          <Button onClick={handleAddDataCatalogueConfirm} color="primary">
            Add
          </Button>
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
          <Button onClick={handleAddUserConfirm} color="primary">
            Add
          </Button>
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
    </StyledPaper>
  );
};

export default Groups;
