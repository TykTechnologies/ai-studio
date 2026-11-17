import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import RequirePermission from './RequirePermission';
import { PermissionsProvider } from '../../context/PermissionsContext';
import { clearIdentity } from '../../utils/identityStore';
import { P } from '../../rbac/permissions';

jest.mock('../../utils/pubClient', () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock('../../../components/common/Icon', () => ({
  __esModule: true,
  default: () => <span data-testid="icon" />,
}));

// Workspaces E3: a route is a capability gate, as on the server. A
// permission on a Workspace resource passes when held in any Workspace;
// a global permission still needs a global role.
const scoped = {
  id: '1',
  attributes: {
    is_admin: false,
    has_admin_access: true,
    rbac_enabled: true,
    permissions: [],
    workspaces: [{ id: 2, name: 'W1', permissions: ['llms:read', 'llms:write'] }],
  },
};

const renderGuard = (props) =>
  render(
    <MemoryRouter>
      <PermissionsProvider identity={scoped}>
        <RequirePermission {...props}>
          <div>Page</div>
        </RequirePermission>
      </PermissionsProvider>
    </MemoryRouter>
  );

describe('RequirePermission and Workspace roles', () => {
  beforeEach(() => clearIdentity());

  it('lets a Workspace-scoped writer open the create and edit pages', () => {
    renderGuard({ permission: P.LLMS_WRITE });
    expect(screen.getByText('Page')).toBeInTheDocument();
  });

  it('anyOf counts Workspace permissions too', () => {
    renderGuard({ anyOf: [P.TOOLS_WRITE, P.LLMS_READ] });
    expect(screen.getByText('Page')).toBeInTheDocument();
  });

  it('a permission held in no Workspace is still refused', () => {
    renderGuard({ permission: P.TOOLS_WRITE });
    expect(screen.queryByText('Page')).not.toBeInTheDocument();
  });

  it('a global permission is not granted by Workspace roles', () => {
    renderGuard({ permission: P.USERS_READ });
    expect(screen.queryByText('Page')).not.toBeInTheDocument();
  });
});
