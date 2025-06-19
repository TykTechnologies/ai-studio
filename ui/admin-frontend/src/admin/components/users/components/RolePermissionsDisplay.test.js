import React from 'react';
import { screen } from '@testing-library/react';
import { renderWithTheme } from '../../../../test-utils/render-with-theme';
import RolePermissionsDisplay from './RolePermissionsDisplay';

jest.mock('../styles', () => ({
  PermissionsTooltipBox: ({ children, ...props }) =>
    <div data-testid="permissions-tooltip-box" {...props}>{children}</div>,
  StyledPermissionIcon: ({ name, ...props }) =>
    <div data-testid="styled-permission-icon" data-icon-name={name} {...props} />
}));

jest.mock('../utils/userRolesConfig', () => ({
  USER_ROLES: [
    {
      value: 'Chat user',
      label: 'Chat user',
      permissions: {
        sections: [
          {
            title: 'Access to Chats',
            items: [
              'Interact with Chats',
              'Add data sources and tools available in their catalogs to chats'
            ]
          }
        ]
      }
    },
    {
      value: 'Developer',
      label: 'Developer',
      permissions: {
        sections: [
          {
            title: 'Access to Chats',
            items: [
              'Interact with Chats',
              'Add data sources and tools available in their catalogs to chats'
            ]
          },
          {
            title: 'Access to AI Portal',
            items: [
              'Use Apps created by the admin',
              'Create and delete their own apps with LLM providers and data sources available in their catalogs'
            ]
          }
        ]
      }
    },
    {
      value: 'Admin',
      label: 'Admin',
      permissions: {
        sections: [
          {
            title: 'Access to Chats',
            items: [
              'Interact with Chats',
              'Add data sources and tools available in their catalogs to chats'
            ]
          },
          {
            title: 'Access to AI Portal',
            items: [
              'Use Apps created by the admin',
              'Create and delete their own apps with LLM providers and data sources available in their catalogs'
            ]
          },
          {
            title: 'Access to Administration',
            items: [
              'CRUD LLM providers, data sources, tools, filters, middleware, Apps, Chats and catalogs',
              'Add, edit and delete Chat users and Developers.',
              'Add, edit and delete Teams.',
              'Monitor usage, iterations, and costs (set up budgets).'
            ]
          }
        ]
      }
    }
  ]
}));

describe('RolePermissionsDisplay', () => {
  const renderComponent = (props = {}) => {
    const defaultProps = {
      selectedRole: null,
      isSuperAdmin: false,
      ...props
    };
    return renderWithTheme(<RolePermissionsDisplay {...defaultProps} />);
  };

  it('returns null when no selectedRole is provided', () => {
    renderComponent();
    expect(screen.queryByTestId('permissions-tooltip-box')).not.toBeInTheDocument();
  });

  it('returns null when selectedRole does not match any role', () => {
    renderComponent({ selectedRole: 'Unknown Role' });
    expect(screen.queryByTestId('permissions-tooltip-box')).not.toBeInTheDocument();
  });

  it('displays permissions for Chat user role', () => {
    renderComponent({ selectedRole: 'Chat user' });
    
    expect(screen.getByTestId('permissions-tooltip-box')).toBeInTheDocument();
    expect(screen.getByText('Access to Chats')).toBeInTheDocument();
    expect(screen.getByText('Interact with Chats')).toBeInTheDocument();
    expect(screen.getByText('Add data sources and tools available in their catalogs to chats')).toBeInTheDocument();
  });

  it('displays permissions for Developer role', () => {
    renderComponent({ selectedRole: 'Developer' });
    
    expect(screen.getByTestId('permissions-tooltip-box')).toBeInTheDocument();
    expect(screen.getByText('Access to Chats')).toBeInTheDocument();
    expect(screen.getByText('Access to AI Portal')).toBeInTheDocument();
    expect(screen.getByText('Use Apps created by the admin')).toBeInTheDocument();
    expect(screen.getByText('Create and delete their own apps with LLM providers and data sources available in their catalogs')).toBeInTheDocument();
  });

  it('displays permissions for Admin role when isSuperAdmin is false', () => {
    renderComponent({ selectedRole: 'Admin', isSuperAdmin: false });
    
    expect(screen.queryByTestId('permissions-tooltip-box')).not.toBeInTheDocument();
  });

  it('displays permissions for Admin role when isSuperAdmin is true', () => {
    renderComponent({ selectedRole: 'Admin', isSuperAdmin: true });
    
    expect(screen.getByTestId('permissions-tooltip-box')).toBeInTheDocument();
    expect(screen.getByText('Access to Chats')).toBeInTheDocument();
    expect(screen.getByText('Access to AI Portal')).toBeInTheDocument();
    expect(screen.getByText('Access to Administration')).toBeInTheDocument();
    expect(screen.getByText('CRUD LLM providers, data sources, tools, filters, middleware, Apps, Chats and catalogs')).toBeInTheDocument();
    expect(screen.getByText('Add, edit and delete Chat users and Developers.')).toBeInTheDocument();
    expect(screen.getByText('Add, edit and delete Teams.')).toBeInTheDocument();
    expect(screen.getByText('Monitor usage, iterations, and costs (set up budgets).')).toBeInTheDocument();
  });

  it('renders styled permission icons for each section', () => {
    renderComponent({ selectedRole: 'Developer' });
    
    const icons = screen.getAllByTestId('styled-permission-icon');
    expect(icons).toHaveLength(2);
    icons.forEach(icon => {
      expect(icon).toHaveAttribute('data-icon-name', 'circle-check');
    });
  });

  it('renders correct structure with sections and items', () => {
    renderComponent({ selectedRole: 'Chat user' });
    
    const section = screen.getByText('Access to Chats');
    expect(section).toBeInTheDocument();
    
    const items = screen.getAllByRole('listitem');
    expect(items).toHaveLength(2);
  });

  it('properly handles multiple sections for complex roles', () => {
    renderComponent({ selectedRole: 'Developer' });
    
    expect(screen.getByText('Access to Chats')).toBeInTheDocument();
    expect(screen.getByText('Access to AI Portal')).toBeInTheDocument();
    
    const sections = screen.getAllByTestId('styled-permission-icon');
    expect(sections).toHaveLength(2);
  });

  it('maintains correct permissions tooltip box width', () => {
    renderComponent({ selectedRole: 'Chat user' });
    
    const tooltipBox = screen.getByTestId('permissions-tooltip-box');
    expect(tooltipBox).toHaveAttribute('width', '50%');
  });
}); 