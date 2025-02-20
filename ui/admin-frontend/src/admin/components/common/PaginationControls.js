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
        padding: "14px 0px 10px 10px",
        borderTop: theme => `1px solid ${theme.palette.border.neutralDefault}`
      }}
    >
      <Select 
        value={pageSize} 
        onChange={onPageSizeChange}
        className="MuiSelect-pagination"
      >
        <MenuItem value={10}>10 per page</MenuItem>
        <MenuItem value={25}>25 per page</MenuItem>
        <MenuItem value={50}>50 per page</MenuItem>
      </Select>
      <Pagination count={totalPages} page={page} onChange={onPageChange} />
    </Box>
  );
};

export default PaginationControls;
