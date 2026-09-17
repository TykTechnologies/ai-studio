import React, { useState, useEffect } from "react";
import { Switch, FormControlLabel, Link as MuiLink } from "@mui/material";
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
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import AddIcon from "@mui/icons-material/Add";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  SecondaryOutlineButton,
  SecondaryLinkButton
} from "../../styles/sharedStyles";
import { usePermissions } from "../../context/PermissionsContext";
import RoleSelect from "../roles/RoleSelect";
import RelationshipPicker from "../common/relationship-picker";
import { listRoles, sortRoles } from "../../services/rbacService";
import ConfirmationDialog from "../common/ConfirmationDialog";
import {
  useUnsavedForm,
  useConfirmNavigation,
} from "../../../components/unsaved-changes";
import { listAll } from "../../utils/listAll";

const WILDCARD_ROLE_SLUGS = ["owner", "administrator"];

const UserForm = () => {
  // With roles active (Enterprise) access is assigned through roles rather
  // than the admin switch; the legacy flags stay for Community Edition.
  const { rbacEnabled, identity } = usePermissions();
  const [roleIds, setRoleIds] = useState([]);
  const [initialRoleIds, setInitialRoleIds] = useState([]);
  const [availableRoles, setAvailableRoles] = useState([]);
  const [confirmSelfDemotion, setConfirmSelfDemotion] = useState(false);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [isAdmin, setIsAdmin] = useState(false);
  const [showPortal, setShowPortal] = useState(true);
  const [showChat, setShowChat] = useState(true);
  const [groups, setGroups] = useState([]);
  // Team membership is edited in the form and committed on save: saveUser
  // diffs `selectedGroups` against `loadedGroupIds` and issues the
  // per-team POST/DELETE calls after the user record is saved. (The old
  // "Add to Team +" committed on click, out of step with the rest of the
  // form -- UX review F-03.)
  const [selectedGroups, setSelectedGroups] = useState([]);
  const [loadedGroupIds, setLoadedGroupIds] = useState([]);
  // True once the user being edited (record and teams) is on screen.
  const [loaded, setLoaded] = useState(false);
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
  const [emailVerified, setEmailVerified] = useState(false);
  const [notificationsEnabled, setNotificationsEnabled] = useState(false);
  const [accessToSSOConfig, setAccessToSSOConfig] = useState(false);

  // Unsaved-changes tracking over every field the form saves; markSaved()
  // runs before the post-save redirect so the guard stays quiet on the way
  // out. Creating a new team inline is not tracked: that POST is immediate
  // and labelled as such; only its assignment to the user waits for save.
  const { markSaved } = useUnsavedForm(
    {
      name,
      email,
      password,
      isAdmin,
      showPortal,
      showChat,
      emailVerified,
      notificationsEnabled,
      accessToSSOConfig,
      roleIds,
      groupIds: selectedGroups.map((group) => String(group.id)),
    },
    { ready: !id || loaded },
  );
  const confirmNavigation = useConfirmNavigation();
  const handleCancel = () => confirmNavigation(() => navigate("/admin/users"));

  useEffect(() => {
    fetchGroups();
    if (id) {
      Promise.all([fetchUser(), fetchUserGroups()]).finally(() => setLoaded(true));
    }
  }, [id]);

  useEffect(() => {
    if (!rbacEnabled) return;
    listRoles()
      .then((list) => setAvailableRoles(sortRoles(list)))
      .catch((error) => console.error("Error fetching roles", error));
  }, [rbacEnabled]);

  const fetchGroups = async () => {
    try {
      const response = await listAll(apiClient, "/groups");
      setGroups(response.data.data || []);
    } catch (error) {
      console.error("Error fetching teams", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch teams",
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
      const ids = (userData.attributes.roles || []).map((r) => Number(r.id));
      setRoleIds(ids);
      setInitialRoleIds(ids);
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
      const userGroups = response.data.data || [];
      setSelectedGroups(userGroups);
      setLoadedGroupIds(userGroups.map((group) => String(group.id)));
    } catch (error) {
      console.error("Error fetching user teams", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch user teams",
        severity: "error",
      });
    }
  };

  const validateForm = () => {
    const newErrors = {};
    if (!name.trim()) newErrors.name = "Name is required";
    if (!email.trim()) newErrors.email = "Email is required";
    if (!id && !password.trim()) newErrors.password = "Password is required";
    // The last team cannot be removed. This used to block the delete click;
    // now it is a validation error on save. A user who arrived with no teams
    // (possible through the API) is not blocked from saving other changes.
    if (id && loadedGroupIds.length > 0 && selectedGroups.length === 0) {
      newErrors.teams = "User must be in at least one team";
    }
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  // Commits the Teams picker: adds what was selected since load and removes
  // what was deselected. Runs after the user record is saved.
  const syncTeams = async (userId) => {
    const selectedIds = selectedGroups.map((group) => String(group.id));
    const toAdd = selectedIds.filter((groupId) => !loadedGroupIds.includes(groupId));
    const toRemove = loadedGroupIds.filter((groupId) => !selectedIds.includes(groupId));

    for (const groupId of toAdd) {
      try {
        await apiClient.post(`/groups/${groupId}/users`, {
          data: {
            id: String(userId),
            type: "users",
          },
        });
      } catch (error) {
        console.error("Error adding user to team", error);
        throw new Error("Failed to add user to team");
      }
    }
    for (const groupId of toRemove) {
      try {
        await apiClient.delete(`/groups/${groupId}/users/${userId}`);
      } catch (error) {
        console.error("Error removing user from team", error);
        throw new Error("Failed to remove user from team");
      }
    }
    setLoadedGroupIds(selectedIds);
  };

  const isFormValid = () => {
    return (
      name.trim() !== "" &&
      email.trim() !== "" &&
      (id || password.trim() !== "")
    );
  };

  // Whether the selected roles include a full-administrator role.
  const selectedHasWildcard = roleIds.some((rid) =>
    availableRoles.some((r) => Number(r.id) === rid && WILDCARD_ROLE_SLUGS.includes(r.attributes.slug))
  );
  const effectiveIsAdmin = rbacEnabled ? selectedHasWildcard : isAdmin;

  const isEditingSelf = Boolean(id) && String(identity?.id) === String(id);
  const removesOwnAdminRole =
    rbacEnabled && isEditingSelf && identity?.isFullAdmin && !selectedHasWildcard &&
    initialRoleIds.some((rid) => availableRoles.some((r) => Number(r.id) === rid && WILDCARD_ROLE_SLUGS.includes(r.attributes.slug)));

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!validateForm() || !isFormValid()) return;
    if (removesOwnAdminRole && !confirmSelfDemotion) {
      setConfirmSelfDemotion(true);
      return;
    }
    await saveUser();
  };

  const saveUser = async () => {
    setConfirmSelfDemotion(false);
    const userData = {
      data: {
        type: "User",
        attributes: {
          name,
          email,
          // Roles are the source of truth in Enterprise; the admin flag is
          // omitted so the server leaves it to the role bindings.
          ...(rbacEnabled ? { role_ids: roleIds } : { is_admin: isAdmin }),
          show_portal: showPortal,
          show_chat: showChat,
          email_verified: emailVerified,
          notifications_enabled: effectiveIsAdmin ? notificationsEnabled : false,
          access_to_sso_config: !rbacEnabled && isAdmin ? accessToSSOConfig : false,
          ...(password && { password }),
        },
      },
    };

    // Set once the user record itself is saved, so a failure in the team
    // calls that follow can be reported as what it is.
    let savedUserId = null;
    try {
      if (id) {
        await apiClient.patch(`/users/${id}`, userData);
        savedUserId = id;
      } else {
        const response = await apiClient.post("/users", userData);
        savedUserId = response.data.data.id;
      }

      await syncTeams(savedUserId);

      markSaved();
      navigate("/admin/users", {
        state: {
          snackbar: {
            message: id
              ? "User updated successfully"
              : "User created and added to the Default team. They cannot sign in until their email is verified.",
            severity: "success",
          },
        },
      });
    } catch (error) {
      if (savedUserId && !error.response) {
        // The user is saved; only a team change was refused.
        console.error("Error saving user teams", error);
        setSnackbar({
          open: true,
          message: `${error.message}. The user itself was saved.`,
          severity: "error",
        });
        return;
      }
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
        message: "Team name cannot be empty",
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
      // The team exists now; its assignment to this user waits for save
      // like every other change on the form.
      setSelectedGroups((prev) => [...prev, newGroup]);
      setNewGroupName("");
      setIsAddingGroup(false);
      setSnackbar({
        open: true,
        message: `Team "${newGroup.attributes?.name ?? newGroupName}" created. It is assigned to this user when you save.`,
        severity: "success",
      });
    } catch (error) {
      console.error("Error adding new team", error);
      setSnackbar({
        open: true,
        message: "Failed to add new team",
        severity: "error",
      });
    }
  };

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">{id ? "Edit user" : "Add user"}</Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          color="inherit"
          to="/admin/users"
        >
          Back to users
        </SecondaryLinkButton>
      </TitleBox>
      <ContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <Grid container spacing={3}>
            {!id && (
              <Grid item xs={12}>
                {/* This form has no Teams field, so every user created here
                    joins Default -- which owns catalogues holding every
                    provider, tool and data source on the instance. That was
                    invisible, and team membership is additive, so assigning a
                    narrow team later changes nothing until Default is also
                    removed by hand. Say it up front. */}
                <Alert severity="info">
                  New users always join the <strong>Default</strong> team, which
                  grants access to everything in the Default catalogs. That is
                  deliberate: Community Edition has no teams, so Default
                  membership is what keeps the two editions consistent. Team
                  membership is additive, so adding a narrower team from the{" "}
                  <MuiLink component={Link} to="/admin/groups">
                    Teams
                  </MuiLink>{" "}
                  page grants access on top rather than restricting it — change
                  what Default grants to narrow what everyone can see. The user
                  cannot sign in until their email is verified.
                </Alert>
              </Grid>
            )}
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
                  {!rbacEnabled && (
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
                  )}
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
                  {rbacEnabled && (
                    <Box mb={2} data-testid="user-roles-field">
                      <RoleSelect
                        value={roleIds}
                        onChange={setRoleIds}
                        roles={availableRoles}
                        helperText="Roles decide what this user can see and do in the administration UI and API. Team roles apply on top."
                      />
                    </Box>
                  )}
                  {effectiveIsAdmin && (
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
                      {!rbacEnabled && (
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
                      )}
                    </>
                  )}
                </Grid>
              </Grid>
            </Grid>
            <Grid item xs={12} data-testid="user-teams-field">
              <RelationshipPicker
                label="Teams"
                itemLabel="team"
                value={selectedGroups}
                onChange={(items) => {
                  setSelectedGroups(items);
                  if (errors.teams) setErrors((prev) => ({ ...prev, teams: undefined }));
                }}
                options={groups}
                getOptionLabel={(group) => group?.attributes?.name ?? ""}
                error={!!errors.teams}
                helperText={
                  errors.teams ||
                  (!id ? "New users also join the Default team automatically." : "")
                }
              />
              {/* Inline "create a new team": the team itself is created at
                  once (it has to exist to be picked); its assignment to
                  this user is saved with the form like any other change. */}
              <Box mt={1.5}>
                {isAddingGroup ? (
                  <Box display="flex" alignItems="center" gap={1} flexWrap="wrap">
                    <TextField
                      size="small"
                      label="New team name"
                      value={newGroupName}
                      onChange={(e) => setNewGroupName(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") {
                          e.preventDefault();
                          handleAddNewGroup();
                        }
                      }}
                      autoComplete="off"
                    />
                    <PrimaryButton
                      variant="contained"
                      onClick={handleAddNewGroup}
                      disabled={!newGroupName.trim()}
                    >
                      Create team
                    </PrimaryButton>
                    <SecondaryOutlineButton
                      onClick={() => {
                        setIsAddingGroup(false);
                        setNewGroupName("");
                      }}
                    >
                      Close
                    </SecondaryOutlineButton>
                    <Typography
                      variant="bodySmallDefault"
                      color="text.defaultSubdued"
                      component="div"
                      sx={{ width: "100%" }}
                    >
                      Creating the team applies immediately; adding this user to it is saved with the form.
                    </Typography>
                  </Box>
                ) : (
                  <SecondaryLinkButton
                    startIcon={<AddIcon />}
                    onClick={() => setIsAddingGroup(true)}
                    sx={{ px: 0 }}
                  >
                    Create a new team
                  </SecondaryLinkButton>
                )}
              </Box>
            </Grid>
            <Grid item xs={12}>
              <Box display="flex" gap={2}>
                <SecondaryOutlineButton onClick={handleCancel}>
                  Cancel
                </SecondaryOutlineButton>
                <PrimaryButton
                  variant="contained"
                  type="submit"
                  disabled={!isFormValid()}
                >
                  {id ? "Update user" : "Add user"}
                </PrimaryButton>
              </Box>
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

      <ConfirmationDialog
        open={confirmSelfDemotion}
        title="Remove your own administrator role?"
        message="You are removing the role that gives you full administrator access. You will lose access to this page and any other page your remaining roles do not cover."
        buttonLabel="Remove my role"
        onConfirm={saveUser}
        onCancel={() => setConfirmSelfDemotion(false)}
        iconName="hexagon-exclamation"
        iconColor="background.buttonCritical"
        titleColor="text.criticalDefault"
        backgroundColor="background.surfaceCriticalDefault"
        borderColor="border.criticalDefaultSubdue"
        primaryButtonComponent="danger"
      />
    </>
  );
};

export default UserForm;
