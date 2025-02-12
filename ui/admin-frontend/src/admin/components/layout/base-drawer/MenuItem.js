import React from 'react';
import PropTypes from 'prop-types';
import {
  List,
  ListItemText,
  Collapse,
} from '@mui/material';
import ExpandLess from '@mui/icons-material/ExpandLess';
import ExpandMore from '@mui/icons-material/ExpandMore';
import { StyledNavLink } from '../../../styles/sharedStyles';
import { StyledListItem, ListItemIcon } from './styles';

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

  if (hasSubItems) {
    return (
      <React.Fragment key={itemId}>
        <StyledListItem
          onClick={() => onExpandClick(itemId, parentId)}
          depth={depth}
        >
          {item.icon && <ListItemIcon>{item.icon}</ListItemIcon>}
          {open && (
            <ListItemText
              primary={item.text}
              primaryTypographyProps={{
                variant: depth > 0 ? 'body2' : 'body1',
              }}
            />
          )}
          {open && (isExpanded ? <ExpandLess /> : <ExpandMore />)}
        </StyledListItem>
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
    <StyledListItem
      component={StyledNavLink}
      to={item.path}
      depth={depth}
      onClick={() => onPathSelect(item.path)}
      {...(item.path === '/admin/' ? { end: true } : {})}
    >
      {item.icon && <ListItemIcon>{item.icon}</ListItemIcon>}
      {open && (
        <ListItemText
          primary={item.text}
          primaryTypographyProps={{
            variant: depth > 0 ? 'body2' : 'body1',
          }}
        />
      )}
    </StyledListItem>
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
