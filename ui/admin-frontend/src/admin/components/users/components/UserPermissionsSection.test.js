import React from 'react';
import { screen, fireEvent } from '@testing-library/react';
import { renderWithTheme } from '../../../../test-utils/render-with-theme';
import UserPermissionsSection from './UserPermissionsSection';

jest.mock('@mui/material', () => require('../../../../test-utils/mui-mocks').muiMaterialMock);

jest.mock('../../../styles/sharedStyles', () => require('../../../../test-utils/styled-component-mocks').sharedStylesMock);

jest.mock('../../common/CollapsibleSection', () => require('../../../../test-utils/component-mocks').collapsibleSectionMock);

jest.mock('./RoleRadioGroup', () => {
  const React = require('react');
  return {
    __esModule: true,
    default: ({ value, onChange, isSuperAdmin }) =>
      React.createElement('div', {
        'data-testid': 'role-radio-group',
        'data-value': value,
        'data-is-super-admin': isSuperAdmin?.toString(),
        onClick: () => onChange && onChange('Developer')
      })
  };
});

jest.mock('./RolePermissionsDisplay', () => {
  const React = require('react');
  return {
    __esModule: true,
    default: ({ selectedRole, isSuperAdmin, width }) =>
      React.createElement('div', {
        'data-testid': 'role-permissions-display',
        'data-selected-role': selectedRole,
        'data-is-super-admin': isSuperAdmin?.toString(),
        'data-width': width
      })
  };
});

jest.mock('../../../utils/docsLinkUtils', () => ({
  createDocsLinkHandler: jest.fn(() => jest.fn())
}));

jest.mock('../../../hooks/useConfig', () => ({
  __esModule: true,
  default: () => ({
    getDocsLink: jest.fn()
  })
}));

describe('UserPermissionsSection', () => {
  const defaultProps = {
    isSuperAdmin: false,
    notificationsEnabled: false,
    setNotificationsEnabled: jest.fn(),
    accessToSSOConfig: false,
    setAccessToSSOConfig: jest.fn(),
    selectedRole: '',
    setSelectedRole: jest.fn()
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders collapsible section with correct title', () => {
    renderWithTheme(<UserPermissionsSection {...defaultProps} />);
    
    const collapsibleSection = screen.getByTestId('collapsible-section');
    expect(collapsibleSection).toBeInTheDocument();
    expect(collapsibleSection).toHaveAttribute('data-title', 'Roles & permissions*');
    expect(collapsibleSection).toHaveAttribute('data-default-expanded', 'true');
  });

  it('renders description text with learn more link', () => {
    renderWithTheme(<UserPermissionsSection {...defaultProps} />);
    
    expect(screen.getByText(/Assign a role to this user to control their access levels/)).toBeInTheDocument();
    expect(screen.getByText(/to features and actions in the AI studio platform/)).toBeInTheDocument();
  });

  it('renders role radio group with correct props', () => {
    renderWithTheme(<UserPermissionsSection {...defaultProps} selectedRole="Developer" />);
    
    const roleRadioGroup = screen.getByTestId('role-radio-group');
    expect(roleRadioGroup).toBeInTheDocument();
    expect(roleRadioGroup).toHaveAttribute('data-value', 'Developer');
    expect(roleRadioGroup).toHaveAttribute('data-is-super-admin', 'false');
  });

  it('renders role permissions display with correct props', () => {
    renderWithTheme(<UserPermissionsSection {...defaultProps} selectedRole="Admin" isSuperAdmin={true} />);
    
    const rolePermissionsDisplay = screen.getByTestId('role-permissions-display');
    expect(rolePermissionsDisplay).toBeInTheDocument();
    expect(rolePermissionsDisplay).toHaveAttribute('data-selected-role', 'Admin');
    expect(rolePermissionsDisplay).toHaveAttribute('data-is-super-admin', 'true');
    expect(rolePermissionsDisplay).toHaveAttribute('data-width', '50%');
  });

  it('calls setSelectedRole when role changes', () => {
    const mockSetSelectedRole = jest.fn();
    renderWithTheme(<UserPermissionsSection {...defaultProps} setSelectedRole={mockSetSelectedRole} />);
    
    const roleRadioGroup = screen.getByTestId('role-radio-group');
    fireEvent.click(roleRadioGroup);
    
    expect(mockSetSelectedRole).toHaveBeenCalledWith('Developer');
  });

  it('does not show admin switches when role is not Admin', () => {
    renderWithTheme(<UserPermissionsSection {...defaultProps} selectedRole="Developer" />);
    
    expect(screen.queryByText('Enable Notifications')).not.toBeInTheDocument();
    expect(screen.queryByText('Allow Identity provider configuration')).not.toBeInTheDocument();
  });

  it('shows admin switches when role is Admin', () => {
    renderWithTheme(<UserPermissionsSection {...defaultProps} selectedRole="Admin" />);
    
    expect(screen.getByText('Enable Notifications')).toBeInTheDocument();
    expect(screen.getByText('Allow Identity provider configuration')).toBeInTheDocument();
  });

  it('renders notifications switch with correct state', () => {
    renderWithTheme(<UserPermissionsSection {...defaultProps} selectedRole="Admin" notificationsEnabled={true} />);
    
    expect(screen.getByText('Enable Notifications')).toBeInTheDocument();
    expect(screen.getAllByTestId('styled-switch')).toHaveLength(2);
    expect(screen.getAllByTestId('styled-switch')[0]).toHaveAttribute('checked');
  });

  it('renders SSO config switch with correct state', () => {
    renderWithTheme(<UserPermissionsSection {...defaultProps} selectedRole="Admin" accessToSSOConfig={true} />);
    
    expect(screen.getByText('Allow Identity provider configuration')).toBeInTheDocument();
    expect(screen.getAllByTestId('styled-switch')).toHaveLength(2);
    expect(screen.getAllByTestId('styled-switch')[1]).toHaveAttribute('checked');
  });

  it('calls setNotificationsEnabled when notifications switch is toggled', () => {
    const mockSetNotificationsEnabled = jest.fn();
    renderWithTheme(
      <UserPermissionsSection 
        {...defaultProps} 
        selectedRole="Admin" 
        setNotificationsEnabled={mockSetNotificationsEnabled}
      />
    );
    
    const switchElements = screen.getAllByTestId('styled-switch');
    const notificationsSwitch = switchElements[0];
    
    fireEvent.click(notificationsSwitch);
    expect(mockSetNotificationsEnabled).toHaveBeenCalledWith(true);
  });

  it('calls setAccessToSSOConfig when SSO config switch is toggled', () => {
    const mockSetAccessToSSOConfig = jest.fn();
    renderWithTheme(
      <UserPermissionsSection 
        {...defaultProps} 
        selectedRole="Admin" 
        setAccessToSSOConfig={mockSetAccessToSSOConfig}
      />
    );
    
    const switchElements = screen.getAllByTestId('styled-switch');
    const ssoConfigSwitch = switchElements[1];
    
    fireEvent.click(ssoConfigSwitch);
    expect(mockSetAccessToSSOConfig).toHaveBeenCalledWith(true);
  });

  it('passes isSuperAdmin prop correctly to child components', () => {
    renderWithTheme(<UserPermissionsSection {...defaultProps} isSuperAdmin={true} />);
    
    const roleRadioGroup = screen.getByTestId('role-radio-group');
    const rolePermissionsDisplay = screen.getByTestId('role-permissions-display');
    
    expect(roleRadioGroup).toHaveAttribute('data-is-super-admin', 'true');
    expect(rolePermissionsDisplay).toHaveAttribute('data-is-super-admin', 'true');
  });

  it('renders with correct layout structure', () => {
    renderWithTheme(<UserPermissionsSection {...defaultProps} />);
    
    const boxes = screen.getAllByTestId('box');
    expect(boxes.length).toBeGreaterThan(0);
  });

  it('has correct display name', () => {
    expect(UserPermissionsSection.displayName).toBe('UserPermissionsSection');
  });
}); 