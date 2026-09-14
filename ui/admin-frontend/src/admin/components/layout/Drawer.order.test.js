import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import Drawer from './Drawer';
import useAdminData from '../../hooks/useAdminData';
import adminTheme from '../../theme';

jest.mock('../../hooks/useAdminData');
jest.mock('../../context/PermissionsContext', () => ({
  usePermissions: () => ({ can: () => true, canAny: () => true, canAll: () => true }),
}));
jest.mock('../../services/pluginLoaderService', () => ({
  __esModule: true,
  default: { getSidebarMenuItems: jest.fn(), initialize: jest.fn() },
}));
jest.mock('../../../components/common/Icon', () => ({
  __esModule: true,
  default: ({ name }) => <span data-testid={`icon-${name}`} />,
}));

const pluginLoaderService = require('../../services/pluginLoaderService').default;

const adminData = (overrides = {}) => ({
  features: {
    feature_portal: true,
    feature_chat: true,
    feature_gateway: true,
    feature_groups: true,
    feature_rbac: true,
    ...overrides.features,
  },
  uiOptions: {},
  config: { is_enterprise: true, ...overrides.config },
  loading: false,
  error: null,
});

const renderDrawer = () =>
  render(
    <ThemeProvider theme={adminTheme}>
      <MemoryRouter initialEntries={['/admin']}>
        <Drawer />
      </MemoryRouter>
    </ThemeProvider>
  );

const topLevelLabels = () =>
  Array.from(document.querySelectorAll('[data-nav-depth="0"]')).map((el) => el.textContent);

describe('Drawer group order', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    Object.defineProperty(window, 'localStorage', {
      value: { getItem: jest.fn(() => null), setItem: jest.fn(), removeItem: jest.fn(), clear: jest.fn() },
      configurable: true,
    });
  });

  it('puts Access and Catalogs before the management groups and Plugins last', async () => {
    pluginLoaderService.getSidebarMenuItems.mockResolvedValue([]);
    useAdminData.mockReturnValue(adminData());
    renderDrawer();
    await screen.findByText('Overview');
    expect(topLevelLabels()).toEqual([
      'Overview',
      'Analytics',
      'Access',
      'Catalogs',
      'LLM management',
      'Context management',
      'AI Portal',
      'Community',
      'Governance',
      'Settings',
      'Chat',
      'Plugins',
    ]);
  });

  it('renders plugin sections after Governance, sorted by label', async () => {
    pluginLoaderService.getSidebarMenuItems.mockResolvedValue([
      { id: 'plugin_2', label: 'Zeta Tools', sub_items: [{ id: 'z1', text: 'Home', path: '/admin/plugins/zeta' }] },
      { id: 'plugin_1', label: 'Asset Catalog', sub_items: [{ id: 'a1', text: 'Overview', path: '/admin/enterprise/asset-catalog/overview' }] },
    ]);
    useAdminData.mockReturnValue(adminData());
    renderDrawer();
    await screen.findByText('Asset Catalog');
    const labels = topLevelLabels();
    const governance = labels.indexOf('Governance');
    expect(labels.slice(governance, governance + 4)).toEqual(['Governance', 'Asset Catalog', 'Zeta Tools', 'Settings']);
    expect(labels[labels.length - 1]).toBe('Plugins');
  });

  it('keeps plugin sections in the same slot when Governance is hidden (CE)', async () => {
    pluginLoaderService.getSidebarMenuItems.mockResolvedValue([
      { id: 'plugin_1', label: 'Asset Catalog', sub_items: [{ id: 'a1', text: 'Overview', path: '/admin/x' }] },
    ]);
    useAdminData.mockReturnValue(adminData({ config: { is_enterprise: false } }));
    renderDrawer();
    await screen.findByText('Asset Catalog');
    const labels = topLevelLabels();
    expect(labels).not.toContain('Governance');
    expect(labels.indexOf('Asset Catalog')).toBe(labels.indexOf('Community') + 1);
    expect(labels.indexOf('Settings')).toBe(labels.indexOf('Asset Catalog') + 1);
  });

  it('renders a single Apps entry in gateway-only mode, under Apps & credentials', async () => {
    pluginLoaderService.getSidebarMenuItems.mockResolvedValue([]);
    useAdminData.mockReturnValue(
      adminData({ features: { feature_portal: false, feature_chat: false, feature_gateway: true } })
    );
    renderDrawer();
    await screen.findByText('Overview');
    const labels = topLevelLabels();
    expect(labels).toContain('Apps & credentials');
    expect(labels).not.toContain('AI Portal');
    expect(labels).not.toContain('Catalogs');
    expect(labels.indexOf('Apps & credentials')).toBe(labels.indexOf('Context management') + 1);
  });
});
