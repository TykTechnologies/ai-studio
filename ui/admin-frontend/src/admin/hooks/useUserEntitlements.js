import { useState, useEffect, useCallback } from 'react';
import pubClient from '../utils/pubClient';
import cacheService from '../utils/cacheService';

const ENTITLEMENTS_CACHE_KEY = 'tyk_ai_studio_admin_userEntitlements';

const useUserEntitlements = (skipInitialFetch = false) => {
  const [userEntitlements, setUserEntitlements] = useState(null);
  const [uiOptions, setUiOptions] = useState(null);
  const [loading, setLoading] = useState(!skipInitialFetch);
  const [error, setError] = useState(null);

  const fetchUserEntitlements = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    const cachedData = cacheService.get(ENTITLEMENTS_CACHE_KEY);
    if (cachedData) {
      setUserEntitlements(cachedData);
      setUiOptions(cachedData.ui_options);
      setLoading(false);
      return cachedData;
    }

    return pubClient.get('/common/me')
      .then(response => {
        const newData = response.data.attributes.entitlements;
        const newUiOptions = response.data.attributes.ui_options;
        
        setUserEntitlements(newData);
        setUiOptions(newUiOptions);
        
        const dataToCache = { ...newData, ui_options: newUiOptions };
        cacheService.set(ENTITLEMENTS_CACHE_KEY, dataToCache, 10000); // 10 seconds expiry
        
        return newData;
      })
      .catch(error => {
        console.error('Failed to fetch user entitlements:', error);
        setError(error);
        throw error;
      })
      .finally(() => {
        setLoading(false);
      });
  }, []);

  useEffect(() => {
    if (!skipInitialFetch) {
      fetchUserEntitlements();
    }
  }, [fetchUserEntitlements, skipInitialFetch]);

  return {
    userEntitlements,
    uiOptions,
    loading,
    error,
    fetchUserEntitlements
  };
};

export default useUserEntitlements;
