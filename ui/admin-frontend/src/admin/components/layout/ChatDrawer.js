import React, { useState, useEffect } from "react";
import {
  Drawer,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,
  Collapse,
  Toolbar,
  useTheme,
} from "@mui/material";
import { Link } from "react-router-dom";
import ChatIcon from "@mui/icons-material/Chat";
import DashboardIcon from "@mui/icons-material/Dashboard";
import ChatBubbleOutlineIcon from "@mui/icons-material/ChatBubbleOutline";
import ExpandLess from "@mui/icons-material/ExpandLess";
import ExpandMore from "@mui/icons-material/ExpandMore";
import { DRAWER_WIDTH } from "../../../constants/layout";

import pubClient from "../../utils/pubClient";
import useSystemFeatures from "../../hooks/useSystemFeatures";

const CACHE_KEY = "userEntitlements";
const CACHE_EXPIRY = 10000; // 10s

const ChatDrawer = () => {
  const { features, loading } = useSystemFeatures();
  const [userEntitlements, setUserEntitlements] = useState(null);
  const [uiOptions, setUiOptions] = useState(null);
  const [openChatRooms, setOpenChatRooms] = useState(true);
  const theme = useTheme();

  useEffect(() => {
    const fetchUserEntitlements = async () => {
      const cachedData = localStorage.getItem(CACHE_KEY);
      if (cachedData) {
        const { data, timestamp } = JSON.parse(cachedData);
        if (Date.now() - timestamp < CACHE_EXPIRY) {
          setUserEntitlements(data);
          setUiOptions(data.ui_options);
          return;
        }
      }

      try {
        const response = await pubClient.get("/common/me");
        const newData = response.data.attributes.entitlements;
        const newUiOptions = response.data.attributes.ui_options;
        setUserEntitlements(newData);
        setUiOptions(newUiOptions);
        localStorage.setItem(
          CACHE_KEY,
          JSON.stringify({
            data: { ...newData, ui_options: newUiOptions },
            timestamp: Date.now(),
          }),
        );
      } catch (error) {
        console.error("Failed to fetch user entitlements:", error);
      }
    };

    fetchUserEntitlements();
  }, []);

  const handleChatRoomsClick = () => {
    setOpenChatRooms(!openChatRooms);
  };

  if (loading) {
    return null;
  }

  return (
    <Drawer
      variant="permanent"
      sx={{
        width: DRAWER_WIDTH,
        flexShrink: 0,
        [`& .MuiDrawer-paper`]: {
          width: DRAWER_WIDTH,
          boxSizing: "border-box",
          backgroundColor: theme.palette.background.default,
          boxShadow: "none",
          border: "none",
          padding: "16px",
          marginTop: "64px",
        },
      }}
    >
      <List
        sx={{
          mt: 2,
          backgroundColor: "#ffffff",
          borderRadius: "16px",
          boxShadow: "0px 4px 20px rgba(0, 0, 0, 0.1)",
          padding: "16px",
        }}
      >
        {features.feature_chat && uiOptions?.show_chat && (
          <>
            <ListItem
              button
              component={Link}
              to="/chat/dashboard"
              sx={{ mb: 1 }}
            >
              <ListItemIcon>
                <DashboardIcon />
              </ListItemIcon>
              <ListItemText
                primary="Dashboard"
                primaryTypographyProps={{ noWrap: true }}
              />
            </ListItem>
            <ListItem button onClick={handleChatRoomsClick} sx={{ mb: 1 }}>
              <ListItemIcon>
                <ChatIcon />
              </ListItemIcon>
              <ListItemText
                primary="Chat Rooms"
                primaryTypographyProps={{ noWrap: true }}
              />
              {openChatRooms ? <ExpandLess /> : <ExpandMore />}
            </ListItem>
            <Collapse in={openChatRooms} timeout="auto" unmountOnExit>
              <List component="div" disablePadding>
                {userEntitlements?.chats?.map((chat) => (
                  <ListItem
                    key={chat.id}
                    button
                    component={Link}
                    to={`/chat/${chat.id}`}
                    sx={{ pl: 4, mb: 1 }}
                  >
                    <ListItemIcon>
                      <ChatBubbleOutlineIcon />
                    </ListItemIcon>
                    <ListItemText
                      primary={chat.attributes.name}
                      primaryTypographyProps={{ noWrap: true }}
                    />
                  </ListItem>
                ))}
              </List>
            </Collapse>
          </>
        )}
      </List>
    </Drawer>
  );
};

export default ChatDrawer;
