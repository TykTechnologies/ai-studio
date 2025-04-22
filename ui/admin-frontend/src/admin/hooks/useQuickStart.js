import { useState, useEffect, useCallback } from 'react';
import useUserEntitlements from './useUserEntitlements';
import apiClient from '../utils/apiClient';

const useQuickStart = () => {
  const [showQuickStart, setShowQuickStart] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  // Create a wrapped version of setShowQuickStart that logs when it's called
  const setShowQuickStartWithLogging = (value) => {
    console.log('setShowQuickStart called with value:', value);
    setShowQuickStart(value);
  };

  const {
    userName,
    fetchUserEntitlements,
    error: entitlementsError
  } = useUserEntitlements(true);

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
          setShowQuickStartWithLogging(true);
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
    setShowQuickStartWithLogging(false);
  };

  const handleQuickStartSkip = () => {
    setShowQuickStartWithLogging(false);
  };

  const combinedError = entitlementsError || error;

  return {
    showQuickStart,
    setShowQuickStart: setShowQuickStartWithLogging, // This will be used by the button in Overview.js
    userName,
    loading,
    error: combinedError,
    handleQuickStartComplete,
    handleQuickStartSkip,
    fetchQuickStartData
  };
};

export default useQuickStart;