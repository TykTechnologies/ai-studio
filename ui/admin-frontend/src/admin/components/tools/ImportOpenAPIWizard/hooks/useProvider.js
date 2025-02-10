import { useState, useCallback } from "react";
import { configureProvider } from "../services/toolService";
import { PROVIDER_TYPES } from "../constants";

const validateProvider = (provider) => {
  if (!provider) return null;
  
  // Handle static providers (already in the correct format)
  if (provider.type && Object.values(PROVIDER_TYPES).includes(provider.type)) {
    return provider;
  }
  
  // Handle backend providers (need to transform from attributes)
  if (provider.attributes?.type && Object.values(PROVIDER_TYPES).includes(provider.attributes.type)) {
    return {
      ...provider,
      type: provider.attributes.type,
      name: provider.attributes.name,
      description: provider.attributes.description,
    };
  }
  
  return null;
};

// Static providers that are always available
const staticProviders = [
  {
    id: "direct",
    type: PROVIDER_TYPES.DIRECT_IMPORT,
    name: "Direct Import",
    description: "Import from URL or upload OpenAPI specification file",
  },
  {
    id: "tyk",
    type: PROVIDER_TYPES.TYK_DASHBOARD,
    name: "Tyk Dashboard",
    description: "Import APIs from your Tyk Dashboard",
  }
];

export const useProvider = () => {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [providers, setProviders] = useState(staticProviders);
  const [selectedProvider, setSelectedProvider] = useState(null);
  const [apis, setApis] = useState([]);

  // No need for useEffect since we initialize with static providers

  const configureSelectedProvider = useCallback(
    async (config) => {
      if (!selectedProvider) {
        setError("No provider selected");
        return;
      }

      if (selectedProvider.type !== PROVIDER_TYPES.TYK_DASHBOARD) {
        setError("Configuration only supported for Tyk Dashboard");
        return;
      }

      setLoading(true);
      setError("");

      try {
        const result = await configureProvider(selectedProvider.id, config);
        setApis(result);
        return result;
      } catch (error) {
        setError(error.message || "Failed to configure provider");
        throw error;
      } finally {
        setLoading(false);
      }
    },
    [selectedProvider]
  );

  const selectProvider = useCallback((provider) => {
    const validProvider = validateProvider(provider);
    setSelectedProvider(validProvider);
    setApis([]); // Reset APIs when provider changes
    setError(""); // Clear any previous errors
  }, []);

  const reset = useCallback(() => {
    setSelectedProvider(null);
    setApis([]);
    setError("");
  }, []);

  return {
    loading,
    error,
    providers,
    selectedProvider,
    apis,
    configureSelectedProvider,
    selectProvider,
    reset,
  };
};
