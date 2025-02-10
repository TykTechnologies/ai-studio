import React from 'react';
import PropTypes from 'prop-types';
import {
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Collapse,
} from '@mui/material';
import ExpandLess from '@mui/icons-material/ExpandLess';
import ExpandMore from '@mui/icons-material/ExpandMore';
import { StyledNavLink } from '../../../styles/sharedStyles';

const MenuItem = ({ 
  item, 
  depth = 0, 
  parentId = null,
  open,
  expandedItems,
  onExpandClick,
  onPathSelect,
}) => {
  const itemId = item.id || item.text;
  const hasSubItems = item.subItems;
  const isExpanded = expandedItems[itemId];
  
  const commonStyles = {
    pl: open ? depth * 4 + 2 : 2,
  };

  if (hasSubItems) {
    return (
      <React.Fragment key={itemId}>
        <ListItem disablePadding>
          <ListItemButton
            onClick={() => onExpandClick(itemId, parentId)}
            sx={{ ...commonStyles, cursor: 'pointer' }}
          >
            {item.icon && <ListItemIcon>{item.icon}</ListItemIcon>}
            {open && (
              <ListItemText
                primary={item.text}
                primaryTypographyProps={{
                  variant: depth > 0 ? 'body2' : 'body1',
                  color: depth > 0 ? 'text.secondary' : 'text.primary',
                }}
              />
            )}
            {open && (isExpanded ? <ExpandLess /> : <ExpandMore />)}
          </ListItemButton>
        </ListItem>
        <Collapse in={isExpanded} timeout="auto" unmountOnExit>
          <List component="div" disablePadding>
            {item.subItems.map((subItem) => (
              <MenuItem
                key={subItem.id || subItem.text}
                item={subItem}
                depth={depth + 1}
                parentId={item.id}
                open={open}
                expandedItems={expandedItems}
                onExpandClick={onExpandClick}
                onPathSelect={onPathSelect}
              />
            ))}
          </List>
        </Collapse>
      </React.Fragment>
    );
  }

  return (
    <ListItem disablePadding>
      <ListItemButton
        component={StyledNavLink}
        to={item.path}
        sx={commonStyles}
        onClick={() => onPathSelect(item.path)}
        {...(item.path === '/admin/' ? { end: true } : {})}
      >
        {item.icon && <ListItemIcon>{item.icon}</ListItemIcon>}
        {open && (
          <ListItemText
            primary={item.text}
            primaryTypographyProps={{
              variant: depth > 0 ? 'body2' : 'body1',
              color: depth > 0 ? 'text.secondary' : 'text.primary',
            }}
          />
        )}
      </ListItemButton>
    </ListItem>
  );
};

MenuItem.propTypes = {
  item: PropTypes.shape({
    id: PropTypes.string,
    text: PropTypes.string.isRequired,
    path: PropTypes.string,
    icon: PropTypes.node,
    subItems: PropTypes.array,
  }).isRequired,
  depth: PropTypes.number,
  parentId: PropTypes.string,
  open: PropTypes.bool.isRequired,
  expandedItems: PropTypes.object.isRequired,
  onExpandClick: PropTypes.func.isRequired,
  onPathSelect: PropTypes.func.isRequired,
};

export default React.memo(MenuItem);
