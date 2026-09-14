import { useState, useCallback, useMemo } from "react";
import usePagination from "./usePagination";

/**
 * Page / page size / search / sort state for a server-backed list, in the
 * shape the list endpoints take (`page`, `page_size`, `search`, `sort` as
 * `name` or `-name`) and the shape DataTable renders.
 *
 * Changing the search term or the sort returns to page 1.
 *
 * @param {{ initialSort?: {field: string, direction: "asc"|"desc"}|null, initialPageSize?: number }} [options]
 */
const useListQuery = ({ initialSort = null, initialPageSize = 10 } = {}) => {
  const pagination = usePagination(1, initialPageSize);
  const { page, pageSize, totalPages, handlePageChange, handlePageSizeChange, updatePaginationData } =
    pagination;
  const [searchTerm, setSearchTerm] = useState("");
  const [sortConfig, setSortConfig] = useState(initialSort);

  const handleSearch = useCallback(
    (value) => {
      setSearchTerm(value || "");
      handlePageChange(1);
    },
    [handlePageChange],
  );

  const handleSortChange = useCallback(
    (next) => {
      setSortConfig(next);
      handlePageChange(1);
    },
    [handlePageChange],
  );

  const sortParam = sortConfig?.field
    ? `${sortConfig.direction === "desc" ? "-" : ""}${sortConfig.field}`
    : undefined;

  const queryParams = useMemo(() => {
    const params = { page, page_size: pageSize };
    if (searchTerm) params.search = searchTerm;
    if (sortParam) params.sort = sortParam;
    return params;
  }, [page, pageSize, searchTerm, sortParam]);

  const tableProps = useMemo(
    () => ({
      enableSearch: true,
      searchTerm,
      onSearch: handleSearch,
      sortConfig,
      onSortChange: handleSortChange,
      pagination: {
        page,
        pageSize,
        totalPages,
        onPageChange: handlePageChange,
        onPageSizeChange: handlePageSizeChange,
      },
    }),
    [searchTerm, handleSearch, sortConfig, handleSortChange, page, pageSize, totalPages, handlePageChange, handlePageSizeChange],
  );

  return {
    ...pagination,
    searchTerm,
    handleSearch,
    sortConfig,
    handleSortChange,
    sortParam,
    queryParams,
    updatePaginationData,
    tableProps,
  };
};

export default useListQuery;
