import React from 'react';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import Can from './Can';
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

const identity = (permissions) => ({
  id: '1',
  attributes: { is_admin: false, has_admin_access: permissions.length > 0, rbac_enabled: true, permissions },
});

const renderWith = (permissions, ui) =>
  render(
    <MemoryRouter>
      <PermissionsProvider identity={identity(permissions)}>{ui}</PermissionsProvider>
    </MemoryRouter>
  );

describe('Can', () => {
  beforeEach(() => clearIdentity());

  it('renders children only when the permission is held', () => {
    renderWith(['llms:write'], (
      <>
        <Can permission={P.LLMS_WRITE}><button>Add LLM</button></Can>
        <Can permission={P.USERS_WRITE}><button>Add user</button></Can>
      </>
    ));
    expect(screen.getByText('Add LLM')).toBeInTheDocument();
    expect(screen.queryByText('Add user')).toBeNull();
  });

  it('supports anyOf, allOf, fallback and the render-prop form', () => {
    renderWith(['llms:write'], (
      <>
        <Can anyOf={[P.USERS_WRITE, P.LLMS_READ]}><span>any</span></Can>
        <Can allOf={[P.USERS_WRITE, P.LLMS_READ]} fallback={<span>locked</span>}><span>all</span></Can>
        <Can permission={P.LLMS_DELETE}>{(allowed) => <span>{allowed ? 'can delete' : 'cannot delete'}</span>}</Can>
      </>
    ));
    expect(screen.getByText('any')).toBeInTheDocument();
    expect(screen.queryByText('all')).toBeNull();
    expect(screen.getByText('locked')).toBeInTheDocument();
    expect(screen.getByText('cannot delete')).toBeInTheDocument();
  });
});

describe('RequirePermission', () => {
  beforeEach(() => clearIdentity());

  it('renders the page when allowed', () => {
    renderWith(['llms:read'], (
      <RequirePermission permission={P.LLMS_READ}><div>LLM list</div></RequirePermission>
    ));
    expect(screen.getByText('LLM list')).toBeInTheDocument();
  });

  it('renders a denial panel naming the permission when not allowed', () => {
    renderWith(['llms:read'], (
      <RequirePermission permission={P.LLMS_WRITE}><div>LLM form</div></RequirePermission>
    ));
    expect(screen.queryByText('LLM form')).toBeNull();
    expect(screen.getByTestId('permission-denied-panel')).toBeInTheDocument();
    expect(screen.getByText(/Llms: write/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /back to overview/i })).toBeInTheDocument();
  });

  it('accepts anyOf', () => {
    renderWith(['data-catalogues:read'], (
      <RequirePermission anyOf={[P.CATALOGUES_READ, P.DATA_CATALOGUES_READ]}><div>Catalogs</div></RequirePermission>
    ));
    expect(screen.getByText('Catalogs')).toBeInTheDocument();
  });
});
