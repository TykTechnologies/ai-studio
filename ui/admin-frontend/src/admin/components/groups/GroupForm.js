import React, { useState, useEffect } from "react";
import { useParams, useNavigate, NavLink } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import { TextField, Button, Typography, CircularProgress, Link } from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledButton,
} from "../../styles/sharedStyles";

const GroupForm = () => {
  const [name, setName] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    if (id) {
      fetchGroup();
    }
  }, [id]);

  const fetchGroup = async () => {
    try {
      setLoading(true);
      const response = await apiClient.get(`/groups/${id}`);
      setName(response.data.data.attributes.name);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching group", error);
      setError("Failed to fetch group");
      setLoading(false);
    }
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setLoading(true);
    setError(null);

    const groupData = {
      data: {
        type: "Group",
        attributes: {
          name,
        },
      },
    };

    try {
      if (id) {
        await apiClient.patch(`/groups/${id}`, groupData);
      } else {
        await apiClient.post("/groups", groupData);
      }
      navigate("/admin/groups");
    } catch (error) {
      console.error("Error saving group", error);
      setError("Failed to save group");
      setLoading(false);
    }
  };

  if (loading) return <CircularProgress />;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="h5">
          {id ? "Edit Group" : "Create Group"}
        </Typography>
        <Link component={NavLink} to="/admin/groups">
          <ArrowBackIcon sx={{ mr: 1 }} />
          Back to Groups
        </Link>
      </TitleBox>
      <ContentBox>
        <form onSubmit={handleSubmit}>
          <TextField
            fullWidth
            label="Group Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            margin="normal"
            required
          />
          {error && (
            <Typography color="error" style={{ marginTop: "10px" }}>
              {error}
            </Typography>
          )}
          <StyledButton
            type="submit"
            variant="contained"
            color="primary"
            style={{ marginTop: "20px" }}
            disabled={loading}
          >
            {id ? "Update Group" : "Create Group"}
          </StyledButton>
        </form>
      </ContentBox>
    </>
  );
};

export default GroupForm;
