import apiClient from '../utils/apiClient';
import { handleApiError } from './utils/errorHandler';

/**
 * Client for the role-based access control API (/api/v1/rbac/*).
 * Roles and bindings use the JSON:API-style envelope used across the API:
 * { data: { type, id, attributes } }.
 */

export const getPermissionCatalogue = async () => {
  try {
    const response = await apiClient.get('/rbac/permissions');
    return response.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const getMyAccess = async () => {
  try {
    const response = await apiClient.get('/rbac/me');
    return response.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const listRoles = async () => {
  try {
    const response = await apiClient.get('/rbac/roles');
    return response.data.data || [];
  } catch (error) {
    throw handleApiError(error);
  }
};

export const getRole = async (id) => {
  try {
    const response = await apiClient.get(`/rbac/roles/${id}`);
    return response.data.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const createRole = async ({ name, description, permissions }) => {
  try {
    const response = await apiClient.post('/rbac/roles', { name, description, permissions });
    return response.data.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const updateRole = async (id, { name, description, permissions }) => {
  try {
    const response = await apiClient.patch(`/rbac/roles/${id}`, { name, description, permissions });
    return response.data.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const deleteRole = async (id) => {
  try {
    await apiClient.delete(`/rbac/roles/${id}`);
  } catch (error) {
    throw handleApiError(error);
  }
};

export const cloneRole = async (id, name) => {
  try {
    const response = await apiClient.post(`/rbac/roles/${id}/clone`, { name });
    return response.data.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const listBindings = async ({ subjectType, subjectId, roleId, scopeType, scopeId } = {}) => {
  try {
    const params = {};
    if (subjectType) params.subject_type = subjectType;
    if (subjectId) params.subject_id = subjectId;
    if (roleId) params.role_id = roleId;
    // scopeType "global" (unscoped only) or "workspace" with scopeId.
    if (scopeType) params.scope_type = scopeType;
    if (scopeId) params.scope_id = scopeId;
    const response = await apiClient.get('/rbac/bindings', { params });
    return response.data.data || [];
  } catch (error) {
    throw handleApiError(error);
  }
};

export const createBinding = async ({ subjectType, subjectId, roleId }) => {
  try {
    const response = await apiClient.post('/rbac/bindings', {
      subject_type: subjectType,
      subject_id: Number(subjectId),
      role_id: Number(roleId),
    });
    return response.data.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const deleteBinding = async (bindingId) => {
  try {
    await apiClient.delete(`/rbac/bindings/${bindingId}`);
  } catch (error) {
    throw handleApiError(error);
  }
};

export const getEffectivePermissions = async (userId) => {
  try {
    const response = await apiClient.get(`/rbac/users/${userId}/effective`);
    return response.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

/**
 * Makes a subject's direct role bindings equal to desiredRoleIds by creating
 * and deleting only the difference. Returns what changed and what failed.
 */
export const syncSubjectRoles = async (subjectType, subjectId, desiredRoleIds) => {
  const desired = new Set((desiredRoleIds || []).map(Number));
  // The server lists only global bindings; the check below keeps the sync
  // to them even if it did not.
  const current = await listBindings({ subjectType, subjectId, scopeType: 'global' });
  const have = new Map(); // roleId -> bindingId
  current.forEach((b) => {
    if (!b.attributes.scope_type) {
      have.set(Number(b.attributes.role_id), b.id);
    }
  });

  const toAdd = [...desired].filter((roleId) => !have.has(roleId));
  const toRemove = [...have.entries()].filter(([roleId]) => !desired.has(roleId));

  const results = await Promise.allSettled([
    ...toRemove.map(([, bindingId]) => deleteBinding(bindingId)),
    ...toAdd.map((roleId) => createBinding({ subjectType, subjectId, roleId })),
  ]);

  const failed = results
    .map((r, i) => ({ r, i }))
    .filter(({ r }) => r.status === 'rejected')
    .map(({ r, i }) => ({
      action: i < toRemove.length ? 'remove' : 'add',
      roleId: i < toRemove.length ? toRemove[i][0] : toAdd[i - toRemove.length],
      error: r.reason,
    }));

  return { added: toAdd, removed: toRemove.map(([roleId]) => roleId), failed };
};

/**
 * Makes a subject's role bindings scoped to one Workspace equal to
 * desiredRoleIds, creating and deleting only the difference. It touches
 * nothing else: global bindings stay with syncSubjectRoles, which in turn
 * never touches scoped ones. Managing scoped bindings needs roles:write
 * globally (D10). Returns what changed and what failed.
 */
export const syncScopedBindings = async (workspaceId, subjectType, subjectId, desiredRoleIds) => {
  const id = Number(workspaceId);
  if (!Number.isInteger(id) || id <= 0) {
    throw new Error(`syncScopedBindings: invalid Workspace id ${workspaceId}`);
  }
  const scopeId = String(id);
  const desired = new Set((desiredRoleIds || []).map(Number));
  // The server lists only this Workspace's bindings; the check below
  // keeps the sync to them even if it did not.
  const current = await listBindings({ subjectType, subjectId, scopeType: 'workspace', scopeId });
  const have = new Map(); // roleId -> bindingId, in this Workspace only
  current.forEach((b) => {
    if (b.attributes.scope_type === 'workspace' && String(b.attributes.scope_id) === scopeId) {
      have.set(Number(b.attributes.role_id), b.id);
    }
  });

  const toAdd = [...desired].filter((roleId) => !have.has(roleId));
  const toRemove = [...have.entries()].filter(([roleId]) => !desired.has(roleId));

  const results = await Promise.allSettled([
    ...toRemove.map(([, bindingId]) => deleteBinding(bindingId)),
    ...toAdd.map((roleId) =>
      createScopedBinding({ subjectType, subjectId, roleId, workspaceId: scopeId })
    ),
  ]);

  const failed = results
    .map((r, i) => ({ r, i }))
    .filter(({ r }) => r.status === 'rejected')
    .map(({ r, i }) => ({
      action: i < toRemove.length ? 'remove' : 'add',
      roleId: i < toRemove.length ? toRemove[i][0] : toAdd[i - toRemove.length],
      error: r.reason,
    }));

  return { added: toAdd, removed: toRemove.map(([roleId]) => roleId), failed };
};

/** Creates a role binding scoped to one Workspace. */
export const createScopedBinding = async ({ subjectType, subjectId, roleId, workspaceId }) => {
  try {
    const response = await apiClient.post('/rbac/bindings', {
      subject_type: subjectType,
      subject_id: Number(subjectId),
      role_id: Number(roleId),
      scope_type: 'workspace',
      scope_id: String(Number(workspaceId)),
    });
    return response.data.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

/** Splits a role list into system roles first, then custom roles, each by name. */
export const sortRoles = (roles) =>
  [...roles].sort((a, b) => {
    if (a.attributes.is_system !== b.attributes.is_system) {
      return a.attributes.is_system ? -1 : 1;
    }
    return a.attributes.name.localeCompare(b.attributes.name);
  });

const rbacService = {
  getPermissionCatalogue,
  getMyAccess,
  listRoles,
  getRole,
  createRole,
  updateRole,
  deleteRole,
  cloneRole,
  listBindings,
  createScopedBinding,
  syncScopedBindings,
  createBinding,
  deleteBinding,
  getEffectivePermissions,
  syncSubjectRoles,
  sortRoles,
};

export default rbacService;
