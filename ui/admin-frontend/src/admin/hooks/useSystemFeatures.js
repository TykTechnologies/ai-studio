import { useState, useEffect } from "react";
import apiClient from "../utils/apiClient";
import axios from "axios";
import { getConfig } from "../../config";

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
        const config = getConfig();
        const response = await axios.get(
          `${config.API_BASE_URL}/common/system`,
          {
            withCredentials: true,
          },
        );
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
