import React, { useState, useEffect } from "react";
import {
  Typography,
  Container,
  Paper,
  Button,
  Grid,
  Card,
  CardContent,
  CardActions,
  CircularProgress,
  Box,
  Pagination,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  IconButton,
} from "@mui/material";
import { useNavigate } from "react-router-dom";
import AddIcon from "@mui/icons-material/Add";
import ArrowForwardIcon from "@mui/icons-material/ArrowForward";
import ChatIcon from "@mui/icons-material/Chat";
import DeleteIcon from "@mui/icons-material/Delete";
import pubClient from "../../admin/utils/pubClient";
import useSystemFeatures from "../../admin/hooks/useSystemFeatures";

const PortalDashboard = () => {
  const { features, loading: featuresLoading } = useSystemFeatures();
  const [apps, setApps] = useState([]);
  const [chatHistory, setChatHistory] = useState([]);
  const [chatRooms, setChatRooms] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [currentPage, setCurrentPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [showChat, setShowChat] = useState(true);
  const [showPortal, setShowPortal] = useState(true);
  const navigate = useNavigate();

  useEffect(() => {
    fetchData();
  }, [currentPage]);

  const fetchData = async () => {
    try {
      setLoading(true);
      const [appsResponse, historyResponse, chatRoomsResponse] =
        await Promise.all([
          pubClient.get("/common/apps"),
          pubClient.get(`/common/history?page_size=5&page=${currentPage}`),
          pubClient.get("/common/me"),
        ]);
      setApps(appsResponse.data.data);
      setChatHistory(historyResponse.data.data);
      setChatRooms(chatRoomsResponse.data.attributes.entitlements.chats || []);
      setShowChat(
        chatRoomsResponse.data.attributes.ui_options?.show_chat ?? true,
      );
      setShowPortal(
        chatRoomsResponse.data.attributes.ui_options?.show_portal ?? true,
      );
      setTotalPages(
        parseInt(historyResponse.headers["x-total-pages"], 10) || 1,
      );
      setLoading(false);
    } catch (err) {
      console.error("Error fetching data:", err);
      setError("Failed to fetch data. Please try again later.");
      setLoading(false);
    }
  };

  const handleCreateApp = () => {
    navigate("/portal/app/new");
  };

  const handleContinueChat = (chatId, sessionId) => {
    navigate(`/portal/chat/${chatId}?continue_id=${sessionId}`);
  };

  const handleStartNewChat = (chatId) => {
    navigate(`/portal/chat/${chatId}`);
  };

  const handlePageChange = (event, value) => {
    setCurrentPage(value);
  };

  const handleDeleteChat = async (chatId) => {
    try {
      // Implementation needed - example:
      // await pubClient.delete(`/common/history/${chatId}`);
      // await fetchData(); // Refresh the data
      console.log("Delete chat:", chatId);
    } catch (err) {
      console.error("Error deleting chat:", err);
      // Handle error appropriately
    }
  };

  if (loading || featuresLoading) {
    return (
      <Container sx={{ display: "flex", justifyContent: "center", mt: 4 }}>
        <CircularProgress />
      </Container>
    );
  }

  if (error) {
    return (
      <Container>
        <Typography color="error" sx={{ textAlign: "center", mt: 4 }}>
          {error}
        </Typography>
      </Container>
    );
  }

  const showPortalFeatures =
    features.feature_portal || features.feature_gateway;

  return (
    <Container maxWidth="lg">
      <Typography variant="h4" component="h1" gutterBottom sx={{ mb: 4 }}>
        Welcome to the AI Portal
      </Typography>

      {showPortalFeatures && !showChat && showPortal && (
        <Paper sx={{ p: 4, textAlign: "center", mb: 4 }}>
          <Typography variant="h6" gutterBottom>
            Create and Manage AI Applications
          </Typography>
          <Typography variant="body1" paragraph>
            Build custom AI applications with our powerful tools and services.
            Apps provide access to LLMs and Data sources via the AI Gateway for
            your code.
          </Typography>
          <Button
            variant="contained"
            color="primary"
            startIcon={<AddIcon />}
            onClick={handleCreateApp}
          >
            Create a new App
          </Button>
        </Paper>
      )}

      {features.feature_chat && showChat && chatRooms.length > 0 && (
        <Box sx={{ mt: 4, mb: 4 }}>
          <Typography variant="h5" gutterBottom sx={{ mb: 2, color: "black" }}>
            Start a new chat session
          </Typography>
          <Grid container spacing={2}>
            {chatRooms
              .sort((a, b) =>
                a.attributes.name.localeCompare(b.attributes.name),
              )
              .map((chat) => (
                <Grid item xs={6} sm={4} md={3} key={chat.id}>
                  <Card
                    sx={{
                      height: "100%",
                      display: "flex",
                      flexDirection: "column",
                      justifyContent: "space-between",
                    }}
                  >
                    <CardContent>
                      <Box sx={{ display: "flex", alignItems: "center" }}>
                        <ChatIcon
                          sx={{ mr: 1, fontSize: 20, color: "text.secondary" }}
                        />
                        <Typography variant="body1" component="div" noWrap>
                          {chat.attributes.name}
                        </Typography>
                      </Box>
                    </CardContent>
                    <CardActions sx={{ justifyContent: "flex-end", p: 1 }}>
                      <Button
                        size="small"
                        onClick={() => handleStartNewChat(chat.id)}
                        endIcon={<ArrowForwardIcon />}
                        sx={{
                          color: "black",
                          "&:hover": {
                            backgroundColor: "rgba(0, 0, 0, 0.04)",
                          },
                        }}
                      >
                        Start Chat
                      </Button>
                    </CardActions>
                  </Card>
                </Grid>
              ))}
          </Grid>
        </Box>
      )}

      {features.feature_chat && showChat && chatHistory.length > 0 && (
        <Box sx={{ mt: 4, mb: 4 }}>
          <Typography variant="h5" gutterBottom sx={{ mb: 2, color: "black" }}>
            Continue where you left off
          </Typography>
          <TableContainer component={Paper}>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell
                    sx={{
                      fontWeight: "bold",
                      backgroundColor: "#f5f5f5",
                      fontSize: "1rem",
                      color: "rgba(0, 0, 0, 0.87)",
                    }}
                  >
                    Conversation
                  </TableCell>
                  <TableCell
                    sx={{
                      fontWeight: "bold",
                      backgroundColor: "#f5f5f5",
                      fontSize: "1rem",
                      color: "rgba(0, 0, 0, 0.87)",
                    }}
                    align="right"
                  >
                    Actions
                  </TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {chatHistory.map((record, index) => (
                  <TableRow
                    key={record.id}
                    sx={{
                      "&:nth-of-type(odd)": {
                        backgroundColor: "#E0F7F6",
                      },
                      "&:hover": { backgroundColor: "rgba(0, 0, 0, 0.08)" },
                    }}
                  >
                    <TableCell component="th" scope="row">
                      {record.attributes.name}
                    </TableCell>
                    <TableCell align="right">
                      <Box
                        sx={{
                          display: "flex",
                          justifyContent: "flex-end",
                          gap: 1,
                        }}
                      >
                        <Button
                          size="small"
                          onClick={() =>
                            handleContinueChat(
                              record.attributes.chat_id,
                              record.attributes.session_id,
                            )
                          }
                          endIcon={<ArrowForwardIcon />}
                          sx={{
                            color: "black",
                            "&:hover": {
                              backgroundColor: "rgba(0, 0, 0, 0.04)",
                            },
                          }}
                        >
                          Continue
                        </Button>
                      </Box>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
          <Box sx={{ display: "flex", justifyContent: "center", mt: 2 }}>
            <Pagination
              count={totalPages}
              page={currentPage}
              onChange={handlePageChange}
              color="primary"
            />
          </Box>
        </Box>
      )}
    </Container>
  );
};

export default PortalDashboard;
