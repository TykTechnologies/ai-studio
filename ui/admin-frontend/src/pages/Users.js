import React, { useState, useEffect, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
  Button,
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
  TextField,
  Box,
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
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";

const Users = () => {
  const navigate = useNavigate();
  const [users, setUsers] = useState([]);
  const [groups, setGroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedUser, setSelectedUser] = useState(null);
  const [openAddToGroupModal, setOpenAddToGroupModal] = useState(false);
  const [selectedGroup, setSelectedGroup] = useState("");
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [isAddingGroup, setIsAddingGroup] = useState(false);
  const [newGroupName, setNewGroupName] = useState("");

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchUsers = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/users", {
        params: {
          page,
          page_size: pageSize,
        },
      });
      setUsers(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching users", error);
      setError("Failed to load users");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, updatePaginationData]);

  const fetchGroups = useCallback(async () => {
    try {
      const response = await apiClient.get("/groups");
      setGroups(response.data.data || []);
    } catch (error) {
      console.error("Error fetching groups", error);
    }
  }, []);

  useEffect(() => {
    fetchUsers();
    fetchGroups();
  }, [fetchUsers, fetchGroups]);

  const handleMenuOpen = (event, user) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedUser(user);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/users/${id}`);
      setSnackbar({
        open: true,
        message: "User deleted successfully",
        severity: "success",
      });
      fetchUsers();
    } catch (error) {
      console.error("Error deleting user", error);
      setSnackbar({
        open: true,
        message: "Failed to delete user",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleUserClick = (user) => {
    navigate(`/users/${user.id}`);
  };

  const handleAddToGroup = () => {
    if (groups.length === 0) {
      setIsAddingGroup(true);
    }
    setOpenAddToGroupModal(true);
    handleMenuClose();
  };

  const handleCloseAddToGroupModal = () => {
    setOpenAddToGroupModal(false);
    setSelectedGroup("");
  };

  const handleAddUserToGroup = async () => {
    if (!selectedGroup || !selectedUser) {
      setSnackbar({
        open: true,
        message: "Please select a group",
        severity: "warning",
      });
      return;
    }

    try {
      await apiClient.post(`/groups/${selectedGroup}/users`, {
        data: {
          id: selectedUser.id.toString(),
          type: "users",
        },
      });
      setSnackbar({
        open: true,
        message: "User added to group successfully",
        severity: "success",
      });
      handleCloseAddToGroupModal();
      fetchUsers();
    } catch (error) {
      console.error("Error adding user to group", error);
      setSnackbar({
        open: true,
        message: "Failed to add user to group",
        severity: "error",
      });
    }
  };

  const handleAddNewGroup = async () => {
    if (!newGroupName.trim()) {
      setSnackbar({
        open: true,
        message: "Group name cannot be empty",
        severity: "warning",
      });
      return;
    }

    try {
      const response = await apiClient.post("/groups", {
        data: {
          type: "Group",
          attributes: {
            name: newGroupName,
          },
        },
      });
      const newGroup = response.data.data;
      setGroups([...groups, newGroup]);
      setNewGroupName("");
      setIsAddingGroup(false);
      setSnackbar({
        open: true,
        message: "New group added successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error adding new group", error);
      setSnackbar({
        open: true,
        message: "Failed to add new group",
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

  if (loading && users.length === 0) {
    return <CircularProgress />;
  }

  if (error && users.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <Box sx={{ p: 0 }}>
      <StyledPaper>
        <TitleBox>
          <Typography variant="h5">Users</Typography>
          <StyledButton
            variant="contained"
            startIcon={<AddIcon />}
            component={Link}
            to="/users/new"
          >
            Add user
          </StyledButton>
        </TitleBox>
        <ContentBox>
          <Table>
            <TableHead>
              <TableRow>
                <StyledTableCell>ID</StyledTableCell>
                <StyledTableCell>Name</StyledTableCell>
                <StyledTableCell>Email</StyledTableCell>
                <StyledTableCell align="right">Actions</StyledTableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {users.length > 0 ? (
                users.map((user) => (
                  <StyledTableRow
                    key={user.id}
                    onClick={() => handleUserClick(user)}
                    sx={{ cursor: "pointer" }}
                  >
                    <TableCell>{user.id}</TableCell>
                    <TableCell>{user.attributes.name}</TableCell>
                    <TableCell>{user.attributes.email}</TableCell>
                    <TableCell align="right">
                      <IconButton
                        onClick={(event) => handleMenuOpen(event, user)}
                      >
                        <MoreVertIcon />
                      </IconButton>
                    </TableCell>
                  </StyledTableRow>
                ))
              ) : (
                <TableRow>
                  <TableCell colSpan={4}>No users found</TableCell>
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
        </ContentBox>
      </StyledPaper>

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleAddToGroup}>Add to Group</MenuItem>
        <MenuItem onClick={() => navigate(`/users/edit/${selectedUser?.id}`)}>
          Edit User
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedUser?.id)}>
          Delete User
        </MenuItem>
      </Menu>

      <StyledDialog
        open={openAddToGroupModal}
        onClose={handleCloseAddToGroupModal}
      >
        <StyledDialogTitle>
          {isAddingGroup ? "Add New Group" : "Add User to Group"}
        </StyledDialogTitle>
        <StyledDialogContent>
          {isAddingGroup ? (
            <TextField
              fullWidth
              label="New Group Name"
              value={newGroupName}
              onChange={(e) => setNewGroupName(e.target.value)}
              sx={{ mt: 2 }}
            />
          ) : (
            <>
              <Typography
                gutterBottom
                sx={(theme) => ({ padding: theme.spacing(2) })}
              >
                Select a group from the dropdown menu below to add the user to
                that group. This action will grant the user permissions
                associated with the selected group.
              </Typography>
              <FormControl fullWidth sx={{ mt: 2 }}>
                <InputLabel>Group</InputLabel>
                <Select
                  value={selectedGroup}
                  onChange={(e) => setSelectedGroup(e.target.value)}
                >
                  {groups.map((group) => (
                    <MenuItem key={group.id} value={group.id}>
                      {group.attributes.name}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </>
          )}
        </StyledDialogContent>
        <DialogActions>
          <Button onClick={handleCloseAddToGroupModal}>Cancel</Button>
          <Button
            onClick={isAddingGroup ? handleAddNewGroup : handleAddUserToGroup}
            color="primary"
          >
            {isAddingGroup ? "Add Group" : "Add to Group"}
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
    </Box>
  );
};

export default Users;
