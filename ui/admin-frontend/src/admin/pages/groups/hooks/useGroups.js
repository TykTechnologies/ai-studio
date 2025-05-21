import { useState, useEffect, useCallback } from "react";
import usePagination from "../../../hooks/usePagination";
import { useDebounce } from "use-debounce";

import { teamsService } from "../../../services/teamsService";

const useGroups = () => {
  const [groups, setGroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [searchTerm, setSearchTerm] = useState("");
  const [debouncedSearchTerm] = useDebounce(searchTerm, 500);
  const [sortConfig, setSortConfig] = useState({ field: "id", direction: "asc" });

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchGroups = useCallback(async () => {
    try {
      setLoading(true);
      const sortParam = sortConfig.direction === "desc" ? `-${sortConfig.field}` : sortConfig.field;

      const params = {
        page,
        page_size: pageSize,
        search: debouncedSearchTerm,
        sort: sortParam,
      };

      const response = await teamsService.getTeams(params);

      setGroups(response.data?.data || []);
      const totalCount = parseInt(response.headers?.["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers?.["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching groups", error);
      setError("Failed to load groups");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, debouncedSearchTerm, sortConfig, updatePaginationData]);

  useEffect(() => {
    fetchGroups();
  }, [fetchGroups]);

  const handleSearch = (value) => {
    setSearchTerm(value);
  };

  const handleSortChange = (newSortConfig) => {
    setSortConfig(newSortConfig);
  };

  return {
    groups,
    loading,
    error,
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    handleSearch,
    sortConfig,
    handleSortChange,
    refreshGroups: fetchGroups,
  };
};

export default useGroups;