import { useState, useEffect, useCallback } from "react";
import pubClient from "../utils/pubClient";

const FEATURES_CACHE_KEY = 'tyk_ai_studio_admin_features';
const CACHE_EXPIRY = 60000;

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
    
    const cachedData = localStorage.getItem(FEATURES_CACHE_KEY);
    if (cachedData) {
      const { data, timestamp } = JSON.parse(cachedData);
      if (Date.now() - timestamp < CACHE_EXPIRY) {
        setFeatures(data);
        setLoading(false);
        return data;
      }
    }
    
    return pubClient.get("/common/system")
      .then(response => {
        const newData = response.data.features;
        
        setFeatures(newData);
        
        localStorage.setItem(
          FEATURES_CACHE_KEY,
          JSON.stringify({
            data: newData,
            timestamp: Date.now(),
          })
        );
        
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
