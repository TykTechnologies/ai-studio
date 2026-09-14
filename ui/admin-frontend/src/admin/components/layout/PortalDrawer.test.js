import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import PortalDrawer from './PortalDrawer';
import adminTheme from '../../theme';

jest.mock('../../hooks/useSystemFeatures', () => () => ({
  features: { feature_portal: true, feature_gateway: true, feature_chat: true },
  loading: false,
}));
jest.mock('../../hooks/useUserEntitlements', () => () => ({
  uiOptions: { show_portal: true },
  userEntitlements: {},
  loading: false,
}));
jest.mock('../../../portal/services/portalPluginLoaderService', () => ({
  __esModule: true,
  default: { getSidebarMenuItems: jest.fn() },
}));
jest.mock('../../utils/pubClient', () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock('../../../components/common/Icon', () => ({
  __esModule: true,
  default: ({ name }) => <span data-testid={`icon-${name}`} />,
}));

const portalPluginLoaderService = require('../../../portal/services/portalPluginLoaderService').default;
const pubClient = require('../../utils/pubClient').default;

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
    portalPluginLoaderService.getSidebarMenuItems.mockResolvedValue([]);
    pubClient.get.mockResolvedValue({
      data: {
        data: [
          { plugin_id: 7, slug: 'agent', name: 'Agents', instances: [{ id: 'a1', name: 'Agent one' }] },
          { plugin_id: 7, slug: 'prompt', name: 'Prompts', instances: [] },
        ],
      },
    });
    Object.defineProperty(window, 'localStorage', {
      value: { getItem: jest.fn(() => null), setItem: jest.fn(), removeItem: jest.fn(), clear: jest.fn() },
      configurable: true,
    });
  });

  it('offers Browse with one entry per type and per plugin resource type with instances', async () => {
    renderDrawer();
    expect(await screen.findByRole('link', { name: 'All assets' })).toHaveAttribute('href', '/portal/catalog');
    expect(screen.getByRole('link', { name: 'LLM providers' })).toHaveAttribute('href', '/portal/catalog/llms');
    expect(screen.getByRole('link', { name: 'Data sources' })).toHaveAttribute('href', '/portal/catalog/datasources');
    expect(screen.getByRole('link', { name: 'Tools' })).toHaveAttribute('href', '/portal/catalog/tools');
    await waitFor(() =>
      expect(screen.getByRole('link', { name: 'Agents' })).toHaveAttribute('href', '/portal/catalog/resources/7/agent')
    );
    expect(screen.queryByRole('link', { name: 'Prompts' })).not.toBeInTheDocument();
    expect(screen.queryByText('Catalogs')).not.toBeInTheDocument();
    expect(screen.getByText('Browse')).toBeInTheDocument();
  });

  it('highlights the type entry on a detail page, not "All assets"', async () => {
    renderDrawer('/portal/catalog/llms/3');
    const llms = await screen.findByRole('link', { name: 'LLM providers' });
    expect(llms).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('link', { name: 'All assets' })).not.toHaveAttribute('aria-current');
  });
});
