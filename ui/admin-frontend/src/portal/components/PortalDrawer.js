// DEPRECATED: This PortalDrawer is no longer used by the main layout.
// The active portal drawer is at: admin/components/layout/PortalDrawer.js
// which uses the BaseDrawer component. This file is kept for reference only.
import React, { useState, useEffect } from "react";
import {
  Drawer,
  Divider,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,
  Collapse,
  Toolbar,
  useTheme,
} from "@mui/material";
import { Link, useLocation, useNavigate } from "react-router-dom";
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
import ExtensionIcon from "@mui/icons-material/Extension";
import SmartToyIcon from "@mui/icons-material/SmartToy";
import WidgetsIcon from "@mui/icons-material/Widgets";

import pubClient from "../../admin/utils/pubClient";
import useSystemFeatures from "../../admin/hooks/useSystemFeatures";
import portalPluginLoaderService from "../services/portalPluginLoaderService";

const drawerWidth = 280;
const CACHE_KEY = "userEntitlements";
const CACHE_EXPIRY = 10000; // 10s

const PortalDrawer = () => {
  const { features, loading } = useSystemFeatures();
  const location = useLocation();
  const navigate = useNavigate();
  const [userEntitlements, setUserEntitlements] = useState(null);
  const [uiOptions, setUiOptions] = useState(null);
  const [openDev, setOpenDev] = useState(true);
  const [openLLMs, setOpenLLMs] = useState(false);
  const [openDatabases, setOpenDatabases] = useState(false);
  const [openTools, setOpenTools] = useState(false); // State for Tools section
  const [openChatRooms, setOpenChatRooms] = useState(true);
  const [pluginMenuItems, setPluginMenuItems] = useState([]);
  const [openPluginSections, setOpenPluginSections] = useState({});
  const theme = useTheme();

  // Load portal plugin sidebar items
  useEffect(() => {
    const loadPluginMenuItems = async () => {
      try {
        const menuItems = await portalPluginLoaderService.getSidebarMenuItems();
        setPluginMenuItems(menuItems);
      } catch (error) {
        console.error("Failed to load portal plugin menu items:", error);
      }
    };
    loadPluginMenuItems();

    const handlePluginRefresh = () => loadPluginMenuItems();
    window.addEventListener("portal-plugin-loader-refreshed", handlePluginRefresh);
    return () => window.removeEventListener("portal-plugin-loader-refreshed", handlePluginRefresh);
  }, []);

  const togglePluginSection = (sectionId) => {
    setOpenPluginSections(prev => ({ ...prev, [sectionId]: !prev[sectionId] }));
  };

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
          })
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

  const handleToolsClick = () => { // Handler for Tools section
    setOpenTools(!openTools);
  };

  const handleChatRoomsClick = () => {
    setOpenChatRooms(!openChatRooms);
  };

  /**
   * Always start a brand new session if user is on the same chat route.
   * If user is on /chat/5 and clicks the same chat 5, we do a quick away to /chat/dashboard
   * then re-navigate to /chat/5. Also remove old session from localStorage.
   */
  const handleChatClick = (chatId) => {
    // Remove any old session:
    localStorage.removeItem('chatSessionId');

    // If user is already on same route, forcibly navigate away, then back:
    if (location.pathname === `/chat/${chatId}`) {
      navigate('/chat/dashboard');
      setTimeout(() => {
        navigate(`/chat/${chatId}`);
      }, 0);
    } else {
      navigate(`/chat/${chatId}`);
    }
  };

  if (loading) {
    return null;
  }

  const showPortalFeatures =
    features.feature_portal || features.feature_gateway;

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
        <ListItem button component={Link} to="/chat/dashboard" sx={{ mb: 1 }}>
          <ListItemIcon>
            <DashboardIcon />
          </ListItemIcon>
          <ListItemText
            primary="Dashboard"
            primaryTypographyProps={{ noWrap: true }}
          />
        </ListItem>

        {showPortalFeatures && uiOptions?.show_portal && (
          <ListItem button component={Link} to="/portal/app/new" sx={{ mb: 1 }}>
            <ListItemIcon>
              <AddCircleOutlineIcon />
            </ListItemIcon>
            <ListItemText
              primary="Create App"
              primaryTypographyProps={{ noWrap: true }}
            />
          </ListItem>
        )}

        {features.feature_chat && uiOptions?.show_chat && (
          <>
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
                    onClick={() => handleChatClick(chat.id)}
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

            <ListItem button component={Link} to="/agents" sx={{ mb: 1 }}>
              <ListItemIcon>
                <SmartToyIcon />
              </ListItemIcon>
              <ListItemText
                primary="Agents"
                primaryTypographyProps={{ noWrap: true }}
              />
            </ListItem>
          </>
        )}

        {showPortalFeatures && uiOptions?.show_portal && (
          <>
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
                <ListItem
                  button
                  onClick={handleLLMsClick}
                  sx={{ pl: 4, mb: 1 }}
                >
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

                {userEntitlements?.tool_catalogues && userEntitlements.tool_catalogues.length > 0 && (
                  <>
                    <ListItem
                      button
                      onClick={handleToolsClick}
                      sx={{ pl: 4, mb: 1 }}
                    >
                      <ListItemIcon>
                        <ExtensionIcon />
                      </ListItemIcon>
                      <ListItemText
                        primary="Tools"
                        primaryTypographyProps={{ noWrap: true }}
                      />
                      {openTools ? <ExpandLess /> : <ExpandMore />}
                    </ListItem>
                    <Collapse in={openTools} timeout="auto" unmountOnExit>
                      <List component="div" disablePadding>
                        {userEntitlements.tool_catalogues.map((toolCatalogue) => (
                          <ListItem
                            key={toolCatalogue.id}
                            button
                            component={Link}
                            to={`/portal/tools/${toolCatalogue.id}`}
                            sx={{ pl: 6, mb: 1 }}
                          >
                            <ListItemText
                              primary={toolCatalogue.attributes.name}
                              primaryTypographyProps={{ noWrap: true }}
                            />
                          </ListItem>
                        ))}
                      </List>
                    </Collapse>
                  </>
                )}
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
          </>
        )}

        {/* Portal plugin sidebar sections */}
        {pluginMenuItems.length > 0 && (
          <>
            <Divider sx={{ my: 1 }} />
            {pluginMenuItems.map((pluginSection) => (
              <React.Fragment key={pluginSection.id}>
                {pluginSection.sub_items && pluginSection.sub_items.length > 1 ? (
                  <>
                    <ListItem
                      button
                      onClick={() => togglePluginSection(pluginSection.id)}
                      sx={{ mb: 1 }}
                    >
                      <ListItemIcon>
                        <WidgetsIcon />
                      </ListItemIcon>
                      <ListItemText
                        primary={pluginSection.label}
                        primaryTypographyProps={{ noWrap: true }}
                      />
                      {openPluginSections[pluginSection.id] ? <ExpandLess /> : <ExpandMore />}
                    </ListItem>
                    <Collapse in={openPluginSections[pluginSection.id]} timeout="auto" unmountOnExit>
                      <List component="div" disablePadding>
                        {pluginSection.sub_items.map((subItem) => (
                          <ListItem
                            key={subItem.id}
                            button
                            component={Link}
                            to={subItem.path}
                            sx={{ pl: 4, mb: 1 }}
                          >
                            <ListItemText
                              primary={subItem.text}
                              primaryTypographyProps={{ noWrap: true }}
                            />
                          </ListItem>
                        ))}
                      </List>
                    </Collapse>
                  </>
                ) : (
                  // Single item - render directly without collapsible section
                  pluginSection.sub_items?.map((subItem) => (
                    <ListItem
                      key={subItem.id}
                      button
                      component={Link}
                      to={subItem.path}
                      sx={{ mb: 1 }}
                    >
                      <ListItemIcon>
                        <WidgetsIcon />
                      </ListItemIcon>
                      <ListItemText
                        primary={subItem.text || pluginSection.label}
                        primaryTypographyProps={{ noWrap: true }}
                      />
                    </ListItem>
                  ))
                )}
              </React.Fragment>
            ))}
          </>
        )}
      </List>
    </Drawer>
  );
};

export default PortalDrawer;