import React from 'react';
import { screen, fireEvent } from '@testing-library/react';
import { renderWithTheme } from '../../../../test-utils/render-with-theme';
import RoleRadioGroup from './RoleRadioGroup';

jest.mock('@mui/material', () => require('../../../../test-utils/mui-mocks').muiMaterialMock);

jest.mock('../../../styles/sharedStyles', () => require('../../../../test-utils/styled-component-mocks').sharedStylesMock);

jest.mock('../styles', () => require('../../../../test-utils/styled-component-mocks').userStylesMock);

jest.mock('../../groups/utils/roleBadgeConfig', () => ({
  roleBadgeConfigs: require('../../../../test-utils/service-mocks').mockRoleBadgeConfigs
}));

jest.mock('../utils/userRolesConfig', () => ({
  USER_ROLES: require('../../../../test-utils/service-mocks').mockUserRoles
}));

describe('RoleRadioGroup', () => {
  const defaultProps = {
    value: '',
    onChange: jest.fn(),
    isSuperAdmin: false
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders form control and radio group', () => {
    renderWithTheme(<RoleRadioGroup {...defaultProps} />);
    
    expect(screen.getByTestId('form-control')).toBeInTheDocument();
    expect(screen.getByTestId('radio-group')).toBeInTheDocument();
  });

  it('renders Chat user and Developer roles when not super admin', () => {
    renderWithTheme(<RoleRadioGroup {...defaultProps} />);
    
    const roleOptions = screen.getAllByTestId('role-option-box');
    expect(roleOptions).toHaveLength(2);
    expect(roleOptions[0]).toHaveAttribute('data-value', 'Chat user');
    expect(roleOptions[1]).toHaveAttribute('data-value', 'Developer');
  });

  it('renders all three roles when super admin', () => {
    renderWithTheme(<RoleRadioGroup {...defaultProps} isSuperAdmin={true} />);
    
    const roleOptions = screen.getAllByTestId('role-option-box');
    expect(roleOptions).toHaveLength(3);
    expect(roleOptions[0]).toHaveAttribute('data-value', 'Chat user');
    expect(roleOptions[1]).toHaveAttribute('data-value', 'Developer');
    expect(roleOptions[2]).toHaveAttribute('data-value', 'Admin');
  });

  it('renders role badges with correct configurations', () => {
    renderWithTheme(<RoleRadioGroup {...defaultProps} />);
    
    const roleBadges = screen.getAllByTestId('role-badge');
    expect(roleBadges[0]).toHaveAttribute('data-bg-color', 'background.buttonPrimaryOutlineHover');
    expect(roleBadges[1]).toHaveAttribute('data-bg-color', 'background.surfaceBrandDefaultPortal');
  });

  it('displays role text and descriptions correctly', () => {
    renderWithTheme(<RoleRadioGroup {...defaultProps} />);
    
    expect(screen.getByText('Chat user')).toBeInTheDocument();
    expect(screen.getAllByText('can access')).toHaveLength(2);
    expect(screen.getByText('Chats')).toBeInTheDocument();
    
    expect(screen.getByText('Developer')).toBeInTheDocument();
    expect(screen.getByText('AI portal and Chats')).toBeInTheDocument();
  });

  it('calls onChange when role is selected', () => {
    const mockOnChange = jest.fn();
    renderWithTheme(<RoleRadioGroup {...defaultProps} onChange={mockOnChange} />);
    
    const radioInputs = screen.getAllByTestId('styled-radio');
    fireEvent.click(radioInputs[1]);
    
    expect(mockOnChange).toHaveBeenCalled();
  });

  it('shows selected value correctly', () => {
    renderWithTheme(<RoleRadioGroup {...defaultProps} value="Developer" />);
    
    const radioGroup = screen.getByTestId('radio-group');
    expect(radioGroup).toHaveAttribute('data-value', 'Developer');
  });

  it('marks last role option correctly when not super admin', () => {
    renderWithTheme(<RoleRadioGroup {...defaultProps} />);
    
    const roleOptions = screen.getAllByTestId('role-option-box');
    expect(roleOptions[0]).toHaveAttribute('data-is-last', 'false');
    expect(roleOptions[1]).toHaveAttribute('data-is-last', 'true');
  });

  it('marks last role option correctly when super admin', () => {
    renderWithTheme(<RoleRadioGroup {...defaultProps} isSuperAdmin={true} />);
    
    const roleOptions = screen.getAllByTestId('role-option-box');
    expect(roleOptions[0]).toHaveAttribute('data-is-last', 'false');
    expect(roleOptions[1]).toHaveAttribute('data-is-last', 'false');
    expect(roleOptions[2]).toHaveAttribute('data-is-last', 'true');
  });

  it('renders styled radio inputs', () => {
    renderWithTheme(<RoleRadioGroup {...defaultProps} />);
    
    const radioInputs = screen.getAllByTestId('styled-radio');
    expect(radioInputs).toHaveLength(2);
  });

  it('handles Admin role selection when super admin', () => {
    const mockOnChange = jest.fn();
    renderWithTheme(<RoleRadioGroup {...defaultProps} isSuperAdmin={true} onChange={mockOnChange} />);
    
    expect(screen.getByText('Admin')).toBeInTheDocument();
    expect(screen.getByText('Admin, AI portal and Chats')).toBeInTheDocument();
    
    const radioInputs = screen.getAllByTestId('styled-radio');
    fireEvent.click(radioInputs[2]);
    
    expect(mockOnChange).toHaveBeenCalled();
  });
}); 