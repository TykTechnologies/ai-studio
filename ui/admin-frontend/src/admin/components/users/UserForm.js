import React, { useState, useEffect } from "react";
import { Switch, FormControlLabel } from "@mui/material";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Button,
  Box,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Alert,
  Typography,
  Grid,
  Snackbar,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  IconButton,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/Delete";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledButton,
  StyledTableRow,
} from "../../styles/sharedStyles";

const UserForm = () => {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [isAdmin, setIsAdmin] = useState(false);
  const [showPortal, setShowPortal] = useState(true);
  const [showChat, setShowChat] = useState(true);
  const [groups, setGroups] = useState([]);
  const [userGroups, setUserGroups] = useState([]);
  const [selectedGroup, setSelectedGroup] = useState("");
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const navigate = useNavigate();
  const { id } = useParams();
  const [isAddingGroup, setIsAddingGroup] = useState(false);
  const [newGroupName, setNewGroupName] = useState("");

  useEffect(() => {
    fetchGroups();
    if (id) {
      fetchUser();
      fetchUserGroups();
    }
  }, [id]);

  const fetchGroups = async () => {
    try {
      const response = await apiClient.get("/groups");
      setGroups(response.data.data || []);
    } catch (error) {
      console.error("Error fetching groups", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch groups",
        severity: "error",
      });
    }
  };

  const fetchUser = async () => {
    try {
      const response = await apiClient.get(`/users/${id}`);
      const userData = response.data.data;
      setName(userData.attributes.name);
      setEmail(userData.attributes.email);
      setIsAdmin(userData.attributes.is_admin);
      setShowPortal(userData.attributes.show_portal ?? true);
      setShowChat(userData.attributes.show_chat ?? true);
    } catch (error) {
      console.error("Error fetching user", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch user details",
        severity: "error",
      });
    }
  };

  const fetchUserGroups = async () => {
    try {
      const response = await apiClient.get(`/users/${id}/groups`);
      setUserGroups(response.data.data || []);
    } catch (error) {
      console.error("Error fetching user groups", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch user groups",
        severity: "error",
      });
    }
  };

  const validateForm = () => {
    const newErrors = {};
    if (!name.trim()) newErrors.name = "Name is required";
    if (!email.trim()) newErrors.email = "Email is required";
    if (!id && !password.trim()) newErrors.password = "Password is required";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const isFormValid = () => {
    return (
      name.trim() !== "" &&
      email.trim() !== "" &&
      (id || password.trim() !== "")
    );
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm() || !isFormValid()) return;
    const userData = {
      data: {
        type: "User",
        attributes: {
          name,
          email,
          is_admin: isAdmin,
          show_portal: showPortal,
          show_chat: showChat,
          ...(password && { password }),
        },
      },
    };

    try {
      if (id) {
        await apiClient.patch(`/users/${id}`, userData);
      } else {
        const response = await apiClient.post("/users", userData);
        const newUserId = response.data.data.id;
        if (selectedGroup) {
          await apiClient.post(`/groups/${selectedGroup}/users`, {
            data: {
              id: newUserId.toString(),
              type: "users",
            },
          });
        }
      }

      setSnackbar({
        open: true,
        message: id ? "User updated successfully" : "User created successfully",
        severity: "success",
      });

      setTimeout(() => navigate("/admin/users"), 2000);
    } catch (error) {
      console.error("Error saving user", error);
      setSnackbar({
        open: true,
        message: "Failed to save user. Please try again.",
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
      setSelectedGroup(newGroup.id);
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

  const handleAddToGroup = async () => {
    if (!selectedGroup) {
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
          id: id,
          type: "users",
        },
      });
      setSnackbar({
        open: true,
        message: "User added to group successfully",
        severity: "success",
      });
      fetchUserGroups();
      setSelectedGroup("");
    } catch (error) {
      console.error("Error adding user to group", error);
      setSnackbar({
        open: true,
        message: "Failed to add user to group",
        severity: "error",
      });
    }
  };

  const handleRemoveFromGroup = async (groupId) => {
    if (userGroups.length <= 1) {
      setSnackbar({
        open: true,
        message: "User must be in at least one group",
        severity: "warning",
      });
      return;
    }

    try {
      await apiClient.delete(`/groups/${groupId}/users/${id}`);
      setSnackbar({
        open: true,
        message: "User removed from group successfully",
        severity: "success",
      });
      fetchUserGroups();
    } catch (error) {
      console.error("Error removing user from group", error);
      setSnackbar({
        open: true,
        message: "Failed to remove user from group",
        severity: "error",
      });
    }
  };

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">{id ? "Edit User" : "Add User"}</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          component={Link}
          color="white"
          to="/admin/users"
        >
          Back to Users
        </Button>
      </TitleBox>
      <ContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <Grid container spacing={3}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                error={!!errors.name}
                helperText={errors.name}
                required
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                error={!!errors.email}
                helperText={errors.email}
                required
              />
            </Grid>
            {!id && (
              <Grid item xs={12}>
                <TextField
                  fullWidth
                  label="Password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  error={!!errors.password}
                  helperText={errors.password}
                  required
                />
              </Grid>
            )}
            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={isAdmin}
                    onChange={(e) => setIsAdmin(e.target.checked)}
                    color="primary"
                  />
                }
                label="Admin User"
              />
            </Grid>
            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={showPortal}
                    onChange={(e) => setShowPortal(e.target.checked)}
                    color="primary"
                  />
                }
                label="Show Portal"
              />
            </Grid>
            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={showChat}
                    onChange={(e) => setShowChat(e.target.checked)}
                    color="primary"
                  />
                }
                label="Show Chat"
              />
            </Grid>
            <Grid item xs={12}>
              <StyledButton
                variant="contained"
                type="submit"
                disabled={!isFormValid()}
              >
                {id ? "Update User" : "Add User"}
              </StyledButton>
            </Grid>
          </Grid>
        </Box>
        {id && (
          <Box mt={4}>
            <Typography variant="h6" gutterBottom>
              User Groups
            </Typography>
            <TableContainer>
              <Table>
                <TableHead>
                  <TableRow>
                    <TableCell>Group Name</TableCell>
                    {userGroups.length > 1 && (
                      <TableCell align="right">Action</TableCell>
                    )}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {userGroups.map((group) => (
                    <StyledTableRow key={group.id}>
                      <TableCell>{group.attributes.name}</TableCell>
                      {userGroups.length > 1 && (
                        <TableCell align="right">
                          <IconButton
                            edge="end"
                            aria-label="delete"
                            onClick={() => handleRemoveFromGroup(group.id)}
                          >
                            <DeleteIcon />
                          </IconButton>
                        </TableCell>
                      )}
                    </StyledTableRow>
                  ))}
                  <TableRow>
                    <TableCell colSpan={userGroups.length > 1 ? 2 : 1}>
                      <Box display="flex" alignItems="center" mt={2}>
                        <FormControl fullWidth>
                          <InputLabel>Add to Group</InputLabel>
                          <Select
                            value={selectedGroup}
                            onChange={(e) => setSelectedGroup(e.target.value)}
                          >
                            {groups
                              .filter(
                                (group) =>
                                  !userGroups.some((ug) => ug.id === group.id),
                              )
                              .map((group) => (
                                <MenuItem key={group.id} value={group.id}>
                                  {group.attributes.name}
                                </MenuItem>
                              ))}
                          </Select>
                        </FormControl>
                        <Button
                          onClick={handleAddToGroup}
                          startIcon={<AddIcon />}
                          variant="contained"
                          sx={{ ml: 2 }}
                        >
                          Add
                        </Button>
                      </Box>
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </TableContainer>
          </Box>
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
    </StyledPaper>
  );
};

export default UserForm;
