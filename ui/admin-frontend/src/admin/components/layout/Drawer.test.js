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
jest.mock('../../context/PermissionsContext', () => ({
  usePermissions: () => ({
    permissions: mockGranted,
    canAny: (perms) => perms.some((p) => mockGranted.has('*') || mockGranted.has(p)),
  }),
}));
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
      text: 'LLM management',
      icon: 'microchip-ai',
      items: [
        { id: 'llms', text: 'LLM providers', path: '/admin/llms', permission: 'llms:read' },
        { id: 'model-prices', text: 'Model prices', path: '/admin/model-prices', permission: 'model-prices:read' },
      ],
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
    expect(topLevelLabels()).toEqual(['Overview', 'LLM management', 'Asset Catalog']);
    expect(screen.getByTestId('icon-microchip-ai')).toBeInTheDocument();
    await expandGroup('LLM management');
    expect(screen.getByText('LLM providers').closest('a')).toHaveAttribute('href', '/admin/llms');
  });

  it('hides an entry whose permission was revoked before the reload lands', async () => {
    mockGranted = new Set(['llms:read', 'plugins:execute']);
    renderDrawer();
    await expandGroup('LLM management');
    await screen.findByText('LLM providers');
    expect(screen.queryByText('Model prices')).not.toBeInTheDocument();
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
