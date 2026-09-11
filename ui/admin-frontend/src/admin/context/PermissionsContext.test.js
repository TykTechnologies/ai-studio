import React from 'react';
import { renderHook, act } from '@testing-library/react';
import { PermissionsProvider, usePermissions } from './PermissionsContext';
import { clearIdentity, setIdentity } from '../utils/identityStore';
import { P } from '../rbac/permissions';

jest.mock('../utils/pubClient', () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));

const me = (attributes) => ({ id: '1', attributes });

const wrapperFor = (identity) => ({ children }) => (
  <PermissionsProvider identity={identity}>{children}</PermissionsProvider>
);

describe('usePermissions', () => {
  beforeEach(() => clearIdentity());

  describe('Community Edition semantics', () => {
    it.each([
      [true, true],
      [false, false],
    ])('is_admin=%s → can() everywhere is %s', (isAdmin, expected) => {
      const identity = me({ is_admin: isAdmin, permissions: isAdmin ? ['*'] : [] });
      const { result } = renderHook(() => usePermissions(), { wrapper: wrapperFor(identity) });
      expect(result.current.rbacEnabled).toBe(false);
      expect(result.current.isFullAdmin).toBe(isAdmin);
      expect(result.current.canAccessAdmin).toBe(expected);
      expect(result.current.can(P.LLMS_WRITE)).toBe(expected);
      expect(result.current.can(P.SSO_PROFILES_DELETE)).toBe(expected);
      expect(result.current.canAny([P.USERS_READ, P.AUDIT_READ])).toBe(expected);
      expect(result.current.canAll([P.USERS_READ, P.AUDIT_READ])).toBe(expected);
    });
  });

  describe('Enterprise semantics', () => {
    const viewer = me({
      is_admin: false,
      has_admin_access: true,
      rbac_enabled: true,
      permissions: ['llms:read', 'tools:write'],
      roles: [{ id: 1, slug: 'viewer', name: 'Viewer', is_system: true }],
    });

    it('evaluates the held set with implied read', () => {
      const { result } = renderHook(() => usePermissions(), { wrapper: wrapperFor(viewer) });
      expect(result.current.rbacEnabled).toBe(true);
      expect(result.current.isFullAdmin).toBe(false);
      expect(result.current.canAccessAdmin).toBe(true);
      expect(result.current.can(P.LLMS_READ)).toBe(true);
      expect(result.current.can(P.LLMS_WRITE)).toBe(false);
      expect(result.current.can(P.TOOLS_READ)).toBe(true);
      expect(result.current.can(P.TOOLS_WRITE)).toBe(true);
      expect(result.current.canAny([P.USERS_READ, P.TOOLS_READ])).toBe(true);
      expect(result.current.canAll([P.USERS_READ, P.TOOLS_READ])).toBe(false);
      expect(result.current.roles[0].slug).toBe('viewer');
    });

    it('follows identity store updates', () => {
      const { result } = renderHook(() => usePermissions(), { wrapper: wrapperFor(viewer) });
      expect(result.current.can(P.USERS_READ)).toBe(false);
      act(() => {
        setIdentity(me({ is_admin: false, has_admin_access: true, rbac_enabled: true, permissions: ['users:read'] }));
      });
      expect(result.current.can(P.USERS_READ)).toBe(true);
      expect(result.current.can(P.TOOLS_WRITE)).toBe(false);
    });
  });

  it('degrades to nothing-allowed outside a provider', () => {
    const { result } = renderHook(() => usePermissions());
    expect(result.current.canAccessAdmin).toBe(false);
    expect(result.current.can(P.LLMS_READ)).toBe(false);
  });
});
