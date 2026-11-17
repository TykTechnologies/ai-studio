import React from 'react';
import { render, screen, within } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider } from '@mui/material/styles';
import adminTheme from '../../theme';
import WorkspaceEffectivePermissions from './WorkspaceEffectivePermissions';
import usePermissionCatalogue from '../../hooks/usePermissionCatalogue';

jest.mock('../../hooks/usePermissionCatalogue');

const catalogue = {
  loading: false,
  actions: ['read', 'write'],
  byKey: new Map([
    ['llms', { key: 'llms', label: 'Model providers' }],
    ['apps', { key: 'apps', label: 'Apps' }],
  ]),
  grouped: [
    {
      group: 'Model management',
      resources: [
        { key: 'llms', label: 'Model providers', actions: ['read', 'write'] },
        { key: 'apps', label: 'Apps', actions: ['read', 'write'] },
      ],
    },
  ],
};

describe('WorkspaceEffectivePermissions', () => {
  beforeEach(() => usePermissionCatalogue.mockReturnValue(catalogue));

  it('renders the global section and one section per Workspace with its roles', () => {
    render(
      <ThemeProvider theme={adminTheme}>
        <WorkspaceEffectivePermissions
          access={{
            permissions: ['apps:read'],
            sources: { 'apps:read': [{ role_id: 2, role_name: 'Viewer', via: 'direct' }] },
            workspaces: [
              {
                id: 4,
                name: 'Ops',
                permissions: ['llms:read', 'llms:write'],
                sources: {
                  'llms:read': [{ role_id: 3, role_name: 'Editor', via: 'group', group_id: 9, group_name: 'Platform' }],
                  'llms:write': [{ role_id: 3, role_name: 'Editor', via: 'group', group_id: 9, group_name: 'Platform' }],
                },
              },
              { id: 6, name: 'Data', permissions: ['apps:write'], roles: [{ id: 7, name: 'App Ops' }] },
            ],
          }}
        />
      </ThemeProvider>
    );
    expect(screen.getByText('Global (every Workspace)')).toBeInTheDocument();
    const ops = screen.getByTestId('effective-workspace-4');
    expect(within(ops).getByText('Workspace: Ops')).toBeInTheDocument();
    expect(within(ops).getByText('Editor via team Platform')).toBeInTheDocument();
    expect(within(ops).getByText(/Model providers/)).toBeInTheDocument();
    const data = screen.getByTestId('effective-workspace-6');
    expect(within(data).getByText('App Ops')).toBeInTheDocument();
  });

  it('has no Workspace sections without scoped roles', () => {
    render(
      <ThemeProvider theme={adminTheme}>
        <WorkspaceEffectivePermissions access={{ permissions: ['*'], workspaces: [] }} />
      </ThemeProvider>
    );
    expect(screen.getByText('Global (every Workspace)')).toBeInTheDocument();
    expect(screen.queryByText(/^Workspace:/)).toBeNull();
  });
});
