import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  List,
  ListItem,
  ListItemText,
  IconButton,
  Button,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Snackbar,
  Alert,
  DialogActions,
} from "@mui/material";
import DeleteIcon from "@mui/icons-material/Delete";
import AddIcon from "@mui/icons-material/Add";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledButton,
  StyledDialog,
  StyledDialogTitle,
  StyledDialogContent,
} from "../../styles/sharedStyles";

const GroupDetail = () => {
  const [group, setGroup] = useState(null);
  const [users, setUsers] = useState([]);
  const [catalogues, setCatalogues] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [openAddUserModal, setOpenAddUserModal] = useState(false);
  const [openAddCatalogueModal, setOpenAddCatalogueModal] = useState(false);
  const [availableUsers, setAvailableUsers] = useState([]);
  const [availableCatalogues, setAvailableCatalogues] = useState([]);
  const [selectedUser, setSelectedUser] = useState("");
  const [selectedCatalogue, setSelectedCatalogue] = useState("");
  const { id } = useParams();
  const navigate = useNavigate();
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

  useEffect(() => {
    fetchGroupDetails();
  }, [id]);

  const fetchGroupDetails = async () => {
    try {
      const [groupResponse, usersResponse, cataloguesResponse] =
        await Promise.all([
          apiClient.get(`/groups/${id}`),
          apiClient.get(`/groups/${id}/users`),
          apiClient.get(`/groups/${id}/catalogues`),
        ]);
      setGroup(groupResponse.data.data);
      setUsers(usersResponse.data.data || []);
      setCatalogues(cataloguesResponse.data.data || []);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching group details", error);
      setError("Failed to load group details");
      setLoading(false);
    }
  };

  const handleAddUser = async () => {
    setOpenAddUserModal(true);
    try {
      const response = await apiClient.get("/users");
      const allUsers = response.data.data || [];
      const groupUserIds = users.map((u) => u.id);
      setAvailableUsers(allUsers.filter((u) => !groupUserIds.includes(u.id)));
    } catch (error) {
      console.error("Error fetching users", error);
    }
  };

  const handleAddCatalogue = async () => {
    setOpenAddCatalogueModal(true);
    try {
      const response = await apiClient.get("/catalogues");
      const allCatalogues = response.data.data || [];
      const groupCatalogueIds = catalogues.map((c) => c.id);
      setAvailableCatalogues(
        allCatalogues.filter((c) => !groupCatalogueIds.includes(c.id)),
      );
    } catch (error) {
      console.error("Error fetching catalogues", error);
    }
  };

  const handleRemoveUser = async (userId) => {
    try {
      await apiClient.delete(`/groups/${id}/users/${userId}`);
      setUsers(users.filter((user) => user.id !== userId));
      setSnackbar({
        open: true,
        message: "User removed from group successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error removing user from group", error);
      setSnackbar({
        open: true,
        message: "Failed to remove user from group",
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
      await apiClient.post(`/groups/${id}/users`, {
        data: {
          id: selectedUser,
          type: "users",
        },
      });
      setOpenAddUserModal(false);
      setSelectedUser("");
      fetchGroupDetails();
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
      await apiClient.post(`/groups/${id}/catalogues`, {
        data: {
          id: selectedCatalogue,
          type: "catalogues",
        },
      });
      setOpenAddCatalogueModal(false);
      setSelectedCatalogue("");
      fetchGroupDetails();
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

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  if (loading) return <CircularProgress />;
  if (error) return <Typography color="error">{error}</Typography>;
  if (!group) return <Typography>Group not found</Typography>;

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">Group Details</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          color="white"
          onClick={() => navigate("/groups")}
        >
          Back to Groups
        </Button>
      </TitleBox>
      <ContentBox>
        <Typography variant="h6">Name: {group.attributes.name}</Typography>
        <Typography variant="h6" style={{ marginTop: "20px" }}>
          Users in Group
        </Typography>
        <List>
          {users.map((user) => (
            <ListItem key={user.id}>
              <ListItemText primary={user.attributes.name} />
              <IconButton
                edge="end"
                aria-label="delete"
                onClick={() => handleRemoveUser(user.id)}
              >
                <DeleteIcon />
              </IconButton>
            </ListItem>
          ))}
        </List>
        <StyledButton
          variant="contained"
          startIcon={<AddIcon />}
          onClick={handleAddUser}
        >
          Add User
        </StyledButton>

        <Typography variant="h6" style={{ marginTop: "20px" }}>
          Catalogues in Group
        </Typography>
        <List>
          {catalogues.map((catalogue) => (
            <ListItem key={catalogue.id}>
              <ListItemText primary={catalogue.attributes.name} />
            </ListItem>
          ))}
        </List>
        <StyledButton
          variant="contained"
          startIcon={<AddIcon />}
          onClick={handleAddCatalogue}
        >
          Add Catalogue
        </StyledButton>
      </ContentBox>

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
              {availableUsers.map((user) => (
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
              {availableCatalogues.map((catalogue) => (
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

export default GroupDetail;
