import { useState, useEffect, useCallback } from 'react';
import useUserEntitlements from './useUserEntitlements';
import useSystemFeatures from './useSystemFeatures';
import useLLMs from './useLLMs';

/**
 * Coordinator hook that fetches all data needed for the Overview page in parallel
 * @returns {Object} Combined data and state from all hooks
 */
const useOverviewData = () => {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  // Initialize hooks with skipInitialFetch=true to prevent automatic fetching
  const { 
    userEntitlements, 
    userName, 
    fetchUserEntitlements, 
    error: entitlementsError 
  } = useUserEntitlements(true);
  
  const { 
    features, 
    fetchFeatures, 
    error: featuresError 
  } = useSystemFeatures(true);
  
  const { 
    hasLLMs, 
    fetchLLMs, 
    error: llmsError 
  } = useLLMs({ skipInitialFetch: true, checkExistenceOnly: true });

  const fetchAllData = useCallback(async () => {
    setLoading(true);
    try {
      // Fetch all data in parallel
      await Promise.all([
        fetchUserEntitlements(),
        fetchFeatures(),
        fetchLLMs()
      ]);
      setError(null);
    } catch (error) {
      console.error('Error fetching overview data:', error);
      setError('Failed to load data');
    } finally {
      setLoading(false);
    }
  }, [fetchUserEntitlements, fetchFeatures, fetchLLMs]);

  useEffect(() => {
    fetchAllData();
  }, [fetchAllData]);

  // Combine errors if any
  const combinedError = entitlementsError || featuresError || llmsError || error;

  return {
    userEntitlements,
    userName,
    features,
    hasLLMs,
    loading,
    error: combinedError,
    fetchAllData
  };
};

export default useOverviewData;