import { useState, useEffect, useCallback, useRef } from "react";
import { useParams } from "react-router-dom";
import { teamsService } from "../../../services/teamsService";

const useTeamMembers = () => {
  const { id } = useParams();
  const [members, setMembers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [currentPage, setCurrentPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const containerRef = useRef(null);

  const fetchMembers = useCallback(async (page = 1, append = false) => {
    if (!id) return;
    
    try {
      if (page === 1) {
        setLoading(true);
      } else {
        setIsLoadingMore(true);
      }

      const params = {
        page,
        page_size: 10,
      };

      const response = await teamsService.getTeamUsers(id, params);
      const newMembers = response.data || [];
      
      if (append) {
        setMembers(prev => [...prev, ...newMembers]);
      } else {
        setMembers(newMembers);
      }

      const pages = response.totalPages || 0;
      setTotalPages(pages);
      setCurrentPage(page);
      setError(null);
    } catch (err) {
      console.error("Error fetching team members", err);
      setError("Failed to load team members");
    } finally {
      if (page === 1) {
        setLoading(false);
      } else {
        setIsLoadingMore(false);
      }
    }
  }, [id]);

  useEffect(() => {
    fetchMembers(1, false);
  }, [fetchMembers]);


  const handleLoadMore = useCallback(() => {
    if (currentPage < totalPages && !isLoadingMore) {
      fetchMembers(currentPage + 1, true);
    }
  }, [currentPage, totalPages, isLoadingMore, fetchMembers]);

  useEffect(() => {
    const container = containerRef?.current;
    if (!container) return;

    const handleScroll = () => {
      if (!container || isLoadingMore) return;

      const { scrollTop, scrollHeight, clientHeight } = container;
      if (scrollHeight - scrollTop - clientHeight < 50) {
        handleLoadMore();
      }
    };

    container.addEventListener('scroll', handleScroll);
    return () => {
        if (container) {
            container.removeEventListener('scroll', handleScroll);
        }
    };
  }, [handleLoadMore, isLoadingMore]);

  return {
    users: members,
    loading,
    error,
    isLoadingMore,
    containerRef,
    handleLoadMore,
    hasMore: currentPage < totalPages,
  };
};

export default useTeamMembers;
