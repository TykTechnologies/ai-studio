import React from 'react';
import { screen, waitFor, fireEvent } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { renderWithTheme } from '../../../test-utils/render-with-theme';
import PluginForm from './PluginForm';
import pluginService from '../../services/pluginService';

// Changing a plugin's command in the edit form never showed the scope
// re-approval step: the form armed it from a handler no field called and
// from hook types the API does not return, and the plugin stayed approved on
// the old binary's scopes (midsommar-clean-6bg5.97). Any command change now
// leads through save, review and approve.

jest.mock('../../services/pluginService', () => {
  const actual = jest.requireActual('../../services/pluginService');
  return {
    __esModule: true,
    ...actual,
    default: {
      // Everything the form does not call in these tests stays the real one.
      getAvailableHookTypes: () => actual.default.getAvailableHookTypes(),
      getPlugin: jest.fn(),
      updatePlugin: jest.fn(),
      createPlugin: jest.fn(),
      getPluginWorkflowStatus: jest.fn(),
      approvePluginScopes: jest.fn(),
      validateAndLoadPlugin: jest.fn(),
    },
  };
});
jest.mock('../../hooks/usePluginPolicy', () => ({ __esModule: true, default: () => ({ policy: null }) }));
jest.mock('../../../components/unsaved-changes', () => ({
  useUnsavedForm: () => ({ markSaved: jest.fn() }),
  useConfirmNavigation: () => (fn) => fn(),
}));
jest.mock('../common/EdgeAvailabilitySection', () => () => null);
jest.mock('../rbac/PublishSwitch', () => () => null);
jest.mock('./PluginConfigurationSection', () => () => null);
jest.mock('../common/AllowInternalHostSwitch', () => () => null);

const mockNavigate = jest.fn();
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
}));

// A gateway-only plugin, as the API returns it: no hook_types.
const stored = {
  id: '7',
  name: 'Rate limiter',
  description: '',
  command: 'file:///plugins/limiter-v1',
  config: {},
  hookType: 'post_auth',
  isActive: true,
  namespace: 'global',
  ociReference: '',
  allowInternalHost: false,
};

const renderForm = () =>
  renderWithTheme(
    <MemoryRouter initialEntries={['/admin/plugins/edit/7']}>
      <Routes>
        <Route path="/admin/plugins/edit/:id" element={<PluginForm mode="edit" />} />
      </Routes>
    </MemoryRouter>,
  );

const commandField = () => screen.getByDisplayValue(/limiter-v/);

describe('PluginForm: a command change asks for the scopes to be approved again', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pluginService.getPlugin.mockResolvedValue(stored);
    pluginService.updatePlugin.mockResolvedValue({ ...stored, command: 'file:///plugins/limiter-v2' });
    pluginService.getPluginWorkflowStatus.mockResolvedValue({
      data: { attributes: { status: 'scopes_pending', service_scopes: ['kv.readwrite'], service_access_authorized: false } },
    });
    pluginService.approvePluginScopes.mockResolvedValue({ status: 'approved' });
  });

  it('warns as soon as the command differs, for a plugin without UI or agent hooks too', async () => {
    renderForm();
    await screen.findByDisplayValue('file:///plugins/limiter-v1');
    expect(screen.queryByTestId('command-change-warning')).not.toBeInTheDocument();

    fireEvent.change(commandField(), { target: { name: 'command', value: 'file:///plugins/limiter-v2' } });
    expect(await screen.findByTestId('command-change-warning')).toHaveTextContent(/re-approval required/i);

    // Back to the stored command: nothing to approve.
    fireEvent.change(commandField(), { target: { name: 'command', value: 'file:///plugins/limiter-v1' } });
    await waitFor(() => expect(screen.queryByTestId('command-change-warning')).not.toBeInTheDocument());
  });

  it('saves, then shows the re-approval step instead of leaving, and approves', async () => {
    renderForm();
    await screen.findByDisplayValue('file:///plugins/limiter-v1');
    fireEvent.change(commandField(), { target: { name: 'command', value: 'file:///plugins/limiter-v2' } });
    fireEvent.click(screen.getByRole('button', { name: 'Update plugin' }));

    await waitFor(() => expect(pluginService.updatePlugin).toHaveBeenCalled());
    expect(pluginService.updatePlugin.mock.calls[0][1].command).toBe('file:///plugins/limiter-v2');

    const step = await screen.findByTestId('scope-reapproval');
    expect(step).toHaveTextContent('kv.readwrite');
    expect(pluginService.getPluginWorkflowStatus).toHaveBeenCalledWith('7');
    expect(mockNavigate).not.toHaveBeenCalled();
    // Nothing was approved before the save, and nothing by the save itself.
    expect(pluginService.approvePluginScopes).not.toHaveBeenCalled();
    expect(pluginService.validateAndLoadPlugin).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: 'Accept' }));
    await waitFor(() => expect(pluginService.approvePluginScopes).toHaveBeenCalledWith('7', true));
    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/admin/plugins', expect.anything()));
  });

  it('leaves the plugin unapproved when the scopes are declined', async () => {
    renderForm();
    await screen.findByDisplayValue('file:///plugins/limiter-v1');
    fireEvent.change(commandField(), { target: { name: 'command', value: 'file:///plugins/limiter-v2' } });
    fireEvent.click(screen.getByRole('button', { name: 'Update plugin' }));
    await screen.findByTestId('scope-reapproval');

    fireEvent.click(screen.getByRole('button', { name: 'Decline' }));
    expect(await screen.findByText(/not approved/i)).toBeInTheDocument();
    expect(pluginService.approvePluginScopes).not.toHaveBeenCalled();
    expect(mockNavigate).not.toHaveBeenCalled();
    expect(screen.getByTestId('scope-reapproval')).toBeInTheDocument();
    // The review's buttons are not submit buttons: the form is not saved again.
    expect(pluginService.updatePlugin).toHaveBeenCalledTimes(1);
  });

  it('saves and leaves as before when the command is unchanged', async () => {
    renderForm();
    await screen.findByDisplayValue('file:///plugins/limiter-v1');
    fireEvent.click(screen.getByRole('button', { name: 'Update plugin' }));

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/admin/plugins', expect.anything()));
    expect(screen.queryByTestId('scope-reapproval')).not.toBeInTheDocument();
    expect(pluginService.getPluginWorkflowStatus).not.toHaveBeenCalled();
    expect(pluginService.approvePluginScopes).not.toHaveBeenCalled();
  });
});
