import React, { useState, useEffect, useCallback, useMemo, memo } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Typography,
  Button,
  Alert,
  MenuItem,
  DialogActions,
  FormControl,
  InputLabel,
  Select,
  TextField,
  Box,
  Chip,
} from "@mui/material";
import { Link } from "react-router-dom";
import {
  TitleBox,
  PrimaryButton,
  StyledDialogContent,
  StyledDialogTitle,
  StyledDialog,
} from "../styles/sharedStyles";
import AddIcon from "@mui/icons-material/Add";
import DataTable from "../components/common/DataTable";
import BulkDeleteConfirmationDialog from "../components/common/BulkDeleteConfirmationDialog";
import BulkResultAlert from "../components/common/BulkResultAlert";
import FeedbackSnackbar, { useFeedbackSnackbar } from "../components/common/FeedbackSnackbar";
import { usePermissions } from "../context/PermissionsContext";
import RoleBadge from "../components/roles/RoleBadge";
import CustomSelectBadge from "../components/common/CustomSelectBadge";
import { roleBadgeConfigs } from "../components/groups/utils/roleBadgeConfig";
import useListQuery from "../hooks/useListQuery";
import useBulkActions, { standardBulkActions } from "../hooks/useBulkActions";
import useSystemFeatures from "../hooks/useSystemFeatures";
import Can from "../components/rbac/Can";
import { P } from "../rbac/permissions";
import { authSourceLabel, setUserDisabled } from "../services/userService";

const ORIGIN_OPTIONS = [
  { value: "", label: "Any origin" },
  { value: "sso", label: "SSO" },
  { value: "local", label: "Self-registered" },
  { value: "admin", label: "Admin-created" },
];
const API_KEY_OPTIONS = [
  { value: "", label: "Any" },
  { value: "true", label: "Issued" },
  { value: "false", label: "None" },
];
const STATUS_OPTIONS = [
  { value: "", label: "Any" },
  { value: "false", label: "Active" },
  { value: "true", label: "Disabled" },
];

const userLabel = (user) => user?.attributes?.name || user?.attributes?.email || String(user?.id ?? "");

const Users = memo(() => {
  const navigate = useNavigate();
  const { rbacEnabled, can } = usePermissions();
  const [users, setUsers] = useState([]);
  const [groups, setGroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [selectedUser, setSelectedUser] = useState(null);
  const [openAddToGroupModal, setOpenAddToGroupModal] = useState(false);
  const [selectedGroup, setSelectedGroup] = useState("");
  const { notify, snackbarProps } = useFeedbackSnackbar();
  const [isAddingGroup, setIsAddingGroup] = useState(false);
  const [newGroupName, setNewGroupName] = useState("");
  // Origin / API key / status filters; "" means any.
  const [originFilter, setOriginFilter] = useState("");
  const [apiKeyFilter, setApiKeyFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const { features } = useSystemFeatures();

  // Helper function to check if we're in gateway-only mode
  const isGatewayOnlyMode = () => {
    return (
      features.feature_gateway &&
      !features.feature_portal &&
      !features.feature_chat
    );
  };

  const { queryParams, updatePaginationData, handlePageChange, tableProps } = useListQuery({
    initialSort: { field: "id", direction: "asc" },
  });

  // A filter change starts again from page 1, like a search does.
  const applyFilter = (setter) => (event) => {
    setter(event.target.value);
    handlePageChange(1);
  };

  const fetchUsers = useCallback(async () => {
    try {
      setLoading(true);
      const params = { ...queryParams };
      if (originFilter) params.auth_source = originFilter;
      if (apiKeyFilter) params.has_api_key = apiKeyFilter;
      if (statusFilter) params.disabled = statusFilter;

      const response = await apiClient.get("/users", { params });
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
  }, [queryParams, updatePaginationData, originFilter, apiKeyFilter, statusFilter]);

  const fetchGroups = useCallback(async () => {
    try {
      const response = await apiClient.get("/groups", { params: { all: true } });
      setGroups(response.data.data || []);
    } catch (error) {
      console.error("Error fetching groups", error);
    }
  }, []);

  useEffect(() => {
    fetchUsers();
  }, [fetchUsers]);

  useEffect(() => {
    fetchGroups();
  }, [fetchGroups]);

  // No /bulk endpoint for users: deletes go one request per item.
  const bulk = useBulkActions({
    items: users,
    resource: "users",
    singular: "user",
    plural: "users",
    nameOf: userLabel,
    notify,
    refresh: fetchUsers,
  });

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/users/${id}`);
      notify("User deleted successfully");
      fetchUsers();
    } catch (error) {
      console.error("Error deleting user", error);
      notify("Failed to delete user", "error");
    }
  };

  const handleUserClick = useCallback((user) => {
    navigate(`/admin/users/${user.id}`);
  }, [navigate]);

  const handleToggleDisabled = async (user) => {
    if (!user) return;
    const disable = !user.attributes.disabled;
    try {
      await setUserDisabled(user.id, disable);
      notify(disable ? "User disabled" : "User enabled");
      fetchUsers();
    } catch (error) {
      console.error("Error updating user status", error);
      notify(error?.message || (disable ? "Failed to disable user" : "Failed to enable user"), "error");
    }
  };

  const handleAddToGroup = useCallback((user) => {
    setSelectedUser(user);
    if (groups.length === 0) {
      setIsAddingGroup(true);
    }
    setOpenAddToGroupModal(true);
  }, [groups.length]);

  const handleCloseAddToGroupModal = useCallback(() => {
    setOpenAddToGroupModal(false);
    setSelectedGroup("");
  }, []);

  const handleAddUserToGroup = async () => {
    if (!selectedGroup || !selectedUser) {
      notify("Please select a team", "warning");
      return;
    }

    try {
      await apiClient.post(`/groups/${selectedGroup}/users`, {
        data: {
          id: selectedUser.id.toString(),
          type: "users",
        },
      });
      notify("User added to team successfully");
      handleCloseAddToGroupModal();
      fetchUsers();
    } catch (error) {
      console.error("Error adding user to group", error);
      notify("Failed to add user to team", "error");
    }
  };

  const handleAddNewGroup = async () => {
    if (!newGroupName.trim()) {
      notify("Team name cannot be empty", "warning");
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
      notify("New team added successfully");
    } catch (error) {
      console.error("Error adding new group", error);
      notify("Failed to add new team", "error");
    }
  };

  const columns = useMemo(() => [
    { field: "id", headerName: "ID", sortable: true },
    { field: "name", headerName: "Name", sortable: true, renderCell: (user) => user.attributes.name },
    { field: "email", headerName: "Email", sortable: true, renderCell: (user) => user.attributes.email },
    {
      field: "email_verified",
      headerName: "Email Verified",
      sortable: true,
      renderCell: (user) => (user.attributes.email_verified ? "Yes" : "No"),
    },
    {
      field: "auth_source",
      headerName: "Origin",
      sortable: true,
      cellProps: (user) => ({ "data-testid": `user-origin-${user.id}` }),
      renderCell: (user) => authSourceLabel(user.attributes.auth_source),
    },
    {
      field: "has_api_key",
      headerName: "API key",
      cellProps: (user) => ({ "data-testid": `user-api-key-${user.id}` }),
      renderCell: (user) => (user.attributes.has_api_key ? "Issued" : "None"),
    },
    {
      field: "disabled",
      headerName: "Status",
      sortable: true,
      cellProps: (user) => ({ "data-testid": `user-status-${user.id}` }),
      renderCell: (user) =>
        user.attributes.disabled ? (
          <Chip label="Disabled" size="small" color="warning" variant="outlined" />
        ) : (
          <Chip label="Active" size="small" color="success" variant="outlined" />
        ),
    },
    rbacEnabled
      ? {
          field: "roles",
          headerName: "Roles",
          renderCell: (user) => (user.attributes.roles || []).map((role) => <RoleBadge key={role.id} role={role} />),
        }
      : {
          field: "is_admin",
          headerName: "Account type",
          sortable: true,
          renderCell: (user) => (
            <CustomSelectBadge config={roleBadgeConfigs[user.attributes.role] || roleBadgeConfigs["Chat user"]} />
          ),
        },
  ], [rbacEnabled]);

  const canWriteUsers = can(P.USERS_WRITE);
  const gatewayOnly = isGatewayOnlyMode();
  const rowActions = [
    // Only show Add to Team if not in gateway-only mode
    { key: "add-to-team", label: "Add to team", onClick: handleAddToGroup, hidden: () => gatewayOnly },
    { key: "edit", label: "Edit user", onClick: (user) => navigate(`/admin/users/edit/${user.id}`) },
    {
      key: "toggle-disabled",
      label: (user) => (user?.attributes?.disabled ? "Enable user" : "Disable user"),
      onClick: handleToggleDisabled,
      hidden: () => !canWriteUsers,
      "data-testid": "user-toggle-disabled",
    },
    { key: "delete", label: "Delete user", onClick: (user) => handleDelete(user.id) },
  ];

  const bulkActions = useMemo(
    () => standardBulkActions({ run: bulk.run, requestDelete: bulk.requestDelete }),
    [bulk.run, bulk.requestDelete],
  );

  if (error && users.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Users</Typography>
        <Can permission={P.USERS_WRITE}>
          <PrimaryButton
            variant="contained"
            startIcon={<AddIcon />}
            component={Link}
            to="/admin/users/new"
          >
            Add user
          </PrimaryButton>
        </Can>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <BulkResultAlert action={bulk.failures?.action} failures={bulk.failures?.failures} onClose={bulk.clearFailures} />
        {/* The search box is DataTable's; the filters sit beside it. */}
        <Box sx={{ mb: 2, display: "flex", gap: 2, flexWrap: "wrap", alignItems: "center" }}>
          <TextField
            select
            size="small"
            label="Origin"
            value={originFilter}
            onChange={applyFilter(setOriginFilter)}
            sx={{ minWidth: 160 }}
            inputProps={{ "data-testid": "users-filter-origin" }}
          >
            {ORIGIN_OPTIONS.map((o) => (
              <MenuItem key={o.value} value={o.value}>{o.label}</MenuItem>
            ))}
          </TextField>
          <TextField
            select
            size="small"
            label="API key"
            value={apiKeyFilter}
            onChange={applyFilter(setApiKeyFilter)}
            sx={{ minWidth: 130 }}
            inputProps={{ "data-testid": "users-filter-api-key" }}
          >
            {API_KEY_OPTIONS.map((o) => (
              <MenuItem key={o.value} value={o.value}>{o.label}</MenuItem>
            ))}
          </TextField>
          <TextField
            select
            size="small"
            label="Status"
            value={statusFilter}
            onChange={applyFilter(setStatusFilter)}
            sx={{ minWidth: 130 }}
            inputProps={{ "data-testid": "users-filter-status" }}
          >
            {STATUS_OPTIONS.map((o) => (
              <MenuItem key={o.value} value={o.value}>{o.label}</MenuItem>
            ))}
          </TextField>
        </Box>
        <Can anyOf={[P.USERS_WRITE, P.GROUPS_WRITE]}>
          {(canAct) => (
            <DataTable
              {...tableProps}
              ariaLabel="Users"
              searchPlaceholder="Search by name or email..."
              columns={columns}
              data={users}
              loading={loading}
              onRowClick={handleUserClick}
              getRowLabel={userLabel}
              actions={canAct ? rowActions : undefined}
              {...(canWriteUsers ? bulk.selectionProps : {})}
              bulkActions={canWriteUsers ? bulkActions : undefined}
              emptyMessage="No users found"
            />
          )}
        </Can>

        <StyledDialog
          open={openAddToGroupModal}
          onClose={handleCloseAddToGroupModal}
        >
          <StyledDialogTitle>
            {isAddingGroup ? "Add New Team" : "Add User to Team"}
          </StyledDialogTitle>
          <StyledDialogContent>
            {isAddingGroup ? (
              <TextField
                fullWidth
                label="New Team Name"
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
                  Select a team from the dropdown menu below to add the user to
                  that team. This action will grant the user permissions
                  associated with the selected team.
                </Typography>
                <FormControl fullWidth sx={{ mt: 2 }}>
                  <InputLabel id="users-team-label">Team</InputLabel>
                  <Select
                    labelId="users-team-label"
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
            <PrimaryButton
              onClick={isAddingGroup ? handleAddNewGroup : handleAddUserToGroup}
              color="primary"
            >
              {isAddingGroup ? "Add Team" : "Add to Team"}
            </PrimaryButton>
          </DialogActions>
        </StyledDialog>

        <BulkDeleteConfirmationDialog
          open={bulk.deleteDialogOpen}
          resourcePath={null}
          objectLabel="user"
          objectLabelPlural="users"
          items={bulk.deleteDialogItems}
          onConfirm={bulk.confirmDelete}
          onCancel={bulk.cancelDelete}
        />

        <FeedbackSnackbar {...snackbarProps} />
      </Box>
    </>
  );
});

Users.displayName = 'Users';

export default Users;
