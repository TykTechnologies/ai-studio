import { useState, useEffect } from 'react';

export const useDrawerState = (storageKey, defaultOpen, defaultExpandedItems) => {
  const getInitialState = () => {
    try {
      const savedState = localStorage.getItem(storageKey);
      if (savedState) {
        const state = JSON.parse(savedState);
        return {
          open: state.isOpen ?? defaultOpen,
          expanded: state.expanded ?? defaultExpandedItems,
        };
      }
    } catch (error) {
      console.error('Error reading from localStorage:', error);
    }
    return {
      open: defaultOpen,
      expanded: defaultExpandedItems,
    };
  };

  const initialState = getInitialState();
  const [open, setOpen] = useState(initialState.open);
  const [expandedItems, setExpandedItems] = useState(initialState.expanded);

  useEffect(() => {
    try {
      const currentState = JSON.parse(localStorage.getItem(storageKey) || '{}');
      localStorage.setItem(
        storageKey,
        JSON.stringify({
          ...currentState,
          isOpen: open,
          expanded: expandedItems,
        })
      );
    } catch (error) {
      console.error('Error updating drawer state:', error);
    }
  }, [open, expandedItems, storageKey]);

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

  return {
    open,
    expandedItems,
    handleDrawerToggle,
    handleExpandClick,
  };
};
