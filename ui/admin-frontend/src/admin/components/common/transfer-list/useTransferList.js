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
      item => !selected.some(s => s[idField] === item[idField])
    );
    
    setFilteredAvailable(filtered);
  }, [available, selected, idField]);

  useEffect(() => {
    const handleScroll = () => {
      if (!rightBoxRef.current || !onLoadMore || !hasMore || isLoadingMore) return;

      const { scrollTop, scrollHeight, clientHeight } = rightBoxRef.current;

      if (scrollHeight - scrollTop - clientHeight < 50) {
        onLoadMore();
      }
    };

    const rightBox = rightBoxRef.current;
    if (rightBox) {
      rightBox.addEventListener('scroll', handleScroll);
    }

    return () => {
      if (rightBox) {
        rightBox.removeEventListener('scroll', handleScroll);
      }
    };
  }, [onLoadMore, hasMore, isLoadingMore, rightBoxRef]);

  const handleSearchChange = useCallback((e) => {
    const value = e.target.value;
    setSearchTerm(value);

    if (onSearch) {
      setIsSearching(true);

      const timeoutId = setTimeout(() => {
        onSearch(value);
        setIsSearching(false);
      }, 500);

      return () => clearTimeout(timeoutId);
    }
  }, [onSearch]);

  const handleAddItem = (item) => {
    if (onChange) {
      const newAvailable = available.filter(
        (i) => i[idField] !== item[idField]
      );
      const newSelected = [...selected, item];
      
      onChange({ selected: newSelected, available: newAvailable });

      if (searchTerm.trim() && onSearch) {
        setIsSearching(true);
        onSearch(searchTerm)
          .finally(() => setIsSearching(false));
      }
    }
  };

  const handleRemoveItem = (item) => {
    if (onChange) {
      const newSelected = selected.filter(
        (i) => i[idField] !== item[idField]
      );
      const newAvailable = [...available, item];
      
      onChange({ selected: newSelected, available: newAvailable });

      if (searchTerm.trim() && onSearch) {
        setIsSearching(true);
        onSearch(searchTerm)
          .finally(() => setIsSearching(false));
      }
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