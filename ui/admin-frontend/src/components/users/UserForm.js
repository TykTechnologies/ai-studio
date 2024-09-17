import React, { useState, useEffect } from "react";
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
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import { StyledPaper, TitleBox, ContentBox } from "../../styles/sharedStyles";

const UserForm = () => {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [groups, setGroups] = useState([]);
  const [selectedGroup, setSelectedGroup] = useState("");
  const [errors, setErrors] = useState({});
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const navigate = useNavigate();
  const { id } = useParams();

  useEffect(() => {
    fetchGroups();
    if (id) {
      fetchUser();
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
    } catch (error) {
      console.error("Error fetching user", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch user details",
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

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm()) return;

    const userData = {
      data: {
        type: "User",
        attributes: {
          name,
          email,
          ...(password && { password }),
        },
      },
    };

    try {
      let response;
      if (id) {
        response = await apiClient.patch(`/users/${id}`, userData);
      } else {
        response = await apiClient.post("/users", userData);
      }

      const userId = response.data.data.id;

      if (selectedGroup) {
        await apiClient.post(`/groups/${selectedGroup}/users`, {
          data: {
            id: userId.toString(),
            type: "users",
          },
        });
      }

      setSnackbar({
        open: true,
        message: id ? "User updated successfully" : "User created successfully",
        severity: "success",
      });

      setTimeout(() => navigate("/users"), 2000);
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

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h4" color="white">
          {id ? "Edit User" : "Add User"}
        </Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/users"
          color="inherit"
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
              <FormControl fullWidth>
                <InputLabel>Group</InputLabel>
                <Select
                  value={selectedGroup}
                  onChange={(e) => setSelectedGroup(e.target.value)}
                >
                  <MenuItem value="">
                    <em>None</em>
                  </MenuItem>
                  {groups.map((group) => (
                    <MenuItem key={group.id} value={group.id}>
                      {group.attributes.name}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Grid>
            <Grid item xs={12}>
              <Button variant="contained" color="primary" type="submit">
                {id ? "Update User" : "Add User"}
              </Button>
            </Grid>
          </Grid>
        </Box>
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
