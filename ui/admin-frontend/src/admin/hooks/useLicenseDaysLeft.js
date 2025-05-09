import { useState, useEffect, useCallback } from "react";
import pubClient from "../utils/pubClient";
import cacheService from "../utils/cacheService";
import { CACHE_KEYS } from "../utils/constants";

const useLicenseDaysLeft = (skipInitialFetch = false) => {
  const [licenseDaysLeft, setLicenseDaysLeft] = useState(null);
  const [loading, setLoading] = useState(!skipInitialFetch);
  const [error, setError] = useState(null);

  const fetchLicenseDaysLeft = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    const cachedDaysLeft = cacheService.get(CACHE_KEYS.LICENSE_DAYS_LEFT);
    if (cachedDaysLeft !== null) {
      setLicenseDaysLeft(cachedDaysLeft);
      setLoading(false);
      return cachedDaysLeft;
    }
    
    return pubClient.get("/common/system")
      .then(response => {
        const daysLeft = response.data.license_days_left;
        setLicenseDaysLeft(daysLeft);

        cacheService.set(CACHE_KEYS.LICENSE_DAYS_LEFT, daysLeft, 300000);
        
        return daysLeft;
      })
      .catch(error => {
        console.error("Error fetching license days left:", error);
        setError(error);
        throw error;
      })
      .finally(() => {
        setLoading(false);
      });
  }, []);

  useEffect(() => {
    if (!skipInitialFetch) {
      fetchLicenseDaysLeft();
    }
  }, [fetchLicenseDaysLeft, skipInitialFetch]);

  return { licenseDaysLeft, loading, error, fetchLicenseDaysLeft };
};

export default useLicenseDaysLeft;