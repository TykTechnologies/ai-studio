import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import adminTheme from '../../theme';
import Roles from './Roles';
import { listRoles, deleteRole } from '../../services/rbacService';
import { usePermissions } from '../../context/PermissionsContext';
import { EnterpriseFeatureError } from '../../utils/apiErrors';

jest.mock('../../services/rbacService', () => ({
  ...jest.requireActual('../../services/rbacService'),
  listRoles: jest.fn(),
  deleteRole: jest.fn(),
  cloneRole: jest.fn(),
}));
jest.mock('../../context/PermissionsContext', () => ({
  usePermissions: jest.fn(),
}));
jest.mock('../../../components/common/Icon', () => ({
  __esModule: true,
  default: ({ name }) => <span data-testid={`icon-${name}`} />,
}));

const role = (id, name, slug, isSystem, permissions = ['llms:read']) => ({
  id: String(id),
  type: 'role',
  attributes: { name, slug, is_system: isSystem, description: `${name} desc`, permissions, users_count: 2, groups_count: 1 },
});

const renderPage = () =>
  render(
    <ThemeProvider theme={adminTheme}>
      <MemoryRouter>
        <Roles />
      </MemoryRouter>
    </ThemeProvider>
  );

describe('Roles page', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    usePermissions.mockReturnValue({ can: () => true, canAny: () => true, canAll: () => true });
    listRoles.mockResolvedValue([
      role(2, 'Editor', 'editor', true),
      role(1, 'Administrator', 'administrator', true, ['*']),
      role(9, 'LLM Ops', 'llm-ops', false, ['llms:read', 'llms:write']),
    ]);
    deleteRole.mockResolvedValue();
  });

  it('lists system roles first with type badges and counts', async () => {
    renderPage();
    const rows = await screen.findAllByTestId(/role-row-/);
    expect(rows.map((r) => r.getAttribute('data-testid'))).toEqual([
      'role-row-administrator',
      'role-row-editor',
      'role-row-llm-ops',
    ]);
    expect(screen.getAllByText('System')).toHaveLength(2);
    expect(screen.getByText('Custom')).toBeInTheDocument();
    expect(screen.getByText('All')).toBeInTheDocument();
    expect(screen.getAllByText('2 users, 1 team')).toHaveLength(3);
  });

  it('does not let system roles be edited or deleted', async () => {
    renderPage();
    await screen.findByTestId('role-row-editor');
    fireEvent.click(screen.getByLabelText('Actions for Editor'));
    expect(screen.getByText('Clone role')).toBeInTheDocument();
    expect(screen.getByText('Edit role')).toHaveAttribute('aria-disabled', 'true');
    expect(screen.getByText('Delete role')).toHaveAttribute('aria-disabled', 'true');
  });

  it('deletes a custom role after confirmation', async () => {
    renderPage();
    await screen.findByTestId('role-row-llm-ops');
    fireEvent.click(screen.getByLabelText('Actions for LLM Ops'));
    fireEvent.click(screen.getByText('Delete role'));
    fireEvent.click(await screen.findByRole('button', { name: /delete role/i }));
    await waitFor(() => expect(deleteRole).toHaveBeenCalledWith('9'));
  });

  it('hides the add button and row actions without roles:write', async () => {
    usePermissions.mockReturnValue({ can: (p) => p === 'roles:read', canAny: () => false, canAll: () => false });
    renderPage();
    await screen.findByTestId('role-row-editor');
    expect(screen.queryByText('Add role')).toBeNull();
    expect(screen.queryByLabelText('Actions for Editor')).toBeNull();
  });

  it('shows the Enterprise prompt in Community Edition', async () => {
    listRoles.mockRejectedValue(new EnterpriseFeatureError('upgrade'));
    renderPage();
    expect(await screen.findByText('Role-based access control')).toBeInTheDocument();
    expect(screen.getByText(/Learn More About Enterprise/)).toBeInTheDocument();
  });
});
