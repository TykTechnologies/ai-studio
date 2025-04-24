import { useState, useEffect, useCallback } from 'react';
import useUserEntitlements from './useUserEntitlements';
import apiClient from '../utils/apiClient';

const useQuickStart = () => {
  const [showQuickStart, setShowQuickStart] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const {
    userName,
    userId,
    userEmail,
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
      .then(([_, appsCount]) => {
        if (appsCount === 0) {
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

  const handleQuickStartSkip = () => {
    setShowQuickStart(false);
  };

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