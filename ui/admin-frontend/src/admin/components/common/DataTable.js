import React, { useState, useCallback, memo } from "react";
import {
  Table,
  TableBody,
  TableHead,
  TableRow,
  IconButton,
  Menu,
  MenuItem,
  InputAdornment,
  Box,
} from "@mui/material";
import SearchIcon from "@mui/icons-material/Search";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import {
  StyledPaper,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  StyledTextField
} from "../../styles/sharedStyles";
import PaginationControls from "./PaginationControls";

const DataTable = memo(({
  columns,
  data,
  actions,
  pagination,
  onRowClick,
  sortConfig,
  onSortChange,
  onSearch,
  searchTerm = "",
  searchPlaceholder = "Search...",
  enableSearch = false,
}) => {
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedItem, setSelectedItem] = useState(null);

  const handleMenuOpen = useCallback((event, item) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedItem(item);
  }, []);

  const handleMenuClose = useCallback(() => {
    setAnchorEl(null);
  }, []);

  const handleActionClick = useCallback((action) => {
    action.onClick(selectedItem);
    handleMenuClose();
  }, [selectedItem, handleMenuClose]);

  const handleHeaderClick = useCallback((column) => {
    if (!column.sortable || !onSortChange) return;
    
    const direction =
      sortConfig?.field === column.field && sortConfig?.direction === "asc"
        ? "desc"
        : "asc";
    
    onSortChange({ field: column.field, direction });
  }, [sortConfig, onSortChange]);

  const handleSearchChange = useCallback((event) => {
    const value = event.target.value;
    if (onSearch) {
      onSearch(value);
    }
  }, [onSearch]);

  return (
    <>
    {enableSearch && (
      <Box sx={{ mb: 2, maxWidth: 400 }}>
        <StyledTextField
          placeholder={searchPlaceholder}
          variant="outlined"
          fullWidth
          value={searchTerm}
          onChange={handleSearchChange}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <SearchIcon color="action" />
              </InputAdornment>
            ),
          }}
        />
      </Box>
    )}
    <StyledPaper>
      <Table>
        <TableHead>
          <TableRow>
            {columns.map((column) => (
              <StyledTableHeaderCell
                key={column.field}
                align={column.align || "left"}
                onClick={() => handleHeaderClick(column)}
                sx={{ cursor: column.sortable ? 'pointer' : 'default', ...column.sx }}
              >
                {column.headerName} 
                {column.sortable && sortConfig?.field === column.field && 
                  (sortConfig?.direction === "asc" ? " ↑" : " ↓")}
              </StyledTableHeaderCell>
            ))}
            {actions && actions.length > 0 && (
              <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
            )}
          </TableRow>
        </TableHead>
        <TableBody>
          {data?.map((item) => (
            <StyledTableRow
              key={item.id || item.key}
              onClick={() => onRowClick?.(item)}
              sx={{ cursor: onRowClick ? "pointer" : "default" }}
            >
              {columns.map((column) => (
                <StyledTableCell
                  key={`${item.id || item.key}-${column.field}`}
                  align={column.align || "left"}
                  sx={column.sx}
                >
                  {column.renderCell 
                    ? column.renderCell(item) 
                    : item[column.field] || "-"}
                </StyledTableCell>
              ))}
              {actions && actions.length > 0 && (
                <StyledTableCell align="right">
                  <IconButton
                    onClick={(event) => handleMenuOpen(event, item)}
                  >
                    <MoreVertIcon />
                  </IconButton>
                </StyledTableCell>
              )}
            </StyledTableRow>
          ))}
        </TableBody>
      </Table>
      
      {pagination && (
        <PaginationControls
          page={pagination.page}
          pageSize={pagination.pageSize}
          totalPages={pagination.totalPages}
          onPageChange={pagination.onPageChange}
          onPageSizeChange={pagination.onPageSizeChange}
        />
      )}

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        {actions && actions.map((action) => (
          <MenuItem
            key={action.label}
            onClick={() => handleActionClick(action)}
          >
            {action.label}
          </MenuItem>
        ))}
      </Menu>
    </StyledPaper>
    </>
  );
});

DataTable.displayName = 'DataTable';

export default DataTable;