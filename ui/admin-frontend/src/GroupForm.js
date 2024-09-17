// src/GroupForm.js
import React, { useState } from "react";
import apiClient from "./apiClient";
import { TextField, Button, Box } from "@mui/material";
import { useNavigate, useParams } from "react-router-dom";
import { useEffect } from "react";

const GroupForm = () => {
  const [name, setName] = useState("");
  const navigate = useNavigate();
  const { id } = useParams(); // For editing existing group
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (id) {
      // Fetch group details to edit
      const fetchGroup = async () => {
        setLoading(true);
        try {
          const response = await apiClient.get(`/groups/${id}`);
          setName(response.data.attributes.name);
          setLoading(false);
        } catch (error) {
          console.error("Error fetching group", error);
          setLoading(false);
        }
      };
      fetchGroup();
    }
  }, [id]);

  const handleSubmit = async (e) => {
    e.preventDefault();

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
        // Update existing group
        await apiClient.patch(`/groups/${id}`, groupData);
      } else {
        // Create new group
        await apiClient.post("/groups", groupData);
      }
      navigate("/groups");
    } catch (error) {
      console.error("Error saving group", error);
    }
  };

  if (loading) {
    return <p>Loading...</p>;
  }

  return (
    <Box component="form" onSubmit={handleSubmit} sx={{ maxWidth: 500 }}>
      <TextField
        fullWidth
        margin="normal"
        label="Group Name"
        value={name}
        onChange={(e) => setName(e.target.value)}
        required
      />
      <Button variant="contained" color="primary" type="submit" sx={{ mt: 2 }}>
        {id ? "Update Group" : "Add Group"}
      </Button>
    </Box>
  );
};

export default GroupForm;
