import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Grid,
  Button,
  Divider,
  Tooltip,
  IconButton,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  StyledButton,
} from "../../styles/sharedStyles";

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const TooltipLabel = ({ label, tooltip }) => (
  <Box display="flex" alignItems="center">
    <FieldLabel>{label}</FieldLabel>
    <Tooltip title={tooltip} arrow placement="top-start">
      <IconButton size="small" sx={{ ml: 0.5 }}>
        <HelpOutlineIcon fontSize="small" />
      </IconButton>
    </Tooltip>
  </Box>
);

const LLMSettingsDetails = () => {
  const [setting, setSetting] = useState(null);
  const [loading, setLoading] = useState(true);
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    fetchSettingDetails();
  }, [id]);

  const fetchSettingDetails = async () => {
    try {
      const response = await apiClient.get(`/llm-settings/${id}`);
      setSetting(response.data.data);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching LLM Setting details", error);
      setLoading(false);
    }
  };

  if (loading) return <CircularProgress />;
  if (!setting) return <Typography>LLM Setting not found</Typography>;

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">LLM Setting Details</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/llm-settings")}
          color="white"
        >
          Back to LLM Settings
        </Button>
      </TitleBox>
      <ContentBox>
        <SectionTitle>Basic Information</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={4}>
            <TooltipLabel
              label="Model Name:"
              tooltip="The name of the language model (e.g., 'gpt-3.5-turbo', 'text-davinci-003')"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.model_name}</FieldValue>
          </Grid>
          <Grid item xs={4}>
            <TooltipLabel
              label="Temperature:"
              tooltip="Controls randomness: 0 is deterministic, 1 is very random. Range: 0 to 1"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.temperature}</FieldValue>
          </Grid>
          <Grid item xs={4}>
            <TooltipLabel
              label="Max Tokens:"
              tooltip="The maximum number of tokens to generate in the response"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.max_tokens}</FieldValue>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Advanced Settings</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={4}>
            <TooltipLabel
              label="Top P:"
              tooltip="Controls diversity via nucleus sampling: 0.5 means half of all likelihood-weighted options are considered. Range: 0 to 1"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.top_p}</FieldValue>
          </Grid>
          <Grid item xs={4}>
            <TooltipLabel
              label="Top K:"
              tooltip="Controls diversity by limiting to k most likely tokens. 0 means no limit"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.top_k}</FieldValue>
          </Grid>
          <Grid item xs={4}>
            <TooltipLabel
              label="Min Length:"
              tooltip="The minimum number of tokens to generate in the response"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.min_length}</FieldValue>
          </Grid>
          <Grid item xs={4}>
            <TooltipLabel
              label="Max Length:"
              tooltip="The maximum number of overall tokens"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.max_length}</FieldValue>
          </Grid>
          <Grid item xs={4}>
            <TooltipLabel
              label="Repetition Penalty:"
              tooltip="Penalizes repetition: 1.0 means no penalty, >1.0 discourages repetition. Typically between 1.0 and 1.5"
            />
          </Grid>
          <Grid item xs={8}>
            <FieldValue>{setting.attributes.repetition_penalty}</FieldValue>
          </Grid>
        </Grid>

        <Box mt={4} display="flex" justifyContent="flex-end">
          <StyledButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/llm-settings/edit/${id}`)}
          >
            Edit LLM Setting
          </StyledButton>
        </Box>
      </ContentBox>
    </StyledPaper>
  );
};

export default LLMSettingsDetails;
