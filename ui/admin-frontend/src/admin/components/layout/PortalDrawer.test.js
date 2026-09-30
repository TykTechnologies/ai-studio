import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import PortalDrawer from './PortalDrawer';
import adminTheme from '../../theme';

jest.mock('../../utils/pubClient', () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock('../../context/PermissionsContext', () => ({
  usePermissions: () => ({ permissions: new Set(), canAny: () => false }),
}));
jest.mock('../../../components/common/Icon', () => ({
  __esModule: true,
  default: ({ name }) => <span data-testid={`icon-${name}`} />,
}));

const pubClient = require('../../utils/pubClient').default;

// The portal menu as GET /common/nav returns it (api/nav.go portalNav).
const portal = [
  { id: 'dashboard', text: 'Overview', icon: 'house', path: '/portal/dashboard' },
  { id: 'my-apps', text: 'Apps', icon: 'grid-2-plus', path: '/portal/apps' },
  {
    id: 'browse',
    text: 'Browse',
    icon: 'rectangle-history',
    items: [
      { id: 'browse-all', text: 'All assets', path: '/portal/catalog', exact: true },
      { id: 'browse-llms', text: 'LLM providers', path: '/portal/catalog/llms' },
      { id: 'browse-datasources', text: 'Data sources', path: '/portal/catalog/datasources' },
      { id: 'browse-tools', text: 'Tools', path: '/portal/catalog/tools' },
      { id: 'browse-resource-7-agent', text: 'Agents', path: '/portal/catalog/resources/7/agent', pluginId: 7 },
    ],
  },
  { id: 'plugin_9', text: 'Reports', icon: 'puzzle-piece', path: '/portal/plugins/reports', pluginId: 9 },
];

const renderDrawer = (path = '/portal/dashboard') =>
  render(
    <ThemeProvider theme={adminTheme}>
      <MemoryRouter initialEntries={[path]}>
        <PortalDrawer open />
      </MemoryRouter>
    </ThemeProvider>
  );

// The four-level "Catalogs → LLM providers → catalog name → cards" tree is
// replaced by one Browse section (UX review D4): everything, then one entry
// per asset type and per plugin resource type.
describe('PortalDrawer', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pubClient.get.mockResolvedValue({ data: { surfaces: [], admin: [], portal, chat: [] } });
    Object.defineProperty(window, 'localStorage', {
      value: { getItem: jest.fn(() => null), setItem: jest.fn(), removeItem: jest.fn(), clear: jest.fn() },
      configurable: true,
    });
  });

  it('renders Browse from the navigation manifest, expanded, with one entry per type', async () => {
    renderDrawer();
    expect(pubClient.get).toHaveBeenCalledWith('/common/nav');
    expect(await screen.findByRole('link', { name: 'All assets' })).toHaveAttribute('href', '/portal/catalog');
    expect(screen.getByRole('link', { name: 'LLM providers' })).toHaveAttribute('href', '/portal/catalog/llms');
    expect(screen.getByRole('link', { name: 'Data sources' })).toHaveAttribute('href', '/portal/catalog/datasources');
    expect(screen.getByRole('link', { name: 'Tools' })).toHaveAttribute('href', '/portal/catalog/tools');
    await waitFor(() =>
      expect(screen.getByRole('link', { name: 'Agents' })).toHaveAttribute('href', '/portal/catalog/resources/7/agent')
    );
    expect(screen.getByRole('link', { name: 'Reports' })).toHaveAttribute('href', '/portal/plugins/reports');
    expect(screen.queryByText('Catalogs')).not.toBeInTheDocument();
    expect(screen.getByText('Browse')).toBeInTheDocument();
  });

  it('highlights the type entry on a detail page, not "All assets"', async () => {
    renderDrawer('/portal/catalog/llms/3');
    const llms = await screen.findByRole('link', { name: 'LLM providers' });
    expect(llms).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('link', { name: 'All assets' })).not.toHaveAttribute('aria-current');
  });

  it('renders an empty drawer when the manifest cannot be loaded', async () => {
    pubClient.get.mockRejectedValue(new Error('offline'));
    renderDrawer();
    await waitFor(() => expect(pubClient.get).toHaveBeenCalled());
    expect(screen.queryByText('Browse')).not.toBeInTheDocument();
  });
});
