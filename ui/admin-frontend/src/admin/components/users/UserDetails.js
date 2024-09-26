import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
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

  if (!user) return <CircularProgress />;

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">User Details</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/users")}
          color="white"
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
        {loading ? (
          <CircularProgress />
        ) : (
          <TableContainer>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell>
                    <Typography variant="h6" color="black">
                      Group Memberships
                    </Typography>
                  </TableCell>
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
      </ContentBox>
    </StyledPaper>
  );
};

export default UserDetails;
