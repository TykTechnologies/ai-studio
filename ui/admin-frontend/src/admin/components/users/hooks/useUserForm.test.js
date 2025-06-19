import React from 'react';
import { screen, waitFor, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { renderWithTheme } from '../../../../test-utils/render-with-theme';
import { mockNavigate } from '../../../../test-utils/service-mocks';
import { useUserForm } from './useUserForm';
import * as userService from '../../../services/userService';

jest.mock('../../../services/userService', () => ({
  createUser: jest.fn(),
  updateUser: jest.fn(),
  getUser: jest.fn(),
  deleteUser: jest.fn(),
}));

jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
}));

const mockCreateUser = userService.createUser;
const mockUpdateUser = userService.updateUser;
const mockGetUser = userService.getUser;
const mockDeleteUser = userService.deleteUser;

const TestComponent = ({ id = null, mockShowSnackbar }) => {
  const hookResult = useUserForm(id, mockShowSnackbar);
  
  return (
    <div>
      <div data-testid="name">{hookResult.name}</div>
      <div data-testid="email">{hookResult.email}</div>
      <div data-testid="password">{hookResult.password}</div>
      <div data-testid="email-verified">{hookResult.emailVerified.toString()}</div>
      <div data-testid="notifications-enabled">{hookResult.notificationsEnabled.toString()}</div>
      <div data-testid="access-to-sso-config">{hookResult.accessToSSOConfig.toString()}</div>
      <div data-testid="selected-role">{hookResult.selectedRole}</div>
      <div data-testid="selected-teams">{JSON.stringify(hookResult.selectedTeams)}</div>
      <div data-testid="loading">{hookResult.loading.toString()}</div>
      <div data-testid="basic-info-valid">{hookResult.basicInfoValid.toString()}</div>
      <div data-testid="warning-dialog-open">{hookResult.warningDialogOpen.toString()}</div>
      
      <input 
        data-testid="name-input" 
        value={hookResult.name} 
        onChange={(e) => hookResult.setName(e.target.value)} 
      />
      
      <input 
        data-testid="email-input" 
        value={hookResult.email} 
        onChange={(e) => hookResult.setEmail(e.target.value)} 
      />
      
      <input 
        data-testid="password-input" 
        value={hookResult.password} 
        onChange={(e) => hookResult.setPassword(e.target.value)} 
      />
      
      <button 
        data-testid="toggle-email-verified" 
        onClick={() => hookResult.setEmailVerified(!hookResult.emailVerified)}
      >
        Toggle Email Verified
      </button>
      
      <button 
        data-testid="toggle-notifications" 
        onClick={() => hookResult.setNotificationsEnabled(!hookResult.notificationsEnabled)}
      >
        Toggle Notifications
      </button>
      
      <button 
        data-testid="toggle-sso-access" 
        onClick={() => hookResult.setAccessToSSOConfig(!hookResult.accessToSSOConfig)}
      >
        Toggle SSO Access
      </button>
      
      <select 
        data-testid="role-select" 
        value={hookResult.selectedRole} 
        onChange={(e) => hookResult.setSelectedRole(e.target.value)}
      >
        <option value="Chat user">Chat user</option>
        <option value="Developer">Developer</option>
        <option value="Admin">Admin</option>
      </select>
      
      <button 
        data-testid="set-teams"
        onClick={() => hookResult.setSelectedTeams([1, 2, 3])}
      >
        Set Teams
      </button>
      
      <button 
        data-testid="set-basic-info-valid"
        onClick={() => hookResult.setBasicInfoValid(true)}
      >
        Set Basic Info Valid
      </button>
      
      <button 
        data-testid="submit-form" 
        onClick={(e) => {
          e.preventDefault = jest.fn();
          hookResult.handleSubmit(e);
        }}
      >
        Submit Form
      </button>
      
      <button 
        data-testid="delete-click" 
        onClick={() => hookResult.handleDeleteClick()}
      >
        Delete
      </button>
      
      <button 
        data-testid="cancel-delete" 
        onClick={() => hookResult.handleCancelDelete()}
      >
        Cancel Delete
      </button>
      
      <button 
        data-testid="confirm-delete" 
        onClick={() => hookResult.handleConfirmDelete()}
      >
        Confirm Delete
      </button>
    </div>
  );
};

describe('useUserForm Hook', () => {
  const mockUserResponse = {
    data: {
      attributes: {
        name: 'Test User',
        email: 'test@example.com',
        role: 'Developer',
        groups: [
          { id: '1' },
          { id: '2' }
        ],
        email_verified: true,
        notifications_enabled: true,
        access_to_sso_config: false
      }
    }
  };

  beforeEach(() => {
    jest.clearAllMocks();
    
    mockGetUser.mockResolvedValue(mockUserResponse);
    mockCreateUser.mockResolvedValue({});
    mockUpdateUser.mockResolvedValue({});
    mockDeleteUser.mockResolvedValue({});
  });

  test('initializes with default values when no ID is provided', () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent mockShowSnackbar={mockShowSnackbar} />);
    
    expect(screen.getByTestId('name').textContent).toBe('');
    expect(screen.getByTestId('email').textContent).toBe('');
    expect(screen.getByTestId('password').textContent).toBe('');
    expect(screen.getByTestId('email-verified').textContent).toBe('false');
    expect(screen.getByTestId('notifications-enabled').textContent).toBe('false');
    expect(screen.getByTestId('access-to-sso-config').textContent).toBe('false');
    expect(screen.getByTestId('selected-role').textContent).toBe('Chat user');
    expect(screen.getByTestId('selected-teams').textContent).toBe('[]');
    expect(screen.getByTestId('loading').textContent).toBe('false');
    expect(screen.getByTestId('basic-info-valid').textContent).toBe('false');
    expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
  });

  test('fetches user data when ID is provided', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent id="123" mockShowSnackbar={mockShowSnackbar} />);
    
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(mockGetUser).toHaveBeenCalledWith('123');
    expect(screen.getByTestId('name').textContent).toBe('Test User');
    expect(screen.getByTestId('email').textContent).toBe('test@example.com');
    expect(screen.getByTestId('selected-role').textContent).toBe('Developer');
    expect(screen.getByTestId('email-verified').textContent).toBe('true');
    expect(screen.getByTestId('notifications-enabled').textContent).toBe('true');
    expect(screen.getByTestId('access-to-sso-config').textContent).toBe('false');
    expect(JSON.parse(screen.getByTestId('selected-teams').textContent)).toEqual([1, 2]);
  });

  test('handles fetch user error', async () => {
    const mockShowSnackbar = jest.fn();
    const error = new Error('Failed to fetch user');
    mockGetUser.mockRejectedValue(error);
    
    renderWithTheme(<TestComponent id="123" mockShowSnackbar={mockShowSnackbar} />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(mockShowSnackbar).toHaveBeenCalledWith('Failed to fetch user', 'error');
  });

  test('updates state values correctly', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent mockShowSnackbar={mockShowSnackbar} />);
    
    const nameInput = screen.getByTestId('name-input');
    const emailInput = screen.getByTestId('email-input');
    const passwordInput = screen.getByTestId('password-input');
    const roleSelect = screen.getByTestId('role-select');
    
    fireEvent.change(nameInput, { target: { value: 'New Name' } });
    expect(screen.getByTestId('name').textContent).toBe('New Name');
    
    fireEvent.change(emailInput, { target: { value: 'new@example.com' } });
    expect(screen.getByTestId('email').textContent).toBe('new@example.com');
    
    fireEvent.change(passwordInput, { target: { value: 'newpassword' } });
    expect(screen.getByTestId('password').textContent).toBe('newpassword');
    
    fireEvent.change(roleSelect, { target: { value: 'Admin' } });
    expect(screen.getByTestId('selected-role').textContent).toBe('Admin');
  });

  test('toggles boolean states correctly', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent mockShowSnackbar={mockShowSnackbar} />);
    
    const toggleEmailVerified = screen.getByTestId('toggle-email-verified');
    const toggleNotifications = screen.getByTestId('toggle-notifications');
    const toggleSSOAccess = screen.getByTestId('toggle-sso-access');
    
    fireEvent.click(toggleEmailVerified);
    expect(screen.getByTestId('email-verified').textContent).toBe('true');
    
    fireEvent.click(toggleNotifications);
    expect(screen.getByTestId('notifications-enabled').textContent).toBe('true');
    
    fireEvent.click(toggleSSOAccess);
    expect(screen.getByTestId('access-to-sso-config').textContent).toBe('true');
  });

  test('sets teams correctly', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent mockShowSnackbar={mockShowSnackbar} />);
    
    const setTeamsButton = screen.getByTestId('set-teams');
    
    fireEvent.click(setTeamsButton);
    
    expect(JSON.parse(screen.getByTestId('selected-teams').textContent)).toEqual([1, 2, 3]);
  });

  test('sets basic info validity correctly', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent mockShowSnackbar={mockShowSnackbar} />);
    
    const setBasicInfoValidButton = screen.getByTestId('set-basic-info-valid');
    
    fireEvent.click(setBasicInfoValidButton);
    
    expect(screen.getByTestId('basic-info-valid').textContent).toBe('true');
  });

  test('handles form submission for creating a new user', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent mockShowSnackbar={mockShowSnackbar} />);
    
    const nameInput = screen.getByTestId('name-input');
    const emailInput = screen.getByTestId('email-input');
    const passwordInput = screen.getByTestId('password-input');
    const setBasicInfoValidButton = screen.getByTestId('set-basic-info-valid');
    const submitButton = screen.getByTestId('submit-form');
    
    fireEvent.change(nameInput, { target: { value: 'Test User' } });
    fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });
    fireEvent.click(setBasicInfoValidButton);
    
    fireEvent.click(submitButton);
    
    await waitFor(() => {
      expect(mockCreateUser).toHaveBeenCalledWith({
        name: 'Test User',
        email: 'test@example.com',
        password: 'password123',
        isAdmin: false,
        showPortal: false,
        showChat: true,
        emailVerified: false,
        notificationsEnabled: false,
        accessToSSOConfig: false,
        groups: []
      });
    });
    
    expect(mockShowSnackbar).toHaveBeenCalledWith('User created successfully', 'success');
    
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/admin/users');
    }, { timeout: 3000 });
  });

  test('handles form submission for updating an existing user', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent id="123" mockShowSnackbar={mockShowSnackbar} />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    const setBasicInfoValidButton = screen.getByTestId('set-basic-info-valid');
    const submitButton = screen.getByTestId('submit-form');
    
    fireEvent.click(setBasicInfoValidButton);
    
    fireEvent.click(submitButton);
    
    await waitFor(() => {
      expect(mockUpdateUser).toHaveBeenCalledWith('123', {
        name: 'Test User',
        email: 'test@example.com',
        isAdmin: false,
        showPortal: true,
        showChat: true,
        emailVerified: true,
        notificationsEnabled: false,
        accessToSSOConfig: false,
        groups: [1, 2]
      });
    });
    
    expect(mockShowSnackbar).toHaveBeenCalledWith('User updated successfully', 'success');
    
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/admin/users');
    }, { timeout: 3000 });
  });

  test('handles form submission with Admin role correctly', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent mockShowSnackbar={mockShowSnackbar} />);
    
    const roleSelect = screen.getByTestId('role-select');
    const toggleNotifications = screen.getByTestId('toggle-notifications');
    const toggleSSOAccess = screen.getByTestId('toggle-sso-access');
    const setBasicInfoValidButton = screen.getByTestId('set-basic-info-valid');
    const submitButton = screen.getByTestId('submit-form');
    
    fireEvent.change(roleSelect, { target: { value: 'Admin' } });
    fireEvent.click(toggleNotifications);
    fireEvent.click(toggleSSOAccess);
    fireEvent.click(setBasicInfoValidButton);
    
    fireEvent.click(submitButton);
    
    await waitFor(() => {
      expect(mockCreateUser).toHaveBeenCalledWith(
        expect.objectContaining({
          isAdmin: true,
          showPortal: true,
          notificationsEnabled: true,
          accessToSSOConfig: true
        })
      );
    });
  });

  test('handles form submission error', async () => {
    const mockShowSnackbar = jest.fn();
    const error = new Error('Failed to create user');
    mockCreateUser.mockRejectedValue(error);
    
    renderWithTheme(<TestComponent mockShowSnackbar={mockShowSnackbar} />);
    
    const setBasicInfoValidButton = screen.getByTestId('set-basic-info-valid');
    const submitButton = screen.getByTestId('submit-form');
    
    fireEvent.click(setBasicInfoValidButton);
    
    fireEvent.click(submitButton);
    
    await waitFor(() => {
      expect(mockShowSnackbar).toHaveBeenCalledWith('Failed to create user', 'error');
    });
  });

  test('does not submit form when basic info is invalid', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent mockShowSnackbar={mockShowSnackbar} />);
    
    const submitButton = screen.getByTestId('submit-form');
    
    fireEvent.click(submitButton);
    
    expect(mockCreateUser).not.toHaveBeenCalled();
    expect(mockUpdateUser).not.toHaveBeenCalled();
  });

  test('handles delete click correctly', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent mockShowSnackbar={mockShowSnackbar} />);
    
    const deleteButton = screen.getByTestId('delete-click');
    
    fireEvent.click(deleteButton);
    
    expect(screen.getByTestId('warning-dialog-open').textContent).toBe('true');
  });

  test('handles cancel delete correctly', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent mockShowSnackbar={mockShowSnackbar} />);
    
    const deleteButton = screen.getByTestId('delete-click');
    const cancelButton = screen.getByTestId('cancel-delete');
    
    fireEvent.click(deleteButton);
    expect(screen.getByTestId('warning-dialog-open').textContent).toBe('true');
    
    fireEvent.click(cancelButton);
    expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
  });

  test('handles confirm delete correctly', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent id="123" mockShowSnackbar={mockShowSnackbar} />);
    
    const confirmDeleteButton = screen.getByTestId('confirm-delete');
    confirmDeleteButton.click();
    
    await waitFor(() => {
      expect(mockDeleteUser).toHaveBeenCalledWith('123');
    });
    
    expect(mockShowSnackbar).toHaveBeenCalledWith('User deleted successfully', 'success');
    expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
    
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/admin/users');
    }, { timeout: 3000 });
  });

  test('handles delete error correctly', async () => {
    const mockShowSnackbar = jest.fn();
    const error = new Error('Failed to delete user');
    mockDeleteUser.mockRejectedValue(error);
    
    renderWithTheme(<TestComponent id="123" mockShowSnackbar={mockShowSnackbar} />);
    
    const confirmDeleteButton = screen.getByTestId('confirm-delete');
    confirmDeleteButton.click();
    
    await waitFor(() => {
      expect(mockShowSnackbar).toHaveBeenCalledWith('Failed to delete user', 'error');
    });
    
    expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
  });

  test('fetches user data again when id changes', async () => {
    const mockShowSnackbar = jest.fn();
    const { rerender } = renderWithTheme(<TestComponent id="123" mockShowSnackbar={mockShowSnackbar} />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(mockGetUser).toHaveBeenCalledWith('123');
    
    const newMockUserResponse = {
      data: {
        attributes: {
          name: 'Different User',
          email: 'different@example.com',
          role: 'Admin',
          groups: [],
          email_verified: false,
          notifications_enabled: false,
          access_to_sso_config: true
        }
      }
    };
    
    mockGetUser.mockResolvedValue(newMockUserResponse);
    
    rerender(<TestComponent id="456" mockShowSnackbar={mockShowSnackbar} />);
    
    await waitFor(() => {
      expect(mockGetUser).toHaveBeenCalledWith('456');
    });
    
    await waitFor(() => {
      expect(screen.getByTestId('name').textContent).toBe('Different User');
    });
  });

  test('handles user with no groups correctly', async () => {
    const mockShowSnackbar = jest.fn();
    const userWithoutGroups = {
      data: {
        attributes: {
          name: 'User Without Groups',
          email: 'nogroups@example.com',
          role: 'Chat user',
          email_verified: false,
          notifications_enabled: false,
          access_to_sso_config: false
        }
      }
    };
    
    mockGetUser.mockResolvedValue(userWithoutGroups);
    
    renderWithTheme(<TestComponent id="123" mockShowSnackbar={mockShowSnackbar} />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(screen.getByTestId('selected-teams').textContent).toBe('[]');
  });
}); 