import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import DashboardIcon from '@mui/icons-material/Dashboard';
import GroupIcon from '@mui/icons-material/Group';
import PersonIcon from '@mui/icons-material/Person';
import BaseDrawer from './BaseDrawer';
import adminTheme from '../../theme';

// Mock MUI components
jest.mock('@mui/material', () => ({
  ...jest.requireActual('@mui/material'),
  Drawer: ({ children, sx, variant, anchor, open, ...props }) => {
    // Filter out MUI-specific props and merge styles
    const { PaperProps, ModalProps, SlideProps, ...filteredProps } = props;
    const style = {
      ...(sx?.width && { width: sx.width }),
      ...(PaperProps?.style || {}),
    };
    return (
      <div role="presentation" style={style} {...filteredProps}>
        {children}
      </div>
    );
  },
  Toolbar: () => <div role="toolbar" />,
  List: ({ children, sx, component, disablePadding, ...props }) => {
    // Transform MUI's mt shorthand to marginTop using theme spacing
    const style = {
      ...sx,
      marginTop: sx?.mt ? testTheme.spacing(sx.mt) : undefined,
    };
    // Filter out MUI-specific props
    const { component: comp, disablePadding: dp, ...filteredProps } = props;
    return <ul style={style} {...filteredProps}>{children}</ul>;
  },
  ListItem: ({ children, button, component: Component = 'li', ...props }) => {
    const Comp = Component || 'li';
    // Filter out MUI-specific props
    const { end, button: buttonProp, ...filteredProps } = props;
    return <Comp {...filteredProps}>{children}</Comp>;
  },
  ListItemIcon: ({ children, ...props }) => {
    // Filter out MUI-specific props
    const { sx, className, ...filteredProps } = props;
    return <span {...filteredProps}>{children}</span>;
  },
  ListItemText: ({ primary, primaryTypographyProps, ...props }) => {
    // Filter out MUI-specific props
    const { sx, inset, className, ...filteredProps } = props;
    return <span {...filteredProps}>{primary}</span>;
  },
  Collapse: ({ children, in: isIn, timeout, unmountOnExit, ...props }) => {
    // Filter out MUI-specific props
    const { orientation, collapsedSize, sx, className, ...filteredProps } = props;
    return isIn ? <div {...filteredProps}>{children}</div> : null;
  },
  IconButton: ({ children, onClick, sx, ...props }) => {
    // Filter out MUI-specific props
    const { edge, size, color, ...filteredProps } = props;
    return <button onClick={onClick} {...filteredProps}>{children}</button>;
  },
  Divider: () => <hr />,
}));

// Mock MUI icons
jest.mock('@mui/icons-material/ChevronLeft', () => () => 'ChevronLeft');
jest.mock('@mui/icons-material/ChevronRight', () => () => 'ChevronRight');
jest.mock('@mui/icons-material/ExpandLess', () => () => 'ExpandLess');
jest.mock('@mui/icons-material/ExpandMore', () => () => 'ExpandMore');
jest.mock('@mui/icons-material/Dashboard', () => () => 'DashboardIcon');
jest.mock('@mui/icons-material/Group', () => () => 'GroupIcon');
jest.mock('@mui/icons-material/Person', () => () => 'PersonIcon');

// Mock StyledNavLink component
jest.mock('../../styles/sharedStyles', () => ({
  StyledNavLink: ({ children, to, component, ...props }) => (
    <a href={to} {...props}>{children}</a>
  ),
}));

// Create test theme
const testTheme = {
  palette: {
    text: {
      primary: '#000000',
      secondary: '#666666',
    },
    background: {
      default: '#ffffff',
    },
  },
  spacing: (value) => value * 8,
};

// Mock theme
jest.mock('../../theme', () => testTheme);

// Wrap component with required providers
const renderWithProviders = (component) => {
  return render(
    <MemoryRouter>
      <ThemeProvider theme={testTheme}>
        {component}
      </ThemeProvider>
    </MemoryRouter>
  );
};

describe('BaseDrawer', () => {
  // Sample menu items for testing
  const testMenuItems = [
    {
      id: 'dashboard',
      text: 'Dashboard',
      icon: <DashboardIcon data-testid="dashboard-icon" />,
      path: '/admin/dashboard',
    },
    {
      id: 'team',
      text: 'Team',
      icon: <GroupIcon data-testid="team-icon" />,
      subItems: [
        {
          id: 'users',
          text: 'Users',
          icon: <PersonIcon data-testid="users-icon" />,
          path: '/admin/users',
        },
      ],
    },
  ];

  it('renders with default props', () => {
    renderWithProviders(<BaseDrawer menuItems={testMenuItems} />);
    
    // Check if drawer is rendered
    expect(screen.getByRole('presentation')).toBeInTheDocument();
    
    // Check if toolbar is shown by default
    expect(screen.getByRole('toolbar')).toBeInTheDocument();
    
    // Check if menu items are rendered
    expect(screen.getByText('Dashboard')).toBeInTheDocument();
    expect(screen.getByText('Team')).toBeInTheDocument();
  });

  it('handles drawer toggle correctly', () => {
    renderWithProviders(<BaseDrawer menuItems={testMenuItems} />);
    
    // Get the toggle button and drawer
    const toggleButton = screen.getByRole('button');
    const drawer = screen.getByRole('presentation');
    
    // Initially drawer should be at full width
    expect(drawer).toHaveStyle({ width: '240px' });
    expect(screen.getByText('Dashboard')).toBeInTheDocument();
    
    // Click toggle button
    fireEvent.click(toggleButton);
    
    // Drawer should be minimized
    expect(drawer).toHaveStyle({ width: '60px' });
  });

  it('expands and collapses menu items with subItems', () => {
    renderWithProviders(<BaseDrawer menuItems={testMenuItems} />);
    
    // Initially subitem should not be present
    expect(screen.queryByText('Users')).not.toBeInTheDocument();
    
    // Click on team menu item
    fireEvent.click(screen.getByText('Team'));
    
    // Subitem should now be present
    expect(screen.getByText('Users')).toBeInTheDocument();
    
    // Click again to collapse
    fireEvent.click(screen.getByText('Team'));
    
    // Subitem should be removed
    expect(screen.queryByText('Users')).not.toBeInTheDocument();
  });

  it('respects defaultExpandedItems prop', () => {
    renderWithProviders(
      <BaseDrawer 
        menuItems={testMenuItems} 
        defaultExpandedItems={{ team: true }}
      />
    );
    
    // Subitem should be present initially
    expect(screen.getByText('Users')).toBeInTheDocument();
  });

  it('applies custom styles correctly', () => {
    const customStyles = { marginTop: 8 }; // 8 units = 64px with theme spacing
    renderWithProviders(
      <BaseDrawer 
        menuItems={testMenuItems} 
        customStyles={customStyles}
      />
    );
    
    // Check if custom margin is applied to the List component
    const list = screen.getByRole('list');
    expect(list).toHaveStyle({ marginTop: '64px' });
  });

  it('handles nested menu items correctly', () => {
    const nestedMenuItems = [
      {
        id: 'parent',
        text: 'Parent',
        subItems: [
          {
            id: 'child',
            text: 'Child',
            subItems: [
              {
                id: 'grandchild',
                text: 'Grandchild',
                path: '/grandchild',
              },
            ],
          },
        ],
      },
    ];

    renderWithProviders(<BaseDrawer menuItems={nestedMenuItems} />);
    
    // Initially only parent should be present
    expect(screen.getByText('Parent')).toBeInTheDocument();
    expect(screen.queryByText('Child')).not.toBeInTheDocument();
    expect(screen.queryByText('Grandchild')).not.toBeInTheDocument();
    
    // Click parent
    fireEvent.click(screen.getByText('Parent'));
    expect(screen.getByText('Child')).toBeInTheDocument();
    
    // Click child
    fireEvent.click(screen.getByText('Child'));
    expect(screen.getByText('Grandchild')).toBeInTheDocument();
  });

  it('renders navigation links correctly', () => {
    renderWithProviders(<BaseDrawer menuItems={testMenuItems} />);
    
    // Check if dashboard link is rendered correctly
    const dashboardLink = screen.getByRole('link', { name: /dashboard/i });
    expect(dashboardLink).toHaveAttribute('href', '/admin/dashboard');
  });

  it('hides toolbar when showToolbar is false', () => {
    renderWithProviders(
      <BaseDrawer 
        menuItems={testMenuItems} 
        showToolbar={false}
      />
    );
    
    // Toolbar should not be present
    expect(screen.queryByRole('toolbar')).not.toBeInTheDocument();
  });

  it('respects minimizedWidth when drawer is closed', () => {
    const customMinimizedWidth = 80;
    renderWithProviders(
      <BaseDrawer 
        menuItems={testMenuItems} 
        minimizedWidth={customMinimizedWidth}
      />
    );
    
    // Click toggle button to close drawer
    fireEvent.click(screen.getByRole('button', { name: /chevronleft/i }));
    
    // Check if drawer width is updated
    const drawer = screen.getByRole('presentation');
    expect(drawer).toHaveStyle({ width: `${customMinimizedWidth}px` });
  });

  it('maintains parent expanded state when expanding nested items', () => {
    const nestedMenuItems = [
      {
        id: 'parent',
        text: 'Parent',
        subItems: [
          {
            id: 'child',
            text: 'Child',
            subItems: [
              {
                id: 'grandchild',
                text: 'Grandchild',
                path: '/grandchild',
              },
            ],
          },
        ],
      },
    ];

    renderWithProviders(<BaseDrawer menuItems={nestedMenuItems} />);
    
    // Expand parent
    fireEvent.click(screen.getByText('Parent'));
    expect(screen.getByText('Child')).toBeInTheDocument();
    
    // Expand child
    fireEvent.click(screen.getByText('Child'));
    expect(screen.getByText('Grandchild')).toBeInTheDocument();
    
    // Parent should still be expanded
    expect(screen.getByText('Child')).toBeInTheDocument();
  });
});
