import { useState, useEffect, useCallback } from "react";
import pubClient from "../utils/pubClient";
import cacheService from "../utils/cacheService";
import { CACHE_KEYS } from "../utils/constants";

const useSystemFeatures = (skipInitialFetch = false) => {
  const [features, setFeatures] = useState({
    feature_portal: false,
    feature_chat: false,
    feature_gateway: false,
    hub_spoke_multi_tenant: false, // Enterprise-only multi-tenant namespace support
  });
  const [loading, setLoading] = useState(!skipInitialFetch);
  const [error, setError] = useState(null);

  const fetchFeatures = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    const cachedFeatures = cacheService.get(CACHE_KEYS.FEATURES);
    if (cachedFeatures) {
      setFeatures(cachedFeatures);
      setLoading(false);
      return cachedFeatures;
    }
    
    return pubClient.get("/common/system")
      .then(response => {
        const newData = response.data.features;
        
        setFeatures(newData);
        cacheService.set(CACHE_KEYS.FEATURES, newData);
        
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
