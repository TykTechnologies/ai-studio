import { useState, useEffect, useCallback } from "react";
import pubClient from "../utils/pubClient";
import cacheService from "../utils/cacheService";

const FEATURES_CACHE_KEY = 'tyk_ai_studio_admin_features';

const useSystemFeatures = (skipInitialFetch = false) => {
  const [features, setFeatures] = useState({
    feature_portal: false,
    feature_chat: false,
    feature_gateway: false,
  });
  const [loading, setLoading] = useState(!skipInitialFetch);
  const [error, setError] = useState(null);

  const fetchFeatures = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    const cachedFeatures = cacheService.get(FEATURES_CACHE_KEY);
    if (cachedFeatures) {
      setFeatures(cachedFeatures);
      setLoading(false);
      return cachedFeatures;
    }
    
    return pubClient.get("/common/system")
      .then(response => {
        const newData = response.data.features;
        
        setFeatures(newData);
        cacheService.set(FEATURES_CACHE_KEY, newData);
        
        return newData;
      })
      .catch(error => {
        console.error("Error fetching system features:", error);
        setError(error);
        throw error;
      })
      .finally(() => {
        setLoading(false);
      });
  }, []);

  useEffect(() => {
    if (!skipInitialFetch) {
      fetchFeatures();
    }
  }, [fetchFeatures, skipInitialFetch]);

  return { features, loading, error, fetchFeatures };
};

export default useSystemFeatures;
