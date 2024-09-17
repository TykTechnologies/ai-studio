// src/Drawer.js
import React from "react";
import { NavLink } from "react-router-dom";
import Drawer from "@mui/material/Drawer";
import Toolbar from "@mui/material/Toolbar";
import List from "@mui/material/List";
import Divider from "@mui/material/Divider";
import ListItem from "@mui/material/ListItem";
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText from "@mui/material/ListItemText";
import DashboardIcon from "@mui/icons-material/Dashboard";
import PeopleIcon from "@mui/icons-material/People";
import SettingsIcon from "@mui/icons-material/Settings";
import BarChartIcon from "@mui/icons-material/BarChart";
import AppsIcon from "@mui/icons-material/Apps";
import GroupIcon from "@mui/icons-material/Group";
import LibraryBooksIcon from "@mui/icons-material/LibraryBooks";
import StorageIcon from "@mui/icons-material/Storage";
import MemoryIcon from "@mui/icons-material/Memory";

const drawerWidth = 240;

const menuItems = [
  { text: "Dashboard", icon: <DashboardIcon />, path: "/" },
  { text: "Users", icon: <PeopleIcon />, path: "/users" },
  { text: "Apps", icon: <AppsIcon />, path: "/apps" },
  { text: "Groups", icon: <GroupIcon />, path: "/groups" },
  { text: "Catalogues", icon: <LibraryBooksIcon />, path: "/catalogues" },
  { text: "Datasources", icon: <StorageIcon />, path: "/datasources" },
  { text: "LLMs", icon: <MemoryIcon />, path: "/llms" },
  { text: "Settings", icon: <SettingsIcon />, path: "/settings" },
  { text: "Reports", icon: <BarChartIcon />, path: "/reports" },
];

const MyDrawer = () => {
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
        {menuItems.map((item) => (
          <ListItem
            button
            key={item.text}
            component={NavLink}
            to={item.path}
            end={item.path === "/"}
            sx={{
              "&.active": {
                backgroundColor: "rgba(0, 0, 0, 0.08)",
              },
              color: "inherit",
              textDecoration: "none",
            }}
          >
            <ListItemIcon>{item.icon}</ListItemIcon>
            <ListItemText primary={item.text} />
          </ListItem>
        ))}
      </List>
      <Divider />
    </Drawer>
  );
};

export default MyDrawer;
