import React, { useState, useEffect } from "react";
import { Switch, FormControlLabel } from "@mui/material";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Box,
  Alert,
  Typography,
  Grid,
  Snackbar,
} from "@mui/material";
import { useNavigate, useParams, Link } from "react-router-dom";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  SecondaryLinkButton,
  TitleContentBox,
  DangerOutlineButton,
} from "../../styles/sharedStyles";
import ChevronLeftIcon from "@mui/icons-material/ChevronLeft";

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
  const [newGroupName, setNewGroupName] = useState("");
  const [emailVerified, setEmailVerified] = useState(false);
  const [notificationsEnabled, setNotificationsEnabled] = useState(false);
  const [accessToSSOConfig, setAccessToSSOConfig] = useState(false);

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
      setEmailVerified(userData.attributes.email_verified ?? false);
      setNotificationsEnabled(userData.attributes.notifications_enabled ?? false);
      setAccessToSSOConfig(userData.attributes.access_to_sso_config ?? false);
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
          email_verified: emailVerified,
          notifications_enabled: isAdmin ? notificationsEnabled : false,
          access_to_sso_config: isAdmin ? accessToSSOConfig : false,
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

  return (
    <>
      <TitleBox>
        <TitleContentBox>
          <SecondaryLinkButton
            component={Link}
            to="/admin/users"
            color="inherit"
            sx={{ mb: 1, px: 0 }}
            startIcon={<ChevronLeftIcon sx={{ mr: -1 }} />}
          >
            back to users
          </SecondaryLinkButton>
          <Typography variant="headingXLarge">
            {id ? "Edit user" : "Create user"}
          </Typography>
        </TitleContentBox>
      </TitleBox>
      <ContentBox sx={{
        maxWidth: {
          xs: '100%',
          sm: '100%',
          md: '100%',
          lg: '75%'
        }
      }}
      >
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
                autoComplete="off"
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
                autoComplete="off"
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
              <Grid container>
                <Grid item xs={2}>
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
                  <Box mt={2}>
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
                  </Box>
                  <Box mt={2}>
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
                  </Box>
                  <Box mt={2}>
                    <FormControlLabel
                      control={
                        <Switch
                          checked={emailVerified}
                          onChange={(e) => setEmailVerified(e.target.checked)}
                          color="primary"
                        />
                      }
                      label="Email Verified"
                    />
                  </Box>
                </Grid>
                <Grid item xs={6}>
                  {isAdmin && (
                    <>
                      <FormControlLabel
                        control={
                          <Switch
                            checked={notificationsEnabled}
                            onChange={(e) => setNotificationsEnabled(e.target.checked)}
                            color="primary"
                          />
                        }
                        label="Enable Notifications"
                      />
                      <Box mt={2}>
                        <FormControlLabel
                          control={
                            <Switch
                              checked={accessToSSOConfig}
                              onChange={(e) => setAccessToSSOConfig(e.target.checked)}
                              color="primary"
                            />
                          }
                          label="Enable access to IdP configuration"
                        />
                      </Box>
                    </>
                  )}
                </Grid>
              </Grid>
            </Grid>
            <Box sx={{ display: "flex", justifyContent: "flex-start", mt: 3, gap: 2 }}>
              <PrimaryButton type="submit" disabled={!isFormValid()}>
                {id ? "Update user" : "Save user"}
              </PrimaryButton>
              {id && (
                <DangerOutlineButton
                  //onClick={handleDeleteClick}
                >
                  Delete user
                </DangerOutlineButton>
              )}
            </Box>
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
    </>
  );
};

export default UserForm;
