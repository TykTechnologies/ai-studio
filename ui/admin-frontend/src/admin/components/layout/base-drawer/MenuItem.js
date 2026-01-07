import React, { memo } from 'react';
import PropTypes from 'prop-types';
import {
  List,
  ListItemText,
  Collapse,
} from '@mui/material';
import ExpandLess from '@mui/icons-material/ExpandLess';
import ExpandMore from '@mui/icons-material/ExpandMore';
import { StyledNavLink } from '../../../styles/sharedStyles';
import { Link } from 'react-router-dom';
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
  isFirstItem,
}) => {
  const itemId = item.id || item.text;
  const hasSubItems = item.subItems;
  const isExpanded = expandedItems[itemId];
  const immediateParentId = parentId || itemId;

  const pathMatches = (itemPath, currentPath, exact = false) => {
    if (!itemPath || !currentPath) return false;
    if (exact) return itemPath === currentPath;

    // Extract pathname without query string for comparison
    const itemPathname = itemPath.split('?')[0];
    const currentPathname = currentPath.split('?')[0];

    // If item has query params, require exact match
    if (itemPath.includes('?')) {
      return itemPath === currentPath;
    }

    // Otherwise, match pathname or pathname prefix
    return itemPathname === currentPathname || currentPathname.startsWith(itemPathname + '/');
  };

  const isItemSelected = (item, currentPath) => {
    // Respect exact flag on items
    if (item.exact) {
      if (item.path === currentPath) return true;
    } else if (pathMatches(item.path, currentPath, item.exact)) {
      return true;
    }
    if (item.subItems) {
      return item.subItems.some(subItem => isItemSelected(subItem, currentPath));
    }
    return false;
  };

  const isSelected = item.exact
    ? selectedPath === item.path
    : isItemSelected(item, selectedPath);
    
  const ListItemComponent = depth === 0 ? ParentListItem : SubListItem;

  if (hasSubItems) {
    const handleItemClick = (e) => {
      e.stopPropagation();
      onExpandClick(itemId, parentId);
    };

    return (
      <React.Fragment key={itemId}>
        <ListItemComponent
          onClick={handleItemClick}
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

  // Determine if this item needs exact matching (has query params or exact flag)
  const needsExactMatch = item.exact || item.path?.includes('?');
  const isLinkSelected = pathMatches(item.path, selectedPath, needsExactMatch);

  // For items needing exact match, use plain Link to avoid NavLink's automatic .active class
  // For regular items, use NavLink with end prop for proper path matching
  if (needsExactMatch) {
    return (
      <ListItemComponent
        component={Link}
        to={item.path}
        depth={depth}
        onClick={() => onPathSelect(item.path)}
        selected={isLinkSelected}
        disableRipple
        disableTouchRipple
        open={open}
        isFirstItem={depth === 0 && isFirstItem}
        className={isLinkSelected ? 'active' : ''}
        style={{ textDecoration: 'none', color: 'inherit' }}
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
  }

  return (
    <ListItemComponent
      component={StyledNavLink}
      to={item.path}
      depth={depth}
      onClick={() => onPathSelect(item.path)}
      selected={isLinkSelected}
      disableRipple
      disableTouchRipple
      open={open}
      isFirstItem={depth === 0 && isFirstItem}
      end
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
    exact: PropTypes.bool,
  }).isRequired,
  depth: PropTypes.number,
  parentId: PropTypes.string,
  rootParentId: PropTypes.string,
  open: PropTypes.bool.isRequired,
  expandedItems: PropTypes.object.isRequired,
  onExpandClick: PropTypes.func.isRequired,
  onPathSelect: PropTypes.func.isRequired,
  selectedPath: PropTypes.string,
  isFirstItem: PropTypes.bool,
};

export default memo(MenuItem);
