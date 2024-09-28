import React, { useState, useEffect, useRef } from "react";
import { useParams, useLocation } from "react-router-dom";
import {
  Box,
  TextField,
  Typography,
  Paper,
  CircularProgress,
} from "@mui/material";
import config from "../../config";

const ChatView = () => {
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

    // Add a delay before connecting
    const timer = setTimeout(() => {
      console.log("Attempting to connect to WebSocket:", wsUrl);
      setupWebSocket();
    }, 1000); // 1 second delay

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
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        height: "calc(100vh - 64px)",
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
              alignSelf: message.type === "user" ? "flex-end" : "flex-start",
              backgroundColor:
                message.type === "user" ? "primary.light" : "grey.200",
              borderRadius: 2,
              p: 1,
              maxWidth: "70%",
              opacity: message.isComplete ? 1 : 0.7,
            }}
          >
            <Typography>{message.content}</Typography>
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
  );
};

export default ChatView;
