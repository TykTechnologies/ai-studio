import React, { useState } from "react";
import Drawer from "@mui/material/Drawer";
import Toolbar from "@mui/material/Toolbar";
import List from "@mui/material/List";
import Divider from "@mui/material/Divider";
import ListItem from "@mui/material/ListItem";
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText from "@mui/material/ListItemText";
import IconButton from "@mui/material/IconButton";
import ChevronLeftIcon from "@mui/icons-material/ChevronLeft";
import ChevronRightIcon from "@mui/icons-material/ChevronRight";
import ExpandLess from "@mui/icons-material/ExpandLess";
import ExpandMore from "@mui/icons-material/ExpandMore";
import Collapse from "@mui/material/Collapse";
import DashboardIcon from "@mui/icons-material/Dashboard";
import PersonIcon from "@mui/icons-material/Person";
import GroupIcon from "@mui/icons-material/Group";
import PeopleIcon from "@mui/icons-material/People";
import SmartToyIcon from "@mui/icons-material/SmartToy";
import SettingsIcon from "@mui/icons-material/Settings";
import AttachMoneyIcon from "@mui/icons-material/AttachMoney";
import StorageIcon from "@mui/icons-material/Storage";
import BuildIcon from "@mui/icons-material/Build";
import DataObjectIcon from "@mui/icons-material/DataObject";
import FolderOpenIcon from "@mui/icons-material/FolderOpen";
import FilterListIcon from "@mui/icons-material/FilterList";
import SettingsInputComponentIcon from "@mui/icons-material/SettingsInputComponent";
import AppsIcon from "@mui/icons-material/Apps";
import WebIcon from "@mui/icons-material/Web";
import ChatIcon from "@mui/icons-material/Chat";
import VpnKeyIcon from "@mui/icons-material/VpnKey";

import { StyledNavLink } from "../../styles/sharedStyles";
import useSystemFeatures from "../../hooks/useSystemFeatures";

const drawerWidth = 240;
const minimizedDrawerWidth = 60;

const MyDrawer = () => {
  const { features, loading } = useSystemFeatures();
  const [open, setOpen] = useState(true);
  const [expandedItems, setExpandedItems] = useState({});

  const handleDrawerToggle = () => {
    setOpen(!open);
  };

  const handleExpandClick = (text) => {
    setExpandedItems((prevState) => ({
      ...prevState,
      [text]: !prevState[text],
    }));
  };

  const getMenuItems = (features) => [
    { text: "Dashboard", icon: <DashboardIcon />, path: "/admin/dash" },
    {
      text: "Team",
      icon: <PeopleIcon />,
      subItems: [
        { text: "Users", icon: <PersonIcon />, path: "/admin/users" },
        { text: "Groups", icon: <GroupIcon />, path: "/admin/groups" },
      ],
    },
    {
      text: "AI",
      icon: <SmartToyIcon />,
      subItems: [
        { text: "LLMs", icon: <SmartToyIcon />, path: "/admin/llms" },
        {
          text: "Call Settings",
          icon: <SettingsIcon />,
          path: "/admin/llm-settings",
        },
        {
          text: "Model Prices",
          icon: <AttachMoneyIcon />,
          path: "/admin/model-prices",
        },
      ],
    },
    {
      text: "Data",
      icon: <DataObjectIcon />,
      subItems: [
        {
          text: "Vector Sources",
          icon: <StorageIcon />,
          path: "/admin/datasources",
        },
        // Only show Tools if feature_chat is enabled
        ...(features.feature_chat
          ? [{ text: "Tools", icon: <BuildIcon />, path: "/admin/tools" }]
          : []),
      ],
    },
    // Only show Gateway if feature_gateway is enabled
    ...(features.feature_gateway
      ? [
          {
            text: "Gateway",
            icon: <SettingsInputComponentIcon />,
            subItems: [
              {
                text: "Filters",
                icon: <FilterListIcon />,
                path: "/admin/filters",
              },
              { text: "Secrets", icon: <VpnKeyIcon />, path: "/admin/secrets" },
            ],
          },
        ]
      : []),
    {
      text: "Portal",
      icon: <WebIcon />,
      subItems: [
        // Only show Apps if either feature_portal or feature_gateway is enabled
        ...(features.feature_portal || features.feature_gateway
          ? [{ text: "Apps", icon: <AppsIcon />, path: "/admin/apps" }]
          : []),
        // Only show Chat Rooms if feature_chat is enabled
        ...(features.feature_chat
          ? [{ text: "Chat Rooms", icon: <ChatIcon />, path: "/admin/chats" }]
          : []),
        {
          text: "Catalogs",
          icon: <FolderOpenIcon />,
          subItems: [
            {
              text: "LLMs",
              icon: <SmartToyIcon />,
              path: "/admin/catalogs/llms",
            },
            {
              text: "Data",
              icon: <DataObjectIcon />,
              path: "/admin/catalogs/data",
            },
            // Only show Tools catalog if feature_chat is enabled
            ...(features.feature_chat
              ? [
                  {
                    text: "Tools",
                    icon: <BuildIcon />,
                    path: "/admin/catalogs/tools",
                  },
                ]
              : []),
          ],
        },
      ],
    },
  ];

  const renderMenuItem = (item, depth = 0) => {
    const commonStyles = {
      pl: open ? depth * 4 + 2 : 2,
    };

    if (item.subItems) {
      return (
        <React.Fragment key={item.text}>
          <ListItem
            button
            onClick={() => handleExpandClick(item.text)}
            sx={commonStyles}
          >
            <ListItemIcon>{item.icon}</ListItemIcon>
            {open && (
              <ListItemText
                primary={item.text}
                primaryTypographyProps={{
                  variant: depth > 0 ? "body2" : "body1",
                  color: depth > 0 ? "text.secondary" : "text.primary",
                }}
              />
            )}
            {open &&
              (expandedItems[item.text] ? <ExpandLess /> : <ExpandMore />)}
          </ListItem>
          <Collapse in={expandedItems[item.text]} timeout="auto" unmountOnExit>
            <List component="div" disablePadding>
              {item.subItems.map((subItem) =>
                renderMenuItem(subItem, depth + 1),
              )}
            </List>
          </Collapse>
        </React.Fragment>
      );
    } else {
      return (
        <ListItem
          key={item.text}
          component={StyledNavLink}
          to={item.path}
          end={item.path === "/admin/"}
          sx={commonStyles}
        >
          <ListItemIcon>{item.icon}</ListItemIcon>
          {open && (
            <ListItemText
              primary={item.text}
              primaryTypographyProps={{
                variant: depth > 0 ? "body2" : "body1",
                color: depth > 0 ? "text.secondary" : "text.primary",
              }}
            />
          )}
        </ListItem>
      );
    }
  };

  if (loading) {
    return <div>Loading...</div>; // Or your preferred loading indicator
  }

  const menuItems = getMenuItems(features);

  return (
    <Drawer
      variant="permanent"
      sx={{
        width: open ? drawerWidth : minimizedDrawerWidth,
        flexShrink: 0,
        [`& .MuiDrawer-paper`]: {
          width: open ? drawerWidth : minimizedDrawerWidth,
          boxSizing: "border-box",
          overflowX: "hidden",
          transition: "width 0.2s",
        },
      }}
    >
      <Toolbar />
      <IconButton
        onClick={handleDrawerToggle}
        sx={{ alignSelf: "flex-end", mr: 1 }}
      >
        {open ? <ChevronLeftIcon /> : <ChevronRightIcon />}
      </IconButton>
      <List>{menuItems.map((item) => renderMenuItem(item))}</List>
      <Divider />
    </Drawer>
  );
};

export default MyDrawer;
