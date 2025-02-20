import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import { TextField, Typography, CircularProgress } from "@mui/material";
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
        <Typography variant="h1">
          {id ? "Edit Group" : "Create Group"}
        </Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          color="inherit"
          onClick={() => navigate("/admin/groups")}
        >
          Back to Groups
        </SecondaryLinkButton>
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
          <PrimaryButton
            type="submit"
            variant="contained"
            color="primary"
            style={{ marginTop: "20px" }}
            disabled={loading}
          >
            {id ? "Update Group" : "Create Group"}
          </PrimaryButton>
        </form>
      </ContentBox>
    </>
  );
};

export default GroupForm;
