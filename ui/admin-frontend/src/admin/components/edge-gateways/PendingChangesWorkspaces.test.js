import React from 'react';
import { render, screen, within } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import { NamespaceChanges } from './PendingChangesPreview';
import { summarizeWorkspaces, workspaceLabel } from './pendingChanges';
import { usePermissions } from '../../context/PermissionsContext';
import edgeGatewayService from '../../services/edgeGatewayService';
import apiClient from '../../utils/apiClient';

jest.mock('../../context/PermissionsContext', () => ({ usePermissions: jest.fn() }));
jest.mock('../../utils/apiClient');

const withWorkspaces = {
  namespace: 'default',
  total: 3,
  baseline: 'push',
  lastPushAt: null,
  changes: [
    { type: 'llm', id: 4, name: 'GPT', change: 'updated', workspaceId: 1, workspaceName: 'Default' },
    { type: 'app', change: 'created', workspaceId: 7, workspaceName: '', hidden: true },
    { type: 'plugin', id: 2, name: 'Guard', change: 'updated', workspaceId: null },
  ],
  byWorkspace: [
    { workspaceId: 1, name: 'Default', total: 1 },
    { workspaceId: 7, name: '', total: 1 },
  ],
  unattributedTotal: 1,
};

const renderChanges = (data) =>
  render(
    <MemoryRouter>
      <NamespaceChanges namespace="default" state={{ data }} />
    </MemoryRouter>
  );

describe('push preview: Workspaces', () => {
  it('summarizes by Workspace, naming by id what the caller cannot name', () => {
    expect(summarizeWorkspaces(withWorkspaces)).toBe('By Workspace: Default 1 · Workspace #7 1 · 1 global');
    expect(summarizeWorkspaces({})).toBe('');
    expect(summarizeWorkspaces({ byWorkspace: [], unattributedTotal: 0 })).toBe('');
    expect(workspaceLabel('', null)).toBe('');
  });

  it('shows each change’s Workspace and the summary (Enterprise)', () => {
    usePermissions.mockReturnValue({ rbacEnabled: true });
    renderChanges(withWorkspaces);
    expect(screen.getByTestId('pending-changes-workspaces')).toHaveTextContent('By Workspace: Default 1 · Workspace #7 1 · 1 global');
    const llm = within(screen.getByTestId('pending-changes-group-llm'));
    expect(llm.getByTestId('pending-change-workspace')).toHaveTextContent('Default');
    const app = within(screen.getByTestId('pending-changes-group-app'));
    expect(app.getByText('An object you cannot see')).toBeInTheDocument();
    expect(app.queryByRole('link')).toBeNull();
    expect(app.getByTestId('pending-change-workspace')).toHaveTextContent('Workspace #7');
    expect(within(screen.getByTestId('pending-changes-group-plugin')).queryByTestId('pending-change-workspace')).toBeNull();
  });

  it('renders as before in Community and without the fields', () => {
    usePermissions.mockReturnValue({ rbacEnabled: false });
    renderChanges(withWorkspaces);
    expect(screen.queryByTestId('pending-changes-workspaces')).toBeNull();
    expect(screen.queryByTestId('pending-change-workspace')).toBeNull();

    usePermissions.mockReturnValue({ rbacEnabled: true });
    renderChanges({ namespace: 'default', total: 1, baseline: 'push', changes: [{ type: 'llm', id: 4, name: 'Old', change: 'updated' }] });
    expect(screen.queryByTestId('pending-changes-workspaces')).toBeNull();
    expect(screen.getByText('Old')).toBeInTheDocument();
  });

  it('the service maps the Workspace fields', async () => {
    apiClient.get.mockResolvedValue({
      data: {
        data: {
          namespace: 'default',
          total: 1,
          changes: [{ type: 'app', change: 'created', workspace_id: 7, hidden: true }],
          by_workspace: [{ workspace_id: 7, name: '', total: 1 }],
          unattributed_total: 0,
        },
      },
    });
    const got = await edgeGatewayService.getPendingChanges('default');
    expect(got.changes[0]).toMatchObject({ workspaceId: 7, workspaceName: '', hidden: true });
    expect(got.byWorkspace).toEqual([{ workspaceId: 7, name: '', total: 1 }]);
    expect(got.unattributedTotal).toBe(0);

    apiClient.get.mockResolvedValue({ data: { data: { namespace: 'default', total: 0, changes: [] } } });
    const old = await edgeGatewayService.getPendingChanges('default');
    expect(old.byWorkspace).toBeNull();
    expect(old.unattributedTotal).toBeNull();
  });
});
