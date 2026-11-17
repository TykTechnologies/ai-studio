import React from 'react';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import adminTheme from '../../theme';
import WorkspaceDetail, { deleteBlockedReason, deleteErrorMessage } from './WorkspaceDetail';
import { deleteWorkspace, getWorkspace, getWorkspaceObjects } from '../../services/workspacesService';
import { listBindings, listRoles, syncScopedBindings } from '../../services/rbacService';
import { listAll } from '../../utils/listAll';
import { usePermissions } from '../../context/PermissionsContext';

const mockNavigate = jest.fn();
jest.mock('react-router-dom', () => ({ ...jest.requireActual('react-router-dom'), useNavigate: () => mockNavigate }));
jest.mock('../../services/workspacesService', () => ({
  ...jest.requireActual('../../services/workspacesService'),
  deleteWorkspace: jest.fn(),
  getWorkspace: jest.fn(),
  getWorkspaceObjects: jest.fn(),
}));
jest.mock('../../services/rbacService', () => ({
  ...jest.requireActual('../../services/rbacService'),
  listBindings: jest.fn(),
  listRoles: jest.fn(),
  syncScopedBindings: jest.fn(),
}));
jest.mock('../../utils/listAll', () => ({ listAll: jest.fn() }));
jest.mock('../../context/PermissionsContext', () => ({ usePermissions: jest.fn() }));
jest.mock('../../../components/common/Icon', () => ({
  __esModule: true,
  default: ({ name }) => <span data-testid={`icon-${name}`} />,
}));

const role = (id, name, slug) => ({ id: String(id), attributes: { name, slug, is_system: slug !== 'custom', description: '' } });
const binding = (id, type, subjectId, roleId, roleName) => ({
  id: String(id),
  attributes: { subject_type: type, subject_id: subjectId, role_id: roleId, role: { name: roleName }, scope_type: 'workspace', scope_id: '5' },
});

const allow = (perms) => {
  const can = (p) => perms.includes('*') || perms.includes(p);
  usePermissions.mockReturnValue({ can, canAny: (ps) => ps.some(can), canAll: (ps) => ps.every(can) });
};

const renderAt = (id = '5') =>
  render(
    <ThemeProvider theme={adminTheme}>
      <MemoryRouter initialEntries={[`/admin/workspaces/${id}`]}>
        <Routes>
          <Route path="/admin/workspaces/:id" element={<WorkspaceDetail />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

describe('WorkspaceDetail', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    allow(['*']);
    getWorkspace.mockResolvedValue({ id: '5', attributes: { name: 'Ops', is_default: false } });
    getWorkspaceObjects.mockResolvedValue({ counts: { llms: 3, filestores: 1 } });
    listRoles.mockResolvedValue([role(1, 'Owner', 'owner'), role(3, 'Editor', 'editor'), role(4, 'Viewer', 'viewer')]);
    listAll.mockImplementation(async (_c, path) =>
      path === '/users'
        ? { data: { data: [{ id: '9', attributes: { name: 'Ann' } }, { id: '10', attributes: { name: 'Bot', account_type: 'service' } }] } }
        : { data: { data: [{ id: '20', attributes: { name: 'Platform' } }] } }
    );
    listBindings.mockResolvedValue([binding(100, 'user', 9, 3, 'Editor')]);
    syncScopedBindings.mockResolvedValue({ added: [], removed: [], failed: [] });
  });

  it('shows who holds roles here and changes them through syncScopedBindings', async () => {
    renderAt();
    const row = await screen.findByTestId('access-row-user-9');
    expect(within(row).getByText('Ann')).toBeInTheDocument();
    expect(within(row).getByText('Editor')).toBeInTheDocument();
    expect(listBindings).toHaveBeenCalledWith({ scopeType: 'workspace', scopeId: '5' });

    fireEvent.click(within(row).getByRole('button', { name: 'Change roles' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.mouseDown(within(dialog).getByLabelText('Roles in this Workspace'));
    fireEvent.click(await screen.findByTestId('role-option-4'));
    fireEvent.keyDown(screen.getByRole('listbox'), { key: 'Escape' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(syncScopedBindings).toHaveBeenCalledWith('5', 'user', '9', [3, 4]));
  });

  it('adds a service account, the Owner role not offered', async () => {
    renderAt();
    await screen.findByTestId('access-row-user-9');
    fireEvent.click(screen.getByRole('button', { name: 'Add user or team' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.mouseDown(within(dialog).getByLabelText('User'));
    const options = within(screen.getByRole('listbox')).getAllByRole('option').map((o) => o.textContent);
    expect(options).toEqual(['Bot (service account)']); // Ann already holds a role here
    fireEvent.click(screen.getByText('Bot (service account)'));
    fireEvent.mouseDown(within(dialog).getByLabelText('Roles in this Workspace'));
    expect(screen.queryByTestId('role-option-1')).toBeNull();
    fireEvent.click(screen.getByTestId('role-option-3'));
    fireEvent.keyDown(screen.getByRole('listbox'), { key: 'Escape' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(syncScopedBindings).toHaveBeenCalledWith('5', 'user', '10', [3]));
  });

  it('removes someone from the Workspace', async () => {
    renderAt();
    const row = await screen.findByTestId('access-row-user-9');
    fireEvent.click(within(row).getByRole('button', { name: 'Remove from this Workspace' }));
    await waitFor(() => expect(syncScopedBindings).toHaveBeenCalledWith('5', 'user', '9', []));
  });

  it('hides Access controls without roles:write', async () => {
    allow(['workspaces:read']);
    renderAt();
    const row = await screen.findByTestId('access-row-user-9');
    expect(within(row).queryByRole('button', { name: 'Change roles' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Add user or team' })).toBeNull();
    expect(screen.queryByTestId('delete-workspace')).toBeNull();
  });

  it('links each object type to its list filtered to the Workspace', async () => {
    renderAt();
    fireEvent.click(await screen.findByRole('tab', { name: 'Objects' }));
    expect(await screen.findByTestId('objects-link-llms')).toHaveAttribute('href', '/admin/llms?workspace_id=5');
    expect(screen.getByText('Files')).toBeInTheDocument();
    expect(screen.queryByTestId('objects-link-filestores')).toBeNull();
  });

  describe('delete', () => {
    it('is blocked while roles are assigned here', async () => {
      renderAt();
      await screen.findByTestId('access-row-user-9');
      await waitFor(() => expect(screen.getByTestId('delete-workspace')).toBeDisabled());
    });

    it('is disabled for the Default', async () => {
      getWorkspace.mockResolvedValue({ id: '5', attributes: { name: 'Default', is_default: true } });
      listBindings.mockResolvedValue([]);
      renderAt();
      await screen.findByText('No one holds a role in this Workspace yet.');
      await waitFor(() => expect(screen.getByTestId('delete-workspace')).toBeDisabled());
    });

    it('is allowed otherwise, says objects move to the Default, and deletes', async () => {
      listBindings.mockResolvedValue([]);
      deleteWorkspace.mockResolvedValue();
      renderAt();
      await screen.findByText('No one holds a role in this Workspace yet.');
      await waitFor(() => expect(screen.getByTestId('delete-workspace')).toBeEnabled());
      fireEvent.click(screen.getByTestId('delete-workspace'));
      expect(await screen.findByText(/moves every object it owns to the Default workspace/)).toBeInTheDocument();
      fireEvent.click(screen.getByRole('button', { name: 'Delete Workspace' }));
      await waitFor(() => expect(deleteWorkspace).toHaveBeenCalledWith('5'));
      expect(mockNavigate).toHaveBeenCalledWith('/admin/workspaces');
    });

    it('explains the refusal codes', () => {
      expect(deleteErrorMessage({ code: 'default_workspace_locked' })).toMatch(/Default workspace cannot be deleted/);
      expect(deleteErrorMessage({ code: 'workspace_in_use' })).toMatch(/Roles are still assigned/);
      expect(deleteBlockedReason({ attributes: { is_default: true } }, 0)).toMatch(/Default/);
      expect(deleteBlockedReason({ attributes: {} }, 2)).toMatch(/Access tab/);
      expect(deleteBlockedReason({ attributes: {} }, 0)).toBe('');
    });
  });
});
