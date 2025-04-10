import React, { useState } from "react";
import {
  Table,
  TableBody,
  TableHead,
  TableRow,
  IconButton,
  Menu,
  MenuItem,
} from "@mui/material";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import {
  StyledPaper,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
} from "../../styles/sharedStyles";
import PaginationControls from "./PaginationControls";

const DataTable = ({
  columns,
  data,
  actions,
  pagination,
  onRowClick,
  sortConfig,
  onSortChange,
}) => {
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedItem, setSelectedItem] = useState(null);

  const handleMenuOpen = (event, item) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedItem(item);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleActionClick = (action) => {
    action.onClick(selectedItem);
    handleMenuClose();
  };

  const handleHeaderClick = (column) => {
    if (!column.sortable || !onSortChange) return;
    
    const direction =
      sortConfig?.field === column.field && sortConfig?.direction === "asc"
        ? "desc"
        : "asc";
    
    onSortChange({ field: column.field, direction });
  };

  return (
    <StyledPaper>
      <Table>
        <TableHead>
          <TableRow>
            {columns.map((column) => (
              <StyledTableHeaderCell
                key={column.field}
                align={column.align || "left"}
                onClick={() => handleHeaderClick(column)}
                sx={{ cursor: column.sortable ? 'pointer' : 'default' }}
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
          {data.map((item) => (
            <StyledTableRow
              key={item.id || item.key}
              onClick={() => onRowClick?.(item)}
              sx={{ cursor: onRowClick ? "pointer" : "default" }}
            >
              {columns.map((column) => (
                <StyledTableCell 
                  key={`${item.id || item.key}-${column.field}`}
                  align={column.align || "left"}
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
  );
};

export default DataTable;