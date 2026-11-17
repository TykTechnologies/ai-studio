import React from 'react';
import { render, screen, waitFor, fireEvent, within } from '@testing-library/react';
import '@testing-library/jest-dom';
import WorkspaceSelect from './WorkspaceSelect';
import { usePermissions } from '../../context/PermissionsContext';
import { listWorkspaces } from '../../services/workspacesService';
import { P } from '../../rbac/permissions';

jest.mock('../../context/PermissionsContext', () => ({ usePermissions: jest.fn() }));
jest.mock('../../services/workspacesService', () => ({ listWorkspaces: jest.fn() }));

const access = ({ rbacEnabled = true, global = [], workspaces = [] }) => ({
  rbacEnabled,
  can: (perm) => global.includes(perm),
  workspaces: workspaces.map((w) => ({ ...w, permissions: new Set(w.permissions) })),
});

describe('WorkspaceSelect', () => {
  beforeEach(() => jest.clearAllMocks());

  it('renders nothing in Community Edition', () => {
    usePermissions.mockReturnValue(access({ rbacEnabled: false, global: [P.LLMS_WRITE] }));
    const { container } = render(<WorkspaceSelect permission={P.LLMS_WRITE} value="" onChange={jest.fn()} />);
    expect(container).toBeEmptyDOMElement();
    expect(listWorkspaces).not.toHaveBeenCalled();
  });

  it('with one choice preselects it and is disabled', async () => {
    usePermissions.mockReturnValue(
      access({ workspaces: [{ id: 2, name: 'W1', permissions: ['llms:write'] }, { id: 3, name: 'W2', permissions: ['llms:read'] }] })
    );
    const onChange = jest.fn();
    render(<WorkspaceSelect permission={P.LLMS_WRITE} value="" onChange={onChange} />);
    await waitFor(() => expect(onChange).toHaveBeenCalledWith(2));
    expect(screen.getByRole('combobox')).toHaveAttribute('aria-disabled', 'true');
    expect(listWorkspaces).not.toHaveBeenCalled();
  });

  it('a scoped caller picks among the Workspaces it may write in', async () => {
    usePermissions.mockReturnValue(
      access({
        workspaces: [
          { id: 2, name: 'W1', permissions: ['llms:write'] },
          { id: 3, name: 'W2', permissions: ['llms:write'] },
          { id: 4, name: 'W3', permissions: ['tools:write'] },
        ],
      })
    );
    const onChange = jest.fn();
    render(<WorkspaceSelect permission={P.LLMS_WRITE} value={2} onChange={onChange} />);
    fireEvent.mouseDown(screen.getByRole('combobox'));
    const listbox = within(screen.getByRole('listbox'));
    expect(listbox.getAllByRole('option').map((o) => o.textContent)).toEqual(['W1', 'W2']);
    fireEvent.click(listbox.getByText('W2'));
    expect(onChange).toHaveBeenCalledWith(3);
  });

  it('a global writer picks among every Workspace', async () => {
    usePermissions.mockReturnValue(access({ global: [P.LLMS_WRITE] }));
    listWorkspaces.mockResolvedValue([
      { id: '1', attributes: { name: 'Default', is_default: true } },
      { id: '2', attributes: { name: 'W1' } },
    ]);
    render(<WorkspaceSelect permission={P.LLMS_WRITE} value={1} onChange={jest.fn()} />);
    await waitFor(() => expect(listWorkspaces).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByRole('combobox')).not.toHaveAttribute('aria-disabled', 'true'));
    fireEvent.mouseDown(screen.getByRole('combobox'));
    expect(within(screen.getByRole('listbox')).getAllByRole('option').map((o) => o.textContent)).toEqual(['Default', 'W1']);
  });
});
