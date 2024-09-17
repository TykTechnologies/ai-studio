import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "./apiClient";
import {
  Typography,
  IconButton,
  CircularProgress,
  Box,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Grid,
  Button,
} from "@mui/material";
import { styled } from "@mui/material/styles";
import DeleteIcon from "@mui/icons-material/Delete";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";

const StyledPaper = styled(Paper)(({ theme }) => ({
  backgroundColor: "#2c2c2c",
  borderRadius: theme.shape.borderRadius * 2,
  overflow: "hidden",
}));

const TitleBox = styled(Box)(({ theme }) => ({
  backgroundColor: "#0B4545",
  padding: theme.spacing(2),
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
}));

const ContentBox = styled(Box)(({ theme }) => ({
  padding: theme.spacing(3),
}));

const StyledTableContainer = styled(TableContainer)(({ theme }) => ({
  marginTop: theme.spacing(2),
  border: `1px solid ${theme.palette.divider}`,
  borderRadius: "4px",
}));

const FieldLabel = styled(Typography)(({ theme }) => ({
  fontWeight: "bold",
  color: theme.palette.text.secondary,
}));

const FieldValue = styled(Typography)(({ theme }) => ({
  color: theme.palette.text.primary,
}));

const UserDetails = () => {
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(true);
  const [userGroups, setUserGroups] = useState([]);
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    fetchUserDetails();
    fetchUserGroups();
  }, [id]);

  const fetchUserDetails = async () => {
    try {
      const response = await apiClient.get(`/users/${id}`);
      setUser(response.data.data);
    } catch (error) {
      console.error("Error fetching user details", error);
    }
  };

  const fetchUserGroups = async () => {
    setLoading(true);
    try {
      const response = await apiClient.get(`/users/${id}/groups`);
      setUserGroups(response.data.data || []);
    } catch (error) {
      console.error("Error fetching user groups", error);
    } finally {
      setLoading(false);
    }
  };

  const handleRemoveFromGroup = async (groupId) => {
    setLoading(true);
    try {
      await apiClient.delete(`/groups/${groupId}/users/${id}`);
      await fetchUserGroups();
    } catch (error) {
      console.error("Error removing user from group", error);
    } finally {
      setLoading(false);
    }
  };

  if (!user) return <CircularProgress />;

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h4" color="white">
          User Details
        </Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/users")}
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
        </Grid>
        <Box
          mb={2}
          display="flex"
          justifyContent="space-between"
          alignItems="center"
        >
          <Typography variant="h5">Groups</Typography>
          <Button
            variant="contained"
            color="primary"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/users/edit/${id}`)}
          >
            Edit User
          </Button>
        </Box>
        {loading ? (
          <CircularProgress />
        ) : (
          <StyledTableContainer component={Paper}>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell>Group Name</TableCell>
                  <TableCell align="right">Action</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {userGroups.length > 0 ? (
                  userGroups.map((group) => (
                    <TableRow key={group.id}>
                      <TableCell>{group.attributes.name}</TableCell>
                      <TableCell align="right">
                        <IconButton
                          edge="end"
                          aria-label="delete"
                          onClick={() => handleRemoveFromGroup(group.id)}
                        >
                          <DeleteIcon />
                        </IconButton>
                      </TableCell>
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell colSpan={2}>
                      User is not a member of any groups
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </StyledTableContainer>
        )}
      </ContentBox>
    </StyledPaper>
  );
};

export default UserDetails;
