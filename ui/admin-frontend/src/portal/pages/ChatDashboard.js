import React, { useState, useEffect } from "react";
import {
  Typography,
  Container,
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
  Button,
  Paper,
} from "@mui/material";
import { useNavigate } from "react-router-dom";
import ArrowForwardIcon from "@mui/icons-material/ArrowForward";
import ChatIcon from "@mui/icons-material/Chat";
import pubClient from "../../admin/utils/pubClient";

const ChatDashboard = () => {
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
      const [historyResponse, chatRoomsResponse] = await Promise.all([
        pubClient.get(`/common/history?page_size=5&page=${currentPage}`),
        pubClient.get("/common/me"),
      ]);
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

  const handleContinueChat = (chatId, sessionId) => {
    navigate(`/chat/${chatId}?continue_id=${sessionId}`);
  };

  const handleStartNewChat = (chatId) => {
    navigate(`/chat/${chatId}`);
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
    <Container
      maxWidth={false} // Change this from "lg" to false
      sx={{
        px: 3,
        py: 3,
        boxSizing: "border-box",
        width: "100%",
      }}
    >
      <Typography variant="h4" component="h1" gutterBottom sx={{ mb: 4 }}>
        Chat Studio
      </Typography>

      {chatRooms.length > 0 && (
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
                      <Box>
                        <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
                          <ChatIcon
                            sx={{ mr: 1, fontSize: 20, color: "text.secondary" }}
                          />
                          <Typography variant="body1" component="div" noWrap>
                            {chat.attributes.name}
                          </Typography>
                        </Box>
                        {chat.attributes.description && (
                          <Typography
                            variant="body2"
                            color="text.secondary"
                            sx={{
                              display: '-webkit-box',
                              WebkitLineClamp: 2,
                              WebkitBoxOrient: 'vertical',
                              overflow: 'hidden',
                              textOverflow: 'ellipsis'
                            }}
                          >
                            {chat.attributes.description}
                          </Typography>
                        )}
                      </Box>
                    </CardContent>
                    <CardActions sx={{ justifyContent: "flex-end", p: 1 }}>
                      <Button
                        size="small"
                        onClick={() => handleStartNewChat(chat.id)}
                        endIcon={<ArrowForwardIcon />}
                        sx={{
                          color: "black",
                          "&:hover": { backgroundColor: "rgba(0, 0, 0, 0.04)" },
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
                {chatHistory.map((record) => (
                  <TableRow
                    key={record.id}
                    sx={{
                      "&:nth-of-type(odd)": { backgroundColor: "#E0F7F6" },
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

export default ChatDashboard;
