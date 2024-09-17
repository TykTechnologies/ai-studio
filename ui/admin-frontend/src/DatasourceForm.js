// src/DatasourceForm.js
import React, { useState, useEffect } from "react";
import apiClient from "./apiClient";
import { TextField, Button, Box, MenuItem } from "@mui/material";
import { useNavigate, useParams } from "react-router-dom";

const DatasourceForm = () => {
  const [name, setName] = useState("");
  const [dbSourceType, setDbSourceType] = useState("");
  const [dbName, setDbName] = useState("");
  const navigate = useNavigate();
  const { id } = useParams(); // For editing existing datasource
  const [loading, setLoading] = useState(false);

  const dbSourceTypes = [
    "MySQL",
    "PostgreSQL",
    "MongoDB",
    // Add other types as needed
  ];

  useEffect(() => {
    if (id) {
      // Fetch datasource details to edit
      const fetchDatasource = async () => {
        setLoading(true);
        try {
          const response = await apiClient.get(`/datasources/${id}`);
          setName(response.data.attributes.name);
          setDbSourceType(response.data.attributes.db_source_type);
          setDbName(response.data.attributes.db_name);
          setLoading(false);
        } catch (error) {
          console.error("Error fetching datasource", error);
          setLoading(false);
        }
      };
      fetchDatasource();
    }
  }, [id]);

  const handleSubmit = async (e) => {
    e.preventDefault();

    const datasourceData = {
      data: {
        type: "Datasource",
        attributes: {
          name,
          db_source_type: dbSourceType,
          db_name: dbName,
          // Include other attributes as needed
        },
      },
    };

    try {
      if (id) {
        // Update existing datasource
        await apiClient.patch(`/datasources/${id}`, datasourceData);
      } else {
        // Create new datasource
        await apiClient.post("/datasources", datasourceData);
      }
      navigate("/datasources");
    } catch (error) {
      console.error("Error saving datasource", error);
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
        label="Datasource Name"
        value={name}
        onChange={(e) => setName(e.target.value)}
        required
      />
      <TextField
        fullWidth
        select
        margin="normal"
        label="DB Source Type"
        value={dbSourceType}
        onChange={(e) => setDbSourceType(e.target.value)}
        required
      >
        {dbSourceTypes.map((type) => (
          <MenuItem key={type} value={type}>
            {type}
          </MenuItem>
        ))}
      </TextField>
      <TextField
        fullWidth
        margin="normal"
        label="Database Name"
        value={dbName}
        onChange={(e) => setDbName(e.target.value)}
      />
      {/* Add more fields as needed */}
      <Button variant="contained" color="primary" type="submit" sx={{ mt: 2 }}>
        {id ? "Update Datasource" : "Add Datasource"}
      </Button>
    </Box>
  );
};

export default DatasourceForm;
