import { useState, useEffect, useCallback } from "react";
import pubClient from "../utils/pubClient";

const useSystemFeatures = (skipInitialFetch = false) => {
  const [features, setFeatures] = useState({
    feature_portal: false,
    feature_chat: false,
    feature_gateway: false,
  });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const fetchFeatures = useCallback(async () => {
    try {
      setLoading(true);
      const response = await pubClient.get("/common/system");
      setFeatures(response.data.features);
      return response.data.features;
    } catch (err) {
      console.error("Error fetching system features:", err);
      setError(err);
      throw err;
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!skipInitialFetch) {
      fetchFeatures();
    }
  }, [fetchFeatures, skipInitialFetch]);

  return { features, loading, error, fetchFeatures };
};

export default useSystemFeatures;
