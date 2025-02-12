import React from 'react';
import PropTypes from 'prop-types';
import { List, Toolbar, Divider } from '@mui/material';
import { ThemeProvider } from '@mui/material/styles';
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import adminTheme from '../../../theme';
import { StyledDrawer, ToggleButton, MenuList } from './styles';
import { useDrawerState } from './hooks';
import MenuItem from './MenuItem';
import { generateRandomId, saveSelectedPath } from './utils';

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
  
  const {
    open,
    expandedItems,
    selectedPath,
    handleDrawerToggle,
    handleExpandClick,
    handlePathSelect,
  } = useDrawerState(STORAGE_KEY, defaultOpen, defaultExpandedItems);

  const currentWidth = open ? drawerWidth : minimizedWidth;

  return (
    <ThemeProvider theme={adminTheme}>
      <StyledDrawer
        variant="permanent"
        width={currentWidth}
      >
        {showToolbar && <Toolbar />}
        <ToggleButton onClick={handleDrawerToggle}>
          {open ? <ChevronLeftIcon /> : <ChevronRightIcon />}
        </ToggleButton>
        <MenuList customMarginTop={customStyles.marginTop}>
          <List>
            {menuItems.map((item) => (
              <MenuItem
                key={item.id || item.text}
                item={item}
                open={open}
                expandedItems={expandedItems}
                onExpandClick={handleExpandClick}
                onPathSelect={handlePathSelect}
                selectedPath={selectedPath}
              />
            ))}
          </List>
          <Divider />
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
    })
  ).isRequired,
  drawerWidth: PropTypes.number,
  minimizedWidth: PropTypes.number,
  showToolbar: PropTypes.bool,
  customStyles: PropTypes.object,
  defaultOpen: PropTypes.bool,
  defaultExpandedItems: PropTypes.object,
};

export default BaseDrawer;
