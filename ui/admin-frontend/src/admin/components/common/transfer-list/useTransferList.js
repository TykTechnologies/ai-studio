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
  const [isPerformingSearchQuery, setIsPerformingSearchQuery] = useState(false);
  const available = availableItems;
  const selected = selectedItems;

  // Update the ref when onSearch changes
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
    if (onSearchRef.current) {
      setIsPerformingSearchQuery(true);
      console.log('[useTransferList] Debounced search: START, isPerformingSearchQuery: true');
      try {
        await onSearchRef.current(value);
        console.log('[useTransferList] Debounced search: API call FINISHED');
      } catch (error) {
        console.error('[useTransferList] Debounced search error:', error);
      } finally {
        setIsPerformingSearchQuery(false);
        console.log('[useTransferList] Debounced search: END, isPerformingSearchQuery: false');
      }
    }
  }, 500);

  const handleSearchChange = useCallback((e) => {
    const value = e.target.value;
    setSearchTerm(value);
    console.log('[useTransferList] handleSearchChange: searchTerm set to', value, 'calling debouncedSearchExecutor');
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

  return {
    leftBoxRef,
    rightBoxRef,
    available,
    selected,
    searchTerm,
    isSearching: (() => {
      const searching = isPerformingSearchQuery || debouncedSearchExecutor.isPending();
      console.log('[useTransferList] isSearching calculated:', searching, { isPerformingSearchQuery, pending: debouncedSearchExecutor.isPending() });
      return searching;
    })(),
    handleSearchChange,
    handleAddItem,
    handleRemoveItem,
  };
};

export default useTransferList;