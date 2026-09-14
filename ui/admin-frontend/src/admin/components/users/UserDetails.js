import React, { useState, useEffect, useCallback, useRef } from "react";
import { useParams, useNavigate, Link as RouterLink } from "react-router-dom";
import { useDebounce } from "use-debounce";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  Grid,
  Link,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import DownloadIcon from "@mui/icons-material/Download";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  StyledTableRow,
  PrimaryButton,
  StyledTableHeaderCell,
  StyledTableCell,
  SecondaryLinkButton,
  SecondaryOutlineButton
} from "../../styles/sharedStyles";
import PaginationControls from "../common/PaginationControls";
import usePagination from "../../hooks/usePagination";
import SearchInput from "../common/SearchInput";
import { Divider } from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import RefreshIcon from "@mui/icons-material/Refresh";
import { IconButton, Tooltip } from "@mui/material";
import ExportProxyLogsModal from "../common/ExportProxyLogsModal";
import { useEdition } from "../../context/EditionContext";
import { usePermissions } from "../../context/PermissionsContext";
import RoleBadge from "../roles/RoleBadge";
import EffectivePermissionsList from "../roles/EffectivePermissionsList";
import CollapsibleSection from "../common/CollapsibleSection";
import Can from "../rbac/Can";
import { P } from "../../rbac/permissions";
import { getEffectivePermissions } from "../../services/rbacService";
import {
  authSourceLabel,
  formatLastLogin,
  revokeApiKey,
  rollApiKey,
  setUserDisabled,
} from "../../services/userService";
import useConfig from "../../hooks/useConfig";
import DeleteOutlineIcon from "@mui/icons-material/DeleteOutline";
import BlockIcon from "@mui/icons-material/Block";
import CheckCircleOutlineIcon from "@mui/icons-material/CheckCircleOutline";
import { Chip, Dialog, DialogTitle, DialogContent, DialogContentText, DialogActions, Button } from "@mui/material";

const UserDetails = () => {
  const { isEnterprise } = useEdition();
  const { rbacEnabled } = usePermissions();
  const [effectiveAccess, setEffectiveAccess] = useState(null);
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(true);
  const [userGroups, setUserGroups] = useState([]);
  const [chatHistory, setChatHistory] = useState([]);
  const [showSnackbar, setShowSnackbar] = useState(false);
  const [snackbarMessage, setSnackbarMessage] = useState("");
  const [chatSearchTerm, setChatSearchTerm] = useState("");
  const [debouncedChatSearch] = useDebounce(chatSearchTerm, 500);
  const [exportModalOpen, setExportModalOpen] = useState(false);
  const [confirmRevokeOpen, setConfirmRevokeOpen] = useState(false);
  const isFirstSearchRender = useRef(true);
  const { id } = useParams();
  const navigate = useNavigate();
  const { config } = useConfig();

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const handleChatSearch = useCallback((value) => {
    setChatSearchTerm(value);
  }, []);

  const handleCopyApiKey = async () => {
    try {
      await navigator.clipboard.writeText(user.attributes.api_key);
      setSnackbarMessage("API Key copied to clipboard");
      setShowSnackbar(true);
    } catch (err) {
      setSnackbarMessage("Failed to copy API Key");
      setShowSnackbar(true);
    }
  };

  // The server only returns another user's key to callers who may manage
  // users; otherwise it sends a hint (the last four characters). A user
  // with no key issued (admin-created, SSO-provisioned, or revoked) has
  // has_api_key=false and shows no mask at all.
  const hasApiKey = Boolean(
    user?.attributes?.has_api_key || user?.attributes?.api_key || user?.attributes?.api_key_hint
  );
  const canSeeApiKey = Boolean(user?.attributes?.api_key);
  const maskedApiKey = user?.attributes?.api_key
    ? `${user.attributes.api_key.substring(0, 4)}${"*".repeat(20)}${user.attributes.api_key.slice(-4)}`
    : user?.attributes?.api_key_hint
      ? `${"*".repeat(24)}${user.attributes.api_key_hint}`
      : "********";
  // Identity-provider accounts cannot be issued a key unless the operator
  // opted in (ALLOW_SSO_USER_API_KEYS); hide the button rather than 403.
  const isSSOUser = user?.attributes?.auth_source === "sso";
  const canIssueApiKey = !isSSOUser || Boolean(config?.allowSSOUserAPIKeys);

  const handleRollApiKey = async () => {
    try {
      const updated = await rollApiKey(id);
      setUser(updated);
      setSnackbarMessage(hasApiKey ? "API Key successfully regenerated" : "API Key issued");
      setShowSnackbar(true);
    } catch (error) {
      console.error("Error rolling API key", error);
      setSnackbarMessage(error?.message || "Failed to regenerate API Key");
      setShowSnackbar(true);
    }
  };

  const handleRevokeApiKey = async () => {
    setConfirmRevokeOpen(false);
    try {
      const updated = await revokeApiKey(id);
      setUser(updated);
      setSnackbarMessage("API Key revoked");
      setShowSnackbar(true);
    } catch (error) {
      console.error("Error revoking API key", error);
      setSnackbarMessage(error?.message || "Failed to revoke API Key");
      setShowSnackbar(true);
    }
  };

  const handleToggleDisabled = async () => {
    const disable = !user?.attributes?.disabled;
    try {
      const updated = await setUserDisabled(id, disable);
      setUser(updated);
      setSnackbarMessage(disable ? "User disabled" : "User enabled");
      setShowSnackbar(true);
    } catch (error) {
      console.error("Error updating user status", error);
      setSnackbarMessage(error?.message || (disable ? "Failed to disable user" : "Failed to enable user"));
      setShowSnackbar(true);
    }
  };

  useEffect(() => {
    if (!rbacEnabled || !id) return undefined;
    let cancelled = false;
    getEffectivePermissions(id)
      .then((data) => {
        if (!cancelled) setEffectiveAccess(data);
      })
      .catch((error) => console.error("Error fetching effective permissions", error));
    return () => {
      cancelled = true;
    };
  }, [rbacEnabled, id, user?.attributes?.roles]);

  const fetchUserDetails = useCallback(async () => {
    try {
      const response = await apiClient.get(`/users/${id}`);
      setUser(response.data.data);
    } catch (error) {
      console.error("Error fetching user details", error);
    }
  }, [id]);

  const fetchUserGroups = useCallback(async () => {
    try {
      const response = await apiClient.get(`/users/${id}/groups`);
      setUserGroups(response.data.data || []);
    } catch (error) {
      console.error("Error fetching user groups", error);
    }
  }, [id]);

  const fetchChatHistory = useCallback(async () => {
    try {
      const params = {
        user_id: id,
        page,
        page_size: pageSize,
      };

      // Only include search param if 2+ characters entered
      if (debouncedChatSearch && debouncedChatSearch.length >= 2) {
        params.search = debouncedChatSearch;
      }

      const response = await apiClient.get(`/chat-history-records`, { params });
      setChatHistory(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
    } catch (error) {
      console.error("Error fetching chat history", error);
    } finally {
      setLoading(false);
    }
  }, [id, page, pageSize, debouncedChatSearch, updatePaginationData]);

  useEffect(() => {
    fetchUserDetails();
    fetchUserGroups();
  }, [fetchUserDetails, fetchUserGroups]);

  useEffect(() => {
    fetchChatHistory();
  }, [fetchChatHistory]);

  // Reset to page 1 when chat search term changes (but not on initial render)
  useEffect(() => {
    if (isFirstSearchRender.current) {
      isFirstSearchRender.current = false;
      return;
    }
    handlePageChange(1);
  }, [debouncedChatSearch, handlePageChange]);

  if (!user) return <CircularProgress />;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">User details</Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/users")}
          color="inherit"
        >
          Back to users
        </SecondaryLinkButton>
      </TitleBox>
      <ContentBox>
        <Grid container spacing={2} mb={4}>
          <Grid item xs={3}>
            <FieldLabel>Name:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{user.attributes.name}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Email:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{user.attributes.email}</FieldValue>
          </Grid>

          <Grid item xs={3}>
            <FieldLabel>Status:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box display="flex" alignItems="center" gap={1}>
              {user.attributes.disabled ? (
                <Chip label="Disabled" size="small" color="warning" variant="outlined" data-testid="user-status-chip" />
              ) : (
                <Chip label="Active" size="small" color="success" variant="outlined" data-testid="user-status-chip" />
              )}
              {user.attributes.disabled && user.attributes.disabled_at && (
                <FieldValue>since {new Date(user.attributes.disabled_at).toLocaleString()}</FieldValue>
              )}
              <Can permission={P.USERS_WRITE}>
                <Tooltip title={user.attributes.disabled ? "Enable user" : "Disable user (blocks every login and API key)"}>
                  <IconButton
                    onClick={handleToggleDisabled}
                    size="small"
                    color={user.attributes.disabled ? "success" : "warning"}
                    data-testid="user-toggle-disabled"
                  >
                    {user.attributes.disabled ? <CheckCircleOutlineIcon /> : <BlockIcon />}
                  </IconButton>
                </Tooltip>
              </Can>
            </Box>
          </Grid>

          <Grid item xs={3}>
            <FieldLabel>Origin:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue data-testid="user-origin">
              {authSourceLabel(user.attributes.auth_source)}
              {user.attributes.sso_profile_id ? ` (profile ${user.attributes.sso_profile_id})` : ""}
            </FieldValue>
          </Grid>

          <Grid item xs={3}>
            <FieldLabel>Last login:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue data-testid="user-last-login">{formatLastLogin(user.attributes)}</FieldValue>
          </Grid>

          <Grid item xs={3}>
            <FieldLabel>API Key:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box display="flex" alignItems="center">
              {hasApiKey ? (
                <>
                  <FieldValue>{maskedApiKey}</FieldValue>
                  {canSeeApiKey && (
                    <Tooltip title="Copy API Key">
                      <IconButton onClick={handleCopyApiKey} size="small">
                        <ContentCopyIcon />
                      </IconButton>
                    </Tooltip>
                  )}
                  <Can permission={P.USERS_WRITE}>
                    <Tooltip title="Regenerate API Key">
                      <IconButton
                        onClick={handleRollApiKey}
                        size="small"
                        color="primary"
                        data-testid="user-roll-api-key"
                      >
                        <RefreshIcon />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="Revoke API Key">
                      <IconButton
                        onClick={() => setConfirmRevokeOpen(true)}
                        size="small"
                        color="error"
                        data-testid="user-revoke-api-key"
                      >
                        <DeleteOutlineIcon />
                      </IconButton>
                    </Tooltip>
                  </Can>
                </>
              ) : (
                <>
                  <FieldValue data-testid="user-no-api-key">No API key issued</FieldValue>
                  <Can permission={P.USERS_WRITE}>
                    {canIssueApiKey ? (
                      <Button
                        onClick={handleRollApiKey}
                        size="small"
                        variant="outlined"
                        sx={{ ml: 2 }}
                        data-testid="user-issue-api-key"
                      >
                        Issue API key
                      </Button>
                    ) : (
                      <Tooltip title="Set ALLOW_SSO_USER_API_KEYS=true to permit API keys for SSO-provisioned users">
                        <FieldValue sx={{ ml: 2, fontStyle: "italic" }} data-testid="user-sso-no-api-key">
                          (not available for SSO-provisioned users)
                        </FieldValue>
                      </Tooltip>
                    )}
                  </Can>
                </>
              )}
            </Box>
            {hasApiKey && (
              <FieldValue data-testid="user-api-key-last-used">
                Last used: {user.attributes.api_key_last_used_at
                  ? new Date(user.attributes.api_key_last_used_at).toLocaleString()
                  : "never"}
              </FieldValue>
            )}
          </Grid>

          {rbacEnabled ? (
            <>
              <Grid item xs={3}>
                <FieldLabel>Roles:</FieldLabel>
              </Grid>
              <Grid item xs={9} data-testid="user-roles">
                {(user.attributes.roles || []).length > 0 ? (
                  user.attributes.roles.map((role) => <RoleBadge key={role.id} role={role} />)
                ) : (
                  <FieldValue>No roles assigned</FieldValue>
                )}
              </Grid>
            </>
          ) : (
            <>
              <Grid item xs={3}>
                <FieldLabel>Admin:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <FieldValue>{user.attributes.is_admin ? "Yes" : "No"}</FieldValue>
              </Grid>
            </>
          )}
          {user.attributes.is_admin && (
            <>
              <Grid item xs={3}>
                <FieldLabel>Notifications:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <FieldValue>
                  {user.attributes.notifications_enabled ? "Enabled" : "Disabled"}
                </FieldValue>
              </Grid>
            </>
          )}
          {user.attributes.is_admin && !rbacEnabled && (
            <>
              <Grid item xs={3}>
                <FieldLabel>Access to IdP configuration:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <FieldValue>
                  {user.attributes.access_to_sso_config ? "Enabled" : "Disabled"}
                </FieldValue>
              </Grid>
            </>
          )}
        </Grid>
        {rbacEnabled && effectiveAccess && (
          <Box mb={4}>
            <CollapsibleSection title="Effective permissions" defaultExpanded={false}>
              <Box sx={{ px: 2, pb: 2 }}>
                <EffectivePermissionsList permissions={effectiveAccess.permissions || []} />
              </Box>
            </CollapsibleSection>
          </Box>
        )}
        <Box
          mb={2}
          display="flex"
          justifyContent="space-between"
          alignItems="center"
        >
          <Typography variant="h5">Teams</Typography>
          <PrimaryButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/users/edit/${id}`)}
          >
            Edit user
          </PrimaryButton>
        </Box>
        <Divider />
        <Box mt={4} mb={2}>
          <Typography variant="h5" sx={{ color: "black" }}>
            Team Membership
          </Typography>
        </Box>
        {loading ? (
          <CircularProgress />
        ) : (
          <StyledPaper>
            <TableContainer>
              <Table>
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {userGroups.length > 0 ? (
                  userGroups.map((group) => (
                    <StyledTableRow key={group.id}>
                      <StyledTableCell>{group.attributes.name}</StyledTableCell>
                    </StyledTableRow>
                  ))
                ) : (
                  <TableRow>
                    <StyledTableCell>User is not a member of any teams</StyledTableCell>
                  </TableRow>
                )}
              </TableBody>
              </Table>
            </TableContainer>
          </StyledPaper>
        )}

        <Box mt={4} mb={2} display="flex" justifyContent="space-between" alignItems="center">
          <Typography variant="h5" sx={{ color: "black" }}>
            Chat History
          </Typography>
          {isEnterprise && (
            <SecondaryOutlineButton
              onClick={() => setExportModalOpen(true)}
              startIcon={<DownloadIcon />}
              size="small"
            >
              Export
            </SecondaryOutlineButton>
          )}
        </Box>
        {loading ? (
          <CircularProgress />
        ) : (
          <>
            <Box sx={{ mb: 2, maxWidth: 400 }}>
              <SearchInput
                value={chatSearchTerm}
                onChange={handleChatSearch}
                placeholder="Search conversations..."
              />
            </Box>
            <StyledPaper>
              <TableContainer>
                <Table>
                <TableHead>
                  <TableRow>
                    <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Chat ID</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Action</StyledTableHeaderCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {chatHistory.length > 0 ? (
                    chatHistory.map((record) => (
                      <StyledTableRow key={record.id}>
                        <StyledTableCell>{record.attributes.name}</StyledTableCell>
                        <StyledTableCell>{record.attributes.chat_id}</StyledTableCell>
                        <StyledTableCell>
                          <Link
                            component={RouterLink}
                            to={`/admin/users/${id}/chat-log/${record.attributes.session_id}`}
                            sx={{ textDecoration: 'underline' }}
                          >
                            View Chat Log
                          </Link>
                        </StyledTableCell>
                      </StyledTableRow>
                    ))
                  ) : (
                    <TableRow>
                      <StyledTableCell colSpan={3}>
                        No chat history records found
                      </StyledTableCell>
                    </TableRow>
                  )}
                </TableBody>
                </Table>
              </TableContainer>
              <PaginationControls
                page={page}
                pageSize={pageSize}
                totalPages={totalPages}
                onPageChange={handlePageChange}
                onPageSizeChange={handlePageSizeChange}
              />
            </StyledPaper>
          </>
        )}

        <Dialog open={confirmRevokeOpen} onClose={() => setConfirmRevokeOpen(false)}>
          <DialogTitle>Revoke API key?</DialogTitle>
          <DialogContent>
            <DialogContentText>
              Any integration using this user's API key will stop working immediately.
              A new key can be issued later.
            </DialogContentText>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setConfirmRevokeOpen(false)}>Cancel</Button>
            <Button onClick={handleRevokeApiKey} color="error" variant="contained" data-testid="user-revoke-api-key-confirm">
              Revoke
            </Button>
          </DialogActions>
        </Dialog>
        <ExportProxyLogsModal
          open={exportModalOpen}
          onClose={() => setExportModalOpen(false)}
          sourceType="user"
          sourceId={parseInt(id)}
          initialSearch={debouncedChatSearch}
        />
      </ContentBox>
    </>
  );
};

export default UserDetails;
