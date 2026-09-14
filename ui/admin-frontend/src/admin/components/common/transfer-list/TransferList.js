import React from "react";
import {
  Box,
  Typography,
  InputAdornment
} from "@mui/material";
import SearchIcon from "@mui/icons-material/Search";
import {
  StyledTextField,
} from "../../../styles/sharedStyles";
import TransferListTable from "./TransferListTable";
import InfiniteScrollContainer from "../InfiniteScrollContainer";
import {
  TransferListContainer,
  TransferBox,
  HeaderBox,
  SearchContainer,
} from "./styles";

const TransferList = ({
  availableItems = [],
  selectedItems = [],
  columns = [],
  leftTitle,
  leftSubtitle,
  rightTitle,
  rightSubtitle,
  idField = "id",
  enableSearch = false,
  searchTerm = "",
  onSearchTermChange,
  isSearching = false,
  onAdd,
  onRemove,
  onLoadMore,
  hasMore = false,
  isLoadingMore = false,
  // Singular noun used in copy ("Loading more users..."); was hardcoded to "users".
  itemLabel = "item",
  // Optional accessor for a human-readable name, used for the per-row
  // "Add {name}" / "Remove {name}" aria-labels.
  getItemName,
  // Accessible name and placeholder for the search box.
  searchLabel,
  searchPlaceholder = "Search",
  disabled = false,
}) => {
  const handleSearchChange = (e) => {
    const value = e.target.value;
    onSearchTermChange?.(value);
  };

  const handleAddItem = (item) => {
    onAdd?.(item);
  };

  const handleRemoveItem = (item) => {
    onRemove?.(item);
  };

  return (
    <TransferListContainer>
      <TransferBox>
        <HeaderBox>
          <Typography variant="headingSmall" color="text.primary">
            {leftTitle}
          </Typography>
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
            {leftSubtitle}
          </Typography>
        </HeaderBox>
        <TransferListTable
          items={selectedItems}
          columns={columns}
          idField={idField}
          isLeftSide={true}
          onRemoveItem={handleRemoveItem}
          getItemName={getItemName}
          disabled={disabled}
        />
      </TransferBox>

      <TransferBox>
        <HeaderBox>
          <Typography variant="headingSmall" color="text.primary">
            {rightTitle}
          </Typography>
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
            {rightSubtitle}
          </Typography>
        </HeaderBox>
        {enableSearch && (
          <SearchContainer>
            <StyledTextField
              placeholder={searchPlaceholder}
              variant="outlined"
              fullWidth
              value={searchTerm}
              onChange={handleSearchChange}
              disabled={disabled}
              inputProps={searchLabel ? { "aria-label": searchLabel } : undefined}
              InputProps={{
                startAdornment: (
                  <InputAdornment position="start">
                    <SearchIcon color="action" />
                  </InputAdornment>
                ),
              }}
            />
          </SearchContainer>
        )}
        {isSearching ? (
          <Box display="flex" justifyContent="center" p={2}>
            <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
              Searching...
            </Typography>
          </Box>
        ) : (
          <InfiniteScrollContainer
            onLoadMore={onLoadMore}
            hasMore={hasMore}
            isLoading={isLoadingMore}
          >
            <TransferListTable
              items={availableItems}
              columns={columns}
              idField={idField}
              isLeftSide={false}
              onAddItem={handleAddItem}
              getItemName={getItemName}
              disabled={disabled}
            />
          </InfiniteScrollContainer>
        )}
        
        {isLoadingMore && !isSearching && (
          <Box display="flex" justifyContent="center" p={2}>
            <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
              Loading more {itemLabel}s...
            </Typography>
          </Box>
        )}
      </TransferBox>
    </TransferListContainer>
  );
};

export default TransferList;