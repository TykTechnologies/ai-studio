import React, { useState, useEffect, useCallback } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Button,
  Stack,
  Paper,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import PaginationControls from "../common/PaginationControls";
import usePagination from "../../hooks/usePagination";
import { StyledPaper, TitleBox, ContentBox } from "../../styles/sharedStyles";

import TruncatedMessage from "./TruncatedMessage";

const UserMessageLog = () => {
  const [loading, setLoading] = useState(true);
  const [chatInfo, setChatInfo] = useState(null);
  const [llmInfo, setLLMInfo] = useState(null);
  const [llmSettings, setLLMSettings] = useState(null);
  const [messages, setMessages] = useState([]);
  const { sessionId } = useParams();
  const navigate = useNavigate();
  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination(1, 2);

  useEffect(() => {
    console.log(`Pagination updated: page ${page}, pageSize ${pageSize}`);
  }, [page, pageSize]);

  useEffect(() => {
    console.log(`Current page: ${page}, pageSize: ${pageSize}`);
  }, [page, pageSize]);
  const fetchMessages = useCallback(async () => {
    try {
      console.log(
        `Fetching messages with page: ${page}, pageSize: ${pageSize}`,
      );
      const response = await apiClient.get(
        `/chat-history-records/messages/${sessionId}`,
        {
          params: { page, page_size: pageSize },
        },
      );
      console.log("API Response:", response.data);
      setMessages(response.data.data);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      return response.data.data;
    } catch (error) {
      console.error("Error fetching messages", error);
    }
  }, [sessionId, page, pageSize, updatePaginationData]);

  const fetchChatInfo = useCallback(async (chatId) => {
    try {
      const chatResponse = await apiClient.get(`/chats/${chatId}`);
      setChatInfo(chatResponse.data.data);
      return chatResponse.data.data;
    } catch (error) {
      console.error("Error fetching chat info", error);
    }
  }, []);

  const fetchLLMInfo = useCallback(async (llmId) => {
    try {
      const response = await apiClient.get(`/llms/${llmId}`);
      setLLMInfo(response.data.data);
    } catch (error) {
      console.error("Error fetching LLM info", error);
    }
  }, []);

  const fetchLLMSettings = useCallback(async (llmSettingsId) => {
    try {
      const response = await apiClient.get(`/llm-settings/${llmSettingsId}`);
      setLLMSettings(response.data.data);
    } catch (error) {
      console.error("Error fetching LLM settings", error);
    }
  }, []);

  useEffect(() => {
    console.log(`Pagination updated: page ${page}, pageSize ${pageSize}`);
  }, [page, pageSize]);

  useEffect(() => {
    const fetchData = async () => {
      const messagesData = await fetchMessages();
      if (messagesData && messagesData.length > 0) {
        const chatId = messagesData[0].attributes.chat_id;
        const chatData = await fetchChatInfo(chatId);
        if (chatData) {
          await Promise.all([
            fetchLLMInfo(chatData.attributes.llm_id),
            fetchLLMSettings(chatData.attributes.llm_settings_id),
          ]);
        }
      }
      setLoading(false);
    };
    fetchData();
  }, [fetchMessages, fetchChatInfo, fetchLLMInfo, fetchLLMSettings]);

  if (loading || !chatInfo || !llmInfo || !llmSettings) {
    return <CircularProgress />;
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="h5" fontWeight="bold">
          Chat Log
        </Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate(-1)}
          color="primary"
        >
          Back
        </Button>
      </TitleBox>
      <ContentBox>
        <Paper elevation={3} sx={{ p: 2, mb: 3 }}>
          <Typography variant="h6" gutterBottom>
            Chat Information
          </Typography>
          <Typography>
            <strong>Name:</strong> {chatInfo.attributes.name}
          </Typography>
          <Typography>
            <strong>Vendor:</strong> {llmInfo.attributes.vendor}
          </Typography>
          <Typography>
            <strong>Model:</strong> {llmSettings.attributes.model_name}
          </Typography>
        </Paper>
        <TableContainer component={Paper}>
          <Table>
            <TableHead>
              <TableRow>
                <TableCell>
                  <Typography fontWeight="bold">Timestamp</Typography>
                </TableCell>
                <TableCell>
                  <Typography fontWeight="bold">Message</Typography>
                </TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {messages.map((message) => (
                <TableRow key={message.id}>
                  <TableCell style={{ verticalAlign: "top" }}>
                    {new Date(message.attributes.created_at).toLocaleString()}
                  </TableCell>
                  <TableCell style={{ verticalAlign: "top" }}>
                    <TruncatedMessage message={message.attributes.content} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
        <Box mt={2}>
          <PaginationControls
            page={page}
            pageSize={pageSize}
            totalPages={totalPages}
            onPageChange={handlePageChange}
            onPageSizeChange={handlePageSizeChange}
          />
        </Box>
      </ContentBox>
    </>
  );
};

export default UserMessageLog;
