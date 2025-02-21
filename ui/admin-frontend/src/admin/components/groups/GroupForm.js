import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import { TextField, Typography, CircularProgress, Box } from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
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
        <Typography variant="headingXLarge">
          {id ? "Edit user group" : "Add user group"}
        </Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          color="inherit"
          onClick={() => navigate("/admin/groups")}
        >
          Back to groups
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">User groups help you organize users and easily manage their access to LLM providers, data sources, and tools through catalogs. Linking user groups to specific catalogs ensures each team can only see and access the LLM provider and or data relevant to them.</Typography>  
      </Box>
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
          <PrimaryButton
            type="submit"
            variant="contained"
            color="primary"
            style={{ marginTop: "20px" }}
            disabled={loading}
          >
            {id ? "Update group" : "Create group"}
          </PrimaryButton>
        </form>
      </ContentBox>
    </>
  );
};

export default GroupForm;
