import { useState, useEffect, useCallback } from "react";
import { useParams } from "react-router-dom";
import { teamsService } from "../../../services/teamsService";

const useGroupDetail = () => {
  const { id } = useParams();
  const [group, setGroup] = useState(null);
  const [users, setUsers] = useState([]);
  const [catalogues, setCatalogues] = useState([]);
  const [dataCatalogues, setDataCatalogues] = useState([]);
  const [toolCatalogues, setToolCatalogues] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const fetchGroup = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    setError(null);
    try {
      const response = await teamsService.getTeam(id);
      const { attributes } = response.data;

      setGroup(response.data);

      setUsers(attributes.users || []);
      setCatalogues(attributes.catalogues || []);
      setDataCatalogues(attributes.data_catalogues || []);
      setToolCatalogues(attributes.tool_catalogues || []);
    } catch (err) {
      setError("Failed to load group details");
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    fetchGroup();
  }, [fetchGroup]);

  return {
    group,
    users,
    catalogues,
    dataCatalogues,
    toolCatalogues,
    loading,
    error,
  };
};

export default useGroupDetail;