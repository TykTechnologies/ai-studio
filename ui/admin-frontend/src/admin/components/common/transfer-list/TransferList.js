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
              placeholder="Search"
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
            />
          </InfiniteScrollContainer>
        )}
        
        {isLoadingMore && !isSearching && (
          <Box display="flex" justifyContent="center" p={2}>
            <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
              Loading more users...
            </Typography>
          </Box>
        )}
      </TransferBox>
    </TransferListContainer>
  );
};

export default TransferList;