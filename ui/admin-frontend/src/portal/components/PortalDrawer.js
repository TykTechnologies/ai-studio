import React, { useState, useEffect } from "react";
import {
  Drawer,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,
  Collapse,
  Toolbar,
} from "@mui/material";
import { Link } from "react-router-dom";
import ChatIcon from "@mui/icons-material/Chat";
import CodeIcon from "@mui/icons-material/Code";
import StorageIcon from "@mui/icons-material/Storage";
import ExpandLess from "@mui/icons-material/ExpandLess";
import ExpandMore from "@mui/icons-material/ExpandMore";
import PsychologyIcon from "@mui/icons-material/Psychology";
import ChatBubbleOutlineIcon from "@mui/icons-material/ChatBubbleOutline";
import AppsIcon from "@mui/icons-material/Apps";

import pubClient from "../../admin/utils/pubClient";

const drawerWidth = 240;
const CACHE_KEY = "userEntitlements";
const CACHE_EXPIRY = 60 * 60 * 1000; // 1 hour in milliseconds

const PortalDrawer = () => {
  const [userEntitlements, setUserEntitlements] = useState(null);
  const [openDev, setOpenDev] = useState(true);
  const [openLLMs, setOpenLLMs] = useState(false);
  const [openDatabases, setOpenDatabases] = useState(false);

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

  return (
    <Drawer
      variant="permanent"
      sx={{
        width: drawerWidth,
        flexShrink: 0,
        [`& .MuiDrawer-paper`]: { width: drawerWidth, boxSizing: "border-box" },
      }}
    >
      <Toolbar />
      <List>
        {/* Chat Rooms */}
        <ListItem button component={Link} to="/portal/chat-rooms">
          <ListItemIcon>
            <ChatIcon />
          </ListItemIcon>
          <ListItemText primary="Chat Rooms" />
        </ListItem>
        {userEntitlements?.chats.map((chat) => (
          <ListItem
            key={chat.id}
            button
            component={Link}
            to={`/portal/chat-rooms/${chat.id}`}
            sx={{ pl: 4 }}
          >
            <ListItemIcon>
              <ChatBubbleOutlineIcon /> {/* Added icon for each chat room */}
            </ListItemIcon>
            <ListItemText primary={chat.attributes.name} />
          </ListItem>
        ))}

        {/* Development Resources */}
        <ListItem button onClick={handleDevClick}>
          <ListItemIcon>
            <CodeIcon />
          </ListItemIcon>
          <ListItemText primary="Development Resources" />
          {openDev ? <ExpandLess /> : <ExpandMore />}
        </ListItem>
        <Collapse in={openDev} timeout="auto" unmountOnExit>
          <List component="div" disablePadding>
            {/* LLMs */}
            <ListItem button onClick={handleLLMsClick} sx={{ pl: 4 }}>
              <ListItemIcon>
                <PsychologyIcon />
              </ListItemIcon>
              <ListItemText primary="LLMs" />
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
                    sx={{ pl: 6 }}
                  >
                    <ListItemText primary={catalogue.attributes.name} />
                  </ListItem>
                ))}
              </List>
            </Collapse>

            {/* Databases */}
            <ListItem button onClick={handleDatabasesClick} sx={{ pl: 4 }}>
              <ListItemIcon>
                <StorageIcon />
              </ListItemIcon>
              <ListItemText primary="Databases" />
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
                    sx={{ pl: 6 }}
                  >
                    <ListItemText primary={dataCatalogue.attributes.name} />
                  </ListItem>
                ))}
              </List>
            </Collapse>
          </List>
        </Collapse>
        <ListItem button component={Link} to="/portal/apps">
          <ListItemIcon>
            <AppsIcon />
          </ListItemIcon>
          <ListItemText primary="My Apps" />
        </ListItem>
      </List>
    </Drawer>
  );
};

export default PortalDrawer;
