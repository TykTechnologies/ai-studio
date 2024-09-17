// src/LLMForm.js
import React, { useState, useEffect } from "react";
import apiClient from "../../utils/apiClient";
import { TextField, Button, Box } from "@mui/material";
import { useNavigate, useParams } from "react-router-dom";

const LLMForm = () => {
  const [name, setName] = useState("");
  const [vendor, setVendor] = useState("");
  const [apiEndpoint, setApiEndpoint] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [active, setActive] = useState(true);
  const navigate = useNavigate();
  const { id } = useParams(); // For editing existing LLM
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (id) {
      // Fetch LLM details to edit
      const fetchLLM = async () => {
        setLoading(true);
        try {
          const response = await apiClient.get(`/llms/${id}`);
          const attrs = response.data.attributes;
          setName(attrs.name);
          setVendor(attrs.vendor);
          setApiEndpoint(attrs.api_endpoint);
          setApiKey(attrs.api_key);
          setActive(attrs.active);
          setLoading(false);
        } catch (error) {
          console.error("Error fetching LLM", error);
          setLoading(false);
        }
      };
      fetchLLM();
    }
  }, [id]);

  const handleSubmit = async (e) => {
    e.preventDefault();

    const llmData = {
      data: {
        type: "LLM",
        attributes: {
          name,
          vendor,
          api_endpoint: apiEndpoint,
          api_key: apiKey,
          active,
          // Include other attributes as needed
        },
      },
    };

    try {
      if (id) {
        // Update existing LLM
        await apiClient.patch(`/llms/${id}`, llmData);
      } else {
        // Create new LLM
        await apiClient.post("/llms", llmData);
      }
      navigate("/llms");
    } catch (error) {
      console.error("Error saving LLM", error);
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
        label="LLM Name"
        value={name}
        onChange={(e) => setName(e.target.value)}
        required
      />
      <TextField
        fullWidth
        margin="normal"
        label="Vendor"
        value={vendor}
        onChange={(e) => setVendor(e.target.value)}
        required
      />
      <TextField
        fullWidth
        margin="normal"
        label="API Endpoint"
        value={apiEndpoint}
        onChange={(e) => setApiEndpoint(e.target.value)}
      />
      <TextField
        fullWidth
        margin="normal"
        label="API Key"
        value={apiKey}
        onChange={(e) => setApiKey(e.target.value)}
        type="password"
      />
      {/* Add more fields as needed */}
      <Button variant="contained" color="primary" type="submit" sx={{ mt: 2 }}>
        {id ? "Update LLM" : "Add LLM"}
      </Button>
    </Box>
  );
};

export default LLMForm;
