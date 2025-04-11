import React, { useState, useEffect, useCallback } from "react";
import { useParams, useNavigate, Link as RouterLink } from "react-router-dom";
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
  SecondaryLinkButton
} from "../../styles/sharedStyles";
import PaginationControls from "../common/PaginationControls";
import usePagination from "../../hooks/usePagination";
import { Divider } from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import RefreshIcon from "@mui/icons-material/Refresh";
import { IconButton, Tooltip } from "@mui/material";

const UserDetails = () => {
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(true);
  const [userGroups, setUserGroups] = useState([]);
  const [chatHistory, setChatHistory] = useState([]);
  const [showSnackbar, setShowSnackbar] = useState(false);
  const [snackbarMessage, setSnackbarMessage] = useState("");
  const { id } = useParams();
  const navigate = useNavigate();

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

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

  const handleRollApiKey = async () => {
    try {
      const response = await apiClient.post(`/users/${id}/roll-api-key`);
      setUser(response.data.data);
      setSnackbarMessage("API Key successfully regenerated");
      setShowSnackbar(true);
    } catch (error) {
      console.error("Error rolling API key", error);
      setSnackbarMessage("Failed to regenerate API Key");
      setShowSnackbar(true);
    }
  };

  const maskedApiKey = user?.attributes?.api_key
    ? `${user.attributes.api_key.substring(0, 4)}${"*".repeat(20)}${user.attributes.api_key.slice(-4)}`
    : "********";

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
      const response = await apiClient.get(`/chat-history-records`, {
        params: {
          user_id: id,
          page,
          page_size: pageSize,
        },
      });
      setChatHistory(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
    } catch (error) {
      console.error("Error fetching chat history", error);
    } finally {
      setLoading(false);
    }
  }, [id, page, pageSize, updatePaginationData]);

  useEffect(() => {
    fetchUserDetails();
    fetchUserGroups();
  }, [fetchUserDetails, fetchUserGroups]);

  useEffect(() => {
    fetchChatHistory();
  }, [fetchChatHistory]);

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
            <FieldLabel>API Key:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box display="flex" alignItems="center">
              <FieldValue>{maskedApiKey}</FieldValue>
              <Tooltip title="Copy API Key">
                <IconButton onClick={handleCopyApiKey} size="small">
                  <ContentCopyIcon />
                </IconButton>
              </Tooltip>
              <Tooltip title="Regenerate API Key">
                <IconButton
                  onClick={handleRollApiKey}
                  size="small"
                  color="primary"
                >
                  <RefreshIcon />
                </IconButton>
              </Tooltip>
            </Box>
          </Grid>

          <Grid item xs={3}>
            <FieldLabel>Admin:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{user.attributes.is_admin ? "Yes" : "No"}</FieldValue>
          </Grid>
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
          {user.attributes.is_admin && (
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
        <Box
          mb={2}
          display="flex"
          justifyContent="space-between"
          alignItems="center"
        >
          <Typography variant="h5">Groups</Typography>
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
            Group Membership
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
                    <StyledTableCell>User is not a member of any groups</StyledTableCell>
                  </TableRow>
                )}
              </TableBody>
              </Table>
            </TableContainer>
          </StyledPaper>
        )}

        <Box mt={4} mb={2}>
          <Typography variant="h5" sx={{ color: "black" }}>
            Chat History
          </Typography>
        </Box>
        {loading ? (
          <CircularProgress />
        ) : (
          <>
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
      </ContentBox>
    </>
  );
};

export default UserDetails;
