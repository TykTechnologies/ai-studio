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

jest.mock('../../context/EditionContext', () => ({ useEdition: () => ({ isEnterprise: true }) }));
jest.mock('../../utils/apiClient', () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));
jest.mock('../../utils/listAll', () => ({ listAll: jest.fn(async () => ({ data: { data: [] } })) }));
jest.mock('../../context/PermissionsContext', () => ({ usePermissions: jest.fn() }));
jest.mock('../../services/rbacService', () => ({ listRoles: jest.fn(), sortRoles: (r) => r }));

const mockNavigate = jest.fn();
let mockParams = {};
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
  useParams: () => mockParams,
}));

const theme = createTheme({
  palette: {
    text: { primary: '#000', defaultSubdued: '#333' },
    background: { buttonPrimaryDefault: '#007bff', buttonPrimaryDefaultHover: '#0069d9', surfaceNeutralDisabled: '#eee' },
    custom: { white: '#fff' },
    primary: { main: '#7b68ee' },
    error: { main: '#dc3545' },
    border: { neutralDefault: '#e0e0e0' },
  },
  typography: { bodyLargeMedium: {}, headingXLarge: {}, bodyLargeDefault: {}, bodyLargeBold: {}, bodySmallDefault: {} },
});

const EDITOR = 5;
const roles = [{ id: String(EDITOR), attributes: { name: 'Editor', slug: 'editor', is_system: true, description: '' } }];

const renderForm = () =>
  render(
    <ThemeProvider theme={theme}>
      <MemoryRouter>
        <UserForm />
      </MemoryRouter>
    </ThemeProvider>
  );

describe('UserForm: service accounts', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = {};
    usePermissions.mockReturnValue({ rbacEnabled: true, identity: { id: '1' }, can: () => true });
    listRoles.mockResolvedValue(roles);
    apiClient.get.mockResolvedValue({ data: { data: [] } });
  });

  it('the service variant hides the person fields', async () => {
    renderForm();
    expect(screen.getByLabelText(/^Email/, { selector: 'input[type="email"]' })).toBeInTheDocument();
    expect(screen.getByLabelText(/Password/)).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText('Service account'));
    for (const label of [/^Email/, /Password/, /Show Portal/, /Show Chat/, /Email Verified/, /Enable Notifications/]) {
      expect(screen.queryByLabelText(label)).toBeNull();
    }
    expect(screen.queryByTestId('user-teams-field')).toBeNull();
    expect(screen.getByLabelText(/Name/)).toBeInTheDocument();
    expect(screen.getByTestId('user-roles-field')).toBeInTheDocument();
  });

  it('creates with account_type and no password, and shows the key once', async () => {
    apiClient.post.mockResolvedValue({ data: { data: { id: '42', attributes: { api_key: 'svc-key-1234567890' } } } });
    renderForm();
    fireEvent.click(screen.getByLabelText('Service account'));
    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Operator' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add service account' }));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalled());
    const [path, body] = apiClient.post.mock.calls[0];
    expect(path).toBe('/users');
    expect(body.data.attributes).toEqual({ name: 'Operator', role_ids: [], account_type: 'service' });
    expect(await screen.findByTestId('issued-api-key')).toHaveTextContent('svc-key-1234567890');
    expect(screen.getByText(/It will not be shown again/)).toBeInTheDocument();
    expect(mockNavigate).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    expect(mockNavigate).toHaveBeenCalledWith('/admin/users', expect.anything());
  });

  it('shows the seat-limit message on a 409 QUOTA_EXCEEDED', async () => {
    apiClient.post.mockRejectedValue({
      response: { status: 409, data: { errors: [{ code: 'QUOTA_EXCEEDED', detail: 'Admin seat limit reached (5 of 5).' }] } },
    });
    renderForm();
    fireEvent.click(screen.getByLabelText('Service account'));
    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Operator' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add service account' }));
    expect(await screen.findByText('Admin seat limit reached (5 of 5).')).toBeInTheDocument();
  });

  it('editing shows the type read-only and never sends it', async () => {
    mockParams = { id: '42' };
    apiClient.get.mockImplementation(async (path) =>
      path === '/users/42'
        ? {
            data: {
              data: {
                id: '42',
                attributes: { name: 'Operator', email: 'svc-x@service-account.invalid', account_type: 'service', roles: [] },
              },
            },
          }
        : { data: { data: [] } }
    );
    apiClient.patch.mockResolvedValue({ data: { data: {} } });
    renderForm();
    expect(await screen.findByTestId('account-type-readonly')).toHaveTextContent('Service account');
    expect(screen.queryByLabelText('User')).toBeNull();
    expect(screen.queryByLabelText(/Password/)).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Update service account' }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalled());
    const [path, body] = apiClient.patch.mock.calls[0];
    expect(path).toBe('/users/42');
    expect(body.data.attributes).toEqual({ name: 'Operator', role_ids: [] });
  });
});
