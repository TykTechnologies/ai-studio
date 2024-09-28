import React, { useState, useEffect, useRef } from "react";
import { useParams, useLocation } from "react-router-dom";
import {
  Box,
  TextField,
  Typography,
  Paper,
  CircularProgress,
  Grid,
} from "@mui/material";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { DragDropContext, Droppable, Draggable } from "react-beautiful-dnd";
import config from "../../config";
import FloatingSection from "./FloatingSection";

const ChatView = () => {
  const [currentlyUsing, setCurrentlyUsing] = useState([]);
  const [databases, setDatabases] = useState([
    { id: "db1", name: "User Database" },
    { id: "db2", name: "Product Database" },
    { id: "db3", name: "Order Database" },
  ]);
  const [tools, setTools] = useState([
    { id: "tool1", name: "Text Summarizer" },
    { id: "tool2", name: "Image Generator" },
    { id: "tool3", name: "Code Analyzer" },
  ]);
  const { chatId } = useParams();
  const location = useLocation();
  const [messages, setMessages] = useState([]);
  const [inputMessage, setInputMessage] = useState("");
  const [isConnected, setIsConnected] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(null);
  const ws = useRef(null);
  const chatWindowRef = useRef(null);

  useEffect(() => {
    const searchParams = new URLSearchParams(location.search);
    const sessionId = searchParams.get("continue_id");
    const wsUrl = `${config.API_BASE_URL}/common/ws/chat/${chatId}${sessionId ? `?session_id=${sessionId}` : ""}`;

    console.log("Preparing to connect to WebSocket:", wsUrl);

    const setupWebSocket = () => {
      ws.current = new WebSocket(wsUrl);

      ws.current.onopen = () => {
        console.log("WebSocket connection established");
        setIsConnected(true);
        setIsLoading(false);
      };

      ws.current.onmessage = (event) => {
        console.log("Received message:", event.data);
        const data = JSON.parse(event.data);
        handleIncomingMessage(data);
      };

      ws.current.onerror = (error) => {
        console.error("WebSocket error:", error);
        setError(`Failed to connect to chat. Error: ${error.message}`);
        setIsLoading(false);
      };

      ws.current.onclose = (event) => {
        console.log(
          `WebSocket connection closed: ${event.code} ${event.reason}`,
        );
        setIsConnected(false);
        setError(`Connection closed: ${event.reason || "Unknown reason"}`);
      };
    };

    const timer = setTimeout(() => {
      console.log("Attempting to connect to WebSocket:", wsUrl);
      setupWebSocket();
    }, 1000);

    return () => {
      clearTimeout(timer);
      if (ws.current) {
        ws.current.close();
      }
    };
  }, [chatId, location.search]);

  const handleIncomingMessage = (data) => {
    console.log("Handling incoming message:", data);

    if (data.type === "session_id") {
      console.log("Received session ID:", data.payload);
      localStorage.setItem("chatSessionId", data.payload);
    } else if (data.type === "stream_chunk") {
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

  useEffect(() => {
    if (chatWindowRef.current) {
      chatWindowRef.current.scrollTop = chatWindowRef.current.scrollHeight;
    }
  }, [messages]);

  const renderMessageContent = (content) => {
    return (
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          p: ({ node, ...props }) => <Typography {...props} />,
          a: ({ node, ...props }) => (
            <a target="_blank" rel="noopener noreferrer" {...props} />
          ),
          code: ({ node, inline, ...props }) => (
            <code
              style={{
                backgroundColor: "#f0f0f0",
                padding: inline ? "2px 4px" : "10px",
                borderRadius: "4px",
                fontFamily: "monospace",
                display: inline ? "inline" : "block",
              }}
              {...props}
            />
          ),
        }}
      >
        {content}
      </ReactMarkdown>
    );
  };

  const onDragEnd = (result) => {
    const { source, destination } = result;

    if (!destination) {
      return;
    }

    let sourceList, setSourceList, destList, setDestList;

    if (source.droppableId === "currentlyUsing") {
      sourceList = currentlyUsing;
      setSourceList = setCurrentlyUsing;
    } else if (source.droppableId === "databases") {
      sourceList = databases;
      setSourceList = setDatabases;
    } else if (source.droppableId === "tools") {
      sourceList = tools;
      setSourceList = setTools;
    }

    if (destination.droppableId === "currentlyUsing") {
      destList = currentlyUsing;
      setDestList = setCurrentlyUsing;
    } else if (destination.droppableId === "databases") {
      destList = databases;
      setDestList = setDatabases;
    } else if (destination.droppableId === "tools") {
      destList = tools;
      setDestList = setTools;
    }

    const [reorderedItem] = sourceList.splice(source.index, 1);
    destList.splice(destination.index, 0, reorderedItem);

    setSourceList([...sourceList]);
    setDestList([...destList]);
  };

  const removeFromCurrentlyUsing = (id) => {
    const item = currentlyUsing.find((i) => i.id === id);
    setCurrentlyUsing(currentlyUsing.filter((i) => i.id !== id));
    if (item.id.startsWith("db")) {
      setDatabases([...databases, item]);
    } else if (item.id.startsWith("tool")) {
      setTools([...tools, item]);
    }
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
      >
        <Typography color="error">{error}</Typography>
      </Box>
    );
  }

  return (
    <DragDropContext onDragEnd={onDragEnd}>
      <Grid container spacing={2} sx={{ height: "calc(100vh - 110px)" }}>
        <Grid item xs={9}>
          <Box
            sx={{
              display: "flex",
              flexDirection: "column",
              height: "100%",
            }}
          >
            <Paper
              ref={chatWindowRef}
              elevation={3}
              sx={{
                flex: 1,
                overflowY: "auto",
                p: 2,
                display: "flex",
                flexDirection: "column",
                gap: 2,
              }}
            >
              {messages.map((message, index) => (
                <Box
                  key={index}
                  sx={{
                    alignSelf:
                      message.type === "user" ? "flex-end" : "flex-start",
                    backgroundColor:
                      message.type === "user" ? "primary.light" : "grey.200",
                    borderRadius: 2,
                    p: 1,
                    maxWidth: "70%",
                    opacity: message.isComplete ? 1 : 0.7,
                  }}
                >
                  {renderMessageContent(message.content)}
                </Box>
              ))}
            </Paper>
            <Box component="form" onSubmit={handleSendMessage} sx={{ p: 2 }}>
              <TextField
                fullWidth
                variant="outlined"
                placeholder="Type your message here..."
                value={inputMessage}
                onChange={(e) => setInputMessage(e.target.value)}
                disabled={!isConnected}
              />
            </Box>
          </Box>
        </Grid>
        <Grid item xs={3}>
          <Box
            sx={{
              display: "flex",
              flexDirection: "column",
              height: "100%",
              gap: 2,
            }}
          >
            <FloatingSection
              title="Currently Using..."
              items={currentlyUsing}
              droppableId="currentlyUsing"
              onRemove={removeFromCurrentlyUsing}
              emptyText="Drag tools and databases here to use them in the chat"
            />
            <FloatingSection
              title="Databases"
              items={databases}
              droppableId="databases"
            />
            <FloatingSection title="Tools" items={tools} droppableId="tools" />
          </Box>
        </Grid>
      </Grid>
    </DragDropContext>
  );
};

export default ChatView;
