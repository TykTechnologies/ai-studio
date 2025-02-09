import { useState, useCallback } from 'react';
import { getProviders, configureProvider } from '../services/toolService';

export const useProvider = () => {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [providers, setProviders] = useState([]);
  const [selectedProvider, setSelectedProvider] = useState(null);
  const [apis, setApis] = useState([]);

  const fetchProviders = useCallback(async () => {
    setLoading(true);
    setError('');

    try {
      const result = await getProviders();
      setProviders(result);
    } catch (error) {
      setError(error.message || 'Failed to fetch providers');
      throw error;
    } finally {
      setLoading(false);
    }
  }, []);

  const configureSelectedProvider = useCallback(async (config) => {
    if (!selectedProvider) {
      setError('No provider selected');
      return;
    }

    setLoading(true);
    setError('');

    try {
      const result = await configureProvider(selectedProvider.id, config);
      setApis(result);
      return result;
    } catch (error) {
      setError(error.message || 'Failed to configure provider');
      throw error;
    } finally {
      setLoading(false);
    }
  }, [selectedProvider]);

  const selectProvider = useCallback((provider) => {
    setSelectedProvider(provider);
    setApis([]); // Reset APIs when provider changes
  }, []);

  const reset = useCallback(() => {
    setSelectedProvider(null);
    setApis([]);
    setError('');
  }, []);

  return {
    loading,
    error,
    providers,
    selectedProvider,
    apis,
    fetchProviders,
    configureSelectedProvider,
    selectProvider,
    reset
  };
};
