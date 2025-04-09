/**
 * Cache service for storing and retrieving data with expiry
 */
const cacheService = {
  /**
   * Default expiry time in milliseconds (1 minute)
   */
  DEFAULT_EXPIRY: 60000,

  /**
   * Get data from cache
   * @param {string} key - The cache key
   * @returns {any|null} - The cached data or null if not found or expired
   */
  get(key) {
    const cachedItem = localStorage.getItem(key);
    if (!cachedItem) return null;

    try {
      const { data, timestamp, expiry } = JSON.parse(cachedItem);
      
      if (this.isExpired(timestamp, expiry)) {
        this.remove(key);
        return null;
      }
      
      return data;
    } catch (error) {
      console.error(`Error parsing cached item for key ${key}:`, error);
      this.remove(key);
      return null;
    }
  },

  /**
   * Store data in cache with expiry
   * @param {string} key - The cache key
   * @param {any} data - The data to cache
   * @param {number} expiry - Expiry time in milliseconds (defaults to 1 minute)
   */
  set(key, data, expiry = this.DEFAULT_EXPIRY) {
    try {
      const cacheItem = {
        data,
        timestamp: Date.now(),
        expiry
      };
      
      localStorage.setItem(key, JSON.stringify(cacheItem));
    } catch (error) {
      console.error(`Error setting cache for key ${key}:`, error);
    }
  },

  /**
   * Remove item from cache
   * @param {string} key - The cache key to remove
   */
  remove(key) {
    localStorage.removeItem(key);
  },

  /**
   * Clear all cache items with a specific prefix
   * @param {string} prefix - The prefix to match (optional)
   */
  clear(prefix) {
    if (prefix) {
      for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i);
        if (key && key.startsWith(prefix)) {
          localStorage.removeItem(key);
        }
      }
    } else {
      localStorage.clear();
    }
  },

  /**
   * Check if a cached item is expired
   * @param {number} timestamp - The timestamp when the item was cached
   * @param {number} expiry - The expiry time in milliseconds
   * @returns {boolean} - True if expired, false otherwise
   */
  isExpired(timestamp, expiry) {
    return Date.now() - timestamp > expiry;
  }
};

export default cacheService;