import { useRef, useCallback } from 'react';

const useRequestDeduplication = () => {
  const requestsRef = useRef(new Map());

  const deduplicateRequest = useCallback(async (key, requestFn) => {
    // If we already have a pending request for this key, return the existing promise
    if (requestsRef.current.has(key)) {
      return requestsRef.current.get(key);
    }

    // Create a new request
    const requestPromise = requestFn()
      .finally(() => {
        // Clean up the request from the map when it completes
        requestsRef.current.delete(key);
      });

    // Store the promise so we can reuse it for duplicate requests
    requestsRef.current.set(key, requestPromise);

    return requestPromise;
  }, []);

  const clearCache = useCallback(() => {
    requestsRef.current.clear();
  }, []);

  return { deduplicateRequest, clearCache };
};

export default useRequestDeduplication;