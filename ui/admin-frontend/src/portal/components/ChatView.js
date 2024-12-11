import React, { useState, useEffect, useRef, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import { Chip } from "@mui/material";
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

import FloatingSection from "./FloatingSection";
import { useDropzone } from "react-dropzone";
import CheckCircleOutlineIcon from "@mui/icons-material/CheckCircleOutline";
import AttachFileIcon from "@mui/icons-material/AttachFile";
import pubClient from "../../admin/utils/pubClient";
import TextareaAutosize from "@mui/material/TextareaAutosize";
import { getConfig } from "../../config"; // Update the import
import SmartToyOutlinedIcon from "@mui/icons-material/SmartToyOutlined";
import SendIcon from "@mui/icons-material/Send";

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
  const [uploadedFiles, setUploadedFiles] = useState([]);
  const [isUploading, setIsUploading] = useState(false);
  const [isFetchingHistory, setIsFetchingHistory] = useState(false);
  const [hasUpdatedChatName, setHasUpdatedChatName] = useState(false);
  const [isNewChat, setIsNewChat] = useState(true);
  const [chatName, setChatName] = useState("");
  const navigate = useNavigate();
  const [showError, setShowError] = useState(false);
  const [expandedGroups, setExpandedGroups] = useState({});

  const [showTools, setShowTools] = useState(true);

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

  const handleKeyDown = (e) => {
    if (e.key === "Enter") {
      // If Shift or Cmd/Ctrl is pressed, allow new line
      if (e.shiftKey || e.metaKey || e.ctrlKey) {
        return;
      }
      e.preventDefault();
      handleSendMessage(e);
    }
  };

  const toggleGroup = (index) => {
    setExpandedGroups((prev) => ({
      ...prev,
      [index]: !prev[index],
    }));
  };

  const groupSystemMessages = (segments) => {
    let groupedSegments = [];
    let currentSystemGroup = [];

    segments.forEach((segment) => {
      if (segment.match(/(:::|\%\%\%)system/)) {
        // Add to current system group
        currentSystemGroup.push(
          segment
            .replace(/(:::|\%\%\%)system\s*([\s\S]*?)(:::|\%\%\%)/, "$2")
            .trim(),
        );
      } else if (segment.trim()) {
        // If we have accumulated system messages, add them as a group
        if (currentSystemGroup.length > 0) {
          groupedSegments.push({
            type: "system-group",
            messages: currentSystemGroup,
          });
          currentSystemGroup = [];
        }
        // Add non-system content
        groupedSegments.push({
          type: "content",
          content: segment,
        });
      }
    });

    // Handle any remaining system messages
    if (currentSystemGroup.length > 0) {
      groupedSegments.push({
        type: "system-group",
        messages: currentSystemGroup,
      });
    }

    return groupedSegments;
  };

  const updateChatName = useCallback(
    async (name) => {
      if (sessionId) {
        try {
          let truncatedName = name.trim().slice(0, 60);
          if (name.length > 60) {
            truncatedName += "...";
          }
          await pubClient.put(
            `/common/chat-history-records/${sessionId}/name`,
            { name: truncatedName },
          );
          setChatName(truncatedName);
        } catch (error) {
          console.error("Error updating chat name:", error);
          setSnackbar({
            open: true,
            message: "Failed to update chat name",
            severity: "error",
          });
        }
      } else {
        console.warn("Cannot update chat name: sessionId is not set");
      }
    },
    [sessionId],
  );

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
        const cachedEntitlements = localStorage.getItem("userEntitlements");
        let userEntitlements;

        if (cachedEntitlements) {
          const parsedData = JSON.parse(cachedEntitlements);
          userEntitlements = parsedData.data; // The actual data is nested under 'data'
        } else {
          const response = await pubClient.get("/me");
          userEntitlements = response.data.attributes.entitlements;
          localStorage.setItem(
            "userEntitlements",
            JSON.stringify({ data: userEntitlements, timestamp: Date.now() }),
          );
        }

        const currentChat = userEntitlements.chats.find(
          (chat) => chat.id === chatId,
        );
        if (currentChat) {
          setShowTools(currentChat.attributes.tool_support);
        } else {
          console.warn(`Chat with id ${chatId} not found in user entitlements`);
        }
      } catch (error) {
        console.error("Error fetching user entitlements:", error);
        setMessages((prevMessages) => [
          ...prevMessages,
          {
            type: "system",
            content: ":::system Error: Failed to load user entitlements:::",
            isComplete: true,
          },
        ]);
      }

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
        setMessages((prevMessages) => [
          ...prevMessages,
          {
            type: "system",
            content: ":::system Error: Failed to load databases and tools:::",
            isComplete: true,
          },
        ]);
      } finally {
        setIsLoading(false);
      }
    };

    fetchData();
  }, [chatId]);

  useEffect(() => {
    setMessages([]);
    setIsLoading(true);
    setError(null);
  }, [chatId]);

  useEffect(() => {
    const searchParams = new URLSearchParams(location.search);
    const continueId = searchParams.get("continue_id");
    const sessionId = searchParams.get("continue_id");
    const currentConfig = getConfig();
    const wsUrl = `${currentConfig.API_BASE_URL}/common/ws/chat/${chatId}${
      sessionId ? `?session_id=${sessionId}` : ""
    }`;

    setIsNewChat(!continueId); // Set isNewChat based on whether there's a continue_id
    setHasUpdatedChatName(false);

    let keepAliveInterval; // Define interval variable

    const setupWebSocket = () => {
      closeWebSocket();

      ws.current = new WebSocket(wsUrl);

      ws.current.onopen = () => {
        setIsConnected(true);
        setIsLoading(false);
        setError(null);

        // Set up keepalive interval
        keepAliveInterval = setInterval(() => {
          if (ws.current && ws.current.readyState === WebSocket.OPEN) {
            ws.current.send(JSON.stringify({ type: "ping" }));
          }
        }, 10000); // Send keepalive every 10 second

        // If there's a continue_id, fetch the chat history
        if (continueId) {
          fetchChatHistory(continueId);
          setSessionId(continueId);
        } else {
          setMessages([]);
        }
      };

      ws.current.onmessage = (event) => {
        const data = JSON.parse(event.data);
        handleIncomingMessage(data);
      };

      ws.current.onerror = (error) => {
        setMessages((prevMessages) => [
          ...prevMessages,
          {
            type: "system",
            content: `:::system Error: Failed to connect to chat. ${error.message}:::`,
            isComplete: true,
          },
        ]);
        setIsLoading(false);
      };

      ws.current.onclose = (event) => {
        setIsConnected(false);
        if (keepAliveInterval) {
          clearInterval(keepAliveInterval);
        }
        if (!event.wasClean) {
          setMessages((prevMessages) => [
            ...prevMessages,
            {
              type: "system",
              content: `:::system Error: Connection closed unexpectedly: ${event.reason || "Unknown reason"}:::`,
              isComplete: true,
            },
          ]);
        }
      };
    };

    const delay = 1000; // 1 second delay, adjust as needed
    const timer = setTimeout(() => {
      setupWebSocket();
    }, delay);

    return () => {
      if (keepAliveInterval) {
        clearInterval(keepAliveInterval);
      }
      closeWebSocket();
    };
  }, [chatId, location.search]);

  const fetchChatHistory = useCallback(async (sessionId) => {
    setIsFetchingHistory(true);
    try {
      const response = await pubClient.get(
        `/common/sessions/${sessionId}/messages?limit=100`,
      );
      const historicalMessages = response.data
        .map((msg) => {
          const parsedContent = JSON.parse(msg.attributes.content);
          if (parsedContent.role === "system") {
            return null; // Ignore system messages
          }
          let content = parsedContent.text;
          if (parsedContent.role === "human") {
            const messageMatch = content.match(/Message:\s*([\s\S]*)/);
            content = messageMatch ? messageMatch[1].trim() : content;
          }
          return {
            type: parsedContent.role === "human" ? "user" : "ai",
            content: content,
            isComplete: true,
          };
        })
        .filter((msg) => msg !== null); // Remove null entries (system messages)
      setMessages(historicalMessages);
    } catch (error) {
      console.error("Error fetching chat history:", error);
      setMessages([
        {
          type: "system",
          content: ":::system Error: Failed to load chat history:::",
          isComplete: true,
        },
      ]);
    } finally {
      setIsFetchingHistory(false);
    }
  }, []);

  const onDrop = useCallback(
    (acceptedFiles) => {
      setIsUploading(true);
      const uploadPromises = acceptedFiles.map((file) => {
        const formData = new FormData();
        formData.append("file", file);
        return pubClient
          .post(`/common/chat-sessions/${sessionId}/upload`, formData, {
            headers: { "Content-Type": "multipart/form-data" },
          })
          .then(() => ({ name: file.name, size: file.size }))
          .catch((error) => {
            setSnackbar({
              open: true,
              message: `Failed to upload ${file.name}: ${error.response?.data?.errors?.[0]?.detail || error.message}`,
              severity: "error",
            });
            return null;
          });
      });

      Promise.all(uploadPromises).then((fileInfos) => {
        const successfulUploads = fileInfos.filter((info) => info !== null);
        setUploadedFiles((prev) => [...prev, ...successfulUploads]);
        setIsUploading(false);
        if (successfulUploads.length > 0) {
          setSnackbar({
            open: true,
            message: `Successfully uploaded ${successfulUploads.length} file(s)`,
            severity: "success",
          });
        }
      });
    },
    [sessionId, setSnackbar],
  );

  const { getRootProps, getInputProps, isDragActive, open } = useDropzone({
    onDrop,
    noClick: true,
    noKeyboard: true,
  });

  const renderUploadIndicator = () => {
    if (isUploading) {
      return <CircularProgress size={20} />;
    }
    if (uploadedFiles.length > 0) {
      return <CheckCircleOutlineIcon color="success" />;
    }
    return null;
  };

  const handleIncomingMessage = (data) => {
    if (data.type === "session_id") {
      setSessionId(data.payload);
      localStorage.setItem("chatSessionId", data.payload);
    } else if (data.type === "stream_chunk" || data.type === "ai_message") {
      setIsLoading(false);
      setMessages((prevMessages) => {
        const newMessages = [...prevMessages];
        const lastMessage = newMessages[newMessages.length - 1];
        let content = data.payload;

        if (data.type === "ai_message") {
          content = JSON.parse(data.payload).text;
        }

        if (
          lastMessage &&
          lastMessage.type === "ai" &&
          !lastMessage.isComplete &&
          data.type === "stream_chunk"
        ) {
          newMessages[newMessages.length - 1] = {
            ...lastMessage,
            content: lastMessage.content + content,
          };
        } else {
          newMessages.push({
            type: "ai",
            content: content,
            isComplete: data.type === "ai_message",
          });

          if (isNewChat && !hasUpdatedChatName && data.type === "ai_message") {
            const newName = content.slice(0, 100).trim();
            updateChatName(newName);
            setHasUpdatedChatName(true);
            setIsNewChat(false);
          }
        }
        return newMessages;
      });
    } else if (data.type === "error") {
      // Add error as a system message
      setMessages((prevMessages) => [
        ...prevMessages,
        {
          type: "system",
          content: `:::system Error: ${data.payload}:::`,
          isComplete: true,
        },
      ]);
      setIsLoading(false);
    } else {
      console.warn("Received unknown message type:", data.type);
    }
  };

  const handleSendMessage = (e) => {
    e.preventDefault();
    if ((inputMessage.trim() || uploadedFiles.length > 0) && isConnected) {
      const message = {
        type: "user_message",
        payload: inputMessage.trim(),
        file_refs: uploadedFiles.map((file) => file.name),
      };
      ws.current.send(JSON.stringify(message));
      setMessages((prevMessages) => [
        ...prevMessages,
        {
          type: "user",
          content: inputMessage.trim(),
          fileRefs: uploadedFiles.map((file) => file.name),
          isComplete: true,
        },
      ]);

      // Update chat name only if it's a new chat and hasn't been updated yet
      if (isNewChat && !hasUpdatedChatName) {
        updateChatName(inputMessage.trim());
        setHasUpdatedChatName(true);
        setIsNewChat(false); // Set isNewChat to false after updating the name
      }

      setInputMessage("");
      setUploadedFiles([]);
    }
  };

  const renderMessageContent = (content) => {
    if (!content) {
      return null;
    }

    const segments = content.split(
      /((?::::|\%\%\%)system[\s\S]*?(?::::|\%\%\%))/g,
    );
    const groupedSegments = groupSystemMessages(segments);

    return (
      <>
        {groupedSegments.map((segment, index) => {
          if (segment.type === "system-group") {
            const isExpanded = expandedGroups[index];
            const messageCount = segment.messages.length;
            const hasMultipleMessages = messageCount > 1;

            return (
              <Box
                key={index}
                sx={{
                  backgroundColor: "#E0F7F6",
                  border: "1px solid #e9ecef",
                  borderRadius: "10px",
                  boxShadow: "0px 4px 8px rgba(0, 0, 0, 0.1)",
                  padding: "12px 12px",
                  margin: "10px 10px",
                  color: "#000000",
                  fontFamily: "monospace",
                  cursor: hasMultipleMessages ? "pointer" : "default",
                }}
                onClick={
                  hasMultipleMessages ? () => toggleGroup(index) : undefined
                }
              >
                {/* First message is always visible */}
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1,
                    backgroundColor: segment.messages[0].startsWith("Error:")
                      ? "#FEE2E2"
                      : "transparent",
                    color: segment.messages[0].startsWith("Error:")
                      ? "#DC2626"
                      : "inherit",
                    padding: "4px 8px",
                    borderRadius: "4px",
                  }}
                >
                  <SmartToyOutlinedIcon
                    sx={{
                      fontSize: "1rem",
                      color: segment.messages[0].startsWith("Error:")
                        ? "#DC2626"
                        : "#666",
                    }}
                  />
                  {segment.messages[0]}
                </Box>

                {/* Show message count and expand/collapse indicator if there are multiple messages */}
                {hasMultipleMessages && (
                  <Box
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                      mt: 1,
                      borderTop: "1px solid rgba(0,0,0,0.1)",
                      pt: 1,
                      color: "#666",
                      fontSize: "0.8rem",
                    }}
                  >
                    <Typography variant="caption">
                      {isExpanded
                        ? "Click to collapse"
                        : `${messageCount - 1} more messages...`}
                    </Typography>
                    <KeyboardArrowDownIcon
                      sx={{
                        transform: isExpanded ? "rotate(180deg)" : "none",
                        transition: "transform 0.2s",
                      }}
                    />
                  </Box>
                )}

                {/* Additional messages shown when expanded */}
                {isExpanded && (
                  <Box sx={{ mt: 1 }}>
                    {segment.messages.slice(1).map((message, msgIndex) => {
                      const isError = message.startsWith("Error:");
                      return (
                        <Box
                          key={msgIndex}
                          sx={{
                            display: "flex",
                            alignItems: "center",
                            gap: 1,
                            backgroundColor: isError
                              ? "#FEE2E2"
                              : "transparent",
                            color: isError ? "#DC2626" : "inherit",
                            padding: "4px 8px",
                            borderRadius: "4px",
                            mt: 1,
                          }}
                          onClick={(e) => e.stopPropagation()}
                        >
                          <SmartToyOutlinedIcon
                            sx={{
                              fontSize: "1rem",
                              color: isError ? "#DC2626" : "#666",
                            }}
                          />
                          {message}
                        </Box>
                      );
                    })}
                  </Box>
                )}
              </Box>
            );
          } else {
            return (
              <ReactMarkdown
                key={index}
                components={{
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
                }}
                remarkPlugins={[remarkGfm]}
              >
                {segment.content}
              </ReactMarkdown>
            );
          }
        })}
      </>
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

  if (isLoading || isFetchingHistory) {
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
            sx={{ p: 1, borderTop: 0, minHeight: "64px", position: "relative" }}
            {...getRootProps()}
          >
            <input {...getInputProps()} />
            <TextField
              fullWidth
              variant="outlined"
              placeholder="Type your message here... (Enter to send, Shift+Enter for new line)"
              value={inputMessage}
              onChange={(e) => setInputMessage(e.target.value)}
              onKeyDown={handleKeyDown}
              disabled={!isConnected}
              multiline
              minRows={1}
              maxRows={4}
              InputProps={{
                inputComponent: TextareaAutosize,
                endAdornment: (
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    {uploadedFiles.length > 0 && (
                      <Chip
                        icon={<AttachFileIcon />}
                        label={uploadedFiles.length}
                        size="small"
                        onDelete={() => setUploadedFiles([])}
                      />
                    )}
                    {renderUploadIndicator()}
                    <IconButton onClick={open} size="small">
                      <AttachFileIcon />
                    </IconButton>
                    <IconButton
                      onClick={handleSendMessage}
                      disabled={
                        !isConnected ||
                        (!inputMessage.trim() && uploadedFiles.length === 0)
                      }
                      size="small"
                    >
                      <SendIcon />
                    </IconButton>
                  </Box>
                ),
              }}
            />
            {isDragActive && (
              <Box
                sx={{
                  position: "absolute",
                  top: 0,
                  left: 0,
                  right: 0,
                  bottom: 0,
                  backgroundColor: "rgba(0, 0, 0, 0.1)",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                }}
              >
                <Typography variant="body2">
                  Drop files here to upload
                </Typography>
              </Box>
            )}
          </Box>
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
            {showTools && (
              <FloatingSection
                key="tools"
                title="Tools"
                items={tools}
                onAdd={(item) => addToCurrentlyUsing(item)}
              />
            )}
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
