import { useState, useEffect, useCallback } from 'react';
import pubClient from '../utils/pubClient';

const CACHE_KEY = 'userEntitlements';
const CACHE_EXPIRY = 10000;

const useUserEntitlements = (skipInitialFetch = false) => {
  const [userEntitlements, setUserEntitlements] = useState(null);
  const [uiOptions, setUiOptions] = useState(null);
  const [userName, setUserName] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const fetchUserEntitlements = useCallback(async () => {
    try {
      setLoading(true);
      
      const cachedData = localStorage.getItem(CACHE_KEY);
      if (cachedData) {
        const { data, userName, timestamp } = JSON.parse(cachedData);
        if (Date.now() - timestamp < CACHE_EXPIRY) {
          setUserEntitlements(data);
          setUiOptions(data.ui_options);
          setUserName(userName);
          setLoading(false);
          return data;
        }
      }

      const response = await pubClient.get('/common/me');
      const newData = response.data.attributes.entitlements;
      const newUiOptions = response.data.attributes.ui_options;
      const newUserName = response.data.attributes.name;
      
      setUserEntitlements(newData);
      setUiOptions(newUiOptions);
      setUserName(newUserName);
      
      localStorage.setItem(
        CACHE_KEY,
        JSON.stringify({
          data: { ...newData, ui_options: newUiOptions },
          userName: newUserName,
          timestamp: Date.now(),
        })
      );
      
      return newData;
    } catch (error) {
      console.error('Failed to fetch user entitlements:', error);
      setError(error);
      throw error;
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!skipInitialFetch) {
      fetchUserEntitlements();
    }
  }, [fetchUserEntitlements, skipInitialFetch]);

  return {
    userEntitlements,
    uiOptions,
    userName,
    loading,
    error,
    fetchUserEntitlements
  };
};

export default useUserEntitlements;
