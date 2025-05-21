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
import useTransferList from "./useTransferList";
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
  onChange,
  idField = "id",
  onSearch,
  enableSearch = false,
  onLoadMore,
  hasMore = true,
  isLoadingMore = false,
}) => {
  const {
    rightBoxRef,
    filteredAvailable,
    selected,
    searchTerm,
    isSearching,
    handleSearchChange,
    handleAddItem,
    handleRemoveItem,
  } = useTransferList({
    availableItems,
    selectedItems,
    idField,
    onChange,
    onSearch,
    onLoadMore,
    hasMore,
    isLoadingMore,
  });

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
          items={selected}
          columns={columns}
          idField={idField}
          isLeftSide={true}
          onRemoveItem={handleRemoveItem}
        />
      </TransferBox>

      <TransferBox ref={rightBoxRef}>
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
          <TransferListTable
            items={filteredAvailable}
            columns={columns}
            idField={idField}
            isLeftSide={false}
            onAddItem={handleAddItem}
          />
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