import { useState, useEffect, useCallback } from 'react';
import apiClient from '../utils/apiClient';

/**
 * Hook for fetching LLMs with pagination and sorting support
 * @param {Object} options - Configuration options
 * @param {boolean} options.skipInitialFetch - Whether to skip the initial fetch
 * @param {number} options.page - Page number for pagination
 * @param {number} options.pageSize - Number of items per page
 * @param {string} options.sortBy - Field to sort by
 * @param {string} options.sortDirection - Sort direction ('asc' or 'desc')
 * @param {boolean} options.checkExistenceOnly - If true, only fetch one item to check if LLMs exist
 * @returns {Object} LLMs data and control functions
 */
const useLLMs = ({
  skipInitialFetch = false,
  page = 1,
  pageSize = 10,
  sortBy = null,
  sortDirection = 'asc',
  checkExistenceOnly = false,
} = {}) => {
  const [llms, setLLMs] = useState([]);
  const [loading, setLoading] = useState(!skipInitialFetch);
  const [error, setError] = useState(null);
  const [totalCount, setTotalCount] = useState(0);
  const [totalPages, setTotalPages] = useState(0);

  const fetchLLMs = useCallback(async (fetchOptions = {}) => {
    try {
      setLoading(true);
      
      // Merge default options with any provided fetch options
      const options = {
        page: fetchOptions.page || page,
        pageSize: fetchOptions.pageSize || (checkExistenceOnly ? 1 : pageSize),
        sortBy: fetchOptions.sortBy || sortBy,
        sortDirection: fetchOptions.sortDirection || sortDirection,
      };
      
      const response = await apiClient.get('/llms', {
        params: {
          page: options.page,
          page_size: options.pageSize,
          sort_by: options.sortBy,
          sort_direction: options.sortDirection,
        },
      });
      
      const data = response.data.data || [];
      setLLMs(data);
      
      // Parse pagination headers if they exist
      if (response.headers['x-total-count'] && response.headers['x-total-pages']) {
        const count = parseInt(response.headers['x-total-count'], 10);
        const pages = parseInt(response.headers['x-total-pages'], 10);
        setTotalCount(count);
        setTotalPages(pages);
      }
      
      setError(null);
      return data;
    } catch (error) {
      console.error('Error fetching LLMs', error);
      setError('Failed to load LLMs');
      throw error;
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, sortBy, sortDirection, checkExistenceOnly]);

  useEffect(() => {
    if (!skipInitialFetch) {
      fetchLLMs();
    }
  }, [fetchLLMs, skipInitialFetch]);

  return {
    llms,
    loading,
    error,
    totalCount,
    totalPages,
    fetchLLMs,
    hasLLMs: llms.length > 0,
  };
};

export default useLLMs;