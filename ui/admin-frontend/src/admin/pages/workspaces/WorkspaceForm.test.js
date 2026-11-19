import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import adminTheme from '../../theme';
import WorkspaceForm from './WorkspaceForm';
import { createWorkspace, getWorkspace, updateWorkspace } from '../../services/workspacesService';

const mockNavigate = jest.fn();
jest.mock('react-router-dom', () => ({ ...jest.requireActual('react-router-dom'), useNavigate: () => mockNavigate }));
jest.mock('../../services/workspacesService', () => ({
  createWorkspace: jest.fn(),
  getWorkspace: jest.fn(),
  updateWorkspace: jest.fn(),
}));

const renderAt = (path) =>
  render(
    <ThemeProvider theme={adminTheme}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/admin/workspaces/new" element={<WorkspaceForm />} />
          <Route path="/admin/workspaces/edit/:id" element={<WorkspaceForm />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

const conflict = (code) => Object.assign(new Error('a Workspace with that name already exists'), { status: 409, code });

describe('WorkspaceForm', () => {
  beforeEach(() => jest.clearAllMocks());

  it('creates a Workspace and opens it', async () => {
    createWorkspace.mockResolvedValue({ id: '7' });
    renderAt('/admin/workspaces/new');
    fireEvent.change(screen.getByTestId('workspace-name'), { target: { value: ' Ops ' } });
    fireEvent.change(screen.getByTestId('workspace-description'), { target: { value: 'Operations' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add Workspace' }));
    await waitFor(() => expect(createWorkspace).toHaveBeenCalledWith({ name: 'Ops', description: 'Operations' }));
    expect(mockNavigate).toHaveBeenCalledWith('/admin/workspaces/7');
  });

  it('shows a duplicate name on the field', async () => {
    createWorkspace.mockRejectedValue(conflict(''));
    renderAt('/admin/workspaces/new');
    fireEvent.change(screen.getByTestId('workspace-name'), { target: { value: 'Ops' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add Workspace' }));
    expect(await screen.findByText('A Workspace with this name already exists')).toBeInTheDocument();
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('requires a name', async () => {
    renderAt('/admin/workspaces/new');
    fireEvent.click(screen.getByRole('button', { name: 'Add Workspace' }));
    expect(await screen.findByText('A name is required')).toBeInTheDocument();
    expect(createWorkspace).not.toHaveBeenCalled();
  });

  it('edits an existing Workspace', async () => {
    getWorkspace.mockResolvedValue({ id: '7', attributes: { name: 'Ops', description: 'd' } });
    updateWorkspace.mockResolvedValue({ id: '7' });
    renderAt('/admin/workspaces/edit/7');
    await waitFor(() => expect(screen.getByTestId('workspace-name')).toHaveValue('Ops'));
    fireEvent.change(screen.getByTestId('workspace-name'), { target: { value: 'Operations' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(updateWorkspace).toHaveBeenCalledWith('7', { name: 'Operations', description: 'd' }));
  });
});
