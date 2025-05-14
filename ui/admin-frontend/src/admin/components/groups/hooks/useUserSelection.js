import { useState, useEffect, useCallback } from "react";
import { getUsers, searchUsers } from "../../../services/userService";

export const useUserSelection = (initialSelectedUsers = [], parentSetSelectedUsers = null) => {
  const [users, setUsers] = useState([]);
  const [availableUsers, setAvailableUsers] = useState([]);
  const [selectedUsers, setSelectedUsers] = useState(initialSelectedUsers);
  const [currentPage, setCurrentPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [currentSearchTerm, setCurrentSearchTerm] = useState("");
  const [lastSearchResults, setLastSearchResults] = useState([]);
  const [loading, setLoading] = useState(false);

  const updateAvailableUsers = useCallback((userList) => {
    const filtered = userList.filter(
      user => !selectedUsers.find(selected => selected.id === user.id)
    );
    setAvailableUsers(filtered);
  }, [selectedUsers]);

  useEffect(() => {
    if (currentSearchTerm) {
      updateAvailableUsers(lastSearchResults);
    } else {
      updateAvailableUsers(users);
    }
  }, [users, selectedUsers, updateAvailableUsers, currentSearchTerm, lastSearchResults]);

  const fetchUsers = useCallback(async (page = 1) => {
    try {
      if (page === 1) {
        setLoading(true);
      } else {
        setIsLoadingMore(true);
      }

      const response = await getUsers(page);

      if (page === 1) {
        setUsers(response.data || []);
      } else {
        setUsers(prevUsers => [...prevUsers, ...(response.data || [])]);
      }

      setTotalPages(response.totalPages);
      setCurrentPage(page);

      if (page === 1) {
        setLoading(false);
      } else {
        setIsLoadingMore(false);
      }
    } catch (error) {
      console.error("Error fetching users", error);
      setLoading(false);
      setIsLoadingMore(false);
    }
  }, [setLoading, setIsLoadingMore, setUsers, setTotalPages, setCurrentPage]);

  const handleUsersChange = useCallback(({ selected }) => {
    setSelectedUsers(selected);
    
    if (parentSetSelectedUsers) {
      parentSetSelectedUsers(selected);
    }
  }, [setSelectedUsers, parentSetSelectedUsers]);

  const handleSearch = useCallback(async (searchTerm, page = 1) => {
    try {
      if (!searchTerm.trim()) {
        setCurrentSearchTerm("");
        updateAvailableUsers(users);
        return;
      }

      setCurrentSearchTerm(searchTerm);

      if (page === 1) {
        setIsLoadingMore(false);
      } else {
        setIsLoadingMore(true);
      }

      const response = await searchUsers(searchTerm, page);

      const searchResults = response.data || [];

      if (page === 1) {
        setLastSearchResults(searchResults);
      } else {
        setLastSearchResults(prevResults => {
          const newResults = searchResults.filter(
            sr => !prevResults.some(pr => pr.id === sr.id)
          );
          return [...prevResults, ...newResults];
        });
      }
      
      setTotalPages(response.totalPages);
      setCurrentPage(page);
      setIsLoadingMore(false);
    } catch (error) {
      console.error("Error searching users", error);
      setIsLoadingMore(false);
    }
  }, [users, updateAvailableUsers, setCurrentSearchTerm, setIsLoadingMore, setTotalPages, setCurrentPage, setLastSearchResults]);

  const handleLoadMore = useCallback(() => {
    if (currentPage < totalPages && !isLoadingMore) {
      if (currentSearchTerm) {
        handleSearch(currentSearchTerm, currentPage + 1);
      } else {
        fetchUsers(currentPage + 1);
      }
    }
  }, [currentPage, totalPages, isLoadingMore, currentSearchTerm, handleSearch, fetchUsers]);

  return {
    users,
    availableUsers,
    selectedUsers,
    setSelectedUsers,
    currentPage,
    totalPages,
    isLoadingMore,
    loading,
    fetchUsers,
    handleUsersChange,
    handleLoadMore,
    handleSearch,
    currentSearchTerm
  };
};