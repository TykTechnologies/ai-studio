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
import { ParentListItem, SubListItem, ListItemIcon } from './styles';

const MenuItem = ({ 
  item, 
  depth = 0, 
  parentId = null,
  rootParentId = null,
  open,
  expandedItems,
  onExpandClick,
  onPathSelect,
  selectedPath,
  disableRipple,
  isFirstItem,
}) => {
  const itemId = item.id || item.text;
  const hasSubItems = item.subItems;
  const isExpanded = expandedItems[itemId];
  const immediateParentId = parentId || itemId;

  const isSelected = selectedPath === item.path || 
    (hasSubItems && item.subItems?.some(subItem => {
      if (subItem.path === selectedPath) return true;
      if (subItem.subItems) {
        return subItem.subItems.some(deepSubItem => deepSubItem.path === selectedPath);
      }
      return false;
    }));

  const ListItemComponent = depth === 0 ? ParentListItem : SubListItem;

  if (hasSubItems) {
    return (
      <React.Fragment key={itemId}>
        <ListItemComponent
          onClick={() => onExpandClick(itemId, parentId)}
          depth={depth}
          selected={isSelected}
          disableRipple
          disableTouchRipple
          isParent={depth === 0}
          rootParentId={immediateParentId}
          itemId={itemId}
          hasSubItems={hasSubItems}
          open={open}
          isFirstItem={depth === 0 && isFirstItem}
        >
          {item.icon && <ListItemIcon>{item.icon}</ListItemIcon>}
          <ListItemText
            primary={item.text}
            primaryTypographyProps={{
              variant: depth > 0 ? 'body2' : 'body1',
            }}
          />
          {isExpanded ? <ExpandLess /> : <ExpandMore />}
        </ListItemComponent>
        <Collapse in={isExpanded} timeout="auto" unmountOnExit>
          <List component="div" disablePadding>
            {item.subItems.map((subItem, index) => (
              <MenuItem
                key={subItem.id || subItem.text}
                item={subItem}
                depth={depth + 1}
                parentId={item.id}
                rootParentId={immediateParentId}
                open={open}
                expandedItems={expandedItems}
                onExpandClick={onExpandClick}
                onPathSelect={onPathSelect}
                selectedPath={selectedPath}
                isFirstItem={index === 0}
              />
            ))}
          </List>
        </Collapse>
      </React.Fragment>
    );
  }

  return (
    <ListItemComponent
      component={StyledNavLink}
      to={item.path}
      depth={depth}
      onClick={() => onPathSelect(item.path)}
      selected={selectedPath === item.path}
      disableRipple
      disableTouchRipple
      rootParentId={immediateParentId}
      itemId={itemId}
      hasSubItems={hasSubItems}
      open={open}
      isFirstItem={depth === 0 && isFirstItem}
      {...(item.path === '/admin/' ? { end: true } : {})}
    >
      {item.icon && <ListItemIcon>{item.icon}</ListItemIcon>}
      <ListItemText
        primary={item.text}
        primaryTypographyProps={{
          variant: depth > 0 ? 'body2' : 'body1',
        }}
      />
    </ListItemComponent>
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
  rootParentId: PropTypes.string,
  open: PropTypes.bool.isRequired,
  expandedItems: PropTypes.object.isRequired,
  onExpandClick: PropTypes.func.isRequired,
  onPathSelect: PropTypes.func.isRequired,
  selectedPath: PropTypes.string,
  disableRipple: PropTypes.bool,
  isFirstItem: PropTypes.bool,
};

export default React.memo(MenuItem);
