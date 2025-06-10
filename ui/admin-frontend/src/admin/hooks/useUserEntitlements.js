import { useState, useEffect, useCallback } from 'react';
import pubClient from '../utils/pubClient';
import cacheService from '../utils/cacheService';
import { CACHE_KEYS } from '../utils/constants';

const useUserEntitlements = (skipInitialFetch = false) => {
  const [userEntitlements, setUserEntitlements] = useState(null);
  const [uiOptions, setUiOptions] = useState(null);
  const [userName, setUserName] = useState(null);
  const [userId, setUserId] = useState(null);
  const [userEmail, setUserEmail] = useState(null);
  const [isSuperAdmin, setIsSuperAdmin] = useState(false);
  const [loading, setLoading] = useState(!skipInitialFetch);
  const [error, setError] = useState(null);

  const fetchUserEntitlements = useCallback(async (skipEntitlements = false) => {
    setLoading(true);
    setError(null);
    
    const cachedData = cacheService.get(CACHE_KEYS.USER_ENTITLEMENTS);
    if (cachedData) {
      setUserEntitlements(cachedData.entitlements);
      setUiOptions(cachedData.ui_options);
      setUserName(cachedData.userName);
      setUserId(cachedData.userId);
      setUserEmail(cachedData.userEmail);
      setIsSuperAdmin(cachedData.isSuperAdmin);
      setLoading(false);
      return cachedData;
    }

    const url = skipEntitlements
      ? '/common/me?skip_entitlements=true'
      : '/common/me';

    return pubClient.get(url)
      .then(response => {
        const newData = response.data.attributes.entitlements;
        const newUiOptions = response.data.attributes.ui_options;
        const newUserName = response.data.attributes.name;
        const newUserId = response.data.id;
        const newUserEmail = response.data.attributes.email;
        const newIsSuperAdmin = response.data.attributes.is_super_admin;
        
        setUserEntitlements(newData);
        setUiOptions(newUiOptions);
        setUserName(newUserName);
        setUserId(newUserId);
        setUserEmail(newUserEmail);
        setIsSuperAdmin(newIsSuperAdmin);
        
        const dataToCache = {
          entitlements: newData,
          ui_options: newUiOptions,
          userName: newUserName,
          userId: newUserId,
          userEmail: newUserEmail,
          isSuperAdmin: newIsSuperAdmin
        };
        cacheService.set(CACHE_KEYS.USER_ENTITLEMENTS, dataToCache, 10000); // 10 seconds expiry
        
        return dataToCache;
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
    userName,
    userId,
    userEmail,
    isSuperAdmin,
    loading,
    error,
    fetchUserEntitlements
  };
};

export default useUserEntitlements;
