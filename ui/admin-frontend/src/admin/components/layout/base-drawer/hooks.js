import { useState, useEffect, useMemo } from 'react';
import { useLocation } from 'react-router-dom';
import { findSelectedItem } from './utils';

export const useDrawerState = (storageKey, defaultOpen, defaultExpandedItems, menuItems = []) => {
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
  // The last visited path is persisted so the tab switcher in MainLayout can
  // return to it; it is NOT what decides the highlight (see `selection`).
  const [selectedPath, setSelectedPath] = useState(initialState.selectedPath);
  const location = useLocation();

  // The highlighted item is derived from the URL on every render rather than
  // from stored state, so a deep link, a browser back button or a redirect
  // all light up the right entry. Longest path wins: /admin/apps/1 selects
  // "Apps", not "Overview".
  const selection = useMemo(
    () => findSelectedItem(menuItems, location.pathname, location.search),
    [menuItems, location.pathname, location.search]
  );
  const selectedKey = selection?.key || null;

  useEffect(() => {
    // Include search params so a chat room link (continue_id) is restored too.
    setSelectedPath(location.pathname + location.search);

    // Open every group on the way to the selected item, leaving the groups
    // the user expanded or collapsed themselves as they were.
    const parentIds = selection?.ancestorIds || [];
    if (parentIds.length > 0) {
      setExpandedItems((prevState) => {
        if (parentIds.every((id) => prevState[id])) {
          return prevState;
        }
        const newState = { ...prevState };
        parentIds.forEach((parentId) => {
          newState[parentId] = true;
        });
        return newState;
      });
    }
  }, [location.pathname, location.search, selection]);

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

  return {
    open,
    expandedItems,
    selectedKey,
    handleDrawerToggle,
    handleExpandClick,
  };
};
