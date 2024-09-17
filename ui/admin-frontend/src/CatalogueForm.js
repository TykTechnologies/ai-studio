// src/CatalogueForm.js
import React, { useState, useEffect } from "react";
import apiClient from "./apiClient";
import { TextField, Button, Box } from "@mui/material";
import { useNavigate, useParams } from "react-router-dom";

const CatalogueForm = () => {
  const [name, setName] = useState("");
  const navigate = useNavigate();
  const { id } = useParams(); // For editing existing catalogue
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (id) {
      // Fetch catalogue details to edit
      const fetchCatalogue = async () => {
        setLoading(true);
        try {
          const response = await apiClient.get(`/catalogues/${id}`);
          setName(response.data.attributes.name);
          setLoading(false);
        } catch (error) {
          console.error("Error fetching catalogue", error);
          setLoading(false);
        }
      };
      fetchCatalogue();
    }
  }, [id]);

  const handleSubmit = async (e) => {
    e.preventDefault();

    const catalogueData = {
      data: {
        type: "Catalogue",
        attributes: {
          name,
        },
      },
    };

    try {
      if (id) {
        // Update existing catalogue
        await apiClient.patch(`/catalogues/${id}`, catalogueData);
      } else {
        // Create new catalogue
        await apiClient.post("/catalogues", catalogueData);
      }
      navigate("/catalogues");
    } catch (error) {
      console.error("Error saving catalogue", error);
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
        label="Catalogue Name"
        value={name}
        onChange={(e) => setName(e.target.value)}
        required
      />
      <Button variant="contained" color="primary" type="submit" sx={{ mt: 2 }}>
        {id ? "Update Catalogue" : "Add Catalogue"}
      </Button>
    </Box>
  );
};

export default CatalogueForm;
