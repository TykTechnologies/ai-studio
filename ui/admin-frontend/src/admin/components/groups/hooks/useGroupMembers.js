import { useState, useCallback } from "react";
import { teamsService } from "../../../services/teamsService";

export const useGroupMembers = (groupId, initialMembers = []) => {
  const [members, setMembers] = useState(initialMembers);
  const [currentPage, setCurrentPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [loading, setLoading] = useState(false);

  const fetchGroupMembers = useCallback(async (page = 1) => {
    if (!groupId) return;
    
    try {
      if (page === 1) {
        setLoading(true);
      } else {
        setIsLoadingMore(true);
      }

      const response = await teamsService.getTeamUsers(groupId, page);

      if (page === 1) {
        setMembers(response.data || []);
      } else {
        setMembers(prevMembers => [...prevMembers, ...(response.data || [])]);
      }

      setTotalPages(response.totalPages);
      setCurrentPage(page);

      if (page === 1) {
        setLoading(false);
      } else {
        setIsLoadingMore(false);
      }
    } catch (error) {
      console.error("Error fetching group members", error);
      setLoading(false);
      setIsLoadingMore(false);
    }
  }, [groupId]);

  const handleLoadMore = useCallback(() => {
    if (currentPage < totalPages && !isLoadingMore) {
      fetchGroupMembers(currentPage + 1);
    }
  }, [currentPage, totalPages, isLoadingMore, fetchGroupMembers]);

  return {
    members,
    setMembers,
    currentPage,
    totalPages,
    isLoadingMore,
    loading,
    fetchGroupMembers,
    handleLoadMore
  };
};