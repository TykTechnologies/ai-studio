import { useState, useEffect, useCallback } from "react";
import { teamsService } from "../../../services/teamsService";
import { useCatalogsSelection } from "../../../components/groups/hooks/useCatalogsSelection";

export const useCatalogsModal = (groupId, features) => {
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
  } = useCatalogsSelection([], [], [], features);

  const fetchGroupCatalogs = useCallback(async () => {
    if (!groupId) return;
    
    setLoadingGroupData(true);
    try {
      const response = await teamsService.getTeam(groupId);
      const { attributes } = response.data;
      
      setSelectedCatalogs(
        attributes.catalogues?.map(cat => ({
          value: cat.id,
          label: cat.attributes.name
        })) || []
      );
      
      setSelectedDataCatalogs(
        attributes.data_catalogues?.map(cat => ({
          value: cat.id,
          label: cat.attributes.name
        })) || []
      );
      
      setSelectedToolCatalogs(
        attributes.tool_catalogues?.map(cat => ({
          value: cat.id,
          label: cat.attributes.name
        })) || []
      );
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