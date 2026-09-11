import React, { createContext, useContext, useEffect, useMemo, useState, useCallback } from 'react';
import {
  getIdentity,
  subscribe,
  refreshIdentity,
  setIdentity as storeIdentity,
} from '../utils/identityStore';
import { hasPermission, toArray } from '../rbac/permissions';

const PermissionsContext = createContext(null);

const emptyAccess = {
  identity: null,
  permissions: new Set(),
  roles: [],
  isFullAdmin: false,
  isAdmin: false,
  hasAdminAccess: false,
  canAccessAdmin: false,
  rbacEnabled: false,
};

/**
 * PermissionsProvider exposes the signed-in user's effective permissions to
 * the React tree. It is fed once by App.js from the /common/me response and
 * stays in step with the identity store thereafter.
 *
 * Community Edition semantics fall out naturally: an admin holds "*", every
 * other user holds nothing, and rbacEnabled is false.
 */
export const PermissionsProvider = ({ identity: initialIdentity, children }) => {
  const [identity, setIdentityState] = useState(() => {
    if (initialIdentity) {
      return storeIdentity(initialIdentity);
    }
    return getIdentity();
  });

  useEffect(() => subscribe((next) => setIdentityState(next)), []);

  useEffect(() => {
    if (initialIdentity) {
      setIdentityState(storeIdentity(initialIdentity));
    }
  }, [initialIdentity]);

  const refresh = useCallback(() => refreshIdentity(), []);

  const value = useMemo(() => {
    const base = identity
      ? {
          identity,
          permissions: identity.permissions,
          roles: identity.roles,
          isFullAdmin: identity.isFullAdmin,
          isAdmin: identity.isAdmin,
          hasAdminAccess: identity.hasAdminAccess,
          canAccessAdmin: identity.hasAdminAccess,
          rbacEnabled: identity.rbacEnabled,
        }
      : emptyAccess;
    const can = (perm) => hasPermission(base.permissions, perm);
    return {
      ...base,
      loading: false,
      can,
      canAny: (perms) => toArray(perms).some(can),
      canAll: (perms) => toArray(perms).every(can),
      refresh,
    };
  }, [identity, refresh]);

  return <PermissionsContext.Provider value={value}>{children}</PermissionsContext.Provider>;
};

/**
 * usePermissions returns { permissions, roles, can, canAny, canAll,
 * isFullAdmin, hasAdminAccess, canAccessAdmin, rbacEnabled, identity,
 * refresh }. Outside a provider it degrades to "nothing allowed" so
 * components stay renderable in isolation (tests, storybook).
 */
export const usePermissions = () => {
  const ctx = useContext(PermissionsContext);
  if (ctx) return ctx;
  const identity = getIdentity();
  const permissions = identity?.permissions || new Set();
  const can = (perm) => hasPermission(permissions, perm);
  return {
    ...emptyAccess,
    ...(identity
      ? {
          identity,
          permissions,
          roles: identity.roles,
          isFullAdmin: identity.isFullAdmin,
          isAdmin: identity.isAdmin,
          hasAdminAccess: identity.hasAdminAccess,
          canAccessAdmin: identity.hasAdminAccess,
          rbacEnabled: identity.rbacEnabled,
        }
      : {}),
    loading: false,
    can,
    canAny: (perms) => toArray(perms).some(can),
    canAll: (perms) => toArray(perms).every(can),
    refresh: refreshIdentity,
  };
};

export default PermissionsContext;
