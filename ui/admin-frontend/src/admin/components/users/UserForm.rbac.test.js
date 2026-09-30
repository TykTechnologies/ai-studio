import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import { createTheme } from '@mui/material';
import UserForm from './UserForm';
import apiClient from '../../utils/apiClient';
import { usePermissions } from '../../context/PermissionsContext';
import { listRoles } from '../../services/rbacService';

jest.mock("../../context/EditionContext", () => ({
  useEdition: () => ({ isEnterprise: false }),
}));
jest.mock('../../utils/apiClient', () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn() },
}));
jest.mock('../../context/PermissionsContext', () => ({
  usePermissions: jest.fn(),
}));
jest.mock('../../services/rbacService', () => ({
  listRoles: jest.fn(),
  sortRoles: (r) => r,
}));

const mockNavigate = jest.fn();
const mockParams = { id: '5' };
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
  useParams: () => mockParams,
}));

const theme = createTheme({
  palette: {
    text: { primary: '#fff', defaultSubdued: '#ccc' },
    background: { buttonPrimaryDefault: '#007bff', buttonPrimaryDefaultHover: '#0069d9' },
    custom: { white: '#fff' },
    primary: { main: '#7b68ee' },
    error: { main: '#dc3545' },
    border: { neutralDefault: '#e0e0e0' },
  },
  typography: { bodyLargeMedium: {}, headingXLarge: {}, bodyLargeDefault: {}, bodyLargeBold: {} },
});

const roles = [
  { id: '1', attributes: { name: 'Administrator', slug: 'administrator', is_system: true, description: '' } },
  { id: '3', attributes: { name: 'Viewer', slug: 'viewer', is_system: true, description: '' } },
  { id: '4', attributes: { name: 'Auditor', slug: 'auditor', is_system: true, description: '' } },
  { id: '5', attributes: { name: 'Editor', slug: 'editor', is_system: true, description: '' } },
];

const userPayload = (extra = {}) => ({
  data: {
    data: {
      id: '5',
      attributes: {
        name: 'Dev',
        email: 'dev@tyk.io',
        is_admin: false,
        show_portal: true,
        show_chat: true,
        email_verified: true,
        notifications_enabled: false,
        access_to_sso_config: false,
        roles: [{ id: 3, name: 'Viewer', slug: 'viewer', is_system: true }],
        ...extra,
      },
    },
  },
});

const renderForm = () =>
  render(
    <ThemeProvider theme={theme}>
      <MemoryRouter>
        <UserForm />
      </MemoryRouter>
    </ThemeProvider>
  );

describe('UserForm with roles (Enterprise)', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    usePermissions.mockReturnValue({ rbacEnabled: true, identity: { id: '99', isFullAdmin: true } });
    listRoles.mockResolvedValue(roles);
    apiClient.get.mockImplementation((url) => {
      if (url === '/groups') return Promise.resolve({ data: { data: [] } });
      if (url === '/users/5') return Promise.resolve(userPayload());
      if (url === '/users/5/groups') return Promise.resolve({ data: { data: [] } });
      return Promise.resolve({ data: { data: [] } });
    });
    apiClient.patch.mockResolvedValue({ data: { data: {} } });
    apiClient.post.mockResolvedValue({ data: { data: { id: '6' } } });
  });

  it('replaces the admin switch with a role selector and sends role_ids, not is_admin', async () => {
    renderForm();
    await screen.findByTestId('user-roles-field');
    expect(screen.queryByLabelText('Admin User')).toBeNull();
    expect(screen.queryByLabelText('Enable access to IdP configuration')).toBeNull();
    await waitFor(() => expect(screen.getByDisplayValue('Dev')).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: /update user/i }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    const [, body] = apiClient.patch.mock.calls[0];
    expect(body.data.attributes.role_ids).toEqual([3]);
    expect(body.data.attributes).not.toHaveProperty('is_admin');
    expect(body.data.attributes.access_to_sso_config).toBe(false);
  });

  it('keeps the Community Edition form unchanged when roles are off', async () => {
    usePermissions.mockReturnValue({ rbacEnabled: false, identity: { id: '99', isFullAdmin: true } });
    renderForm();
    await waitFor(() => expect(screen.getByDisplayValue('Dev')).toBeInTheDocument());
    expect(screen.getByLabelText('Admin User')).toBeInTheDocument();
    expect(screen.queryByTestId('user-roles-field')).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: /update user/i }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    const [, body] = apiClient.patch.mock.calls[0];
    expect(body.data.attributes.is_admin).toBe(false);
    expect(body.data.attributes).not.toHaveProperty('role_ids');
  });

  // Team roles belong to the team: preselecting them made saving the form
  // turn them into direct roles. Roles the host application assigns are
  // shown locked.
  it('edits direct roles only, and shows host-assigned roles locked', async () => {
    apiClient.get.mockImplementation((url) => {
      if (url === '/users/5') {
        return Promise.resolve(userPayload({
          roles: [
            { id: 3, name: 'Viewer', slug: 'viewer', is_system: true, via: 'direct' },
            { id: 4, name: 'Auditor', slug: 'auditor', is_system: true, via: 'host' },
            { id: 5, name: 'Editor', slug: 'editor', is_system: true, via: 'group', group_id: 2, group_name: 'Dev team' },
          ],
        }));
      }
      return Promise.resolve({ data: { data: [] } });
    });
    renderForm();
    await screen.findByTestId('user-roles-field');
    expect(await screen.findByTestId('role-chip-locked-4')).toBeInTheDocument();
    expect(screen.getByText(/assigned by the application AI Studio is embedded in/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /update user/i }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    const [, body] = apiClient.patch.mock.calls[0];
    expect(body.data.attributes.role_ids.sort()).toEqual([3, 4]);
    expect(body.data.attributes.role_ids).not.toContain(5);
  });
});
