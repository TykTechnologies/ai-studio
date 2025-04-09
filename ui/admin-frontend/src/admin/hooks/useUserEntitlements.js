import { useState, useEffect, useCallback } from 'react';
import pubClient from '../utils/pubClient';

const ENTITLEMENTS_CACHE_KEY = 'tyk_ai_studio_admin_userEntitlements';
const CACHE_EXPIRY = 10000;

const useUserEntitlements = (skipInitialFetch = false) => {
  const [userEntitlements, setUserEntitlements] = useState(null);
  const [uiOptions, setUiOptions] = useState(null);
  const [loading, setLoading] = useState(!skipInitialFetch);
  const [error, setError] = useState(null);

  const fetchUserEntitlements = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    const cachedData = localStorage.getItem(ENTITLEMENTS_CACHE_KEY);
    if (cachedData) {
      const { data, timestamp } = JSON.parse(cachedData);
      if (Date.now() - timestamp < CACHE_EXPIRY) {
        setUserEntitlements(data);
        setUiOptions(data.ui_options);
        setLoading(false);
        return data;
      }
    }

    return pubClient.get('/common/me')
      .then(response => {
        const newData = response.data.attributes.entitlements;
        const newUiOptions = response.data.attributes.ui_options;
        
        setUserEntitlements(newData);
        setUiOptions(newUiOptions);
        
        localStorage.setItem(
          ENTITLEMENTS_CACHE_KEY,
          JSON.stringify({
            data: { ...newData, ui_options: newUiOptions },
            timestamp: Date.now(),
          })
        );
        
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
