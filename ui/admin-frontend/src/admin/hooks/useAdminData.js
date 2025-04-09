import { useState, useEffect, useCallback } from 'react';
import useUserEntitlements from './useUserEntitlements';
import useSystemFeatures from './useSystemFeatures';
import useConfig from './useConfig';

const useAdminData = () => {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const { 
    uiOptions, 
    fetchUserEntitlements, 
    error: entitlementsError 
  } = useUserEntitlements(true);
  
  const { 
    features, 
    fetchFeatures, 
    error: featuresError 
  } = useSystemFeatures(true);
  
  const { 
    config, 
    fetchConfig, 
    error: configError 
  } = useConfig(true);

  const fetchAllData = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    return Promise.all([
      fetchUserEntitlements(),
      fetchFeatures(),
      fetchConfig()
    ])
      .catch(error => {
        console.error('Error fetching admin data:', error);
        setError('Failed to load data');
      })
      .finally(() => {
        setLoading(false);
      });
  }, [fetchUserEntitlements, fetchFeatures, fetchConfig]);

  useEffect(() => {
    fetchAllData();
  }, [fetchAllData]);

  const combinedError = entitlementsError || featuresError || configError || error;

  return {
    uiOptions,
    features,
    config,
    loading,
    error: combinedError,
    fetchAllData
  };
};

export default useAdminData;