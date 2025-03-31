/**
 * Utility functions for clipboard operations
 */

/**
 * Copies text to clipboard with optional notification callback
 * 
 * @param {string} text - The text to copy to clipboard
 * @param {string} [fieldName] - Optional field name for notification purposes
 * @param {Function} [onSuccess] - Optional callback function to handle success notification
 * @param {Function} [onError] - Optional callback function to handle error notification
 * @returns {Promise<boolean>} - Success status
 */
export const copyToClipboard = async (text, fieldName, onSuccess, onError) => {
  try {
    await navigator.clipboard.writeText(text);
    
    // Log to console
    console.log(`Text${fieldName ? ` (${fieldName})` : ''} copied to clipboard`);
    
    // Call success callback if provided
    onSuccess?.(fieldName);
    
    return true;
  } catch (err) {
    // Log error to console
    console.error(`Failed to copy text${fieldName ? ` (${fieldName})` : ''}: `, err);
    
    // Call error callback if provided
    onError?.(fieldName, err);
    
    return false;
  }
};