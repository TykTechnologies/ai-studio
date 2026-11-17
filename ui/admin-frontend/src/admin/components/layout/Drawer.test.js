import React from 'react';
import { render, screen, waitFor, act, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import Drawer from './Drawer';
import pubClient from '../../utils/pubClient';
import adminTheme from '../../theme';

jest.mock('../../utils/pubClient', () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
let mockGranted = new Set(['*']);
let mockWorkspaces = [];
jest.mock('../../context/PermissionsContext', () => {
  const { makeCanAnywhere } = jest.requireActual('../../context/PermissionsContext');
  return {
    usePermissions: () => ({
      permissions: mockGranted,
      canAny: (perms) => perms.some((p) => mockGranted.has('*') || mockGranted.has(p)),
      canAnywhere: makeCanAnywhere(mockGranted, mockWorkspaces),
    }),
  };
});
jest.mock('../../../components/common/Icon', () => ({
  __esModule: true,
  default: ({ name }) => <span data-testid={`icon-${name}`} />,
}));

const manifest = {
  surfaces: [{ id: 'admin', text: 'Admin', path: '/admin' }],
  admin: [
    { id: 'overview', text: 'Overview', icon: 'house', path: '/admin', exact: true },
    {
      id: 'llm-management',
      text: 'Model management',
      icon: 'microchip-ai',
      items: [
        { id: 'llms', text: 'Model providers', path: '/admin/llms', permission: 'llms:read' },
        { id: 'model-prices', text: 'Model prices', path: '/admin/model-prices', permission: 'model-prices:read' },
      ],
    },
    {
      id: 'access',
      text: 'Access',
      icon: 'users',
      items: [{ id: 'users', text: 'Users', path: '/admin/users', permission: 'users:read' }],
    },
    {
      id: 'plugin_1',
      text: 'Asset Catalog',
      icon: 'puzzle-piece',
      pluginId: 1,
      permission: 'plugins:execute',
      items: [{ id: 'a1', text: 'Overview', path: '/admin/ac', permission: 'plugins:execute' }],
    },
  ],
};

const renderDrawer = () =>
  render(
    <ThemeProvider theme={adminTheme}>
      <MemoryRouter initialEntries={['/admin']}>
        <Drawer />
      </MemoryRouter>
    </ThemeProvider>
  );

const expandGroup = async (label) => {
  fireEvent.click(await screen.findByText(label));
};

const topLevelLabels = () =>
  Array.from(document.querySelectorAll('[data-nav-depth="0"]')).map((el) => el.textContent);

describe('admin Drawer', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGranted = new Set(['*']);
    mockWorkspaces = [];
    pubClient.get.mockResolvedValue({ data: manifest });
    Object.defineProperty(window, 'localStorage', {
      value: { getItem: jest.fn(() => null), setItem: jest.fn(), removeItem: jest.fn(), clear: jest.fn() },
      configurable: true,
    });
  });

  it('renders the menu from /common/nav, in the order the server gives', async () => {
    renderDrawer();
    await screen.findByText('Asset Catalog');
    expect(pubClient.get).toHaveBeenCalledWith('/common/nav');
    expect(topLevelLabels()).toEqual(['Overview', 'Model management', 'Access', 'Asset Catalog']);
    expect(screen.getByTestId('icon-microchip-ai')).toBeInTheDocument();
    await expandGroup('Model management');
    expect(screen.getByText('Model providers').closest('a')).toHaveAttribute('href', '/admin/llms');
  });

  it('hides an entry whose permission was revoked before the reload lands', async () => {
    mockGranted = new Set(['llms:read', 'plugins:execute']);
    renderDrawer();
    await expandGroup('Model management');
    await screen.findByText('Model providers');
    expect(screen.queryByText('Model prices')).not.toBeInTheDocument();
  });

  it('shows a Workspace-scoped Editor the entries their Workspace role grants, and no global ones (.57)', async () => {
    mockGranted = new Set();
    mockWorkspaces = [
      { id: 4, name: 'W1', permissions: new Set(['llms:write', 'tools:write', 'apps:write']) },
    ];
    renderDrawer();
    await expandGroup('Model management');
    expect(await screen.findByText('Model providers')).toBeInTheDocument();
    expect(screen.queryByText('Model prices')).not.toBeInTheDocument();
    expect(screen.queryByText('Access')).not.toBeInTheDocument();
    expect(screen.queryByText('Users')).not.toBeInTheDocument();
    expect(screen.queryByText('Asset Catalog')).not.toBeInTheDocument();
    expect(topLevelLabels()).toEqual(['Overview', 'Model management']);
  });

  it('shows a portal-only user (no roles anywhere) nothing but the unguarded entries', async () => {
    mockGranted = new Set();
    mockWorkspaces = [];
    renderDrawer();
    await screen.findByText('Overview');
    expect(topLevelLabels()).toEqual(['Overview']);
  });

  it('reloads when plugin UIs change', async () => {
    renderDrawer();
    await screen.findByText('Asset Catalog');
    pubClient.get.mockResolvedValue({ data: { ...manifest, admin: manifest.admin.slice(0, 2) } });
    act(() => {
      window.dispatchEvent(new Event('plugin-loader-refreshed'));
    });
    await waitFor(() => expect(screen.queryByText('Asset Catalog')).not.toBeInTheDocument());
    expect(pubClient.get).toHaveBeenCalledTimes(2);
  });

  it('keeps the menu it has when a reload fails', async () => {
    renderDrawer();
    await screen.findByText('Asset Catalog');
    pubClient.get.mockRejectedValue(new Error('offline'));
    act(() => {
      window.dispatchEvent(new Event('plugin-loader-refreshed'));
    });
    await waitFor(() => expect(pubClient.get).toHaveBeenCalledTimes(2));
    expect(screen.getByText('Asset Catalog')).toBeInTheDocument();
  });

  it('renders an empty drawer when the first load fails', async () => {
    pubClient.get.mockRejectedValue(new Error('offline'));
    renderDrawer();
    await waitFor(() => expect(pubClient.get).toHaveBeenCalled());
    await waitFor(() => expect(topLevelLabels()).toEqual([]));
  });
});
