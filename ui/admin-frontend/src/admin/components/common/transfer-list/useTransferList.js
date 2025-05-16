import { useState, useEffect, useCallback, useRef } from "react";

const useTransferList = ({
  availableItems = [],
  selectedItems = [],
  idField = "id",
  onChange,
  onSearch,
  onLoadMore,
  hasMore = true,
  isLoadingMore = false,
}) => {
  const rightBoxRef = useRef(null);
  const [searchTerm, setSearchTerm] = useState("");
  const [isSearching, setIsSearching] = useState(false);
  const [filteredAvailable, setFilteredAvailable] = useState([]);
  const available = availableItems;
  const selected = selectedItems;

  useEffect(() => {
    const filtered = available.filter(
      item => selected.every(s => s[idField] !== item[idField])
    );
    
    setFilteredAvailable(filtered);
  }, [available, selected, idField]);

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

  const handleSearchChange = useCallback((e) => {
    const value = e.target.value;
    setSearchTerm(value);

    if (onSearch) {
      setIsSearching(true);

      setTimeout(() => {
        onSearch(value);
        setIsSearching(false);
      }, 500);
    }
  }, [onSearch]);

  const handleAddItem = (item) => {
    if (onChange) {
      const newAvailable = available.filter(
        (i) => i[idField] !== item[idField]
      );
      const newSelected = [...selected, item];
      
      onChange({ selected: newSelected, available: newAvailable });

    }
  };

  const handleRemoveItem = (item) => {
    if (onChange) {
      const newSelected = selected.filter(
        (i) => i[idField] !== item[idField]
      );
      const newAvailable = [...available, item];
      
      onChange({ selected: newSelected, available: newAvailable });

    }
  };

  return {
    rightBoxRef,
    available,
    selected,
    filteredAvailable,
    searchTerm,
    isSearching,
    handleSearchChange,
    handleAddItem,
    handleRemoveItem,
  };
};

export default useTransferList;