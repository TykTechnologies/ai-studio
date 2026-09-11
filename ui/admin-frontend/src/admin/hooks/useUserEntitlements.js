import { useState, useEffect, useCallback } from 'react';
import { getIdentity, refreshIdentity, subscribe } from '../utils/identityStore';

const shapeOf = (identity) =>
  identity
    ? {
        entitlements: identity.entitlements,
        ui_options: identity.uiOptions,
        userName: identity.name,
        userId: identity.id,
        userEmail: identity.email,
      }
    : null;

/**
 * Exposes the signed-in user's entitlements. Reads from the identity store
 * populated at boot (one /common/me call for the whole app); call
 * fetchUserEntitlements to force a refresh after something changed.
 */
const useUserEntitlements = (skipInitialFetch = false) => {
  const [identity, setIdentity] = useState(() => getIdentity());
  const [loading, setLoading] = useState(!skipInitialFetch && !getIdentity());
  const [error, setError] = useState(null);

  useEffect(() => subscribe((next) => setIdentity(next)), []);

  const fetchUserEntitlements = useCallback(async () => {
    const current = getIdentity();
    if (current) {
      setIdentity(current);
      setLoading(false);
      return shapeOf(current);
    }
    setLoading(true);
    setError(null);
    try {
      const next = await refreshIdentity();
      setIdentity(next);
      return shapeOf(next);
    } catch (err) {
      console.error('Failed to fetch user entitlements:', err);
      setError(err);
      throw err;
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!skipInitialFetch && !getIdentity()) {
      fetchUserEntitlements().catch(() => {});
    }
  }, [fetchUserEntitlements, skipInitialFetch]);

  const shape = shapeOf(identity);

  return {
    userEntitlements: shape?.entitlements ?? null,
    uiOptions: shape?.ui_options ?? null,
    userName: shape?.userName ?? null,
    userId: shape?.userId ?? null,
    userEmail: shape?.userEmail ?? null,
    loading,
    error,
    fetchUserEntitlements,
  };
};

export default useUserEntitlements;
