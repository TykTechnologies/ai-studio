import { useState, useEffect } from "react";
import pubClient from "../utils/pubClient";

const useSystemFeatures = () => {
  const [features, setFeatures] = useState({
    feature_portal: false,
    feature_chat: false,
    feature_gateway: false,
  });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    const fetchFeatures = async () => {
      try {
        const response = await pubClient.get("/common/system");
        setFeatures(response.data.features);
      } catch (err) {
        console.error("Error fetching system features:", err);
        setError(err);
      } finally {
        setLoading(false);
      }
    };

    fetchFeatures();
  }, []);

  return { features, loading, error };
};

export default useSystemFeatures;
