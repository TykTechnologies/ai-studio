import { useState, useEffect, useCallback, useMemo } from 'react';
import { useDebouncedCallback } from 'use-debounce';
import { getUsers } from '../services/userService';

export function useTransferListAvailableUsers({ groupId, excludeIds = [], pageSize = 10, searchDebounceMs = 400 }) {
  const [items, setItems] = useState([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [isInitialLoading, setIsInitialLoading] = useState(true);
  const [isSearching, setIsSearching] = useState(false);
  const [recentlyRemoved, setRecentlyRemoved] = useState([]);
  const idField = 'id';

  const resetAll = useCallback(() => {
    setItems([]);
    setSearchTerm('');
    setPage(1);
    setTotalPages(1);
    setIsLoadingMore(false);
    setIsInitialLoading(true);
    setIsSearching(false);
    setRecentlyRemoved([]);
  }, []);


  const internalFetch = useCallback(async (targetPage = 1, term = '') => {
    if (!groupId) return;

    const firstPage = targetPage === 1;

    const params = {
      exclude_group_id: groupId,
      page: targetPage,
      page_size: pageSize,
      search: term
    };

    try {
      const { data = [], meta, totalPages: respTotalPages } = await getUsers(targetPage, params);
      const newTotal =
        meta?.last_page ||
        meta?.totalPages ||
        meta?.total_pages ||
        respTotalPages ||
        1;

      setTotalPages(newTotal);
      setPage(targetPage);

      setItems(prev => {
        if (firstPage) {
          return data;
        }

        const existingIds = new Set(prev.map(i => i[idField]));
        const unique = data.filter(u => !existingIds.has(u[idField]));
        return [...prev, ...unique];
      });
    } catch (err) {
      setItems([]);
    } finally {
      if (firstPage) {
        setIsInitialLoading(false);
        setIsSearching(false);
      } else {
        setIsLoadingMore(false);
      }
    }
  }, [groupId, pageSize, idField]);

  useEffect(() => {
    if (groupId) {
        resetAll();
        internalFetch(1, '');
    }
  }, [groupId, internalFetch, resetAll]);

  const debouncedSearch = useDebouncedCallback((value) => {
    setItems([]);
    internalFetch(1, value);
  }, searchDebounceMs);

  const search = useCallback((value) => {
    setIsSearching(true);
    setSearchTerm(value);
    debouncedSearch(value);
  }, [debouncedSearch]);

  const loadMore = useCallback(() => {
    const next = page + 1;
    if (next <= totalPages && !isLoadingMore) {
      internalFetch(next, searchTerm);
    }
  }, [page, totalPages, isLoadingMore, searchTerm, internalFetch]);

  const addItem = useCallback((item) => {
    setRecentlyRemoved((prev) => {
      if (prev.some((r) => r[idField] === item[idField])) return prev;
      return [item, ...prev];
    });
  }, [idField]);

  const removeItem = useCallback((item) => {
    setItems((prev) => prev.filter((i) => i[idField] !== item[idField]));
    setRecentlyRemoved((prev) => prev.filter((r) => r[idField] !== item[idField]));
  }, [idField]);

  const excludedSet = useMemo(() => new Set(excludeIds), [excludeIds]);

  const displayItems = useMemo(() => {
    const baseItems = items.filter(i => !excludedSet.has(i[idField]));

    if (searchTerm.trim().length > 0) {
      return baseItems;
    }

    const filteredRecentlyRemoved = recentlyRemoved.filter(r => !excludedSet.has(r[idField]));

    return [
      ...filteredRecentlyRemoved,
      ...baseItems.filter(i => !filteredRecentlyRemoved.some(r => r[idField] === i[idField]))
    ];
  }, [items, recentlyRemoved, searchTerm, idField, excludedSet]);

  return {
    items: displayItems,
    loading: isInitialLoading,
    isSearching,
    isLoadingMore,
    hasMore: page < totalPages,
    searchTerm,
    search,
    loadMore,
    addItem,
    removeItem,
    reset: resetAll,
  };
} 