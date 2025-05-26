import { useState, useEffect, useCallback } from "react";
import { teamsService } from "../../../services/teamsService";
import { useCatalogsSelection } from "../../../components/groups/hooks/useCatalogsSelection";

export const useCatalogsModal = (groupId) => {
  const [loadingGroupData, setLoadingGroupData] = useState(false);
  
  const {
    catalogs,
    selectedCatalogs,
    setSelectedCatalogs,
    dataCatalogs,
    selectedDataCatalogs,
    setSelectedDataCatalogs,
    toolCatalogs,
    selectedToolCatalogs,
    setSelectedToolCatalogs,
    loading: loadingCatalogs,
    error,
    fetchCatalogs
  } = useCatalogsSelection([], [], []);

  const fetchGroupCatalogs = useCallback(async () => {
    if (!groupId) return;
    
    setLoadingGroupData(true);
    try {
      const response = await teamsService.getTeam(groupId);
      const { attributes } = response.data;
      
      if (attributes.catalogues) {
        setSelectedCatalogs(attributes.catalogues.map(cat => ({
          value: cat.id,
          label: cat.attributes.name
        })));
      }
      
      if (attributes.data_catalogues) {
        setSelectedDataCatalogs(attributes.data_catalogues.map(cat => ({
          value: cat.id,
          label: cat.attributes.name
        })));
      }
      
      if (attributes.tool_catalogues) {
        setSelectedToolCatalogs(attributes.tool_catalogues.map(cat => ({
          value: cat.id,
          label: cat.attributes.name
        })));
      }
    } catch (error) {
      console.error("Error fetching group catalogs:", error);
    } finally {
      setLoadingGroupData(false);
    }
  }, [groupId, setSelectedCatalogs, setSelectedDataCatalogs, setSelectedToolCatalogs]);

  useEffect(() => {
    if (groupId) {
      fetchGroupCatalogs();
    }
  }, [groupId, fetchGroupCatalogs]);

  return {
    catalogs,
    selectedCatalogs,
    setSelectedCatalogs,
    dataCatalogs,
    selectedDataCatalogs,
    setSelectedDataCatalogs,
    toolCatalogs,
    selectedToolCatalogs,
    setSelectedToolCatalogs,
    loading: loadingCatalogs || loadingGroupData,
    error,
    fetchCatalogs,
    fetchGroupCatalogs
  };
};