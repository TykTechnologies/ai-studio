import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider } from '@mui/material/styles';
import testTheme from '../../../utils/testTheme';
import Step4AgentConfiguration from './Step4AgentConfiguration';
import { PermissionsProvider } from '../../../context/PermissionsContext';
import { clearIdentity } from '../../../utils/identityStore';
import { listAll } from '../../../utils/listAll';
import pluginService from '../../../services/pluginService';

// Workspaces .56: the plugin wizard's agent step chooses the Workspace the
// agent is created in, and shows a Workspace refusal on that field.

jest.mock('../../../utils/apiClient');
jest.mock('../../../utils/listAll', () => ({ listAll: jest.fn() }));
jest.mock('../../../services/pluginService', () => ({ __esModule: true, default: { getPlugin: jest.fn() } }));
jest.mock('../../agents/AgentFormFields', () => ({ __esModule: true, default: () => <div data-testid="agent-fields" /> }));

const identity = (attributes) => ({ id: '9', attributes: { is_admin: false, has_admin_access: true, permissions: [], ...attributes } });
const one = identity({ rbac_enabled: true, workspaces: [{ id: 2, name: 'W1', permissions: ['agents:write'] }] });
const community = identity({ is_admin: true, rbac_enabled: false, permissions: ['*'] });

const renderStep = (who, props = {}) =>
  render(
    <ThemeProvider theme={testTheme}>
      <PermissionsProvider identity={who}>
        <Step4AgentConfiguration
          pluginId={4}
          pluginData={{ name: 'Helper', description: 'd', config: {} }}
          onComplete={jest.fn()}
          onSkip={jest.fn()}
          loading={false}
          disabled={false}
          {...props}
        />
      </PermissionsProvider>
    </ThemeProvider>,
  );

const openForm = async () => {
  fireEvent.click(screen.getByRole('button', { name: 'Yes, Configure Agent' }));
  await screen.findByTestId('agent-fields');
};

describe('Step4AgentConfiguration Workspace', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    listAll.mockResolvedValue({ data: { data: [] } });
    pluginService.getPlugin.mockResolvedValue({});
  });

  it('with one Workspace preselects it', async () => {
    renderStep(one);
    await openForm();
    await waitFor(() => expect(screen.getByTestId('workspace-select')).toHaveTextContent('W1'));
    expect(screen.getByRole('combobox', { name: 'Workspace' })).toHaveAttribute('aria-disabled', 'true');
  });

  it('shows a Workspace refusal on the field', async () => {
    renderStep(one, { workspaceError: 'You can create this in several Workspaces; choose one.' });
    await openForm();
    expect(screen.getByText(/choose one/)).toBeInTheDocument();
  });

  it('Community: no Workspace select', async () => {
    renderStep(community);
    await openForm();
    expect(screen.queryByTestId('workspace-select')).not.toBeInTheDocument();
  });
});
