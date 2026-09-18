import React from 'react';
import { screen, waitFor } from '@testing-library/react';
import { renderWithTheme } from '../../../test-utils/render-with-theme';
import userEvent from '@testing-library/user-event';
import PluginUpgradeDialog from './PluginUpgradeDialog';
import pluginService from '../../services/pluginService';
import pluginLoaderService from '../../services/pluginLoaderService';

jest.mock('../../services/pluginService', () => ({
  __esModule: true,
  default: {
    previewPluginUpgrade: jest.fn(),
    upgradePlugin: jest.fn(),
  },
}));

jest.mock('../../services/pluginLoaderService', () => ({
  __esModule: true,
  default: { refresh: jest.fn() },
}));

const plugin = { id: '7', name: 'Cache' };

const basePreview = {
  plugin_id: 7,
  plugin_name: 'Cache',
  marketplace_id: 'com.tyk.cache',
  installed_version: '1.0.0',
  target_version: '1.2.0',
  latest_version: '1.2.0',
  is_downgrade: false,
  same_version: false,
  scopes: { current: ['kv.read'], target: ['kv.read'], added: [], removed: [] },
  hooks: { current: ['post_auth'], target: ['post_auth'], added: [], removed: [] },
  config_issues: [],
  warnings: [],
  affects_edges: false,
  changelog: '# Changelog\n\n## [1.2.0]\n\n- Faster cache lookups',
  versions: [
    { version: '1.2.0', deprecated: false, installed: false },
    { version: '1.0.0', deprecated: false, installed: true },
  ],
};

const renderDialog = (props = {}) => {
  const onUpgraded = jest.fn();
  const onClose = jest.fn();
  renderWithTheme(<PluginUpgradeDialog open plugin={plugin} onClose={onClose} onUpgraded={onUpgraded} {...props} />);
  return { onUpgraded, onClose };
};

describe('PluginUpgradeDialog', () => {
  beforeEach(() => {
    pluginService.previewPluginUpgrade.mockResolvedValue(basePreview);
    pluginService.upgradePlugin.mockResolvedValue({ from_version: '1.0.0', to_version: '1.2.0', affects_edges: false });
    pluginLoaderService.refresh.mockResolvedValue();
  });

  test('previews the latest version and upgrades keeping the configuration', async () => {
    const { onUpgraded } = renderDialog();

    expect(await screen.findByText(/configuration and all plugin data are kept/i)).toBeInTheDocument();
    expect(pluginService.previewPluginUpgrade).toHaveBeenCalledWith('7', '');
    expect(screen.getByText('v1.0.0')).toBeInTheDocument();
    expect(screen.getByText('Faster cache lookups')).toBeInTheDocument();
    expect(screen.getAllByText('Changelog')).toHaveLength(1);

    await userEvent.click(screen.getByRole('button', { name: 'Upgrade to v1.2.0' }));

    await waitFor(() => expect(onUpgraded).toHaveBeenCalled());
    expect(pluginService.upgradePlugin).toHaveBeenCalledWith('7', {
      version: '1.2.0',
      approvedScopes: [],
      allowDowngrade: false,
    });
    expect(pluginLoaderService.refresh).toHaveBeenCalled();
  });

  test('new permissions have to be approved before upgrading', async () => {
    pluginService.previewPluginUpgrade.mockResolvedValue({
      ...basePreview,
      scopes: { current: ['kv.read'], target: ['kv.read', 'llms.proxy'], added: ['llms.proxy'], removed: [] },
    });
    renderDialog();

    expect(await screen.findByText('This version requests new permissions')).toBeInTheDocument();
    const confirm = screen.getByRole('button', { name: 'Upgrade to v1.2.0' });
    expect(confirm).toBeDisabled();

    await userEvent.click(screen.getByRole('checkbox', { name: 'Approve these permissions' }));
    expect(confirm).toBeEnabled();

    await userEvent.click(confirm);
    await waitFor(() =>
      expect(pluginService.upgradePlugin).toHaveBeenCalledWith('7', {
        version: '1.2.0',
        approvedScopes: ['llms.proxy'],
        allowDowngrade: false,
      })
    );
  });

  test('a downgrade needs an explicit confirmation', async () => {
    pluginService.previewPluginUpgrade.mockResolvedValue({
      ...basePreview,
      installed_version: '1.2.0',
      target_version: '1.0.0',
      is_downgrade: true,
    });
    renderDialog();

    const confirm = await screen.findByRole('button', { name: 'Downgrade to v1.0.0' });
    expect(confirm).toBeDisabled();

    await userEvent.click(screen.getByRole('checkbox', { name: 'I understand, downgrade anyway' }));
    await userEvent.click(confirm);

    await waitFor(() =>
      expect(pluginService.upgradePlugin).toHaveBeenCalledWith('7', {
        version: '1.0.0',
        approvedScopes: [],
        allowDowngrade: true,
      })
    );
  });

  test('config mismatches warn but do not block', async () => {
    pluginService.previewPluginUpgrade.mockResolvedValue({
      ...basePreview,
      config_issues: ['(root): region is required'],
      affects_edges: true,
    });
    renderDialog();

    expect(await screen.findByText('(root): region is required')).toBeInTheDocument();
    expect(screen.getByText(/also runs on edge gateways/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Upgrade to v1.2.0' })).toBeEnabled();
  });

  test('tells the admin when the upgrade was rolled back', async () => {
    const error = new Error('new version failed to start, the previous version was restored: bad config');
    error.rolledBack = true;
    pluginService.upgradePlugin.mockRejectedValue(error);
    const { onUpgraded } = renderDialog();

    await userEvent.click(await screen.findByRole('button', { name: 'Upgrade to v1.2.0' }));

    expect(await screen.findByText(/The upgrade was rolled back/)).toBeInTheDocument();
    expect(screen.getByText(/bad config/)).toBeInTheDocument();
    expect(onUpgraded).not.toHaveBeenCalled();
  });

  test('already on the target: nothing to apply, the picker is still offered', async () => {
    pluginService.previewPluginUpgrade.mockResolvedValue({
      ...basePreview,
      installed_version: '1.2.0',
      same_version: true,
      versions: [
        { version: '1.2.0', deprecated: false, installed: true },
        { version: '1.0.0', deprecated: false, installed: false },
      ],
    });
    renderDialog();

    expect(await screen.findByText(/already on v1.2.0/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Upgrade to v1.2.0' })).toBeDisabled();
    expect(screen.getByLabelText('Target version')).toBeInTheDocument();
  });
});
