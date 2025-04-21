/**
 * Creates an onClick handler for documentation links
 * @param {Function} getDocsLink - Function to get the documentation link from a key
 * @param {string} linkKey - The key for the documentation link
 * @returns {Function} - onClick handler that opens the documentation link in a new tab
 */
export const createDocsLinkHandler = (getDocsLink, linkKey) => {
  return () => {
    const link = getDocsLink(linkKey);
    if (link) {
      window.open(link, '_blank');
    }
  };
};