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
import DashboardIcon from "@mui/icons-material/Dashboard";
import ChatIcon from "@mui/icons-material/Chat";
import CodeIcon from "@mui/icons-material/Code";
import StorageIcon from "@mui/icons-material/Storage";
import ExpandLess from "@mui/icons-material/ExpandLess";
import ExpandMore from "@mui/icons-material/ExpandMore";
import PsychologyIcon from "@mui/icons-material/Psychology";
import ChatBubbleOutlineIcon from "@mui/icons-material/ChatBubbleOutline";
import AppsIcon from "@mui/icons-material/Apps";
import AddCircleOutlineIcon from "@mui/icons-material/AddCircleOutline";

import pubClient from "../../admin/utils/pubClient";
const drawerWidth = 280; // Increased drawer width
const CACHE_KEY = "userEntitlements";
const CACHE_EXPIRY = 10000; // 10s
const PortalDrawer = () => {
  const [userEntitlements, setUserEntitlements] = useState(null);
  const [openDev, setOpenDev] = useState(true);
  const [openLLMs, setOpenLLMs] = useState(false);
  const [openDatabases, setOpenDatabases] = useState(false);
  const [openChatRooms, setOpenChatRooms] = useState(true);
  const theme = useTheme();

  useEffect(() => {
    const fetchUserEntitlements = async () => {
      const cachedData = localStorage.getItem(CACHE_KEY);
      if (cachedData) {
        const { data, timestamp } = JSON.parse(cachedData);
        if (Date.now() - timestamp < CACHE_EXPIRY) {
          setUserEntitlements(data);
          return;
        }
      }

      try {
        const response = await pubClient.get("/common/me");
        const newData = response.data.attributes.entitlements;
        setUserEntitlements(newData);
        localStorage.setItem(
          CACHE_KEY,
          JSON.stringify({
            data: newData,
            timestamp: Date.now(),
          }),
        );
      } catch (error) {
        console.error("Failed to fetch user entitlements:", error);
      }
    };

    fetchUserEntitlements();
  }, []);

  const handleDevClick = () => {
    setOpenDev(!openDev);
  };

  const handleLLMsClick = () => {
    setOpenLLMs(!openLLMs);
  };

  const handleDatabasesClick = () => {
    setOpenDatabases(!openDatabases);
  };

  const handleChatRoomsClick = () => {
    setOpenChatRooms(!openChatRooms);
  };

  return (
    <Drawer
      variant="permanent"
      sx={{
        width: drawerWidth,
        flexShrink: 0,
        [`& .MuiDrawer-paper`]: {
          width: drawerWidth,
          boxSizing: "border-box",
          backgroundColor: theme.palette.background.default,
          boxShadow: "none",
          border: "none",
          padding: "16px",
        },
      }}
    >
      <Toolbar sx={{ height: theme.spacing(12) }} />
      <List
        sx={{
          mt: 2,
          backgroundColor: "#ffffff",
          borderRadius: "16px",
          boxShadow: "0px 4px 20px rgba(0, 0, 0, 0.1)",
          padding: "16px",
        }}
      >
        <ListItem button component={Link} to="/portal/dashboard" sx={{ mb: 1 }}>
          <ListItemIcon>
            <DashboardIcon />
          </ListItemIcon>
          <ListItemText
            primary="Dashboard"
            primaryTypographyProps={{ noWrap: true }}
          />
        </ListItem>
        <ListItem button component={Link} to="/portal/app/new" sx={{ mb: 1 }}>
          <ListItemIcon>
            <AddCircleOutlineIcon />
          </ListItemIcon>
          <ListItemText
            primary="Create App"
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
            {userEntitlements?.chats.map((chat) => (
              <ListItem
                key={chat.id}
                button
                component={Link}
                to={`/portal/chat/${chat.id}`}
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
        <ListItem button onClick={handleDevClick} sx={{ mb: 1 }}>
          <ListItemIcon>
            <CodeIcon />
          </ListItemIcon>
          <ListItemText
            primary="Resources"
            primaryTypographyProps={{ noWrap: true }}
          />
          {openDev ? <ExpandLess /> : <ExpandMore />}
        </ListItem>
        <Collapse in={openDev} timeout="auto" unmountOnExit>
          <List component="div" disablePadding>
            <ListItem button onClick={handleLLMsClick} sx={{ pl: 4, mb: 1 }}>
              <ListItemIcon>
                <PsychologyIcon />
              </ListItemIcon>
              <ListItemText
                primary="LLMs"
                primaryTypographyProps={{ noWrap: true }}
              />
              {openLLMs ? <ExpandLess /> : <ExpandMore />}
            </ListItem>
            <Collapse in={openLLMs} timeout="auto" unmountOnExit>
              <List component="div" disablePadding>
                {userEntitlements?.catalogues.map((catalogue) => (
                  <ListItem
                    key={catalogue.id}
                    button
                    component={Link}
                    to={`/portal/llms/${catalogue.id}`}
                    sx={{ pl: 6, mb: 1 }}
                  >
                    <ListItemText
                      primary={catalogue.attributes.name}
                      primaryTypographyProps={{ noWrap: true }}
                    />
                  </ListItem>
                ))}
              </List>
            </Collapse>

            <ListItem
              button
              onClick={handleDatabasesClick}
              sx={{ pl: 4, mb: 1 }}
            >
              <ListItemIcon>
                <StorageIcon />
              </ListItemIcon>
              <ListItemText
                primary="Databases"
                primaryTypographyProps={{ noWrap: true }}
              />
              {openDatabases ? <ExpandLess /> : <ExpandMore />}
            </ListItem>
            <Collapse in={openDatabases} timeout="auto" unmountOnExit>
              <List component="div" disablePadding>
                {userEntitlements?.data_catalogues.map((dataCatalogue) => (
                  <ListItem
                    key={dataCatalogue.id}
                    button
                    component={Link}
                    to={`/portal/databases/${dataCatalogue.id}`}
                    sx={{ pl: 6, mb: 1 }}
                  >
                    <ListItemText
                      primary={dataCatalogue.attributes.name}
                      primaryTypographyProps={{ noWrap: true }}
                    />
                  </ListItem>
                ))}
              </List>
            </Collapse>
          </List>
        </Collapse>
        <ListItem button component={Link} to="/portal/apps" sx={{ mb: 1 }}>
          <ListItemIcon>
            <AppsIcon />
          </ListItemIcon>
          <ListItemText
            primary="My Apps"
            primaryTypographyProps={{ noWrap: true }}
          />
        </ListItem>
      </List>
    </Drawer>
  );
};

export default PortalDrawer;
