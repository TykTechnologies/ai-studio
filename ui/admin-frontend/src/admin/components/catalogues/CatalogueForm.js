import React, { useState, useEffect } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  TextField,
  Button,
  Box,
  Typography,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  IconButton,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Snackbar,
  Alert,
  CircularProgress,
} from "@mui/material";
import DeleteIcon from "@mui/icons-material/Delete";
import AddIcon from "@mui/icons-material/Add";
import { commitPendingSelection } from "../../utils/pendingSelection";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  PrimaryButton,
} from "../../styles/sharedStyles";

const CatalogueForm = () => {
  const [catalogue, setCatalogue] = useState({ name: "" });
  const [llms, setLLMs] = useState([]);
  const [availableLLMs, setAvailableLLMs] = useState([]);
  const [selectedLLM, setSelectedLLM] = useState("");
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();
  const { id } = useParams();

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [catalogueResponse, llmsResponse] = await Promise.all([
          id ? apiClient.get(`/catalogues/${id}`) : Promise.resolve(null),
          apiClient.get("/llms"),
        ]);

        if (catalogueResponse) {
          setCatalogue(catalogueResponse.data.data.attributes);
          const catalogueLLMsResponse = await apiClient.get(
            `/catalogues/${id}/llms`,
          );
          setLLMs(catalogueLLMsResponse.data.data);
        }

        setAvailableLLMs(
          llmsResponse.data.data.filter((llm) => llm.attributes.active),
        );
      } catch (error) {
        console.error("Error fetching data", error);
        setSnackbar({
          open: true,
          message: "Error fetching data",
          severity: "error",
        });
      }
      setLoading(false);
    };

    fetchData();
  }, [id]);

  const handleChange = (e) => {
    setCatalogue({ ...catalogue, [e.target.name]: e.target.value });
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setLoading(true);

    // The + button is an accelerator, not the commit. A selection the user made
    // and left in the picker is still something they asked for, so fold it in
    // rather than silently saving a catalog without it.
    const desiredLLMs = commitPendingSelection(llms, selectedLLM, availableLLMs);
    if (desiredLLMs !== llms) {
      setLLMs(desiredLLMs);
      setSelectedLLM("");
    }

    const catalogueData = {
      data: {
        type: "Catalogue",
        attributes: catalogue,
      },
    };

    try {
      let catalogueId;
      if (id) {
        await apiClient.patch(`/catalogues/${id}`, catalogueData);
        catalogueId = id;
      } else {
        const response = await apiClient.post("/catalogues", catalogueData);
        catalogueId = response.data.data.id;
      }

      setSnackbar({
        open: true,
        message: `Catalog ${id ? "updated" : "created"} successfully`,
        severity: "success",
      });

      // Now handle LLM additions/removals
      await updateCatalogueLLMs(catalogueId, desiredLLMs);

      setTimeout(() => navigate("/admin/catalogs/llms"), 2000);
    } catch (error) {
      console.error("Error saving catalog", error);
      setSnackbar({
        open: true,
        message: "Error saving catalog",
        severity: "error",
      });
      setLoading(false);
    }
  };

  // Takes the desired LLM list explicitly rather than reading `llms` from the
  // closure, so the caller can include a selection the user picked but never
  // added with the + button.
  const updateCatalogueLLMs = async (catalogueId, desiredLLMs) => {
    try {
      const currentLLMs = id
        ? (await apiClient.get(`/catalogues/${catalogueId}/llms`)).data.data
        : [];

      // Remove LLMs that are no longer in the list
      for (let llm of currentLLMs) {
        if (!desiredLLMs.find((l) => l.id === llm.id)) {
          await apiClient.delete(`/catalogues/${catalogueId}/llms/${llm.id}`);
        }
      }

      // Add new LLMs
      for (let llm of desiredLLMs) {
        if (!currentLLMs.find((l) => l.id === llm.id)) {
          await apiClient.post(`/catalogues/${catalogueId}/llms`, {
            data: { id: llm.id, type: "LLM" },
          });
        }
      }

      setSnackbar({
        open: true,
        message: "LLMs updated successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error updating catalog LLMs", error);
      setSnackbar({
        open: true,
        message: "Error updating LLMs",
        severity: "error",
      });
    }
  };

  const handleAddLLM = () => {
    const next = commitPendingSelection(llms, selectedLLM, availableLLMs);
    if (next !== llms) {
      setLLMs(next);
      setSelectedLLM("");
    }
  };

  const handleRemoveLLM = (llmId) => {
    setLLMs(llms.filter((llm) => llm.id !== llmId));
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  if (loading) {
    return <CircularProgress />;
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">
          {id ? "Edit LLM catalog" : "Add LLM catalog"}
        </Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          component={Link}
          to="/admin/catalogs/llms"
          color="inherit"
        >
          Back to catalogs
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Catalogs are collections of LLM providers that you can assign to specific teams to manage access easily.</Typography>  
      </Box>
      <ContentBox>
        <Box component="form" onSubmit={handleSubmit}>
          <TextField
            fullWidth
            margin="normal"
            label="Catalog Name"
            name="name"
            value={catalogue.name}
            onChange={handleChange}
            required
          />

          <Typography variant="h6" sx={{ mt: 2 }}>
            LLMs in this Catalog:
          </Typography>
          <List>
            {llms.map((llm) => (
              <ListItem key={llm.id}>
                <ListItemText primary={llm.attributes.name} />
                <ListItemSecondaryAction>
                  <IconButton
                    edge="end"
                    aria-label="delete"
                    onClick={() => handleRemoveLLM(llm.id)}
                  >
                    <DeleteIcon />
                  </IconButton>
                </ListItemSecondaryAction>
              </ListItem>
            ))}
          </List>

          <Box sx={{ display: "flex", alignItems: "center", mt: 2 }}>
            <FormControl fullWidth sx={{ mr: 1 }}>
              <InputLabel id="catalogueform-add-llm-label">Add LLM</InputLabel>
              <Select
              labelId="catalogueform-add-llm-label"
              id="catalogueform-add-llm"
                value={selectedLLM}
                onChange={(e) => setSelectedLLM(e.target.value)}
                label="Add LLM"
              >
                {availableLLMs
                  .filter((llm) => !llms.find((l) => l.id === llm.id))
                  .map((llm) => (
                    <MenuItem key={llm.id} value={llm.id}>
                      {llm.attributes.name}
                    </MenuItem>
                  ))}
              </Select>
            </FormControl>
            <Button
              variant="contained"
              color="secondary"
              onClick={handleAddLLM}
              disabled={!selectedLLM}
              aria-label="Add LLM provider to catalog"
            >
              <AddIcon />
            </Button>
          </Box>

          <Box mt={3}>
            <PrimaryButton type="submit" variant="contained" color="primary">
              {id ? "Update catalog" : "Create catalog"}
            </PrimaryButton>
          </Box>
        </Box>
      </ContentBox>

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={handleCloseSnackbar}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={handleCloseSnackbar}
          severity={snackbar.severity}
          sx={{ width: "100%" }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </>
  );
};

export default CatalogueForm;
