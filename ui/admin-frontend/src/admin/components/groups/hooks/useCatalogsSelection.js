import { useState, useCallback, useEffect } from "react";
import { getCatalogues, getDataCatalogues, getToolCatalogues } from "../../../services/catalogsService";
import { getFeatureFlags } from "../../../utils/featureUtils";

export const useCatalogsSelection = (
  initialCatalogs = [],
  initialDataCatalogs = [],
  initialToolCatalogs = [],
  features = { feature_gateway: false, feature_portal: false, feature_chat: false }
) => {
  const [catalogs, setCatalogs] = useState([]);
  const [selectedCatalogs, setSelectedCatalogs] = useState(initialCatalogs);
  
  const [dataCatalogs, setDataCatalogs] = useState([]);
  const [selectedDataCatalogs, setSelectedDataCatalogs] = useState(initialDataCatalogs);
  
  const [toolCatalogs, setToolCatalogs] = useState([]);
  const [selectedToolCatalogs, setSelectedToolCatalogs] = useState(initialToolCatalogs);
  
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const { isGatewayOnly, isPortalOnly, isChatOnly } = getFeatureFlags(features);

  const fetchCatalogs = useCallback(async () => {
    if (isGatewayOnly) return;
    
    setLoading(true);
    setError(null);
    
    try {
      const [catalogsRes, dataCatalogsRes, toolCatalogsRes] = await Promise.all([
        (isPortalOnly || (!isGatewayOnly && !isChatOnly)) ? getCatalogues(1, true) : [],
        !isGatewayOnly ? getDataCatalogues(1, true) : [],
        (isChatOnly || (!isGatewayOnly && !isPortalOnly)) ? getToolCatalogues(1, true) : []
      ]);

      setCatalogs(catalogsRes || []);
      setDataCatalogs(dataCatalogsRes || []);
      setToolCatalogs(toolCatalogsRes || []);
      
    } catch (err) {
      setError("Failed to load catalogs");
    } finally {
      setLoading(false);
    }
  }, [isGatewayOnly, isPortalOnly, isChatOnly]);

  const formatCatalogsForSelect = useCallback((catalogItems) => {
    if (!Array.isArray(catalogItems)) return [];
    
    return catalogItems.map(catalog => 
      catalog ? {
        value: catalog.id,
        label: catalog.attributes?.name || `Catalog ${catalog.id}`
      } : null
    ).filter(Boolean);
  }, []);

  useEffect(() => {
    fetchCatalogs();
  }, [fetchCatalogs]);

  return {
    catalogs: formatCatalogsForSelect(catalogs),
    selectedCatalogs,
    setSelectedCatalogs,
    
    dataCatalogs: formatCatalogsForSelect(dataCatalogs),
    selectedDataCatalogs,
    setSelectedDataCatalogs,
    
    toolCatalogs: formatCatalogsForSelect(toolCatalogs),
    selectedToolCatalogs,
    setSelectedToolCatalogs,
    
    loading,
    error,
    fetchCatalogs
  };
};