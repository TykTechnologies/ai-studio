import React, { useState, useCallback, useEffect, useMemo, useRef, memo } from "react";
import {
  Table,
  TableBody,
  TableHead,
  TableRow,
  IconButton,
  Menu,
  MenuItem,
  Box,
  Checkbox,
  TableSortLabel,
  Skeleton,
  Tooltip,
  Typography,
} from "@mui/material";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import { useDebounce } from "use-debounce";
import {
  StyledPaper,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
} from "../../styles/sharedStyles";
import PaginationControls from "./PaginationControls";
import SearchInput from "./SearchInput";
import BulkActionsToolbar from "./BulkActionsToolbar";

/**
 * The one list table (UX review M3 / F-21). Every admin list renders through
 * it so search, sorting, selection, bulk actions, loading and empty states
 * look and behave the same everywhere.
 *
 * Props (all optional except columns/data):
 * - columns: [{ field, headerName, sortable, align, sx, renderCell(item), cellProps(item) }]
 * - data: rows; `rowKey` (default "id") names the identity field.
 * - actions: [{ key, label | label(item), onClick(item), disabled | disabled(item),
 *     disabledReason, hidden(item), icon, "data-testid" }] or a function
 *     (item) => actions. Rendered as the per-row overflow menu.
 * - pagination: { page, pageSize, totalPages, onPageChange, onPageSizeChange }
 * - onRowClick(item)
 * - sortConfig: { field, direction } (legacy { key, direction } accepted);
 *   onSortChange({ field, direction }).
 * - enableSearch, searchTerm, onSearch(term), searchPlaceholder,
 *   searchDebounceMs (default 400; the input is debounced here, so pages
 *   receive the settled term).
 * - loading: skeleton rows when there is no data yet, aria-busy always.
 * - emptyState: node shown instead of the table when there is no data at all;
 *   emptyMessage: text for the in-table empty row (defaults to
 *   "No matches for “term”" while a search is active, else "No results").
 * - selectable, selectedIds, onSelectionChange(ids), bulkActions:
 *   [{ key, label, onClick(selectedItems), disabled(selectedItems),
 *   disabledReason, danger }] — a toolbar appears above the table while
 *   something is selected.
 * - rowProps(item): extra props for the row (data-testid and the like).
 * - getRowLabel(item): the name used in "Select {name}" / "Actions for {name}".
 * - ariaLabel / aria-label: the table's accessible name.
 */

const normaliseSort = (sortConfig) => {
  if (!sortConfig) return null;
  const field = sortConfig.field ?? sortConfig.key ?? null;
  if (!field) return null;
  return { field, direction: sortConfig.direction === "desc" ? "desc" : "asc" };
};

export const defaultRowLabel = (item) => {
  const attributes = item?.attributes || {};
  const label =
    attributes.name ??
    item?.name ??
    attributes.var_name ??
    attributes.model_name ??
    attributes.email ??
    item?.email ??
    item?.id ??
    item?.key;
  return label === undefined || label === null ? "" : String(label);
};

const cellValue = (item, field) => {
  const value = item?.[field];
  return value === undefined || value === null || value === "" ? "-" : value;
};

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
  searchDebounceMs = 400,
  loading = false,
  emptyState,
  emptyMessage,
  selectable = false,
  selectedIds,
  onSelectionChange,
  bulkActions,
  rowKey = "id",
  rowProps,
  getRowLabel = defaultRowLabel,
  ariaLabel,
  "aria-label": ariaLabelAttr,
  skeletonRowCount = 5,
}) => {
  const rows = useMemo(() => (Array.isArray(data) ? data : []), [data]);
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedItem, setSelectedItem] = useState(null);

  // --- search: the input is debounced here so pages see the settled term ---
  const [searchInput, setSearchInput] = useState(searchTerm);
  const [debouncedInput] = useDebounce(searchInput, searchDebounceMs);
  const lastEmittedRef = useRef(searchTerm);

  // An external reset (the page clearing its term) syncs the box; the page
  // echoing back what we just emitted does not, so typing is never clobbered.
  useEffect(() => {
    if (searchTerm !== lastEmittedRef.current) {
      lastEmittedRef.current = searchTerm;
      setSearchInput(searchTerm);
    }
  }, [searchTerm]);

  useEffect(() => {
    if (!onSearch) return;
    if (debouncedInput === lastEmittedRef.current) return;
    lastEmittedRef.current = debouncedInput;
    onSearch(debouncedInput);
  }, [debouncedInput, onSearch]);

  // --- row menu ---
  const handleMenuOpen = useCallback((event, item) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedItem(item);
  }, []);

  const handleMenuClose = useCallback(() => {
    setAnchorEl(null);
  }, []);

  const handleActionClick = useCallback((action) => {
    action.onClick?.(selectedItem);
    handleMenuClose();
  }, [selectedItem, handleMenuClose]);

  const hasActions =
    typeof actions === "function" || (Array.isArray(actions) && actions.length > 0);

  const menuActions = useMemo(() => {
    if (!hasActions || !selectedItem) return [];
    const list = typeof actions === "function" ? actions(selectedItem) : actions;
    return (list || []).filter((action) => !(typeof action.hidden === "function" && action.hidden(selectedItem)));
  }, [actions, hasActions, selectedItem]);

  // --- sorting ---
  const activeSort = normaliseSort(sortConfig);

  const handleHeaderClick = useCallback((column) => {
    if (!column.sortable || !onSortChange) return;
    const direction =
      activeSort?.field === column.field && activeSort?.direction === "asc"
        ? "desc"
        : "asc";
    onSortChange({ field: column.field, direction });
  }, [activeSort, onSortChange]);

  // --- selection ---
  const keyOf = useCallback((item) => String(item?.[rowKey] ?? item?.key ?? ""), [rowKey]);
  const selectedSet = useMemo(
    () => new Set((selectedIds || []).map((id) => String(id))),
    [selectedIds],
  );
  const pageKeys = useMemo(() => rows.map(keyOf), [rows, keyOf]);
  const selectedOnPage = useMemo(
    () => pageKeys.filter((key) => selectedSet.has(key)),
    [pageKeys, selectedSet],
  );
  const allOnPageSelected = rows.length > 0 && selectedOnPage.length === rows.length;
  const someOnPageSelected = selectedOnPage.length > 0 && !allOnPageSelected;
  const selectedItems = useMemo(
    () => rows.filter((item) => selectedSet.has(keyOf(item))),
    [rows, selectedSet, keyOf],
  );
  const selectedCount = selectedIds ? selectedIds.length : 0;

  const emitSelection = useCallback((keys) => {
    onSelectionChange?.(keys);
  }, [onSelectionChange]);

  const handleToggleAll = useCallback(() => {
    const current = (selectedIds || []).map((id) => String(id));
    if (allOnPageSelected) {
      const pageSet = new Set(pageKeys);
      emitSelection(current.filter((id) => !pageSet.has(id)));
    } else {
      emitSelection(Array.from(new Set([...current, ...pageKeys])));
    }
  }, [selectedIds, allOnPageSelected, pageKeys, emitSelection]);

  const handleToggleRow = useCallback((item) => {
    const key = keyOf(item);
    const current = (selectedIds || []).map((id) => String(id));
    emitSelection(
      current.includes(key) ? current.filter((id) => id !== key) : [...current, key],
    );
  }, [selectedIds, keyOf, emitSelection]);

  // --- empty / loading ---
  const isEmpty = !loading && rows.length === 0;
  const showSkeleton = loading && rows.length === 0;
  const columnCount = columns.length + (selectable ? 1 : 0) + (hasActions ? 1 : 0);
  const resolvedEmptyMessage =
    emptyMessage ?? (searchTerm ? `No matches for “${searchTerm}”` : "No results");
  const tableLabel = ariaLabel || ariaLabelAttr;

  if (isEmpty && emptyState) {
    return <>{emptyState}</>;
  }

  const showBulkToolbar = selectable && selectedCount > 0 && Array.isArray(bulkActions) && bulkActions.length > 0;

  return (
    <>
    {enableSearch && (
      <Box sx={{ mb: 2, maxWidth: 400 }}>
        <SearchInput
          value={searchInput}
          onChange={setSearchInput}
          placeholder={searchPlaceholder}
        />
      </Box>
    )}
    <StyledPaper>
      {showBulkToolbar && (
        <BulkActionsToolbar
          count={selectedCount}
          selectedItems={selectedItems}
          actions={bulkActions}
          onClear={() => emitSelection([])}
        />
      )}
      <Table aria-label={tableLabel} aria-busy={loading ? "true" : undefined}>
        <TableHead>
          <TableRow>
            {selectable && (
              <StyledTableHeaderCell padding="checkbox">
                <Checkbox
                  checked={allOnPageSelected}
                  indeterminate={someOnPageSelected}
                  onChange={handleToggleAll}
                  disabled={rows.length === 0}
                  inputProps={{ "aria-label": "Select all on this page" }}
                />
              </StyledTableHeaderCell>
            )}
            {columns.map((column) => {
              const isSorted = activeSort?.field === column.field;
              const canSort = Boolean(column.sortable && onSortChange);
              return (
                <StyledTableHeaderCell
                  key={column.field}
                  align={column.align || "left"}
                  sortDirection={isSorted ? activeSort.direction : false}
                  sx={column.sx}
                >
                  {canSort ? (
                    <TableSortLabel
                      active={isSorted}
                      direction={isSorted ? activeSort.direction : "asc"}
                      onClick={() => handleHeaderClick(column)}
                      aria-label={`Sort by ${column.headerName}`}
                    >
                      {column.headerName}
                    </TableSortLabel>
                  ) : (
                    column.headerName
                  )}
                </StyledTableHeaderCell>
              );
            })}
            {hasActions && (
              <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
            )}
          </TableRow>
        </TableHead>
        <TableBody sx={loading && rows.length > 0 ? { opacity: 0.6 } : undefined}>
          {showSkeleton &&
            Array.from({ length: skeletonRowCount }).map((_, index) => (
              <TableRow key={`skeleton-${index}`} data-testid="skeleton-row">
                {Array.from({ length: columnCount }).map((__, cellIndex) => (
                  <StyledTableCell key={cellIndex}>
                    <Skeleton variant="text" />
                  </StyledTableCell>
                ))}
              </TableRow>
            ))}
          {isEmpty && (
            <TableRow>
              <StyledTableCell colSpan={columnCount} align="center">
                <Typography variant="bodyMediumDefault" color="text.defaultSubdued" data-testid="data-table-empty">
                  {resolvedEmptyMessage}
                </Typography>
              </StyledTableCell>
            </TableRow>
          )}
          {rows.map((item) => {
            const key = keyOf(item);
            const label = getRowLabel(item);
            const isSelected = selectedSet.has(key);
            return (
              <StyledTableRow
                key={key}
                onClick={() => onRowClick?.(item)}
                sx={{ cursor: onRowClick ? "pointer" : "default" }}
                selected={isSelected}
                aria-selected={selectable ? isSelected : undefined}
                {...(rowProps ? rowProps(item) : {})}
              >
                {selectable && (
                  <StyledTableCell padding="checkbox" onClick={(event) => event.stopPropagation()}>
                    <Checkbox
                      checked={isSelected}
                      onChange={() => handleToggleRow(item)}
                      inputProps={{ "aria-label": `Select ${label}` }}
                    />
                  </StyledTableCell>
                )}
                {columns.map((column) => (
                  <StyledTableCell
                    key={`${key}-${column.field}`}
                    align={column.align || "left"}
                    sx={column.sx}
                    {...(column.cellProps ? column.cellProps(item) : {})}
                  >
                    {column.renderCell
                      ? column.renderCell(item)
                      : cellValue(item, column.field)}
                  </StyledTableCell>
                ))}
                {hasActions && (
                  <StyledTableCell align="right">
                    <IconButton
                      onClick={(event) => handleMenuOpen(event, item)}
                      aria-label={`Actions for ${label}`}
                      data-testid="row-actions"
                    >
                      <MoreVertIcon />
                    </IconButton>
                  </StyledTableCell>
                )}
              </StyledTableRow>
            );
          })}
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
        {menuActions.map((action) => {
          const label = typeof action.label === "function" ? action.label(selectedItem) : action.label;
          const disabled =
            typeof action.disabled === "function" ? action.disabled(selectedItem) : Boolean(action.disabled);
          const menuItem = (
            <MenuItem
              key={action.key || label}
              disabled={disabled}
              onClick={() => handleActionClick(action)}
              data-testid={action["data-testid"]}
            >
              {action.icon}
              {label}
            </MenuItem>
          );
          if (disabled && action.disabledReason) {
            return (
              <Tooltip key={action.key || label} title={action.disabledReason} placement="left">
                <span>{menuItem}</span>
              </Tooltip>
            );
          }
          return menuItem;
        })}
      </Menu>
    </StyledPaper>
    </>
  );
});

DataTable.displayName = 'DataTable';

export default DataTable;
