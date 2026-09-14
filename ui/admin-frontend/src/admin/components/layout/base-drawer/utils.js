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

/**
 * Key that identifies one node of the menu tree. Ids are unique within a
 * group but not across groups ("Apps" can live under two different parents),
 * so a node is addressed by the chain of ids from the root.
 * @param {object} item
 * @param {string|null} parentKey
 * @returns {string}
 */
export const menuItemKey = (item, parentKey = null) => {
  const own = item.id || item.text;
  return parentKey ? `${parentKey}>${own}` : own;
};

/**
 * How well an item's path matches the current location, as the number of
 * matched characters, or -1 when it does not match at all.
 *
 * - a path with a query string must equal pathname + search exactly
 * - an `exact` item must equal the pathname exactly (so "/admin" does not
 *   light up on every admin page)
 * - anything else matches its own pathname or any route beneath it
 */
const matchLength = (item, pathname, fullPath) => {
  if (!item?.path) return -1;
  if (item.path.includes('?')) {
    return item.path === fullPath ? item.path.length : -1;
  }
  if (item.exact) {
    return item.path === pathname ? item.path.length : -1;
  }
  if (pathname === item.path || pathname.startsWith(`${item.path}/`)) {
    return item.path.length;
  }
  return -1;
};

/**
 * Picks the single menu item a location should highlight: the item whose
 * path is the longest match for the current pathname. "/admin/apps/1"
 * therefore selects "Apps" (and not "Overview" at "/admin"), and
 * "/admin/catalogs/llms/2" selects the catalog entry rather than the
 * "/admin/llms" one. Ties go to the first item in menu order.
 *
 * @param {Array} items - menu tree ({ id, text, path, exact, subItems })
 * @param {string} pathname - location.pathname
 * @param {string} [search] - location.search, for items whose path carries a query string
 * @returns {{ key: string, item: object, ancestorKeys: string[], ancestorIds: string[] } | null}
 */
export const findSelectedItem = (items, pathname, search = '') => {
  const fullPath = `${pathname || ''}${search || ''}`;
  let best = null;

  const visit = (list, ancestors) => {
    for (const item of list || []) {
      if (!item) continue;
      const parent = ancestors[ancestors.length - 1];
      const key = menuItemKey(item, parent ? parent.key : null);
      const length = matchLength(item, pathname, fullPath);
      if (length > (best ? best.length : -1)) {
        best = {
          key,
          item,
          length,
          ancestorKeys: ancestors.map((a) => a.key),
          ancestorIds: ancestors.map((a) => a.id),
        };
      }
      if (Array.isArray(item.subItems) && item.subItems.length > 0) {
        visit(item.subItems, [...ancestors, { key, id: item.id || item.text }]);
      }
    }
  };

  visit(items, []);
  if (!best) return null;
  const { length, ...selection } = best;
  return selection;
};
