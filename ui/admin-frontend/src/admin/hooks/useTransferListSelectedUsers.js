import { useEffect, useState, useCallback } from 'react';
import { teamsService } from '../services/teamsService';

export function useTransferListSelectedUsers({ groupId, idField = 'id' } = {}) {
  const [members, setMembers] = useState([]);

  const addMember = useCallback(
    (item) => {
      setMembers((prev) => {
        if (prev.some((i) => i[idField] === item[idField])) {
          return prev;
        }
        return [item, ...prev];
      });
    },
    [idField]
  );

  const removeMember = useCallback(
    (item) => {
      setMembers((prev) => prev.filter((i) => i[idField] !== item[idField]));
    },
    [idField]
  );

  const reset = useCallback((newItems = []) => {
    setMembers(newItems);
  }, []);

  const [loading, setLoading] = useState(true);

  const fetchMembers = useCallback(async () => {
    if (!groupId) {
      setMembers([]);
      setLoading(false);
      return;
    }

    try {
      const resp = await teamsService.getTeamUsers(groupId, { all: true });
      setMembers(resp.data || []);
    } catch (err) {
      setMembers([]);
    } finally {
      setLoading(false);
    }
  }, [groupId]);

  useEffect(() => {
    fetchMembers();
  }, [fetchMembers]);

  return {
    members,
    setMembers,
    addMember,
    removeMember,
    loading,
    reset,
  };
} 