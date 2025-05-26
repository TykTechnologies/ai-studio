import { useState, useEffect, useCallback } from "react";
import { getUsers } from "../../../services/userService";
import { teamsService } from "../../../services/teamsService";

export const useTeamMembersModal = (groupId) => {
  const [users, setUsers] = useState([]);
  const [availableUsers, setAvailableUsers] = useState([]);
  const [selectedUsers, setSelectedUsers] = useState([]);
  const [recentlyRemovedUsers, setRecentlyRemovedUsers] = useState([]);
  const [currentPage, setCurrentPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [currentSearchTerm, setCurrentSearchTerm] = useState("");
  const [lastSearchResults, setLastSearchResults] = useState([]);
  const [searchCurrentPage, setSearchCurrentPage] = useState(1);
  const [searchTotalPages, setSearchTotalPages] = useState(1);
  const [isSearchLoadingMore, setIsSearchLoadingMore] = useState(false);
  const [loading, setLoading] = useState(false);

  const idField = "id";

  const resetState = useCallback(() => {
    setUsers([]);
    setAvailableUsers([]);
    setSelectedUsers([]);
    setRecentlyRemovedUsers([]);
    setCurrentPage(1);
    setTotalPages(1);
    setIsLoadingMore(false);
    setCurrentSearchTerm("");
    setLastSearchResults([]);
    setSearchCurrentPage(1);
    setSearchTotalPages(1);
    setIsSearchLoadingMore(false);
    setLoading(false);
  }, []);

  const fetchGroupMembers = useCallback(async () => {
    if (!groupId) {
      return;
    }
    setLoading(true);
    try {
      const response = await teamsService.getTeamUsers(groupId, { all: true });      
      const newSelectedUsers = response.data || [];
      setSelectedUsers(newSelectedUsers);
    } catch (error) {
      console.error("[fetchGroupMembers] Error fetching group members:", error);
      setSelectedUsers([]);
    } finally {
    }
  }, [groupId]);

  const fetchUsers = useCallback(async (page = 1) => {
    if (!groupId) {
      return;
    }

    if (page === 1) {
      setLoading(true);
    } else {
      setIsLoadingMore(true);
    }

    try {
      const options = { exclude_group_id: groupId, page };
      const response = await getUsers(page, options);
      const responseData = response.data || [];
      
      if (page === 1) {
        setUsers(responseData);
      } else {
        setUsers(prevUsers => {
          const newUsers = responseData.filter(newUser => !prevUsers.some(existingUser => existingUser[idField] === newUser[idField]));
          return [...prevUsers, ...newUsers];
        });
      }
      const newTotalPages = response.meta?.last_page || response.totalPages || 1;
      setTotalPages(newTotalPages);
      setCurrentPage(page);
    } catch (error) {
      console.error("[fetchUsers] Error fetching users:", error);
      if (page === 1) {
        setUsers([]);
      }
      setTotalPages(1);
    } finally {
      if (page === 1) {
        setLoading(false);
      }
      setIsLoadingMore(false);
    }
  }, [groupId, idField]);

  useEffect(() => {
    if (groupId) {
      setUsers([]);
      setSelectedUsers([]);
      setAvailableUsers([]);
      setRecentlyRemovedUsers([]);
      setCurrentPage(1);
      setTotalPages(1);
      setIsLoadingMore(false);
      setCurrentSearchTerm("");
      setLastSearchResults([]);
      setSearchCurrentPage(1);
      setSearchTotalPages(1);
      setIsSearchLoadingMore(false);

      (async () => {
        await fetchGroupMembers(); 
        await fetchUsers(1);     
      })();
    } else {
      resetState();
    }
  }, [groupId, fetchGroupMembers, fetchUsers, resetState]);


  useEffect(() => {
    let sourceList = currentSearchTerm ? lastSearchResults : users;
    const selectedIds = selectedUsers.map(u => u[idField]);

    let displayList = sourceList.filter(user => !selectedIds.includes(user[idField]));
    
    if (!currentSearchTerm) {
      const uniqueRecentlyRemoved = recentlyRemovedUsers.filter(
        removedUser => !selectedIds.includes(removedUser[idField]) && 
                       !displayList.some(dlUser => dlUser[idField] === removedUser[idField])
      );
      if (uniqueRecentlyRemoved.length > 0) {
        displayList = [...uniqueRecentlyRemoved, ...displayList];
      }
    }

    setAvailableUsers(prevAvailable => {
      if (prevAvailable.length === displayList.length && prevAvailable.every((u, i) => u[idField] === displayList[i]?.[idField])) {
        return prevAvailable;
      }
      return displayList;
    });
  }, [users, selectedUsers, lastSearchResults, currentSearchTerm, recentlyRemovedUsers, idField]);


  const handleUserAdded = useCallback((item) => {
    setSelectedUsers(prev => {
      if (!prev.some(user => user[idField] === item[idField])) {
        return [...prev, item];
      }
      return prev;
    });
    setRecentlyRemovedUsers(prev => {
      const newRecentlyRemoved = prev.filter(user => user[idField] !== item[idField]);
      if (newRecentlyRemoved.length === prev.length) {
        return prev;
      }
      return newRecentlyRemoved;
    });
  }, [idField]);

  const handleUserRemoved = useCallback((item) => {
    setSelectedUsers(prev => {
      const newSelected = prev.filter(user => user[idField] !== item[idField]);
      if (newSelected.length === prev.length) {
        return prev;
      }
      return newSelected;
    });
    setRecentlyRemovedUsers(prev => {
      if (!prev.some(user => user[idField] === item[idField])) {
        return [item, ...prev];
      }
      return prev;
    });
  }, [idField]);

  const handleUsersChange = useCallback(({ selected }) => {
    setSelectedUsers(currentSelected => {
      if (currentSelected === selected) return currentSelected;
      if (currentSelected.length === selected.length && currentSelected.every((u, i) => u[idField] === selected[i]?.[idField])) {
        return currentSelected;
      }
      return selected;
    });
  }, []);

  const handleSearch = useCallback(async (searchTerm, page = 1) => {
    if (!groupId) {
      return;
    }
    
    setCurrentSearchTerm(searchTerm);
    
    if (!searchTerm.trim()) {
      setLastSearchResults([]);
      setSearchCurrentPage(1);
      setSearchTotalPages(1);
      return;
    }

    if (page === 1) {
      setLastSearchResults([]);
    } else {
      setIsSearchLoadingMore(true);
    }

    try {
      const options = { exclude_group_id: groupId, search: searchTerm, page };
      const response = await getUsers(page, options);
      const searchResults = response.data || [];
      
      if (page === 1) {
        setLastSearchResults(searchResults);
      } else {
        setLastSearchResults(prev => {
          const newResults = searchResults.filter(newRes => !prev.some(existRes => existRes[idField] === newRes[idField]));
          return [...prev, ...newResults];
        });
      }
      const newSearchTotalPages = response.meta?.last_page || response.totalPages || 1;
      setSearchTotalPages(newSearchTotalPages);
      setSearchCurrentPage(page);
    } catch (error) {
      console.error("[handleSearch] Error searching users:", error);
      if (page === 1) {
        setLastSearchResults([]);
      }
      setSearchTotalPages(1);
    } finally {
      if (page === 1) {
      }
      setIsSearchLoadingMore(false);
    }
  }, [groupId, idField]);

  const handleLoadMore = useCallback(() => {
    if (currentSearchTerm) {
      if (searchCurrentPage < searchTotalPages && !isSearchLoadingMore) {
        handleSearch(currentSearchTerm, searchCurrentPage + 1);
      }
    } else {
      if (currentPage < totalPages && !isLoadingMore) {
        fetchUsers(currentPage + 1);
      }
    }
  }, [
    currentPage, totalPages, isLoadingMore, fetchUsers,
    currentSearchTerm, searchCurrentPage, searchTotalPages, isSearchLoadingMore, handleSearch
  ]);
  


  return {
    availableUsers,
    selectedUsers,
    setSelectedUsers, 
    hasMore: currentSearchTerm ? searchCurrentPage < searchTotalPages : currentPage < totalPages,
    isLoadingMore: currentSearchTerm ? isSearchLoadingMore : isLoadingMore,
    loading,
    handleUsersChange, 
    handleLoadMore,
    handleSearch,
    currentSearchTerm,
    handleUserAdded, 
    handleUserRemoved, 
  };
}