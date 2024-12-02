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
import VpnKeyIcon from "@mui/icons-material/VpnKey"; // Add this import at the top

import { StyledNavLink } from "../../styles/sharedStyles";

const drawerWidth = 240;
const minimizedDrawerWidth = 60;

const menuItems = [
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
      { text: "Tools", icon: <BuildIcon />, path: "/admin/tools" },
    ],
  },
  {
    text: "Gateway",
    icon: <SettingsInputComponentIcon />,
    subItems: [
      { text: "Filters", icon: <FilterListIcon />, path: "/admin/filters" },
      { text: "Secrets", icon: <VpnKeyIcon />, path: "/admin/secrets" },
    ],
  },
  {
    text: "Portal",
    icon: <WebIcon />,
    subItems: [
      { text: "Apps", icon: <AppsIcon />, path: "/admin/apps" },
      { text: "Chat Rooms", icon: <ChatIcon />, path: "/admin/chats" }, // Add this line
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
          { text: "Tools", icon: <BuildIcon />, path: "/admin/catalogs/tools" },
        ],
      },
    ],
  },
];

const MyDrawer = () => {
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
