import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import UserForm from './UserForm';
import { useUserForm } from './hooks/useUserForm';
import useUserEntitlements from '../../hooks/useUserEntitlements';
import { IsAdminRole } from './utils/userRolesConfig';
import * as reactRouterDom from 'react-router-dom';
import { renderWithTheme } from '../../../test-utils/render-with-theme';

jest.mock('@mui/material', () => require('../../../test-utils/mui-mocks').muiMaterialMock);
jest.mock('@mui/icons-material/ChevronLeft', () => () => 'ChevronLeftIcon');
jest.mock('./hooks/useUserForm');
jest.mock('../../hooks/useUserEntitlements');
jest.mock('./utils/userRolesConfig');
jest.mock('../../styles/sharedStyles', () => require('../../../test-utils/styled-component-mocks').sharedStylesMock);
jest.mock('./styles', () => require('../../../test-utils/styled-component-mocks').userFormStylesMock);
jest.mock('react-router-dom', () => ({
  useNavigate: jest.fn(),
  useParams: jest.fn(),
  Link: jest.fn()
}));
jest.mock('../../components/common/AlertSnackbar', () => ({
  __esModule: true,
  default: ({ open, message, severity, onClose }) => (
    <div data-testid="alert-snackbar" data-open={open} data-message={message} data-severity={severity}>
      <button onClick={onClose} data-testid="close-snackbar">Close</button>
    </div>
  )
}));
jest.mock('../../components/common/ConfirmationDialog', () => ({
  __esModule: true,
  default: ({ open, title, message, onConfirm, onCancel }) => {
    if (!open) return null;
    return (
      <div data-testid="confirmation-dialog" data-open={open} data-title={title} data-message={message}>
        <button onClick={onConfirm} data-testid="confirm-button">Delete user</button>
        <button onClick={onCancel} data-testid="cancel-button">Cancel</button>
      </div>
    );
  }
}));
jest.mock('./components/UserFormBasicInfo', () => ({
  __esModule: true,
  default: ({ name, setName, email, setEmail, password, setPassword, emailVerified, setEmailVerified, notificationsEnabled, setNotificationsEnabled, accessToSSOConfig, setAccessToSSOConfig, setBasicInfoValid, basicInfoValid, ...props }) => (
    <div data-testid="user-form-basic-info" />
  )
}));
jest.mock('./components/UserPermissionsSection', () => ({
  __esModule: true,
  default: ({ selectedRole, setSelectedRole, isSuperAdmin, ...props }) => (
    <div data-testid="user-permissions-section" />
  )
}));
jest.mock('./components/ManageTeamsSection', () => ({
  __esModule: true,
  default: ({ selectedTeams, setSelectedTeams, ...props }) => (
    <div data-testid="manage-teams-section" />
  )
}));

describe('UserForm', () => {
  const mockNavigate = jest.fn();
  const mockFetchUserEntitlements = jest.fn();
  const mockHandleSubmit = jest.fn(e => e.preventDefault());
  const mockHandleDeleteClick = jest.fn();
  const mockHandleCancelDelete = jest.fn();
  const mockHandleConfirmDelete = jest.fn();
  
  beforeEach(() => {
    jest.clearAllMocks();
    
    reactRouterDom.useNavigate.mockImplementation(() => mockNavigate);
    reactRouterDom.useParams.mockImplementation(() => ({ id: '123' }));
    reactRouterDom.Link.mockImplementation(props => props.children);
    
    useUserEntitlements.mockReturnValue({
      isSuperAdmin: false,
      fetchUserEntitlements: mockFetchUserEntitlements
    });
    
    useUserForm.mockReturnValue({
      name: 'Test User',
      setName: jest.fn(),
      email: 'test@example.com',
      setEmail: jest.fn(),
      password: '',
      setPassword: jest.fn(),
      emailVerified: false,
      setEmailVerified: jest.fn(),
      notificationsEnabled: false,
      setNotificationsEnabled: jest.fn(),
      accessToSSOConfig: false,
      setAccessToSSOConfig: jest.fn(),
      selectedRole: 'Chat user',
      setSelectedRole: jest.fn(),
      selectedTeams: [],
      setSelectedTeams: jest.fn(),
      loading: false,
      handleSubmit: mockHandleSubmit,
      setBasicInfoValid: jest.fn(),
      basicInfoValid: true,
      warningDialogOpen: false,
      handleDeleteClick: mockHandleDeleteClick,
      handleCancelDelete: mockHandleCancelDelete,
      handleConfirmDelete: mockHandleConfirmDelete
    });

    IsAdminRole.mockReturnValue(false);
  });

  it('renders loading state when loading is true', () => {
    useUserForm.mockReturnValueOnce({
      ...useUserForm(),
      loading: true
    });

    render(<UserForm />);
    expect(screen.getByTestId('circular-progress')).toBeInTheDocument();
  });

  it('renders form correctly for creating a new user', () => {
    reactRouterDom.useParams.mockImplementationOnce(() => ({}));
    
    render(<UserForm />);
    expect(screen.getByText('Create user')).toBeInTheDocument();
    expect(screen.getByText('Save user')).toBeInTheDocument();
    expect(screen.queryByText('Delete user')).not.toBeInTheDocument();
  });

  it('renders form correctly for editing an existing user', () => {
    renderWithTheme(<UserForm />);
    expect(screen.getByText('Edit user')).toBeInTheDocument();
    expect(screen.getByText('Update user')).toBeInTheDocument();
    expect(screen.getByTestId('danger-outline-button')).toBeInTheDocument();
  });

  it('calls handleSubmit when form is submitted', () => {
    render(<UserForm />);
    const form = screen.getByTestId('box');
    fireEvent.submit(form);
    expect(mockHandleSubmit).toHaveBeenCalled();
  });

  it('disables submit button when basicInfoValid is false', () => {
    useUserForm.mockReturnValueOnce({
      ...useUserForm(),
      basicInfoValid: false
    });

    render(<UserForm />);
    const submitButton = screen.getByText('Update user');
    expect(submitButton.disabled).toBeTruthy();
  });

  it('enables submit button when basicInfoValid is true', () => {
    useUserForm.mockReturnValueOnce({
      ...useUserForm(),
      basicInfoValid: true
    });

    render(<UserForm />);
    const submitButton = screen.getByText('Update user');
    expect(submitButton.disabled).toBeFalsy();
  });

  it('calls handleDeleteClick when delete button is clicked', () => {
    renderWithTheme(<UserForm />);
    const deleteButton = screen.getByTestId('danger-outline-button');
    fireEvent.click(deleteButton);
    expect(mockHandleDeleteClick).toHaveBeenCalled();
  });

  it('fetches user entitlements on mount', () => {
    render(<UserForm />);
    expect(mockFetchUserEntitlements).toHaveBeenCalledWith(true);
  });

  it('redirects when editing admin user without super admin privileges', () => {
    useUserForm.mockReturnValueOnce({
      ...useUserForm(),
      selectedRole: 'Admin'
    });
    
    IsAdminRole.mockReturnValue(true);
    
    render(<UserForm />);
    expect(mockNavigate).toHaveBeenCalledWith('/admin/users/123');
  });

  it('does not show delete button for super admin', () => {
    useUserEntitlements.mockReturnValueOnce({
      isSuperAdmin: true,
      fetchUserEntitlements: mockFetchUserEntitlements
    });

    render(<UserForm />);
    expect(screen.queryByText('Delete user')).not.toBeInTheDocument();
  });

  it('renders confirmation dialog when delete is clicked', () => {
    useUserForm.mockReturnValueOnce({
      ...useUserForm(),
      warningDialogOpen: true
    });

    render(<UserForm />);
    expect(screen.getByTestId('confirmation-dialog')).toBeInTheDocument();
    expect(screen.getByTestId('confirmation-dialog')).toHaveAttribute('data-title', 'Delete User');
    expect(screen.getByTestId('confirmation-dialog')).toHaveAttribute('data-message', 'This will delete all records of this user, and they will no longer have access to Tyk AI Studio.');
  });

  it('calls handleConfirmDelete when delete is confirmed', () => {
    useUserForm.mockReturnValueOnce({
      ...useUserForm(),
      warningDialogOpen: true
    });

    render(<UserForm />);
    const confirmButton = screen.getByTestId('confirm-button');
    fireEvent.click(confirmButton);
    expect(mockHandleConfirmDelete).toHaveBeenCalled();
  });
});