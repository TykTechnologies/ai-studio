import React, { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "./apiClient";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Paper,
  Typography,
  Button,
  IconButton,
  CircularProgress,
  Alert,
  Menu,
  MenuItem,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  FormControl,
  InputLabel,
  Select,
  Snackbar,
  Box,
} from "@mui/material";
import { styled } from "@mui/material/styles";
import { Link } from "react-router-dom";
import MoreVertIcon from "@mui/icons-material/MoreVert";

const StyledPaper = styled(Paper)(({ theme }) => ({
  borderRadius: theme.shape.borderRadius * 2,
  boxShadow: theme.shadows[5],
  overflow: "hidden",
}));

const TitleBox = styled(Box)(({ theme }) => ({
  backgroundColor: "#0B4545",
  padding: theme.spacing(2),
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
}));

const ContentBox = styled(Box)(({ theme }) => ({
  padding: theme.spacing(2),
}));

const StyledTableCell = styled(TableCell)(({ theme }) => ({
  fontWeight: "bold",
}));

const StyledTableRow = styled(TableRow)(({ theme }) => ({
  "&:nth-of-type(odd)": {
    backgroundColor: "rgba(255, 255, 255, 0.1)",
  },
  "&:nth-of-type(even)": {
    backgroundColor: "rgba(255, 255, 255, 0.15)",
  },
  "&:hover": {
    backgroundColor: "rgba(255, 255, 255, 0.2)",
  },
}));

const StyledIconButton = styled(IconButton)(({ theme }) => ({
  color: theme.palette.primary.main,
}));

const StyledDialog = styled(Dialog)(({ theme }) => ({
  "& .MuiDialog-paper": {
    borderRadius: "12px",
    backgroundColor: "#2c2c2c",
  },
}));

const StyledDialogTitle = styled(DialogTitle)(({ theme }) => ({
  backgroundColor: "#0B4545",
  color: theme.palette.common.white,
  padding: theme.spacing(2),
}));

const StyledDialogContent = styled(DialogContent)(({ theme }) => ({
  padding: theme.spacing(3),
}));

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

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    try {
      const [usersResponse, groupsResponse] = await Promise.all([
        apiClient.get("/users"),
        apiClient.get("/groups"),
      ]);
      setUsers(usersResponse.data.data || []);
      setGroups(groupsResponse.data.data || []);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching data", error);
      setError("Failed to load data");
      setLoading(false);
    }
  };

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
      setUsers(users.filter((user) => user.id !== id));
      setSnackbar({
        open: true,
        message: "User deleted successfully",
        severity: "success",
      });
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
      fetchData();
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

  if (loading) {
    return <CircularProgress />;
  }

  if (error) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <Box sx={{ p: 3 }}>
      <StyledPaper sx={{ width: "100%", overflow: "hidden" }}>
        <TitleBox>
          <Typography variant="h5" color="white" sx={{ fontWeight: "bold" }}>
            Users
          </Typography>
          <Button
            variant="contained"
            color="secondary"
            component={Link}
            to="/users/new"
            sx={{ borderRadius: 20 }}
          >
            Add User
          </Button>
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
                      <StyledIconButton
                        onClick={(event) => handleMenuOpen(event, user)}
                      >
                        <MoreVertIcon />
                      </StyledIconButton>
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
        </ContentBox>
      </StyledPaper>

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleAddToGroup}>Add to Group</MenuItem>
        <MenuItem onClick={() => handleDelete(selectedUser?.id)}>
          Delete User
        </MenuItem>
      </Menu>

      <StyledDialog
        open={openAddToGroupModal}
        onClose={handleCloseAddToGroupModal}
      >
        <StyledDialogTitle>Add User to Group</StyledDialogTitle>
        <StyledDialogContent>
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
        </StyledDialogContent>
        <DialogActions>
          <Button onClick={handleCloseAddToGroupModal}>Cancel</Button>
          <Button onClick={handleAddUserToGroup} color="primary">
            Add to Group
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
