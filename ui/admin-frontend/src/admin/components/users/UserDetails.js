import React, { useState, useEffect, useCallback } from "react";
import { useParams, useNavigate, Link as RouterLink } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Grid,
  Button,
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
  StyledButton,
} from "../../styles/sharedStyles";
import PaginationControls from "../common/PaginationControls";
import usePagination from "../../hooks/usePagination";
import { Divider } from "@mui/material";

const UserDetails = () => {
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(true);
  const [userGroups, setUserGroups] = useState([]);
  const [chatHistory, setChatHistory] = useState([]);
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
        <Typography variant="h5">User Details</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/users")}
          color="inherit"
        >
          Back to Users
        </Button>
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
            <FieldLabel>Admin:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{user.attributes.is_admin ? "Yes" : "No"}</FieldValue>
          </Grid>
        </Grid>
        <Box
          mb={2}
          display="flex"
          justifyContent="space-between"
          alignItems="center"
        >
          <Typography variant="h5">Groups</Typography>
          <StyledButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/users/edit/${id}`)}
          >
            Edit User
          </StyledButton>
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
          <TableContainer>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell>Name</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {userGroups.length > 0 ? (
                  userGroups.map((group) => (
                    <StyledTableRow key={group.id}>
                      <TableCell>{group.attributes.name}</TableCell>
                    </StyledTableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell>User is not a member of any groups</TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>
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
            <TableContainer>
              <Table>
                <TableHead>
                  <TableRow>
                    <TableCell>Name</TableCell>
                    <TableCell>Chat ID</TableCell>
                    <TableCell>Action</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {chatHistory.length > 0 ? (
                    chatHistory.map((record) => (
                      <StyledTableRow key={record.id}>
                        <TableCell>{record.attributes.name}</TableCell>
                        <TableCell>{record.attributes.chat_id}</TableCell>
                        <TableCell>
                          <RouterLink
                            to={`/admin/users/${id}/chat-log/${record.attributes.session_id}`}
                          >
                            <Link variant="body2">View Chat Log</Link>
                          </RouterLink>
                        </TableCell>
                      </StyledTableRow>
                    ))
                  ) : (
                    <TableRow>
                      <TableCell colSpan={3}>
                        No chat history records found
                      </TableCell>
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
          </>
        )}
      </ContentBox>
    </>
  );
};

export default UserDetails;
