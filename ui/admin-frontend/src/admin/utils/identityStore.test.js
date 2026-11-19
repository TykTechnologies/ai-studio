import {
  buildIdentity,
  setIdentity,
  getIdentity,
  clearIdentity,
  subscribe,
  hasPermissionNow,
  hasAny,
  hasAll,
  canAccessAdminNow,
  refreshIdentity,
} from './identityStore';
import pubClient from './pubClient';

jest.mock('./pubClient', () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));

const me = (attributes, id = '7') => ({ id, attributes });

describe('identityStore', () => {
  beforeEach(() => {
    clearIdentity();
    jest.clearAllMocks();
  });

  it('derives full admin from is_admin (Community Edition payload)', () => {
    const identity = buildIdentity(me({ is_admin: true, permissions: ['*'], ui_options: { show_chat: true } }));
    expect(identity.isFullAdmin).toBe(true);
    expect(identity.hasAdminAccess).toBe(true);
    expect(identity.rbacEnabled).toBe(false);
    expect(identity.permissions.has('*')).toBe(true);
    expect(identity.uiOptions).toEqual({ show_chat: true });
  });

  it('treats a user with no permissions as off the admin surface', () => {
    const identity = buildIdentity(me({ is_admin: false, permissions: [] }));
    expect(identity.isFullAdmin).toBe(false);
    expect(identity.hasAdminAccess).toBe(false);
  });

  it('honours has_admin_access and roles from an Enterprise payload', () => {
    const identity = buildIdentity(
      me({
        is_admin: false,
        has_admin_access: true,
        rbac_enabled: true,
        permissions: ['llms:read', 'tools:write'],
        roles: [{ id: 3, slug: 'viewer', name: 'Viewer', is_system: true }],
      })
    );
    expect(identity.isFullAdmin).toBe(false);
    expect(identity.hasAdminAccess).toBe(true);
    expect(identity.rbacEnabled).toBe(true);
    expect(identity.roles).toHaveLength(1);
  });

  it('puts a user whose only roles are Workspace-scoped on the admin surface (.57)', () => {
    const identity = buildIdentity(
      me({
        is_admin: false,
        has_admin_access: false,
        admin_nav_hint: true,
        rbac_enabled: true,
        permissions: [],
        workspaces: [{ id: 4, name: 'W1', permissions: ['llms:write'] }],
      })
    );
    expect(identity.hasAdminAccess).toBe(true);
    expect(identity.hasGlobalAdminAccess).toBe(false);
    expect(identity.isFullAdmin).toBe(false);
    setIdentity(identity.raw);
    expect(canAccessAdminNow()).toBe(true);
    // No global permission is implied by the hint.
    expect(hasPermissionNow('llms:write')).toBe(false);
  });

  it('answers permission checks with implied read and wildcard semantics', () => {
    setIdentity(me({ is_admin: false, permissions: ['tools:write'] }));
    expect(hasPermissionNow('tools:write')).toBe(true);
    expect(hasPermissionNow('tools:read')).toBe(true);
    expect(hasPermissionNow('tools:delete')).toBe(false);
    expect(hasPermissionNow('llms:read')).toBe(false);
    expect(hasAny(['llms:read', 'tools:read'])).toBe(true);
    expect(hasAll(['llms:read', 'tools:read'])).toBe(false);
    expect(canAccessAdminNow()).toBe(true);

    setIdentity(me({ is_admin: true, permissions: ['*'] }));
    expect(hasPermissionNow('sso-profiles:delete')).toBe(true);

    clearIdentity();
    expect(hasPermissionNow('tools:read')).toBe(false);
    expect(canAccessAdminNow()).toBe(false);
  });

  it('notifies subscribers and refreshes from /common/me', async () => {
    const listener = jest.fn();
    const unsubscribe = subscribe(listener);
    setIdentity(me({ is_admin: false, permissions: [] }));
    expect(listener).toHaveBeenCalledTimes(1);

    pubClient.get.mockResolvedValue({ data: me({ is_admin: false, permissions: ['llms:read'] }) });
    const refreshed = await refreshIdentity();
    expect(pubClient.get).toHaveBeenCalledWith('/common/me');
    expect(refreshed.permissions.has('llms:read')).toBe(true);
    expect(getIdentity()).toBe(refreshed);
    expect(listener).toHaveBeenCalledTimes(2);

    unsubscribe();
    clearIdentity();
    expect(listener).toHaveBeenCalledTimes(2);
  });

  it('maps Workspaces and the role label', () => {
    const id = buildIdentity(
      me({
        permissions: [],
        account_type: 'Service account',
        workspaces: [{ id: '4', name: 'Ops', permissions: ['llms:write', 'apps:read'] }],
      })
    );
    expect(id.roleLabel).toBe('Service account');
    expect(id.workspaces).toHaveLength(1);
    expect(id.workspaces[0].id).toBe(4);
    expect(id.workspaces[0].name).toBe('Ops');
    expect(id.workspaces[0].permissions.has('llms:write')).toBe(true);

    const none = buildIdentity(me({ permissions: [] }));
    expect(none.workspaces).toEqual([]);
    expect(none.roleLabel).toBe('');
  });
});
