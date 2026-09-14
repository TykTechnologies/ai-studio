import React, { memo } from 'react';
import PropTypes from 'prop-types';
import {
  List,
  ListItemText,
  Collapse,
} from '@mui/material';
import ExpandLess from '@mui/icons-material/ExpandLess';
import ExpandMore from '@mui/icons-material/ExpandMore';
import { Link } from 'react-router-dom';
import { ParentListItem, SubListItem, ListItemIcon } from './styles';
import { menuItemKey } from './utils';

/**
 * One node of the drawer. Which node is highlighted is decided once per
 * drawer (see useDrawerState / findSelectedItem) and handed down as
 * `selectedKey`; a node is selected when it is that item or one of its
 * ancestors. Nothing here matches paths itself, so two entries can never
 * light up at once.
 */
const MenuItem = ({
  item,
  depth = 0,
  parentId = null,
  parentKey = null,
  rootParentId = null,
  open,
  expandedItems,
  onExpandClick,
  selectedKey,
  isFirstItem,
}) => {
  const itemId = item.id || item.text;
  const itemKey = menuItemKey(item, parentKey);
  const hasSubItems = item.subItems;
  const isExpanded = expandedItems[itemId];
  const immediateParentId = parentId || itemId;

  const isSelected = Boolean(
    selectedKey && (selectedKey === itemKey || selectedKey.startsWith(`${itemKey}>`))
  );

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
          data-nav-depth={depth}
          data-nav-id={itemId}
          data-nav-selected={isSelected ? 'true' : undefined}
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
                parentKey={itemKey}
                rootParentId={immediateParentId}
                open={open}
                expandedItems={expandedItems}
                onExpandClick={onExpandClick}
                selectedKey={selectedKey}
                isFirstItem={index === 0}
              />
            ))}
          </List>
        </Collapse>
      </React.Fragment>
    );
  }

  // A plain Link rather than NavLink: NavLink applies its own `.active`
  // class from a prefix match of its own, which is exactly how "/admin"
  // and "/admin/apps" ended up highlighted together.
  return (
    <ListItemComponent
      component={Link}
      to={item.path}
      depth={depth}
      selected={isSelected}
      disableRipple
      disableTouchRipple
      open={open}
      isFirstItem={depth === 0 && isFirstItem}
      className={isSelected ? 'active' : ''}
      style={{ textDecoration: 'none', color: 'inherit' }}
      aria-current={isSelected ? 'page' : undefined}
      data-nav-depth={depth}
      data-nav-id={itemId}
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
    permission: PropTypes.oneOfType([PropTypes.string, PropTypes.arrayOf(PropTypes.string)]),
  }).isRequired,
  depth: PropTypes.number,
  parentId: PropTypes.string,
  parentKey: PropTypes.string,
  rootParentId: PropTypes.string,
  open: PropTypes.bool.isRequired,
  expandedItems: PropTypes.object.isRequired,
  onExpandClick: PropTypes.func.isRequired,
  selectedKey: PropTypes.string,
  isFirstItem: PropTypes.bool,
};

export default memo(MenuItem);
