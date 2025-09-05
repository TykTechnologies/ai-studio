import React, { useState, useCallback, memo, useMemo } from "react";
import { FixedSizeList as List } from "react-window";
import {
  Table,
  TableHead,
  TableRow,
  IconButton,
  Menu,
  MenuItem,
  InputAdornment,
  Box,
  Typography,
  TableContainer,
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

const ROW_HEIGHT = 65; // Standard row height in pixels

const VirtualizedTableRow = memo(({ index, style, data }) => {
  const { items, columns, actions, onRowClick, onMenuOpen } = data;
  const item = items[index];

  if (!item) return null;

  return (
    <div style={style}>
      <StyledTableRow
        key={item.id || item.key}
        onClick={() => onRowClick?.(item)}
        sx={{ 
          cursor: onRowClick ? "pointer" : "default",
          display: "flex",
          alignItems: "center",
          height: ROW_HEIGHT,
          borderBottom: "1px solid #e0e0e0"
        }}
      >
        {columns.map((column) => (
          <StyledTableCell
            key={`${item.id || item.key}-${column.field}`}
            align={column.align || "left"}
            sx={{
              ...column.sx,
              flex: column.width ? `0 0 ${column.width}px` : "1",
              minWidth: column.minWidth || "100px",
              padding: "8px 16px",
              borderBottom: "none"
            }}
          >
            {column.renderCell 
              ? column.renderCell(item) 
              : item[column.field] || "-"}
          </StyledTableCell>
        ))}
        {actions && actions.length > 0 && (
          <StyledTableCell 
            align="right"
            sx={{
              flex: "0 0 80px",
              padding: "8px 16px",
              borderBottom: "none"
            }}
          >
            <IconButton
              onClick={(event) => onMenuOpen(event, item)}
            >
              <MoreVertIcon />
            </IconButton>
          </StyledTableCell>
        )}
      </StyledTableRow>
    </div>
  );
});

VirtualizedTableRow.displayName = 'VirtualizedTableRow';

const VirtualizedDataTable = memo(({
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
  height = 400, // Default virtual height
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

  // Memoize the data passed to react-window
  const itemData = useMemo(() => ({
    items: data || [],
    columns,
    actions,
    onRowClick,
    onMenuOpen: handleMenuOpen,
  }), [data, columns, actions, onRowClick, handleMenuOpen]);

  return (
    <>
      {enableSearch && (
        <Box sx={{ mb: 3 }}>
          <Typography variant="bodyLargeBold" color="text.primary" sx={{ mb: 1 }}>
            Table Search
          </Typography>
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
        {/* Table Header */}
        <Table>
          <TableHead>
            <TableRow sx={{ display: "flex" }}>
              {columns.map((column) => (
                <StyledTableHeaderCell
                  key={column.field}
                  align={column.align || "left"}
                  onClick={() => handleHeaderClick(column)}
                  sx={{ 
                    cursor: column.sortable ? 'pointer' : 'default',
                    ...column.sx,
                    flex: column.width ? `0 0 ${column.width}px` : "1",
                    minWidth: column.minWidth || "100px",
                    padding: "16px"
                  }}
                >
                  {column.headerName} 
                  {column.sortable && sortConfig?.field === column.field && 
                    (sortConfig?.direction === "asc" ? " ↑" : " ↓")}
                </StyledTableHeaderCell>
              ))}
              {actions && actions.length > 0 && (
                <StyledTableHeaderCell 
                  align="right"
                  sx={{
                    flex: "0 0 80px",
                    padding: "16px"
                  }}
                >
                  Actions
                </StyledTableHeaderCell>
              )}
            </TableRow>
          </TableHead>
        </Table>

        {/* Virtualized Table Body */}
        <TableContainer>
          {data && data.length > 0 ? (
            <List
              height={Math.min(height, data.length * ROW_HEIGHT)}
              itemCount={data.length}
              itemSize={ROW_HEIGHT}
              itemData={itemData}
            >
              {VirtualizedTableRow}
            </List>
          ) : (
            <Box sx={{ p: 4, textAlign: "center" }}>
              <Typography variant="body2" color="text.secondary">
                No data available
              </Typography>
            </Box>
          )}
        </TableContainer>
        
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

VirtualizedDataTable.displayName = 'VirtualizedDataTable';

export default VirtualizedDataTable;