import React from 'react';
import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithRouterAndTheme } from '../../../test-utils/render-with-theme';
import PluginList from './PluginList';
import pluginService from '../../services/pluginService';
import { usePermissions } from '../../context/PermissionsContext';

jest.mock('../../services/pluginService', () => ({
  __esModule: true,
  default: {
    listPlugins: jest.fn(),
    getAvailableHookTypes: jest.fn(),
    getHookTypeLabel: jest.fn(),
  },
}));

jest.mock('../../context/PermissionsContext', () => ({
  __esModule: true,
  usePermissions: jest.fn(),
}));

// The dialog has its own suite; here it only has to open for the right plugin.
jest.mock('./PluginUpgradeDialog', () => ({
  __esModule: true,
  default: ({ open, plugin }) => (open ? <div data-testid="upgrade-dialog">{plugin.name}</div> : null),
}));

const plugins = [
  {
    id: '1',
    name: 'Cache',
    description: 'Response cache',
    hookType: 'post_auth',
    namespace: 'global',
    isActive: true,
    version: '1.0.0',
    marketplace: { marketplaceId: 'com.tyk.cache', installedVersion: '1.0.0', availableVersion: '1.2.0', updateAvailable: true },
  },
  {
    id: '2',
    name: 'Local Filter',
    description: 'Built in house',
    hookType: 'pre_auth',
    namespace: 'global',
    isActive: true,
    version: '',
    marketplace: null,
  },
];

const renderList = () => renderWithRouterAndTheme(<PluginList />);

describe('PluginList marketplace upgrades', () => {
  beforeEach(() => {
    pluginService.listPlugins.mockResolvedValue({ data: plugins, meta: { total_count: 2 }, updatesAvailable: 1 });
    pluginService.getAvailableHookTypes.mockReturnValue([]);
    pluginService.getHookTypeLabel.mockImplementation((hook) => hook);
    usePermissions.mockReturnValue({ can: () => true });
  });

  test('highlights the plugin that has a newer marketplace version', async () => {
    renderList();

    expect(await screen.findByText('1 plugin has a newer version in the marketplace.', { exact: false })).toBeInTheDocument();
    expect(screen.getByText('v1.0.0')).toBeInTheDocument();
    expect(screen.getByText('Update: v1.2.0')).toBeInTheDocument();
    expect(screen.getAllByText(/^Update: /)).toHaveLength(1);
  });

  test('the update chip opens the upgrade dialog for that plugin', async () => {
    renderList();

    await userEvent.click(await screen.findByText('Update: v1.2.0'));

    expect(screen.getByTestId('upgrade-dialog')).toHaveTextContent('Cache');
  });

  test('without permission to manage plugins the update is shown but cannot be started', async () => {
    usePermissions.mockReturnValue({ can: () => false });
    renderList();

    await userEvent.click(await screen.findByText('Update: v1.2.0'));
    expect(screen.queryByTestId('upgrade-dialog')).not.toBeInTheDocument();
  });

  test('no banner when everything is up to date', async () => {
    pluginService.listPlugins.mockResolvedValue({
      data: [{ ...plugins[0], marketplace: { ...plugins[0].marketplace, updateAvailable: false } }],
      meta: { total_count: 1 },
      updatesAvailable: 0,
    });
    renderList();

    expect(await screen.findByText('Cache')).toBeInTheDocument();
    expect(screen.queryByText(/newer version in the marketplace/)).not.toBeInTheDocument();
    expect(screen.queryByText(/^Update: /)).not.toBeInTheDocument();
  });
});
