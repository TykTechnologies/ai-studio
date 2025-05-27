import { useState, useEffect, useCallback, useRef } from "react";
import { getUsers } from "../../../services/userService";
import { teamsService } from "../../../services/teamsService";

export const useUserSelection = (groupId, initialSelectedUsers = [], parentSetSelectedUsers = null) => {
  const [users, setUsers] = useState([]);
  const [availableUsers, setAvailableUsers] = useState([]);
  const [selectedUsers, setSelectedUsers] = useState(initialSelectedUsers);
  const newlySelectedIdsRef = useRef([]);
  const [recentlyRemovedUsers, setRecentlyRemovedUsers] = useState([]);
  const [currentPage, setCurrentPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [currentSearchTerm, setCurrentSearchTerm] = useState("");
  const [lastSearchResults, setLastSearchResults] = useState([]);
  const [loading, setLoading] = useState(false);

  const idField = "id";

  const fetchGroupMembers = useCallback(async () => {
    if (!groupId) return; 
    try {
      setLoading(true);
      
      const response = await teamsService.getTeamUsers(groupId, { all: true });
      
      setSelectedUsers(response.data || []);
      newlySelectedIdsRef.current = [];
      setRecentlyRemovedUsers([]);
      
      if (parentSetSelectedUsers) {
        parentSetSelectedUsers(response.data || []);
      }
    } catch (error) {
      console.error("Error fetching group members:", error);
    } finally {
      setLoading(false);
    }
  }, [groupId, parentSetSelectedUsers]);

  useEffect(() => {
    if (groupId) {
      fetchGroupMembers();
    }
  }, [groupId, fetchGroupMembers]);

  const updateAvailableUsers = useCallback((userList) => {
    if (currentSearchTerm) {
      setAvailableUsers(userList);
    } else {
      const filteredList = userList.filter(
        user => !recentlyRemovedUsers.some(removedUser => removedUser[idField] === user[idField])
      );
      setAvailableUsers([...recentlyRemovedUsers, ...filteredList]);
    }
  }, [recentlyRemovedUsers, idField, currentSearchTerm]);

  useEffect(() => {
    if (currentSearchTerm) {
      updateAvailableUsers(lastSearchResults);
    } else {
      updateAvailableUsers(users);
    }
  }, [users, updateAvailableUsers, currentSearchTerm, lastSearchResults]);

  const fetchUsers = useCallback(async (page = 1) => {
    try {
      if (page === 1) {
        setLoading(true);
      } else {
        setIsLoadingMore(true);
      }

      const options = groupId ? { exclude_group_id: groupId } : {};
      const response = await getUsers(page, options);

      const responseData = response.data || [];
      const filteredData = responseData.filter(user => !newlySelectedIdsRef.current.includes(user[idField]));

      if (page === 1) {
        setUsers(filteredData);
      } else {
        setUsers(prevUsers => [...prevUsers, ...filteredData]);
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
  }, [groupId, idField]);

  const handleUserAdded = (item) => {
    newlySelectedIdsRef.current = [...newlySelectedIdsRef.current, item[idField]];
    setRecentlyRemovedUsers(prev => prev.filter(user => user[idField] !== item[idField]));
  };

  const handleUserRemoved = (item) => {
    newlySelectedIdsRef.current = newlySelectedIdsRef.current.filter(id => id !== item[idField]);
    setRecentlyRemovedUsers(prev => {
      if (!prev.some(user => user[idField] === item[idField])) {
        return [item, ...prev];
      }
      return prev;
    });
  };

  const handleUsersChange = useCallback(({ selected, available }) => {
    setSelectedUsers(selected);
    
    if (available) {
      if (currentSearchTerm) {
        setLastSearchResults(available);
      } else {
        setUsers(available);
      }
    }
    
    if (parentSetSelectedUsers) {
      parentSetSelectedUsers(selected);
    }
  }, [setSelectedUsers, parentSetSelectedUsers, currentSearchTerm]);

  const handleSearch = useCallback(async (searchTerm, page = 1) => {
    try {
      setCurrentSearchTerm(searchTerm);

      if (page === 1) {
        setIsLoadingMore(false);
      } else {
        setIsLoadingMore(true);
      }

      const options = groupId ? { exclude_group_id: groupId, search: searchTerm } : { search: searchTerm };
      const response = await getUsers(page, options);
      
      const searchResults = response.data || [];
      const filteredResults = searchResults.filter(user => !newlySelectedIdsRef.current.includes(user[idField]));

      if (searchTerm) {
        if (page === 1) {
          setLastSearchResults(filteredResults);
        } else {
          setLastSearchResults(prev => [...prev, ...filteredResults]);
        }
      } else {
        setLastSearchResults([]);
        if (page === 1) {
          setUsers(filteredResults);
        } else {
          setUsers(prev => [...prev, ...filteredResults]);
        }
      }
      
      setTotalPages(response.totalPages);
      setCurrentPage(page);
      setIsLoadingMore(false);
    } catch (error) {
      console.error("Error searching users", error);
      setIsLoadingMore(false);
    }
  }, [groupId, idField]);

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
    currentSearchTerm,
    fetchGroupMembers,
    handleUserAdded,
    handleUserRemoved,
  };
};