import apiClient from '../utils/apiClient';
import { handleApiError } from './utils/errorHandler';
import { errorCode } from '../utils/apiErrors';

/**
 * Client for the Workspaces API (/api/v1/workspaces, Enterprise). Every
 * route answers 402 (enterprise_required) in Community Edition; callers
 * render the upgrade prompt on a 402.
 */

/**
 * Error codes of the Workspaces API the pages explain specifically
 * (authz.CodeDefaultWorkspaceLocked, authz.CodeWorkspaceInUse). A duplicate
 * name is a 409 without a code.
 */
export const WORKSPACE_ERRORS = Object.freeze({
  DEFAULT_LOCKED: 'default_workspace_locked',
  IN_USE: 'workspace_in_use',
});

// Keeps the HTTP status and the error code on the thrown Error, which
// handleApiError drops, so pages can tell the 409s apart.
const call = async (fn) => {
  try {
    return await fn();
  } catch (error) {
    const out = handleApiError(error);
    if (out !== error) {
      out.status = error?.response?.status;
      out.code = errorCode(error);
    }
    throw out;
  }
};

/** The Workspaces the caller can work in (every one for a global reader). */
export const listWorkspaces = () => call(async () => (await apiClient.get('/workspaces')).data.data || []);

export const getWorkspace = (id) => call(async () => (await apiClient.get(`/workspaces/${id}`)).data.data);

export const createWorkspace = ({ name, description }) =>
  call(async () => (await apiClient.post('/workspaces', { name, description })).data.data);

/** Only the given fields change (PATCH); is_default cannot be set. */
export const updateWorkspace = (id, fields) =>
  call(async () => (await apiClient.patch(`/workspaces/${id}`, fields)).data.data);

export const deleteWorkspace = (id) => call(async () => apiClient.delete(`/workspaces/${id}`));

/** The objects stamped with the Workspace, by type. */
export const getWorkspaceObjects = (id) =>
  call(async () => (await apiClient.get(`/workspaces/${id}/objects`)).data.data);

const workspacesService = {
  listWorkspaces,
  getWorkspace,
  createWorkspace,
  updateWorkspace,
  deleteWorkspace,
  getWorkspaceObjects,
};

export default workspacesService;
