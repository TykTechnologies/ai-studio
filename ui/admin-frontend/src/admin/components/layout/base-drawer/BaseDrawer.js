import React, { useMemo } from 'react';
import PropTypes from 'prop-types';
import { List, Toolbar } from '@mui/material';
import { ThemeProvider } from '@mui/material/styles';
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import adminTheme from '../../../theme';
import { StyledDrawer, ToggleButton, MenuList } from './styles';
import { useDrawerState } from './hooks';
import MenuItem from './MenuItem';
import { generateRandomId, filterMenuItems } from './utils';

const BaseDrawer = ({
  id = generateRandomId(),
  menuItems,
  drawerWidth = 260,
  minimizedWidth = 60,
  showToolbar = true,
  customStyles = {},
  defaultOpen = true,
  defaultExpandedItems = {},
  isItemAllowed,
}) => {
  const STORAGE_KEY = `drawer_state_${id}`;

  // Permission filtering happens once here so every drawer (admin, chat,
  // portal, plugin sections) shares the same rule: items carry an optional
  // `permission` and the caller decides who may see them.
  const visibleItems = useMemo(
    () => (isItemAllowed ? filterMenuItems(menuItems, isItemAllowed) : menuItems),
    [menuItems, isItemAllowed]
  );
  
  const {
    open,
    expandedItems,
    selectedPath,
    handleDrawerToggle,
    handleExpandClick,
    handlePathSelect,
  } = useDrawerState(STORAGE_KEY, defaultOpen, defaultExpandedItems, visibleItems);

  const currentWidth = open ? drawerWidth : minimizedWidth;

  return (
    <ThemeProvider theme={adminTheme}>
      <StyledDrawer
        variant="permanent"
        width={currentWidth}
        open={open}
      >
        {showToolbar && <Toolbar />}
        <ToggleButton onClick={handleDrawerToggle}>
          {open ? <ChevronLeftIcon /> : <ChevronRightIcon />}
        </ToggleButton>
        <MenuList customMarginTop={customStyles.marginTop} open={open}>
          <List>
            {visibleItems.map((item, index) => (
              <MenuItem
                key={item.id || item.text}
                item={item}
                open={open}
                expandedItems={expandedItems}
                onExpandClick={handleExpandClick}
                onPathSelect={handlePathSelect}
                selectedPath={selectedPath}
                isFirstItem={index === 0}
              />
            ))}
          </List>
        </MenuList>
      </StyledDrawer>
    </ThemeProvider>
  );
};

BaseDrawer.propTypes = {
  id: PropTypes.string,
  menuItems: PropTypes.arrayOf(
    PropTypes.shape({
      id: PropTypes.string,
      text: PropTypes.string.isRequired,
      path: PropTypes.string,
      icon: PropTypes.node,
      subItems: PropTypes.array,
      permission: PropTypes.oneOfType([PropTypes.string, PropTypes.arrayOf(PropTypes.string)]),
    })
  ).isRequired,
  isItemAllowed: PropTypes.func,
  drawerWidth: PropTypes.number,
  minimizedWidth: PropTypes.number,
  showToolbar: PropTypes.bool,
  customStyles: PropTypes.object,
  defaultOpen: PropTypes.bool,
  defaultExpandedItems: PropTypes.object,
};

export default BaseDrawer;
