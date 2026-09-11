/**
 * Module-level store for the signed-in user's identity and effective
 * permissions, fed by GET /common/me. It exists so code outside React
 * (the API client, context providers that mount before the router) can ask
 * "may this user do X" without reaching for a global on window.
 *
 * React code should use usePermissions() from PermissionsContext, which
 * subscribes to this store.
 */
import { hasPermission, toArray, FULL_ADMIN } from '../rbac/permissions';

let identity = null;
const listeners = new Set();

/** Builds the identity view from a raw /common/me payload ({ id, attributes }). */
export const buildIdentity = (me) => {
  if (!me) return null;
  const attributes = me.attributes || {};
  const permissionList = Array.isArray(attributes.permissions) ? attributes.permissions : [];
  const isFullAdmin = attributes.is_admin === true || permissionList.includes(FULL_ADMIN);
  const permissions = new Set(isFullAdmin ? [FULL_ADMIN, ...permissionList] : permissionList);
  const hasAdminAccess =
    typeof attributes.has_admin_access === 'boolean'
      ? attributes.has_admin_access
      : isFullAdmin || permissions.size > 0;
  return {
    id: me.id,
    email: attributes.email,
    name: attributes.name,
    isAdmin: attributes.is_admin === true,
    isFullAdmin,
    hasAdminAccess,
    rbacEnabled: attributes.rbac_enabled === true,
    permissions,
    roles: Array.isArray(attributes.roles) ? attributes.roles : [],
    uiOptions: attributes.ui_options || {},
    entitlements: attributes.entitlements || null,
    raw: me,
  };
};

const notify = () => {
  listeners.forEach((fn) => {
    try {
      fn(identity);
    } catch (e) {
      console.error('identity listener failed', e);
    }
  });
};

export const setIdentity = (me) => {
  identity = buildIdentity(me);
  notify();
  return identity;
};

export const clearIdentity = () => {
  identity = null;
  notify();
};

export const getIdentity = () => identity;

export const subscribe = (fn) => {
  listeners.add(fn);
  return () => listeners.delete(fn);
};

/** Re-fetches /common/me and updates the store. */
export const refreshIdentity = async () => {
  // Required lazily so importing the store never drags the HTTP client (and
  // axios) into modules that only want to read the current identity.
  const pubClient = require('./pubClient').default;
  const response = await pubClient.get('/common/me');
  return setIdentity(response.data);
};

export const hasPermissionNow = (perm) => hasPermission(identity?.permissions, perm);

export const hasAny = (perms) => toArray(perms).some((p) => hasPermissionNow(p));

export const hasAll = (perms) => toArray(perms).every((p) => hasPermissionNow(p));

export const canAccessAdminNow = () => identity?.hasAdminAccess === true;

/** @deprecated read-only shim for the old window.adminEntitlements global. */
export const legacyAdminEntitlements = () =>
  identity?.isFullAdmin
    ? { is_admin: true, ui_options: identity.uiOptions, entitlements: identity.entitlements }
    : undefined;
