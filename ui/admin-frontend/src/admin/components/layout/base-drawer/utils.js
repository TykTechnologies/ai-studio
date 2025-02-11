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
