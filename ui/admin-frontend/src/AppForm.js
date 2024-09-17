// src/AppForm.js
import React, { useState } from "react";
import apiClient from "./apiClient";
import { TextField, Button, Box } from "@mui/material";
import { useNavigate } from "react-router-dom";

const AppForm = ({ app }) => {
  const [name, setName] = useState(app ? app.attributes.name : "");
  const [description, setDescription] = useState(
    app ? app.attributes.description : "",
  );
  const navigate = useNavigate();

  const handleSubmit = async (e) => {
    e.preventDefault();

    const appData = {
      data: {
        type: "App",
        attributes: {
          name,
          description,
        },
      },
    };

    try {
      if (app) {
        // Update existing app
        await apiClient.patch(`/apps/${app.id}`, appData);
      } else {
        // Create new app
        await apiClient.post("/apps", appData);
      }
      navigate("/apps");
    } catch (error) {
      console.error("Error saving app", error);
    }
  };

  return (
    <Box component="form" onSubmit={handleSubmit} sx={{ maxWidth: 500 }}>
      <TextField
        fullWidth
        margin="normal"
        label="Name"
        value={name}
        onChange={(e) => setName(e.target.value)}
        required
      />
      <TextField
        fullWidth
        margin="normal"
        label="Description"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        multiline
        rows={4}
      />
      <Button variant="contained" color="primary" type="submit" sx={{ mt: 2 }}>
        {app ? "Update App" : "Add App"}
      </Button>
    </Box>
  );
};

export default AppForm;
