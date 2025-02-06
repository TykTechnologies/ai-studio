import React, { useState } from "react";
import {
  Drawer,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,
  Collapse,
  Toolbar,
  IconButton,
  Divider,
  useTheme,
} from "@mui/material";
import ChevronLeftIcon from "@mui/icons-material/ChevronLeft";
import ChevronRightIcon from "@mui/icons-material/ChevronRight";
import ExpandLess from "@mui/icons-material/ExpandLess";
import ExpandMore from "@mui/icons-material/ExpandMore";
import { StyledNavLink } from "../../styles/sharedStyles";

const BaseDrawer = ({
  menuItems,
  drawerWidth = 240,
  minimizedWidth = 60,
  showToolbar = true,
  customStyles = {},
  defaultOpen = true,
  defaultExpandedItems = {},
}) => {
  const [open, setOpen] = useState(defaultOpen);
  const [expandedItems, setExpandedItems] = useState(defaultExpandedItems);

  const handleDrawerToggle = () => {
    setOpen(!open);
  };

  const handleExpandClick = (itemId, parentId = null) => {
    setExpandedItems((prevState) => {
      // If this is a nested item, we need to ensure parent stays open
      if (parentId) {
        return {
          ...prevState,
          [parentId]: true, // Keep parent expanded
          [itemId]: !prevState[itemId], // Toggle current item
        };
      }
      return {
        ...prevState,
        [itemId]: !prevState[itemId],
      };
    });
  };

  const renderMenuItem = (item, depth = 0, parentId = null) => {
    const hasSubItems = item.subItems && item.subItems.length > 0;
    const isExpanded = expandedItems[item.id];
    const commonStyles = {
      pl: open ? depth * 4 + 2 : 2,
    };

    if (hasSubItems) {
      return (
        <React.Fragment key={item.id}>
          <ListItem
            button
            onClick={() => handleExpandClick(item.id, parentId)}
            sx={commonStyles}
          >
            {item.icon && <ListItemIcon>{item.icon}</ListItemIcon>}
            {open && (
              <ListItemText
                primary={item.text}
                primaryTypographyProps={{
                  variant: depth > 0 ? "body2" : "body1",
                  color: depth > 0 ? "text.secondary" : "text.primary",
                }}
              />
            )}
            {open && (isExpanded ? <ExpandLess /> : <ExpandMore />)}
          </ListItem>
          <Collapse in={isExpanded} timeout="auto" unmountOnExit>
            <List component="div" disablePadding>
              {item.subItems.map((subItem) => renderMenuItem(subItem, depth + 1, item.id))}
            </List>
          </Collapse>
        </React.Fragment>
      );
    } else {
      return (
        <ListItem
          button
          component={StyledNavLink}
          to={item.path}
          sx={commonStyles}
          end={item.path === "/admin/"}
        >
          {item.icon && <ListItemIcon>{item.icon}</ListItemIcon>}
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

  const currentWidth = open ? drawerWidth : minimizedWidth;

  return (
    <Drawer
      variant="permanent"
      sx={{
        width: currentWidth,
        flexShrink: 0,
        [`& .MuiDrawer-paper`]: {
          width: currentWidth,
          boxSizing: "border-box",
          overflow: "visible",
          transition: "width 0.2s",
          ...customStyles,
        },
      }}
    >
      {showToolbar && <Toolbar />}
      <IconButton
        onClick={handleDrawerToggle}
        sx={{
          backgroundColor: "#ECECEF",
          alignSelf: "flex-end",
          mr: 1,
          position: "absolute",
          left: "calc(100% - 20px)",
          top: "78px",
          zIndex: 9,
        }}
      >
        {open ? <ChevronLeftIcon /> : <ChevronRightIcon />}
      </IconButton>
      <List>
        {menuItems.map((item) => renderMenuItem(item))}
      </List>
      <Divider />
    </Drawer>
  );
};

export default BaseDrawer; 