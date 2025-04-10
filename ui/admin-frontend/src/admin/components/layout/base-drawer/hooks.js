import { useState, useEffect } from 'react';
import { useLocation } from 'react-router-dom';

export const useDrawerState = (storageKey, defaultOpen, defaultExpandedItems) => {
  const getInitialState = () => {
    try {
      const savedState = localStorage.getItem(storageKey);
      if (savedState) {
        const state = JSON.parse(savedState);
        return {
          open: state.isOpen ?? defaultOpen,
          expanded: state.expanded ?? defaultExpandedItems,
          selectedPath: state.selectedPath ?? '',
        };
      }
    } catch (error) {
      console.error('Error reading from localStorage:', error);
    }
    return {
      open: defaultOpen,
      expanded: defaultExpandedItems,
      selectedPath: '',
    };
  };

  const initialState = getInitialState();
  const [open, setOpen] = useState(initialState.open);
  const [expandedItems, setExpandedItems] = useState(initialState.expanded);
  const [selectedPath, setSelectedPath] = useState(initialState.selectedPath);
  const location = useLocation();
  
  // Update selectedPath when location changes
  useEffect(() => {
    const path = location.pathname;
    setSelectedPath(path);
  }, [location.pathname]);

  useEffect(() => {
    try {
      const currentState = JSON.parse(localStorage.getItem(storageKey) || '{}');
      localStorage.setItem(
        storageKey,
        JSON.stringify({
          ...currentState,
          isOpen: open,
          expanded: expandedItems,
          selectedPath,
        })
      );
    } catch (error) {
      console.error('Error updating drawer state:', error);
    }
  }, [open, expandedItems, selectedPath, storageKey]);

  const handleDrawerToggle = () => setOpen(!open);
  
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

  const handlePathSelect = (path) => {
    setSelectedPath(path);
  };

  return {
    open,
    expandedItems,
    selectedPath,
    handleDrawerToggle,
    handleExpandClick,
    handlePathSelect,
  };
};
