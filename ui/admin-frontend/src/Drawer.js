import React from "react";
import { NavLink } from "react-router-dom";
import Drawer from "@mui/material/Drawer";
import Toolbar from "@mui/material/Toolbar";
import List from "@mui/material/List";
import Divider from "@mui/material/Divider";
import ListItem from "@mui/material/ListItem";
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText from "@mui/material/ListItemText";
import { styled } from "@mui/material/styles";
import DashboardIcon from "@mui/icons-material/Dashboard";
import PeopleIcon from "@mui/icons-material/People";
import AppsIcon from "@mui/icons-material/Apps";
import GroupIcon from "@mui/icons-material/Group";
import LibraryBooksIcon from "@mui/icons-material/LibraryBooks";
import StorageIcon from "@mui/icons-material/Storage";
import MemoryIcon from "@mui/icons-material/Memory";

const drawerWidth = 240;

const StyledNavLink = styled(NavLink)(({ theme }) => ({
  textDecoration: "none",
  color: theme.palette.text.primary,
  "&.active": {
    backgroundColor: "#008080", // Teal color matching AppBar background
    color: theme.palette.common.white, // White text for better contrast
    "& .MuiListItemIcon-root": {
      color: theme.palette.common.white, // White icon for better contrast
    },
  },
  "&:hover": {
    backgroundColor: "#20B2AA", // Light teal color for hover
  },
}));

const menuItems = [
  { text: "Dashboard", icon: <DashboardIcon />, path: "/" },
  { text: "Users", icon: <PeopleIcon />, path: "/users" },
  { text: "Apps", icon: <AppsIcon />, path: "/apps" },
  { text: "Groups", icon: <GroupIcon />, path: "/groups" },
  { text: "Catalogues", icon: <LibraryBooksIcon />, path: "/catalogues" },
  { text: "Datasources", icon: <StorageIcon />, path: "/datasources" },
  { text: "LLMs", icon: <MemoryIcon />, path: "/llms" },
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
            key={item.text}
            component={StyledNavLink}
            to={item.path}
            end={item.path === "/"}
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
