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
} from "@mui/material";
import { useNavigate } from "react-router-dom";
import AddIcon from "@mui/icons-material/Add";
import ArrowForwardIcon from "@mui/icons-material/ArrowForward";
import ChatIcon from "@mui/icons-material/Chat";
import pubClient from "../../admin/utils/pubClient";

const PortalDashboard = () => {
  const [apps, setApps] = useState([]);
  const [chatHistory, setChatHistory] = useState([]);
  const [chatRooms, setChatRooms] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [currentPage, setCurrentPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
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

  if (loading) {
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

  return (
    <Container maxWidth="lg">
      <Typography variant="h4" component="h1" gutterBottom sx={{ mb: 4 }}>
        Welcome to our AI Portal
      </Typography>

      {apps.length === 0 && (
        <Paper sx={{ p: 4, textAlign: "center", mb: 4 }}>
          <Typography variant="h6" gutterBottom>
            Apps provide access to LLMs and Data sources via the AI Gateway
          </Typography>
          <Typography variant="body1" paragraph>
            Create your first app to get started.
          </Typography>
          <Button
            variant="contained"
            color="primary"
            startIcon={<AddIcon />}
            onClick={handleCreateApp}
          >
            Create your first App
          </Button>
        </Paper>
      )}

      {chatRooms.length > 0 && (
        <Box sx={{ mt: 4, mb: 4 }}>
          <Typography variant="h5" gutterBottom sx={{ mb: 2, color: "black" }}>
            Jump into a new chat...
          </Typography>
          <Grid container spacing={2}>
            {chatRooms.map((chat) => (
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

      {chatHistory.length > 0 && (
        <Box sx={{ mt: 4, mb: 4 }}>
          <Typography variant="h5" gutterBottom sx={{ mb: 2, color: "black" }}>
            Continue where you left off
          </Typography>
          <Grid container spacing={3}>
            {chatHistory.map((record) => (
              <Grid item xs={12} sm={6} md={4} key={record.id}>
                <Card>
                  <CardContent>
                    <Typography variant="h7" component="div">
                      {record.attributes.name}
                    </Typography>
                  </CardContent>
                  <CardActions sx={{ justifyContent: "flex-end", p: 1 }}>
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
                  </CardActions>
                </Card>
              </Grid>
            ))}
          </Grid>
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
