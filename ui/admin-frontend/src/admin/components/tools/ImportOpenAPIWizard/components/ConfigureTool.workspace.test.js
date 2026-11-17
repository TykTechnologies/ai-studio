import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import ConfigureTool from './ConfigureTool';
import { PermissionsProvider } from '../../../../context/PermissionsContext';
import { clearIdentity } from '../../../../utils/identityStore';

// Workspaces .54: the wizard's last step offers the Workspace the imported
// tool is created in, preselected with one choice, and shows refusals.

jest.mock('../../../common/privacy/PrivacyLevelInput', () => ({ __esModule: true, default: () => null }));
jest.mock('../../ToolAccessMethods', () => ({ __esModule: true, default: () => null }));

const identity = (attributes) => ({ id: '9', attributes: { is_admin: false, has_admin_access: true, permissions: [], ...attributes } });
const one = identity({ rbac_enabled: true, workspaces: [{ id: 2, name: 'W1', permissions: ['tools:write'] }] });
const community = identity({ is_admin: true, rbac_enabled: false, permissions: ['*'] });

const config = { name: 'Weather', description: 'd', privacy_score: 25, operations: [], auth_schema_name: '', auth_key: '', workspace_id: null };

const renderStep = (who, props = {}) =>
  render(
    <PermissionsProvider identity={who}>
      <ConfigureTool toolConfig={config} onConfigChange={jest.fn()} loading={false} error="" {...props} />
    </PermissionsProvider>,
  );

describe('ConfigureTool Workspace', () => {
  beforeEach(() => clearIdentity());

  it('with one Workspace preselects it', async () => {
    const onConfigChange = jest.fn();
    renderStep(one, { onConfigChange });
    expect(screen.getByTestId('workspace-select')).toBeInTheDocument();
    await waitFor(() => expect(onConfigChange).toHaveBeenCalledWith(expect.objectContaining({ workspace_id: 2 })));
  });

  it('shows a Workspace refusal on the field', () => {
    renderStep(one, { workspaceError: 'You can create this in several Workspaces; choose one.' });
    expect(screen.getByText(/choose one/)).toBeInTheDocument();
  });

  it('Community: no Workspace select', () => {
    renderStep(community);
    expect(screen.queryByTestId('workspace-select')).not.toBeInTheDocument();
  });
});
