import { useState, useCallback } from "react";
const usePagination = (initialPage = 1, initialPageSize = 10) => {
  const [page, setPage] = useState(initialPage);
  const [pageSize, setPageSize] = useState(initialPageSize);
  const [totalCount, setTotalCount] = useState(0);
  const [totalPages, setTotalPages] = useState(0);

  const handlePageChange = (event, newPage) => {
    setPage(newPage);
  };

  const handlePageSizeChange = (event) => {
    setPageSize(event.target.value);
    setPage(1);
  };

  const updatePaginationData = useCallback((totalCount, totalPages) => {
    setTotalCount(totalCount);
    setTotalPages(totalPages);
  }, []);

  return {
    page,
    pageSize,
    totalCount,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  };
};

export default usePagination;
