import React, { useState, useEffect, useCallback, memo, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { useDebounce } from "use-debounce";
import apiClient from "../utils/apiClient";
import SearchInput from "../components/common/SearchInput";
import {
  Table,
  TableBody,
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
import { usePermissions } from "../context/PermissionsContext";
import RoleBadge from "../components/roles/RoleBadge";
import CustomSelectBadge from "../components/common/CustomSelectBadge";
import { roleBadgeConfigs } from "../components/groups/utils/roleBadgeConfig";
import usePagination from "../hooks/usePagination";
import useSystemFeatures from "../hooks/useSystemFeatures";
import Can from "../components/rbac/Can";
import { P } from "../rbac/permissions";
import { authSourceLabel, setUserDisabled } from "../services/userService";
import { Chip } from "@mui/material";

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

const Users = memo(() => {
  const navigate = useNavigate();
  const { rbacEnabled } = usePermissions();
  const [users, setUsers] = useState([]);
  const [groups, setGroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [sortField, setSortField] = useState("id");
  const [sortOrder, setSortOrder] = useState("asc");
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
  const [searchTerm, setSearchTerm] = useState("");
  const [debouncedSearchTerm] = useDebounce(searchTerm, 500);
  // Origin / API key / status filters; "" means any.
  const [originFilter, setOriginFilter] = useState("");
  const [apiKeyFilter, setApiKeyFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const isFirstRender = useRef(true);
  const { features } = useSystemFeatures();

  // Helper function to check if we're in gateway-only mode
  const isGatewayOnlyMode = () => {
    return (
      features.feature_gateway &&
      !features.feature_portal &&
      !features.feature_chat
    );
  };

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
      const params = {
        page,
        page_size: pageSize,
        sort: `${sortOrder === "desc" ? "-" : ""}${sortField}`,
      };

      // Only include search param if 2+ characters entered
      if (debouncedSearchTerm && debouncedSearchTerm.length >= 2) {
        params.search = debouncedSearchTerm;
      }
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
  }, [page, pageSize, updatePaginationData, sortField, sortOrder, debouncedSearchTerm, originFilter, apiKeyFilter, statusFilter]);

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

  // Reset to page 1 when search term or a filter changes (but not on initial render)
  useEffect(() => {
    if (isFirstRender.current) {
      isFirstRender.current = false;
      return;
    }
    handlePageChange(1);
  }, [debouncedSearchTerm, originFilter, apiKeyFilter, statusFilter, handlePageChange]);

  const handleSearch = useCallback((value) => {
    setSearchTerm(value);
  }, []);

  const handleMenuOpen = useCallback((event, user) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedUser(user);
  }, []);

  const handleMenuClose = useCallback(() => {
    setAnchorEl(null);
  }, []);

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

  const handleUserClick = useCallback((user) => {
    navigate(`/admin/users/${user.id}`);
  }, [navigate]);

  const handleToggleDisabled = async () => {
    if (!selectedUser) return;
    const disable = !selectedUser.attributes.disabled;
    try {
      await setUserDisabled(selectedUser.id, disable);
      setSnackbar({
        open: true,
        message: disable ? "User disabled" : "User enabled",
        severity: "success",
      });
      fetchUsers();
    } catch (error) {
      console.error("Error updating user status", error);
      setSnackbar({
        open: true,
        message: error?.message || (disable ? "Failed to disable user" : "Failed to enable user"),
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleAddToGroup = useCallback(() => {
    if (groups.length === 0) {
      setIsAddingGroup(true);
    }
    setOpenAddToGroupModal(true);
    handleMenuClose();
  }, [groups.length, handleMenuClose]);

  const handleCloseAddToGroupModal = useCallback(() => {
    setOpenAddToGroupModal(false);
    setSelectedGroup("");
  }, []);

  const handleAddUserToGroup = async () => {
    if (!selectedGroup || !selectedUser) {
      setSnackbar({
        open: true,
        message: "Please select a team",
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
        message: "User added to team successfully",
        severity: "success",
      });
      handleCloseAddToGroupModal();
      fetchUsers();
    } catch (error) {
      console.error("Error adding user to group", error);
      setSnackbar({
        open: true,
        message: "Failed to add user to team",
        severity: "error",
      });
    }
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
      setNewGroupName("");
      setIsAddingGroup(false);
      setSnackbar({
        open: true,
        message: "New team added successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error adding new group", error);
      setSnackbar({
        open: true,
        message: "Failed to add new team",
        severity: "error",
      });
    }
  };

  const handleCloseSnackbar = useCallback((event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  }, [snackbar]);

  if (loading && users.length === 0) {
    return <CircularProgress />;
  }

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
        <Box sx={{ mb: 2, display: "flex", gap: 2, flexWrap: "wrap", alignItems: "center" }}>
          <Box sx={{ width: 400, maxWidth: "100%" }}>
            <SearchInput
              value={searchTerm}
              onChange={handleSearch}
              placeholder="Search by name or email..."
            />
          </Box>
          <TextField
            select
            size="small"
            label="Origin"
            value={originFilter}
            onChange={(e) => setOriginFilter(e.target.value)}
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
            onChange={(e) => setApiKeyFilter(e.target.value)}
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
            onChange={(e) => setStatusFilter(e.target.value)}
            sx={{ minWidth: 130 }}
            inputProps={{ "data-testid": "users-filter-status" }}
          >
            {STATUS_OPTIONS.map((o) => (
              <MenuItem key={o.value} value={o.value}>{o.label}</MenuItem>
            ))}
          </TextField>
        </Box>
        <StyledPaper>
          <Table>
            <TableHead>
              <TableRow>
                <StyledTableHeaderCell
                  onClick={() => {
                    setSortOrder(sortField === "id" ? (sortOrder === "asc" ? "desc" : "asc") : "asc");
                    setSortField("id");
                  }}
                  sx={{ cursor: 'pointer' }}
                >
                  ID {sortField === "id" && (sortOrder === "asc" ? "↑" : "↓")}
                </StyledTableHeaderCell>
                <StyledTableHeaderCell
                  onClick={() => {
                    setSortOrder(sortField === "name" ? (sortOrder === "asc" ? "desc" : "asc") : "asc");
                    setSortField("name");
                  }}
                  sx={{ cursor: 'pointer' }}
                >
                  Name {sortField === "name" && (sortOrder === "asc" ? "↑" : "↓")}
                </StyledTableHeaderCell>
                <StyledTableHeaderCell
                  onClick={() => {
                    setSortOrder(sortField === "email" ? (sortOrder === "asc" ? "desc" : "asc") : "asc");
                    setSortField("email");
                  }}
                  sx={{ cursor: 'pointer' }}
                >
                  Email {sortField === "email" && (sortOrder === "asc" ? "↑" : "↓")}
                </StyledTableHeaderCell>
                <StyledTableHeaderCell
                  onClick={() => {
                    setSortOrder(sortField === "email_verified" ? (sortOrder === "asc" ? "desc" : "asc") : "asc");
                    setSortField("email_verified");
                  }}
                  sx={{ cursor: 'pointer' }}
                >
                  Email Verified {sortField === "email_verified" && (sortOrder === "asc" ? "↑" : "↓")}
                </StyledTableHeaderCell>
                <StyledTableHeaderCell
                  onClick={() => {
                    setSortOrder(sortField === "auth_source" ? (sortOrder === "asc" ? "desc" : "asc") : "asc");
                    setSortField("auth_source");
                  }}
                  sx={{ cursor: 'pointer' }}
                >
                  Origin {sortField === "auth_source" && (sortOrder === "asc" ? "↑" : "↓")}
                </StyledTableHeaderCell>
                <StyledTableHeaderCell>API key</StyledTableHeaderCell>
                <StyledTableHeaderCell
                  onClick={() => {
                    setSortOrder(sortField === "disabled" ? (sortOrder === "asc" ? "desc" : "asc") : "asc");
                    setSortField("disabled");
                  }}
                  sx={{ cursor: 'pointer' }}
                >
                  Status {sortField === "disabled" && (sortOrder === "asc" ? "↑" : "↓")}
                </StyledTableHeaderCell>
                {rbacEnabled ? (
                  <StyledTableHeaderCell>Roles</StyledTableHeaderCell>
                ) : (
                  <StyledTableHeaderCell
                    onClick={() => {
                      setSortOrder(sortField === "is_admin" ? (sortOrder === "asc" ? "desc" : "asc") : "asc");
                      setSortField("is_admin");
                    }}
                    sx={{ cursor: 'pointer' }}
                  >
                    Account type {sortField === "is_admin" && (sortOrder === "asc" ? "↑" : "↓")}
                  </StyledTableHeaderCell>
                )}
                <StyledTableHeaderCell align="right">
                  Actions
                </StyledTableHeaderCell>
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
                    <StyledTableCell>{user.id}</StyledTableCell>
                    <StyledTableCell>{user.attributes.name}</StyledTableCell>
                    <StyledTableCell>{user.attributes.email}</StyledTableCell>
                    <StyledTableCell>
                      {user.attributes.email_verified ? "Yes" : "No"}
                    </StyledTableCell>
                    <StyledTableCell data-testid={`user-origin-${user.id}`}>
                      {authSourceLabel(user.attributes.auth_source)}
                    </StyledTableCell>
                    <StyledTableCell data-testid={`user-api-key-${user.id}`}>
                      {user.attributes.has_api_key ? "Issued" : "None"}
                    </StyledTableCell>
                    <StyledTableCell data-testid={`user-status-${user.id}`}>
                      {user.attributes.disabled ? (
                        <Chip label="Disabled" size="small" color="warning" variant="outlined" />
                      ) : (
                        <Chip label="Active" size="small" color="success" variant="outlined" />
                      )}
                    </StyledTableCell>
                    <StyledTableCell>
                      {rbacEnabled
                        ? (user.attributes.roles || []).map((role) => <RoleBadge key={role.id} role={role} />)
                        : <CustomSelectBadge config={roleBadgeConfigs[user.attributes.role] || roleBadgeConfigs["Chat user"]} />}
                    </StyledTableCell>
                    <StyledTableCell align="right">
                      <Can anyOf={[P.USERS_WRITE, P.GROUPS_WRITE]}>
                        <IconButton
                          onClick={(event) => handleMenuOpen(event, user)}
                        >
                          <MoreVertIcon />
                        </IconButton>
                      </Can>
                    </StyledTableCell>
                  </StyledTableRow>
                ))
              ) : (
                <TableRow>
                  <StyledTableCell colSpan={9}>No users found</StyledTableCell>
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

        <Menu
          anchorEl={anchorEl}
          open={Boolean(anchorEl)}
          onClose={handleMenuClose}
        >
          {/* Only show Add to Team if not in gateway-only mode */}
          {!isGatewayOnlyMode() && (
            <MenuItem onClick={handleAddToGroup}>Add to team</MenuItem>
          )}
          <MenuItem
            onClick={() => navigate(`/admin/users/edit/${selectedUser?.id}`)}
          >
            Edit user
          </MenuItem>
          <Can permission={P.USERS_WRITE}>
            <MenuItem onClick={handleToggleDisabled} data-testid="user-toggle-disabled">
              {selectedUser?.attributes?.disabled ? "Enable user" : "Disable user"}
            </MenuItem>
          </Can>
          <MenuItem onClick={() => handleDelete(selectedUser?.id)}>
            Delete user
          </MenuItem>
        </Menu>

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
    </>
  );
});

Users.displayName = 'Users';

export default Users;
