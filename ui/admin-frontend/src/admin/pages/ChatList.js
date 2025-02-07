import React, { useState, useEffect, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
  IconButton,
  CircularProgress,
  Alert,
  Menu,
  MenuItem,
  Snackbar,
  Box,
} from "@mui/material";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import AddIcon from "@mui/icons-material/Add";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  StyledButton,
} from "../styles/sharedStyles";
import InfoTooltip from "../components/common/InfoTooltip";
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";

const ChatList = () => {
  const navigate = useNavigate();
  const [chats, setChats] = useState([]);
  const [llms, setLLMs] = useState({});
  const [llmSettings, setLLMSettings] = useState({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedChat, setSelectedChat] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [sortConfig, setSortConfig] = useState({ key: null, direction: "asc" });

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchChats = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/chats", {
        params: {
          page,
          page_size: pageSize,
          sort_by: sortConfig.key,
          sort_direction: sortConfig.direction,
        },
      });
      setChats(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching chats", error);
      setError("Failed to load chats");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, sortConfig, updatePaginationData]);

  useEffect(() => {
    fetchChats();
  }, [fetchChats]);

  useEffect(() => {
    fetchLLMs();
    fetchLLMSettings();
  }, []);

  const fetchLLMs = async () => {
    try {
      const response = await apiClient.get("/llms");
      const llmMap = {};
      response.data.data.forEach((llm) => {
        llmMap[llm.id] = llm.attributes.name;
      });
      setLLMs(llmMap);
    } catch (error) {
      console.error("Error fetching LLMs", error);
    }
  };

  const fetchLLMSettings = async () => {
    try {
      const response = await apiClient.get("/llm-settings");
      const settingsMap = {};
      response.data.data.forEach((setting) => {
        settingsMap[setting.id] = setting.attributes.model_name;
      });
      setLLMSettings(settingsMap);
    } catch (error) {
      console.error("Error fetching LLM Settings", error);
    }
  };

  const handleMenuOpen = (event, chat) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedChat(chat);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/chats/${id}`);
      setSnackbar({
        open: true,
        message: "Chat deleted successfully",
        severity: "success",
      });
      fetchChats();
    } catch (error) {
      console.error("Error deleting chat", error);
      setSnackbar({
        open: true,
        message: "Failed to delete chat",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleChatClick = (chat) => {
    navigate(`/admin/chats/${chat.id}`);
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  const handleSort = (key) => {
    let direction = "asc";
    if (sortConfig.key === key && sortConfig.direction === "asc") {
      direction = "desc";
    }
    setSortConfig({ key, direction });
  };

  const handleAddChat = () => {
    navigate("/admin/chats/new");
  };

  if (loading && chats.length === 0) {
    return <CircularProgress />;
  }

  if (error && chats.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <>
      <>
        <TitleBox top="64px">
          <Box display="flex" alignItems="center">
            <InfoTooltip title="Chat rooms are portal areas where your users can have one-on-one chats with specific LLMs, and the tools and data sources that are granted to their group. They can be associated with one or more groups." />
            <Typography variant="h5">Chat Rooms</Typography>
          </Box>

          <StyledButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddChat}
          >
            Add Chat Room
          </StyledButton>
        </TitleBox>
        <ContentBox>
          {chats.length === 0 ? (
            <EmptyStateWidget
              title="No chat rooms created yet"
              description="Chat rooms are portal areas where your users can have one-on-one chats with specific LLMs, and the tools and data sources that are granted to their group. They can be associated with one or more groups. Create a new chat room by clicking the button below."
              buttonText="Add Chat Room"
              buttonIcon={<AddIcon />}
              onButtonClick={handleAddChat}
            />
          ) : (
            <StyledPaper>
              <Table>
                <TableHead>
                  <TableRow>
                    <StyledTableHeaderCell onClick={() => handleSort("name")}>
                      Name
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell onClick={() => handleSort("description")}>
                      Description
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell onClick={() => handleSort("llm_id")}>
                      LLM
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell
                      onClick={() => handleSort("llm_settings_id")}
                    >
                      LLM Settings
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell>Groups</StyledTableHeaderCell>
                    <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {chats.map((chat) => (
                    <StyledTableRow
                      key={chat.id}
                      onClick={() => handleChatClick(chat)}
                      sx={{ cursor: "pointer" }}
                    >
                      <StyledTableCell>{chat.attributes.name}</StyledTableCell>
                      <StyledTableCell>
                        {chat.attributes.description || "-"}
                      </StyledTableCell>
                      <StyledTableCell>
                        {llms[chat.attributes.llm_id] || "Unknown LLM"}
                      </StyledTableCell>
                      <StyledTableCell>
                        {llmSettings[chat.attributes.llm_settings_id] ||
                          "Unknown Settings"}
                      </StyledTableCell>
                      <StyledTableCell>
                        {chat.attributes.groups
                          .map((group) => group.attributes.name)
                          .join(", ")}
                      </StyledTableCell>
                      <StyledTableCell align="right">
                        <IconButton
                          onClick={(event) => handleMenuOpen(event, chat)}
                        >
                          <MoreVertIcon />
                        </IconButton>
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
          )}
        </ContentBox>
      </>

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem
          onClick={() => navigate(`/admin/chats/edit/${selectedChat?.id}`)}
        >
          Edit Chat Room
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedChat?.id)}>
          Delete Chat Room
        </MenuItem>
      </Menu>

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

export default ChatList;
