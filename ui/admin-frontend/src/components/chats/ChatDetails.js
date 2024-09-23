import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Grid,
  Button,
  Chip,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  StyledButton,
} from "../../styles/sharedStyles";

const ChatDetails = () => {
  const [chat, setChat] = useState(null);
  const [llm, setLLM] = useState(null);
  const [llmSettings, setLLMSettings] = useState(null);
  const [loading, setLoading] = useState(true);
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    fetchChatDetails();
  }, [id]);

  const fetchChatDetails = async () => {
    try {
      const chatResponse = await apiClient.get(`/chats/${id}`);
      setChat(chatResponse.data.data);

      const llmResponse = await apiClient.get(
        `/llms/${chatResponse.data.data.attributes.llm_id}`,
      );
      setLLM(llmResponse.data.data);

      const llmSettingsResponse = await apiClient.get(
        `/llm-settings/${chatResponse.data.data.attributes.llm_settings_id}`,
      );
      setLLMSettings(llmSettingsResponse.data.data);

      setLoading(false);
    } catch (error) {
      console.error("Error fetching chat details", error);
      setLoading(false);
    }
  };

  if (loading) return <CircularProgress />;
  if (!chat) return <Typography>Chat not found</Typography>;

  return (
    <StyledPaper>
      <TitleBox>
        <Typography variant="h5">Chat Room Details</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/chats")}
          color="white"
        >
          Back to Chat Rooms
        </Button>
      </TitleBox>
      <ContentBox>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>Name:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{chat.attributes.name}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>LLM Settings:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              {llmSettings ? llmSettings.attributes.model_name : "Loading..."}
            </FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>LLM:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{llm ? llm.attributes.name : "Loading..."}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Groups:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
              {chat.attributes.groups.map((group) => (
                <Chip key={group.id} label={group.attributes.name} />
              ))}
            </Box>
          </Grid>
        </Grid>

        <Box mt={4}>
          <StyledButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/chats/edit/${id}`)}
          >
            Edit Chat Room
          </StyledButton>
        </Box>
      </ContentBox>
    </StyledPaper>
  );
};

export default ChatDetails;
