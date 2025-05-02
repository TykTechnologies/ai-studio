import { useState, useEffect, useCallback } from 'react';
import useUserEntitlements from './useUserEntitlements';
import apiClient from '../utils/apiClient';
import { skipQuickStartForUser } from '../services/userService';
import cacheService from '../utils/cacheService';
import { CACHE_KEYS } from '../utils/constants';

const useQuickStart = () => {
  const [showQuickStart, setShowQuickStart] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const {
    userName,
    userId,
    userEmail,
    userEntitlements,
    fetchUserEntitlements,
    error: entitlementsError
  } = useUserEntitlements(true);

  const currentUser = userId ? {
    id: userId,
    name: userName,
    email: userEmail
  } : null;

  const fetchAppsCount = useCallback(async () => {
    try {
      const response = await apiClient.get('/apps/count');
      const count = response.data.count || 0;
      return count;
    } catch (error) {
      console.error('Error fetching apps count:', error);
      return 0;
    }
  }, []);

  const fetchQuickStartData = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    return Promise.all([
      fetchUserEntitlements(),
      fetchAppsCount()
    ])
      .then(([userEntitlements, appsCount]) => {
        if (appsCount === 0 && !userEntitlements?.ui_options?.skip_quick_start) {
          setShowQuickStart(true);
        }
      })
      .catch(error => {
        console.error('Error fetching quick start data:', error);
        setError('Failed to load data');
      })
      .finally(() => {
        setLoading(false);
      });
  }, [fetchUserEntitlements, fetchAppsCount]);

  useEffect(() => {
    fetchQuickStartData();
  }, [fetchQuickStartData]);

  const handleQuickStartComplete = () => {
    setShowQuickStart(false);
  };

  const handleQuickStartSkip = useCallback(async () => {
    if (userId && !userEntitlements?.ui_options?.skip_quick_start) {
      try {
        await skipQuickStartForUser(userId);
        cacheService.remove(CACHE_KEYS.USER_ENTITLEMENTS);
      } catch (error) {
        console.error('Error marking quick start as skipped:', error);
      }
    }
    setShowQuickStart(false);
  }, [userId, userEntitlements]);

  const combinedError = entitlementsError || error;

  return {
    showQuickStart,
    setShowQuickStart: setShowQuickStart,
    currentUser,
    loading,
    error: combinedError,
    handleQuickStartComplete,
    handleQuickStartSkip,
    fetchQuickStartData
  };
};

export default useQuickStart;