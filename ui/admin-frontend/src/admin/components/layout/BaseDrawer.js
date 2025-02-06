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
  const theme = useTheme();

  const handleDrawerToggle = () => {
    setOpen(!open);
  };

  const handleExpandClick = (itemId) => {
    setExpandedItems((prevState) => ({
      ...prevState,
      [itemId]: !prevState[itemId],
    }));
  };

  const renderMenuItem = (item, depth = 0) => {
    const hasSubItems = item.subItems && item.subItems.length > 0;
    const isExpanded = expandedItems[item.id || item.text];
    const commonStyles = {
      pl: open ? depth * 4 + 2 : 2,
      mb: 1,
      ...(item.sx || {}),
    };

    return (
      <React.Fragment key={item.id || item.text}>
        <ListItem
          button
          component={hasSubItems ? 'div' : StyledNavLink}
          to={hasSubItems ? undefined : item.path}
          onClick={hasSubItems ? () => handleExpandClick(item.id || item.text) : undefined}
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
                noWrap: true,
              }}
            />
          )}
          {hasSubItems && open && (
            isExpanded ? <ExpandLess /> : <ExpandMore />
          )}
        </ListItem>

        {hasSubItems && (
          <Collapse in={isExpanded} timeout="auto" unmountOnExit>
            <List component="div" disablePadding>
              {item.subItems.map((subItem) => renderMenuItem(subItem, depth + 1))}
            </List>
          </Collapse>
        )}
      </React.Fragment>
    );
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
          backgroundColor: theme.palette.background.default,
          boxShadow: "none",
          border: "none",
          padding: "16px",
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
          backgroundColor: theme.palette.grey[100],
          alignSelf: "flex-end",
          mr: 1,
          position: "absolute",
          left: "calc(100% - 20px)",
          top: "78px",
          zIndex: 9,
          "&:hover": {
            backgroundColor: theme.palette.grey[200],
          },
        }}
      >
        {open ? <ChevronLeftIcon /> : <ChevronRightIcon />}
      </IconButton>
      <List
        sx={{
          mt: 2,
          backgroundColor: theme.palette.background.paper,
          borderRadius: "16px",
          boxShadow: theme.shadows[1],
          padding: "16px",
          "& .MuiListItem-root": {
            borderRadius: "8px",
            "&:hover": {
              backgroundColor: theme.palette.action.hover,
            },
            "&.active": {
              backgroundColor: theme.palette.primary.main,
              color: theme.palette.primary.contrastText,
              "& .MuiListItemIcon-root": {
                color: theme.palette.primary.contrastText,
              },
            },
          },
        }}
      >
        {menuItems.map((item) => renderMenuItem(item))}
      </List>
    </Drawer>
  );
};

export default BaseDrawer; 