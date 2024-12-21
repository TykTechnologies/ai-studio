import React from "react";
import { Box, Select, MenuItem, Pagination } from "@mui/material";

const PaginationControls = ({
  page,
  pageSize,
  totalPages,
  onPageChange,
  onPageSizeChange,
}) => {
  return (
    <Box
      sx={{
        display: "flex",
        justifyContent: "space-between",
        alignItems: "center",
        mt: 2,
        padding: "0 20px 20px 20px"
      }}
    >
      <Select value={pageSize} onChange={onPageSizeChange}>
        <MenuItem value={10}>10 per page</MenuItem>
        <MenuItem value={25}>25 per page</MenuItem>
        <MenuItem value={50}>50 per page</MenuItem>
      </Select>
      <Pagination count={totalPages} page={page} onChange={onPageChange} />
    </Box>
  );
};

export default PaginationControls;
