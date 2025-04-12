import React, { useState, useEffect } from "react";
import {
  Typography,
  Grid,
  Card,
  CardContent,
  CardActions,
  CircularProgress,
  Box,
  Pagination,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  Button
} from "@mui/material";
import { useNavigate, useLocation } from "react-router-dom";
import ChatIcon from "@mui/icons-material/Chat";
import pubClient from "../../admin/utils/pubClient";
import {
  StyledTableCell,
  StyledTableRow,
  StyledPaper,
  TitleBox,
  ContentBox,
} from "../../admin/styles/sharedStyles";

const ChatDashboard = () => {
  const [chatHistory, setChatHistory] = useState([]);
  const [chatRooms, setChatRooms] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [currentPage, setCurrentPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [user, setUser] = useState(null);
  const navigate = useNavigate();
  const location = useLocation();

  const fetchData = React.useCallback(async () => {
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
      setUser(chatRoomsResponse.data.attributes.name);
    } catch (err) {
      console.error("Error fetching data:", err);
      setError("Failed to fetch data. Please try again later.");
      setLoading(false);
    }
  }, [currentPage]);

  useEffect(() => {
    fetchData();
  }, [currentPage, fetchData]);

  const shouldStartNewChat = () => {
    const currentPath = location.pathname;
    const searchParams = new URLSearchParams(location.search);
    return currentPath.startsWith('/chat/') && searchParams.has('continue_id');
  };

  const handleContinueChat = (chatId, sessionId) => {
    if (shouldStartNewChat()) {
      navigate(`/chat/${chatId}`);
    } else {
      navigate(`/chat/${chatId}?continue_id=${sessionId}`);
    }
  };

  const handleStartNewChat = (chatId) => {
    navigate(`/chat/${chatId}`);
  };
  
  const handleDeleteChat = async (chatId) => {
    if (window.confirm('Are you sure you want to delete this chat?')) {
      try {
        await pubClient.patch(`/chat-history-records/${chatId}/visibility`, {
          hidden: true
        });
        
        // Refresh the chat history
        fetchData();
      } catch (error) {
        console.error('Error deleting chat:', error);
        setError("Failed to delete chat. Please try again later.");
      }
    }
  };

  const handlePageChange = (event, value) => {
    setCurrentPage(value);
  };

  if (loading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", mt: 4 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return (
      <Box sx={{ textAlign: "center", mt: 4 }}>
        <Typography color="error">
          {error}
        </Typography>
      </Box>
    );
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Chats</Typography>
      </TitleBox>
      <ContentBox>
        <Box sx={{ p: 7 }}>
          <Typography variant="headingXLarge">
            Hi {user}, welcome!
          </Typography>
        </Box>

        {chatRooms.length > 0 && (
        <Box sx={{ p: 7, pt: 0 }}>
          <Typography variant="headingLarge">
            Explore chats
          </Typography>
          <Grid container spacing={2} sx={{ mt: 1 }}>
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
                        <Box
                          sx={{ display: "flex", alignItems: "center", mb: 1 }}
                        >
                          <ChatIcon
                            sx={{
                              mr: 1,
                              fontSize: 20,
                              color: "text.primary",
                            }}
                          />
                          <Typography variant="headingMedium" component="div" noWrap>
                            {chat.attributes.name}
                          </Typography>
                        </Box>
                        {chat.attributes.description && (
                          <Typography
                            variant="bodyLargeDefault"
                            color="text.defaultSubdued"
                            sx={{
                              display: "-webkit-box",
                              WebkitLineClamp: 2,
                              WebkitBoxOrient: "vertical",
                              overflow: "hidden",
                              textOverflow: "ellipsis",
                              mt: 3,
                            }}
                          >
                            {chat.attributes.description}
                          </Typography>
                        )}
                      </Box>
                    </CardContent>
                    <CardActions sx={{ 
                      justifyContent: "flex-end", 
                      p: 2,
                      mt: 2,
                      borderTop: (theme) => `1px solid ${theme.palette.border.neutralDefaultSubdued}`,
                    }}>
                      <Button
                        onClick={() => handleStartNewChat(chat.id)}
                      >
                        Start chat
                      </Button>
                    </CardActions>
                  </Card>
                </Grid>
              ))}
          </Grid>
        </Box>
      )}

      {chatHistory.length > 0 && (
        <Box sx={{ pl: 7, pr: 7 }}>
          <Typography variant="headingLarge">
            Continue where you left off
          </Typography>
          <TableContainer component={StyledPaper} sx={{ mt: 2 }}>
            <Table>
              <TableBody>
                {chatHistory.map((record) => (
                  <StyledTableRow key={record.id}>
                    <StyledTableCell>{record.attributes.name}</StyledTableCell>
                    <StyledTableCell align="right">
                      <Button
                        onClick={() => handleDeleteChat(record.id)}
                        color="error"
                        sx={{ mr: 1 }}
                      >
                        Delete
                      </Button>
                      <Button
                        onClick={() =>
                          handleContinueChat(
                            record.attributes.chat_id,
                            record.attributes.session_id,
                          )
                        }
                      >
                        Continue
                      </Button>
                    </StyledTableCell>
                  </StyledTableRow>
                ))}
              </TableBody>
            </Table>
            <Box sx={{ 
              display: "flex", 
              justifyContent: "center", 
              p: 1,
              borderTop: (theme) => `1px solid ${theme.palette.border.neutralDefault}`
            }}>
              <Pagination
                count={totalPages}
                page={currentPage}
                onChange={handlePageChange}
              />
            </Box>
          </TableContainer>
        </Box>
        )}
      </ContentBox>
    </>
  );
};

export default ChatDashboard;
