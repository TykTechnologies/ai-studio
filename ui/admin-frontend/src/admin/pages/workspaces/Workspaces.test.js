import React from 'react';
import { render, screen, within, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import adminTheme from '../../theme';
import Workspaces from './Workspaces';
import { listWorkspaces, getWorkspaceObjects } from '../../services/workspacesService';
import { usePermissions } from '../../context/PermissionsContext';
import { EnterpriseFeatureError } from '../../utils/apiErrors';

const mockNavigate = jest.fn();
jest.mock('react-router-dom', () => ({ ...jest.requireActual('react-router-dom'), useNavigate: () => mockNavigate }));
jest.mock('../../services/workspacesService', () => ({ listWorkspaces: jest.fn(), getWorkspaceObjects: jest.fn() }));
jest.mock('../../context/PermissionsContext', () => ({ usePermissions: jest.fn() }));
jest.mock('../../../components/common/Icon', () => ({
  __esModule: true,
  default: ({ name }) => <span data-testid={`icon-${name}`} />,
}));

// The Default owns these; the list shows their sum.
const DEFAULT_COUNTS = { llms: 2, apps: 1 };
const DEFAULT_TOTAL = String(Object.values(DEFAULT_COUNTS).reduce((a, b) => a + b, 0));

const renderPage = () =>
  render(
    <ThemeProvider theme={adminTheme}>
      <MemoryRouter>
        <Workspaces />
      </MemoryRouter>
    </ThemeProvider>
  );

const allow = (perms) => {
  const can = (p) => perms.includes(p);
  usePermissions.mockReturnValue({ can, canAny: (ps) => ps.some(can), canAll: (ps) => ps.every(can) });
};

describe('Workspaces page', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    allow(['workspaces:read', 'workspaces:write']);
    getWorkspaceObjects.mockImplementation(async (id) => ({ counts: id === '1' ? DEFAULT_COUNTS : { llms: 0 } }));
  });

  it('shows the upgrade prompt in Community Edition (402)', async () => {
    listWorkspaces.mockRejectedValue(new EnterpriseFeatureError('Enterprise feature'));
    renderPage();
    expect(await screen.findByText(/Group LLMs, Apps, tools/)).toBeInTheDocument();
  });

  it('lists the Default first with its badge and the object counts', async () => {
    listWorkspaces.mockResolvedValue([
      { id: '2', attributes: { name: 'Ops', description: 'Operations' } },
      { id: '1', attributes: { name: 'Default', is_default: true } },
    ]);
    renderPage();
    const first = await screen.findByTestId('workspace-row-1');
    const rows = within(screen.getByTestId('workspaces-table')).getAllByRole('row');
    expect(rows[1]).toBe(first);
    expect(within(first).getByTestId('default-badge')).toBeInTheDocument();
    expect(within(screen.getByTestId('workspace-row-2')).queryByTestId('default-badge')).toBeNull();
    expect(await within(first).findByText(DEFAULT_TOTAL)).toBeInTheDocument();
    expect(screen.getByText('Operations')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('workspace-row-2'));
    expect(mockNavigate).toHaveBeenCalledWith('/admin/workspaces/2');
  });

  it('offers create only with workspaces:write', async () => {
    listWorkspaces.mockResolvedValue([]);
    renderPage();
    expect(await screen.findByRole('button', { name: /Add Workspace/ })).toBeInTheDocument();

    allow(['workspaces:read']);
    listWorkspaces.mockResolvedValue([]);
    renderPage();
    expect(screen.getAllByRole('button', { name: /Add Workspace/ })).toHaveLength(1);
  });
});
