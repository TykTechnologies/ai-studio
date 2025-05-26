import { useState, useEffect, useCallback, useRef } from "react";
import { useDebouncedCallback } from 'use-debounce';

const useTransferList = ({
  availableItems = [],
  selectedItems = [],
  idField = "id",
  onChange,
  onSearch,
  onLoadMore,
  hasMore = true,
  isLoadingMore = false,
  onItemAdded,
  onItemRemoved,
}) => {
  const rightBoxRef = useRef(null);
  const leftBoxRef = useRef(null);
  const onSearchRef = useRef(onSearch);
  const [searchTerm, setSearchTerm] = useState("");
  const [isSearching, setIsSearching] = useState(false);
  const available = availableItems;
  const selected = selectedItems;
  const prevAvailableItemsRef = useRef(availableItems);

  useEffect(() => {
    onSearchRef.current = onSearch;
  }, [onSearch]);

  useEffect(() => {
    const rightBox = rightBoxRef?.current;
    const handleScroll = () => {
      if (!rightBox|| !onLoadMore || !hasMore || isLoadingMore) return;

      const { scrollTop, scrollHeight, clientHeight } = rightBox;

      if (scrollHeight - scrollTop - clientHeight < 50) {
        onLoadMore();
      }
    };

    if (rightBox) {
      rightBox.addEventListener('scroll', handleScroll);
      
      return () => {
        rightBox.removeEventListener('scroll', handleScroll);
      };
    }
    
    return () => {}; 
  }, [onLoadMore, hasMore, isLoadingMore]);

  const debouncedSearchExecutor = useDebouncedCallback(async (value) => {
    if (!onSearchRef.current) {
      setIsSearching(false);
      return;
    }

    setIsSearching(true);
    try {
      await onSearchRef.current(value);
    } catch (error) {
      console.error('[useTransferList] Debounced search error:', error);
    }
  }, 500);

  const handleSearchChange = useCallback((e) => {
    const value = e.target.value;
    setSearchTerm(value);
    setIsSearching(true);
    debouncedSearchExecutor(value);
  }, [debouncedSearchExecutor]);

  const handleAddItem = (item) => {
    if (onChange) {
      const newAvailable = available.filter(
        (i) => i[idField] !== item[idField]
      );
      const newSelected = [item, ...selected];
      
      onChange({ selected: newSelected, available: newAvailable });
      
      if (onItemAdded) {
        onItemAdded(item);
      }
    }
  };

  const handleRemoveItem = (item) => {
    if (onChange) {
      const newSelected = selected.filter(
        (i) => i[idField] !== item[idField]
      );
      const newAvailable = [item, ...available];
      
      onChange({ selected: newSelected, available: newAvailable });
      
      if (onItemRemoved) {
        onItemRemoved(item);
      }
    }
  };

  useEffect(() => {
    const availableChanged = prevAvailableItemsRef.current !== availableItems;

    if (isSearching && availableChanged) {
      setIsSearching(false);
    }

    prevAvailableItemsRef.current = availableItems;
  }, [availableItems, isSearching]);

  return {
    leftBoxRef,
    rightBoxRef,
    available,
    selected,
    searchTerm,
    isSearching,
    handleSearchChange,
    handleAddItem,
    handleRemoveItem,
  };
};

export default useTransferList;