import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import RequirePermission from './RequirePermission';
import { PermissionsProvider } from '../../context/PermissionsContext';
import { clearIdentity } from '../../utils/identityStore';
import { P, hasPluginGrant } from '../../rbac/permissions';

jest.mock('../../utils/pubClient', () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock('../../../components/common/Icon', () => ({
  __esModule: true,
  default: () => <span data-testid="icon" />,
}));

const identity = (permissions) => ({
  id: '1',
  attributes: { is_admin: false, has_admin_access: permissions.length > 0, rbac_enabled: true, permissions },
});

// The descriptor the plugins route uses: the platform read or any plugin grant.
const pluginsRoute = { test: (access) => access.can(P.PLUGINS_READ) || hasPluginGrant(access.permissions), label: P.PLUGINS_READ };

const renderWith = (permissions) =>
  render(
    <MemoryRouter>
      <PermissionsProvider identity={identity(permissions)}>
        <RequirePermission permission={pluginsRoute}>
          <div>Plugin pages</div>
        </RequirePermission>
      </PermissionsProvider>
    </MemoryRouter>
  );

describe('RequirePermission with a runtime predicate', () => {
  beforeEach(() => clearIdentity());

  it('admits the platform permission', () => {
    renderWith(['plugins:read']);
    expect(screen.getByText('Plugin pages')).toBeInTheDocument();
  });

  it('admits a per-plugin grant and the umbrella grant', () => {
    renderWith(['plugin:com.example.assets:read']);
    expect(screen.getByText('Plugin pages')).toBeInTheDocument();
  });

  it('denies everyone else and names the primary permission', () => {
    renderWith(['llms:read']);
    expect(screen.queryByText('Plugin pages')).toBeNull();
    expect(screen.getByText(/Plugins: read/)).toBeInTheDocument();
  });
});
