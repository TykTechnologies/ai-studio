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
  isOwner: false,
  workspaces: [],
};

/**
 * makeCanIn builds canIn(workspaceId, perm): the permission is held globally
 * (which covers every Workspace) or through a role scoped to that Workspace.
 * Workspace ids are the ones API responses carry (a stored 0 is already
 * reported as the Default's id).
 */
export const makeCanIn = (permissions, workspaces) => {
  const byId = new Map((workspaces || []).map((w) => [w.id, w.permissions]));
  return (workspaceId, perm) => {
    if (hasPermission(permissions, perm)) return true;
    if (workspaceId === undefined || workspaceId === null || workspaceId === '') return false;
    const held = byId.get(Number(workspaceId));
    return Boolean(held) && hasPermission(held, perm);
  };
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
          isOwner: identity.isOwner === true,
          workspaces: identity.workspaces || [],
        }
      : emptyAccess;
    const can = (perm) => hasPermission(base.permissions, perm);
    return {
      ...base,
      loading: false,
      can,
      canIn: makeCanIn(base.permissions, base.workspaces),
      canAny: (perms) => toArray(perms).some(can),
      canAll: (perms) => toArray(perms).every(can),
      refresh,
    };
  }, [identity, refresh]);

  return <PermissionsContext.Provider value={value}>{children}</PermissionsContext.Provider>;
};

/**
 * usePermissions returns { permissions, roles, can, canIn, canAny, canAll,
 * isFullAdmin, hasAdminAccess, canAccessAdmin, rbacEnabled, workspaces,
 * identity, refresh }. Outside a provider it degrades to "nothing allowed" so
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
          isOwner: identity.isOwner === true,
          workspaces: identity.workspaces || [],
        }
      : {}),
    loading: false,
    can,
    canIn: makeCanIn(permissions, identity?.workspaces),
    canAny: (perms) => toArray(perms).some(can),
    canAll: (perms) => toArray(perms).every(can),
    refresh: refreshIdentity,
  };
};

export default PermissionsContext;
