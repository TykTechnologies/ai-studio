import React, { useState, useEffect, useCallback } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Table,
  TableBody,
  TableHead,
  Paper,
  TableRow,
  Box
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import DownloadIcon from "@mui/icons-material/Download";
import PaginationControls from "../common/PaginationControls";
import usePagination from "../../hooks/usePagination";
import {
  SecondaryLinkButton,
  SecondaryOutlineButton,
  TitleBox,
  ContentBox,
  StyledPaper,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
} from "../../styles/sharedStyles";

import TruncatedMessage from "./TruncatedMessage";

const UserMessageLog = () => {
  const [loading, setLoading] = useState(true);
  const [exporting, setExporting] = useState(false);
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

  const handleExport = useCallback(async () => {
    setExporting(true);
    try {
      // Fetch all messages for this session (using a large page size to get all)
      const response = await apiClient.get(
        `/chat-history-records/messages/${sessionId}`,
        { params: { page: 1, page_size: 10000 } }
      );

      const allMessages = response.data.data || [];

      // Build export data structure
      const exportData = {
        session_id: sessionId,
        chat_name: chatInfo?.attributes?.name || "Unknown",
        chat_id: chatInfo?.id || null,
        vendor: llmInfo?.attributes?.vendor || "Unknown",
        model: llmSettings?.attributes?.model_name || "Unknown",
        exported_at: new Date().toISOString(),
        total_messages: allMessages.length,
        messages: allMessages.map(msg => ({
          id: msg.id,
          content: msg.attributes.content,
          created_at: msg.attributes.created_at
        }))
      };

      // Create and download JSON file
      const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `chat-export-${sessionId}.json`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
    } catch (error) {
      console.error("Error exporting chat", error);
    } finally {
      setExporting(false);
    }
  }, [sessionId, chatInfo, llmInfo, llmSettings]);

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
        <Typography variant="headingXLarge" fontWeight="bold">
          Chat Log
        </Typography>
        <Box sx={{ display: "flex", gap: 2 }}>
          <SecondaryOutlineButton
            startIcon={exporting ? <CircularProgress size={16} color="inherit" /> : <DownloadIcon />}
            onClick={handleExport}
            disabled={exporting}
            size="small"
          >
            {exporting ? "Exporting..." : "Export"}
          </SecondaryOutlineButton>
          <SecondaryLinkButton
            startIcon={<ArrowBackIcon />}
            onClick={() => navigate(-1)}
            color="primary"
          >
            Back
          </SecondaryLinkButton>
        </Box>
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
        <StyledPaper>
          <Table>
            <TableHead>
              <TableRow>
                <StyledTableHeaderCell sx={{ verticalAlign: "top" }}>
                  Timestamp
                </StyledTableHeaderCell>
                <StyledTableHeaderCell sx={{ verticalAlign: "top" }}>
                  Message
                </StyledTableHeaderCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {messages.map((message) => (
                <StyledTableRow key={message.id}>
                  <StyledTableCell sx={{ verticalAlign: "top" }}>
                    {new Date(message.attributes.created_at).toLocaleString()}
                  </StyledTableCell>
                  <StyledTableCell sx={{ verticalAlign: "top" }}>
                    <TruncatedMessage message={message.attributes.content} />
                  </StyledTableCell>
                </StyledTableRow>
              ))}
            </TableBody>
          </Table>
          <PaginationControls
            page={page}
            pageSize={pageSize}
            totalPages={totalPages}
            onPageChange={handlePageChange}
            onPageSizeChange={handlePageSizeChange}
          />
        </StyledPaper>
      </ContentBox>
    </>
  );
};

export default UserMessageLog;
