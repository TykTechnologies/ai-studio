import { useState, useEffect, useCallback } from 'react';
import useUserEntitlements from './useUserEntitlements';
import apiClient from '../utils/apiClient';
import { skipQuickStartForUser } from '../services/userService';
import cacheService from '../utils/cacheService';
import { CACHE_KEYS } from '../utils/constants';
import { usePermissions } from '../context/PermissionsContext';
import { P } from '../rbac/permissions';

const useQuickStart = () => {
  const [showQuickStart, setShowQuickStart] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  // Dismissing Quick Start is stored on the user record, which needs
  // users:write globally. Someone without it (a role held only within a
  // Workspace, a read-only role) could never dismiss it for good, and the
  // attempt answered 403, so Quick Start is not offered to them.
  const { can } = usePermissions();
  const canDismiss = can(P.USERS_WRITE);

  const {
    userName,
    userId,
    userEmail,
    userEntitlements,
    fetchUserEntitlements,
    error: entitlementsError
  } = useUserEntitlements(true);

  const currentUser = userId ? {
    id: userId,
    name: userName,
    email: userEmail
  } : null;

  const fetchAppsCount = useCallback(async () => {
    try {
      const response = await apiClient.get('/apps/count');
      const count = response.data.count || 0;
      return count;
    } catch (error) {
      console.error('Error fetching apps count:', error);
      return 0;
    }
  }, []);

  const fetchQuickStartData = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    return Promise.all([
      fetchUserEntitlements(),
      fetchAppsCount(),
    ])
      .then(([userEntitlements, appsCount, daysLeft]) => {
        const skipQuickStart = userEntitlements?.ui_options?.skip_quick_start;
        if (appsCount === 0 && !skipQuickStart && canDismiss) {
          setShowQuickStart(true);
        }
      })
      .catch(error => {
        console.error('Error fetching quick start data:', error);
        setError('Failed to load data');
      })
      .finally(() => {
        setLoading(false);
      });
  }, [fetchUserEntitlements, fetchAppsCount, canDismiss]);

  useEffect(() => {
    fetchQuickStartData();
  }, [fetchQuickStartData]);

  const handleQuickStartComplete = () => {
    setShowQuickStart(false);
  };

  const handleQuickStartSkip = useCallback(async () => {
    if (userId && canDismiss && !userEntitlements?.ui_options?.skip_quick_start) {
      try {
        await skipQuickStartForUser(userId);
        cacheService.remove(CACHE_KEYS.USER_ENTITLEMENTS);
      } catch (error) {
        console.error('Error marking quick start as skipped:', error);
      }
    }
    setShowQuickStart(false);
  }, [userId, userEntitlements, canDismiss]);

  const combinedError = entitlementsError || error;

  return {
    showQuickStart,
    setShowQuickStart,
    currentUser,
    loading,
    error: combinedError,
    handleQuickStartComplete,
    handleQuickStartSkip,
    fetchQuickStartData,
    showLicenseBanner: false,
    licenseDaysLeft: null,
  };
};

export default useQuickStart;