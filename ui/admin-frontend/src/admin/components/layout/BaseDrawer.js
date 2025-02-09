import React, { useState, useEffect } from "react";
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
} from "@mui/material";
import { ThemeProvider } from "@mui/material/styles";
import adminTheme from "../../theme";
import ChevronLeftIcon from "@mui/icons-material/ChevronLeft";
import ChevronRightIcon from "@mui/icons-material/ChevronRight";
import ExpandLess from "@mui/icons-material/ExpandLess";
import ExpandMore from "@mui/icons-material/ExpandMore";
import { StyledNavLink } from "../../styles/sharedStyles";

const generateRandomId = () => Math.random().toString(36).substring(7);

const BaseDrawer = ({
  id = generateRandomId(),
  menuItems,
  drawerWidth = 240,
  minimizedWidth = 60,
  showToolbar = true,
  customStyles = {},
  defaultOpen = true,
  defaultExpandedItems = {},
}) => {
  const STORAGE_KEY = `drawer_state_${id}`;
  
  const getInitialState = () => {
    try {
      const savedState = localStorage.getItem(STORAGE_KEY);
      if (savedState) {
        const state = JSON.parse(savedState);
        return {
          open: state.isOpen ?? defaultOpen,
          expanded: state.expanded ?? defaultExpandedItems
        };
      }
    } catch (error) {
      console.error('Error reading from localStorage:', error);
    }
    return {
      open: defaultOpen,
      expanded: defaultExpandedItems
    };
  };

  const initialState = getInitialState();
  const [open, setOpen] = useState(initialState.open);
  const [expandedItems, setExpandedItems] = useState(initialState.expanded);

  useEffect(() => {
    try {
      const currentState = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      localStorage.setItem(STORAGE_KEY, JSON.stringify({
        ...currentState,
        isOpen: open,
        expanded: expandedItems
      }));
    } catch (error) {
      console.error('Error updating drawer state:', error);
    }
  }, [open, expandedItems, STORAGE_KEY]);

  const saveSelectedPath = (path) => {
    try {
      const currentState = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      localStorage.setItem(STORAGE_KEY, JSON.stringify({
        ...currentState,
        selectedPath: path
      }));
    } catch (error) {
      console.error('Error saving selected path:', error);
    }
  };

  const handleDrawerToggle = () => {
    setOpen(!open);
  };

  const handleExpandClick = (itemId, parentId = null) => {
    setExpandedItems((prevState) => {
      const newState = { ...prevState };
      newState[itemId] = !prevState[itemId];
      if (parentId && !prevState[itemId]) {
        newState[parentId] = true;
      }
      return newState;
    });
  };

  const renderMenuItem = (item, depth = 0, parentId = null) => {
    const itemId = item.id || item.text;
    const hasSubItems = item.subItems;
    const isExpanded = expandedItems[itemId];
    const commonStyles = {
      pl: open ? depth * 4 + 2 : 2,
    };

    if (hasSubItems) {
      return (
        <React.Fragment key={itemId}>
          <ListItem
            button
            onClick={() => handleExpandClick(itemId, parentId)}
            sx={{ ...commonStyles, cursor: 'pointer' }}
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
              {hasSubItems && item.subItems.map((subItem) => renderMenuItem(subItem, depth + 1, item.id))}
            </List>
          </Collapse>
        </React.Fragment>
      );
    }

    return (
      <ListItem
        button
        component={StyledNavLink}
        to={item.path}
        sx={commonStyles}
        onClick={() => saveSelectedPath(item.path)}
        {...(item.path === "/admin/" ? { end: true } : {})}
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
  };

  const currentWidth = open ? drawerWidth : minimizedWidth;

  return (
    <ThemeProvider theme={adminTheme}>
      <Drawer
      variant="permanent"
      sx={{
        width: currentWidth,
        flexShrink: 0,
        [`& .MuiDrawer-paper`]: {
          width: currentWidth,
          boxSizing: "border-box",
          overflow: "visible",
          transition: "width 0.2s"
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
      <List 
        sx={{ mt: customStyles.marginTop ? customStyles.marginTop : 0 }}
      >
        {menuItems.map((item) => renderMenuItem(item))}
      </List>
      <Divider />
      </Drawer>
    </ThemeProvider>
  );
};

export default BaseDrawer;
