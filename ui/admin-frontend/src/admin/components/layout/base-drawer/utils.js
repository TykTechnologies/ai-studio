/**
 * Generates a random ID for the drawer instance
 * @returns {string} Random string ID
 */
export const generateRandomId = () => Math.random().toString(36).substring(7);

/**
 * Saves the selected path to localStorage
 * @param {string} storageKey - The key used for localStorage
 * @param {string} path - The path to save
 */
export const saveSelectedPath = (storageKey, path) => {
  try {
    const currentState = JSON.parse(localStorage.getItem(storageKey) || '{}');
    localStorage.setItem(
      storageKey,
      JSON.stringify({
        ...currentState,
        selectedPath: path,
      })
    );
  } catch (error) {
    console.error('Error saving selected path:', error);
  }
};

export const findParentItemsForPath = (items, currentPath) => {
  const result = [];
  
  for (const item of items) {
    if (item.subItems?.length > 0) {
      const hasMatchingChild = item.subItems?.some(subItem =>
        subItem.path === currentPath || currentPath?.startsWith(subItem.path + '/')
      );
      
      if (hasMatchingChild && item.id) {
        result.push(item.id);
      }
    }
  }
  
  return result;
};

/**
 * Drops menu items the user may not see. A leaf is kept when isItemAllowed
 * returns true for it; a group is kept only if at least one of its sub-items
 * survives (or it has a path of its own and is allowed).
 * @param {Array} items - menu items ({ id, text, path, permission, subItems })
 * @param {(item: object) => boolean} isItemAllowed
 * @returns {Array} filtered items
 */
export const filterMenuItems = (items, isItemAllowed = () => true) => {
  const out = [];
  for (const item of items || []) {
    if (!item) continue;
    if (Array.isArray(item.subItems) && item.subItems.length > 0) {
      const subItems = filterMenuItems(item.subItems, isItemAllowed);
      if (subItems.length === 0) continue;
      if (item.permission && !isItemAllowed(item)) continue;
      out.push({ ...item, subItems });
      continue;
    }
    if (Array.isArray(item.subItems) && item.subItems.length === 0 && !item.path) {
      // A declared-but-empty group is nothing to show.
      continue;
    }
    if (isItemAllowed(item)) {
      out.push(item);
    }
  }
  return out;
};
