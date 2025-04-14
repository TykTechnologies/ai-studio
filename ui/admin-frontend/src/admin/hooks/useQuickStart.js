import { useState, useEffect, useCallback } from 'react';
import useUserEntitlements from './useUserEntitlements';
import useLLMs from './useLLMs';

const useQuickStart = () => {
  const [showQuickStart, setShowQuickStart] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const { 
    userName, 
    fetchUserEntitlements, 
    error: entitlementsError 
  } = useUserEntitlements(true);
  
  const { 
    hasLLMs, 
    fetchLLMs, 
    error: llmsError 
  } = useLLMs({ 
    skipInitialFetch: true,
    checkExistenceOnly: true 
  });

  const fetchQuickStartData = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    return Promise.all([
      fetchUserEntitlements(),
      fetchLLMs()
    ])
      .then(() => {
        if (!hasLLMs) {
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
  }, [fetchUserEntitlements, fetchLLMs]);

  useEffect(() => {
    fetchQuickStartData();
  }, [fetchQuickStartData]);

  const handleQuickStartComplete = () => {
    setShowQuickStart(false);
  };

  const handleQuickStartSkip = () => {
    setShowQuickStart(false);
  };

  const combinedError = entitlementsError || llmsError || error;

  return {
    showQuickStart,
    setShowQuickStart,
    userName,
    loading,
    error: combinedError,
    handleQuickStartComplete,
    handleQuickStartSkip,
    fetchQuickStartData
  };
};

export default useQuickStart;