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
        // Console logs removed

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
    const searchParams = new URLSearchParams(location.search);
    const sessionId = searchParams.get("continue_id");
    const wsUrl = `${config.API_BASE_URL}/common/ws/chat/${chatId}${sessionId ? `?session_id=${sessionId}` : ""}`;
    const setupWebSocket = () => {
      ws.current = new WebSocket(wsUrl);

      ws.current.onopen = () => {
        setIsConnected(true);
        setIsLoading(false);
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
        setError(`Connection closed: ${event.reason || "Unknown reason"}`);
      };
    };

    const timer = setTimeout(() => {
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
      setSessionId(data.payload);
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
    console.log("onDragEnd result:", result);
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

    if (!sourceList || !destList) {
      console.error("Source or destination list is undefined");
      return;
    }

    const [reorderedItem] = sourceList.splice(source.index, 1);
    destList.splice(destination.index, 0, reorderedItem);

    setSourceList([...sourceList]);
    setDestList([...destList]);
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
        // Optionally, you can set an error state here to display to the user
      }
    } catch (error) {
      console.error("Error adding item to chat session:", error);
      // Optionally, you can set an error state here to display to the user
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
                    message.type === "user" ? "#e3f2fd" : "grey.200",
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
  );
};

export default ChatView;
