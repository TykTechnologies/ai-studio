import { useState, useCallback, useEffect } from "react";
import { getCatalogues, getDataCatalogues, getToolCatalogues } from "../../../services/catalogsService";

export const useCatalogsSelection = (
  initialCatalogs = [],
  initialDataCatalogs = [],
  initialToolCatalogs = []
) => {
  const [catalogs, setCatalogs] = useState([]);
  const [selectedCatalogs, setSelectedCatalogs] = useState(initialCatalogs);
  
  const [dataCatalogs, setDataCatalogs] = useState([]);
  const [selectedDataCatalogs, setSelectedDataCatalogs] = useState(initialDataCatalogs);
  
  const [toolCatalogs, setToolCatalogs] = useState([]);
  const [selectedToolCatalogs, setSelectedToolCatalogs] = useState(initialToolCatalogs);
  
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const fetchCatalogs = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    try {
      const [catalogsResponse, dataCatalogsResponse, toolCatalogsResponse] = await Promise.all([
        getCatalogues(1, true),
        getDataCatalogues(1, true),
        getToolCatalogues(1, true)
      ]);

      setCatalogs(catalogsResponse || []);
      setDataCatalogs(dataCatalogsResponse || []);
      setToolCatalogs(toolCatalogsResponse || []);

      setLoading(false);
    } catch (error) {
      console.error("Error fetching catalogs:", error);
      setError("Failed to load catalogs");
      setLoading(false);
    }
  }, []);

  const formatCatalogsForSelect = useCallback((catalogItems) => {
    if (!Array.isArray(catalogItems)) {
      return [];
    }
    
    return catalogItems.map(catalog => {
      if (!catalog) return null;
      
      return {
        value: catalog.id,
        label: catalog.attributes?.name || `Catalog ${catalog.id}`
      };
    }).filter(Boolean);
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