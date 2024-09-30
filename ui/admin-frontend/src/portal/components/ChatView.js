import React, { useState, useEffect, useRef } from "react";
import { useParams, useLocation } from "react-router-dom";
import {
  Box,
  TextField,
  Typography,
  Paper,
  CircularProgress,
  Grid,
  Snackbar,
  Alert,
  Button,
} from "@mui/material";

import IconButton from "@mui/material/IconButton";
import KeyboardArrowDownIcon from "@mui/icons-material/KeyboardArrowDown";

import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import config from "../../config";
import FloatingSection from "./FloatingSection";
import pubClient from "../../admin/utils/pubClient";

const ChatView = () => {
  const [currentlyUsing, setCurrentlyUsing] = useState([]);
  const [databases, setDatabases] = useState([]);
  const [tools, setTools] = useState([]);
  const { chatId } = useParams();
  const location = useLocation();
  const [messages, setMessages] = useState([]);
  const [inputMessage, setInputMessage] = useState("");
  const [isConnected, setIsConnected] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(null);
  const [sessionId, setSessionId] = useState(null);
  const ws = useRef(null);
  const chatWindowRef = useRef(null);

  const messageContainerRef = useRef(null);

  const [autoScroll, setAutoScroll] = useState(true);

  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "error",
  });

  const closeWebSocket = () => {
    if (ws.current) {
      ws.current.close();
      ws.current = null;
    }
  };

  const scrollToBottom = () => {
    if (messageContainerRef.current) {
      const scrollHeight = messageContainerRef.current.scrollHeight;
      const height = messageContainerRef.current.clientHeight;
      const maxScrollTop = scrollHeight - height;
      messageContainerRef.current.scrollTo({
        top: maxScrollTop > 0 ? maxScrollTop : 0,
        behavior: "smooth",
      });
    }
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  useEffect(() => {
    if (autoScroll) {
      scrollToBottom();
    }
  }, [messages, autoScroll]);

  useEffect(() => {
    const handleScroll = () => {
      if (messageContainerRef.current) {
        const { scrollHeight, clientHeight, scrollTop } =
          messageContainerRef.current;
        const isScrolledToBottom = scrollHeight - clientHeight <= scrollTop + 1;
        setAutoScroll(isScrolledToBottom);
      }
    };

    const messageContainer = messageContainerRef.current;
    if (messageContainer) {
      messageContainer.addEventListener("scroll", handleScroll);
    }

    return () => {
      if (messageContainer) {
        messageContainer.removeEventListener("scroll", handleScroll);
      }
    };
  }, []);

  useEffect(() => {
    const messageContainer = messageContainerRef.current;

    if (!messageContainer) return;

    const resizeObserver = new ResizeObserver(() => {
      if (autoScroll) {
        scrollToBottom();
      }
    });

    resizeObserver.observe(messageContainer);

    return () => {
      resizeObserver.unobserve(messageContainer);
    };
  }, [autoScroll]);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [databasesResponse, toolsResponse] = await Promise.all([
          pubClient.get("/common/accessible-datasources"),
          pubClient.get("/common/accessible-tools"),
        ]);

        const newDatabases = databasesResponse.data.map((db) => ({
          id: db.id.toString(),
          name: db.attributes.name,
          type: "database",
          description: db.attributes.short_description,
          icon: db.attributes.icon,
        }));

        const newTools = toolsResponse.data.map((tool) => ({
          id: tool.id.toString(),
          name: tool.attributes.name,
          type: "tool",
          description: tool.attributes.description,
          toolType: tool.attributes.tool_type,
        }));

        setDatabases(newDatabases);
        setTools(newTools);
        setIsLoading(false);
      } catch (error) {
        console.error("Error fetching data:", error);
        setError("Failed to load databases and tools");
        setIsLoading(false);
      }
    };

    fetchData();
  }, []);

  useEffect(() => {
    setMessages([]);
    setIsLoading(true);
    setError(null);
  }, [chatId]);

  useEffect(() => {
    const searchParams = new URLSearchParams(location.search);
    const sessionId = searchParams.get("continue_id");
    const wsUrl = `${config.API_BASE_URL}/common/ws/chat/${chatId}${
      sessionId ? `?session_id=${sessionId}` : ""
    }`;

    const setupWebSocket = () => {
      closeWebSocket();

      ws.current = new WebSocket(wsUrl);

      ws.current.onopen = () => {
        setIsConnected(true);
        setIsLoading(false);
        setError(null);
        setMessages([]);
      };

      ws.current.onmessage = (event) => {
        const data = JSON.parse(event.data);
        handleIncomingMessage(data);
      };

      ws.current.onerror = (error) => {
        setError(`Failed to connect to chat. Error: ${error.message}`);
        setIsLoading(false);
      };

      ws.current.onclose = (event) => {
        setIsConnected(false);
        if (!event.wasClean) {
          setError(
            `Connection closed unexpectedly: ${event.reason || "Unknown reason"}`,
          );
        }
      };
    };

    setupWebSocket();

    return () => {
      closeWebSocket();
    };
  }, [chatId, location.search]);

  const handleIncomingMessage = (data) => {
    console.log("Handling incoming message:", data);

    if (data.type === "session_id") {
      console.log("Received session ID:", data.payload);
      setSessionId(data.payload);
      localStorage.setItem("chatSessionId", data.payload);
    } else if (data.type === "stream_chunk") {
      setIsLoading(false);
      setMessages((prevMessages) => {
        const newMessages = [...prevMessages];
        const lastMessage = newMessages[newMessages.length - 1];
        if (
          lastMessage &&
          lastMessage.type === "ai" &&
          !lastMessage.isComplete
        ) {
          newMessages[newMessages.length - 1] = {
            ...lastMessage,
            content: lastMessage.content + data.payload,
          };
        } else {
          newMessages.push({
            type: "ai",
            content: data.payload,
            isComplete: false,
          });
        }
        return newMessages;
      });
    } else if (data.type === "ai_message") {
      setIsLoading(false);
      setMessages((prevMessages) => {
        const newMessages = [...prevMessages];
        const lastMessage = newMessages[newMessages.length - 1];
        if (
          lastMessage &&
          lastMessage.type === "ai" &&
          !lastMessage.isComplete
        ) {
          newMessages[newMessages.length - 1] = {
            ...lastMessage,
            content: data.payload,
            isComplete: true,
          };
        } else {
          newMessages.push({
            type: "ai",
            content: data.payload,
            isComplete: true,
          });
        }
        return newMessages;
      });
    } else if (data.type === "error") {
      setError(data.payload);
      setIsLoading(false);
    } else {
      console.warn("Received unknown message type:", data.type);
    }
  };

  const handleSendMessage = (e) => {
    e.preventDefault();
    if (inputMessage.trim() && isConnected) {
      const message = { type: "user_message", payload: inputMessage.trim() };
      ws.current.send(JSON.stringify(message));
      setMessages((prevMessages) => [
        ...prevMessages,
        { type: "user", content: inputMessage.trim(), isComplete: true },
      ]);
      setInputMessage("");
    }
  };

  const renderMessageContent = (content) => {
    const components = {
      p: ({ node, ...props }) => <Typography {...props} />,
      a: ({ node, ...props }) => (
        <a target="_blank" rel="noopener noreferrer" {...props} />
      ),
      code: ({ node, inline, className, children, ...props }) => {
        const match = /language-(\w+)/.exec(className || "");

        if (inline) {
          return (
            <code className="inline-code" {...props}>
              {children}
            </code>
          );
        }

        return match ? (
          <pre
            style={{
              margin: "8px 0",
              padding: "10px",
              backgroundColor: "#f0f0f0",
              borderRadius: "4px",
              overflowX: "auto",
            }}
          >
            <code
              className={className}
              style={{
                fontFamily: "monospace",
                fontSize: "0.9em",
                whiteSpace: "pre-wrap",
                wordBreak: "break-word",
              }}
              {...props}
            >
              {children}
            </code>
          </pre>
        ) : (
          <code className={className} {...props}>
            {children}
          </code>
        );
      },
    };

    return (
      <ReactMarkdown components={components} remarkPlugins={[remarkGfm]}>
        {content}
      </ReactMarkdown>
    );
  };

  const removeFromCurrentlyUsing = async (item) => {
    try {
      let response;
      if (item.type === "database") {
        response = await pubClient.delete(
          `/common/chat-sessions/${sessionId}/datasources/${item.id}`,
        );
      } else if (item.type === "tool") {
        response = await pubClient.delete(
          `/common/chat-sessions/${sessionId}/tools/${item.id}`,
        );
      }

      if (response.status === 200 || response.status === 204) {
        setCurrentlyUsing((prevItems) =>
          prevItems.filter((i) => i.uniqueId !== item.uniqueId),
        );
        if (item.type === "database") {
          setDatabases((prevDatabases) => [...prevDatabases, item]);
        } else if (item.type === "tool") {
          setTools((prevTools) => [...prevTools, item]);
        }
      } else {
        console.error("Failed to remove item from chat session");
      }
    } catch (error) {
      console.error("Error removing item from chat session:", error);
    }
  };

  const addToCurrentlyUsing = async (item) => {
    try {
      let response;
      if (item.type === "database") {
        response = await pubClient.post(
          `/common/chat-sessions/${sessionId}/datasources`,
          { datasource_id: parseInt(item.id) },
        );
      } else if (item.type === "tool") {
        response = await pubClient.post(
          `/common/chat-sessions/${sessionId}/tools`,
          {
            tool_id: item.id,
          },
        );
      }

      if (response.status === 200 || response.status === 201) {
        const uniqueId = `${item.type}-${item.id}`;
        setCurrentlyUsing((prevItems) => [...prevItems, { ...item, uniqueId }]);
        if (item.type === "database") {
          setDatabases((prevDatabases) =>
            prevDatabases.filter((db) => db.id !== item.id),
          );
        } else if (item.type === "tool") {
          setTools((prevTools) =>
            prevTools.filter((tool) => tool.id !== item.id),
          );
        }
      } else {
        console.error("Failed to add item to chat session", response);
        setSnackbar({
          open: true,
          message: "Failed to add item to chat session",
          severity: "error",
        });
      }
    } catch (error) {
      console.error("Error adding item to chat session:", error);
      let errorMessage = "Failed to add item to chat session";
      if (error.response && error.response.data && error.response.data.errors) {
        errorMessage = error.response.data.errors[0].detail || errorMessage;
      }
      setSnackbar({
        open: true,
        message: errorMessage,
        severity: "error",
      });
    }
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  if (isLoading) {
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        height="100vh"
      >
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        height="100vh"
        flexDirection="column"
      >
        <Typography color="error" gutterBottom>
          {error}
        </Typography>
        <Button variant="contained" onClick={() => setError(null)}>
          Dismiss
        </Button>
      </Box>
    );
  }

  return (
    <Box
      sx={{
        height: "85vh",
        display: "flex",
        flexDirection: "column",
        "& .inline-code": {
          display: "inline-block",
          padding: "2px 4px",
          color: "#232629",
          backgroundColor: "rgb(240, 240, 240)",
          borderRadius: "3px",
          fontFamily: "monospace",
          fontSize: "0.9em",
        },
      }}
    >
      <Grid container sx={{ flexGrow: 1, overflow: "hidden" }}>
        <Grid
          item
          xs={9}
          sx={{ height: "100%", display: "flex", flexDirection: "column" }}
        >
          <Paper
            elevation={0}
            sx={{
              flexGrow: 1,
              display: "flex",
              flexDirection: "column",
              overflow: "hidden",
              height: "100%",
            }}
          >
            <Box
              ref={messageContainerRef}
              sx={{
                flexGrow: 1,
                overflowY: "auto",
                display: "flex",
                flexDirection: "column",
                scrollBehavior: "smooth",
                "&::-webkit-scrollbar": {
                  width: "0.4em",
                },
                "&::-webkit-scrollbar-track": {
                  boxShadow: "inset 0 0 6px rgba(0,0,0,0.00)",
                },
                "&::-webkit-scrollbar-thumb": {
                  backgroundColor: "rgba(0,0,0,.1)",
                  outline: "1px solid slategrey",
                },
              }}
            >
              {messages.map((message, index) => (
                <Box
                  key={index}
                  sx={{
                    width: "100%",
                    p: 2,
                    borderTop: index > 0 ? "1px solid #e0e0e0" : "none",
                    borderBottom:
                      index === messages.length - 1
                        ? "1px solid #e0e0e0"
                        : "none",
                    opacity: message.isComplete ? 1 : 0.7,
                  }}
                >
                  <Typography
                    variant="subtitle2"
                    sx={{ fontWeight: "bold", mb: 1 }}
                  >
                    {message.type === "user" ? "You:" : "Assistant:"}
                  </Typography>
                  {renderMessageContent(message.content)}
                </Box>
              ))}
            </Box>

            {!autoScroll && (
              <IconButton
                onClick={scrollToBottom}
                sx={{
                  position: "absolute",
                  bottom: 70,
                  right: 20,
                  backgroundColor: "background.paper",
                  "&:hover": { backgroundColor: "action.hover" },
                }}
              >
                <KeyboardArrowDownIcon />
              </IconButton>
            )}
          </Paper>

          <Box
            component="form"
            onSubmit={handleSendMessage}
            sx={{ p: 1, borderTop: 0, height: "64px" }}
          >
            <TextField
              fullWidth
              variant="outlined"
              placeholder="Type your message here..."
              value={inputMessage}
              onChange={(e) => setInputMessage(e.target.value)}
              disabled={!isConnected}
            />
          </Box>
        </Grid>

        <Grid item xs={3} sx={{ height: "100%", overflowY: "auto" }}>
          <Box
            sx={{
              display: "flex",
              flexDirection: "column",
              height: "100%",
              gap: 1,
              p: 1,
              overflowY: "auto",
            }}
          >
            <FloatingSection
              key="currentlyUsing"
              title="Currently Using..."
              items={currentlyUsing}
              onRemove={removeFromCurrentlyUsing}
              emptyText="Click + on tools and databases to use them in the chat"
            />
            <FloatingSection
              key="databases"
              title="Databases"
              items={databases}
              onAdd={(item) => addToCurrentlyUsing(item)}
            />
            <FloatingSection
              key="tools"
              title="Tools"
              items={tools}
              onAdd={(item) => addToCurrentlyUsing(item)}
            />
          </Box>
        </Grid>
      </Grid>

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
    </Box>
  );
};

export default ChatView;
