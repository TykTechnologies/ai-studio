import React from 'react';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import DashboardIcon from '@mui/icons-material/Dashboard';
import GroupIcon from '@mui/icons-material/Group';
import PersonIcon from '@mui/icons-material/Person';
import BaseDrawer from './BaseDrawer';
import adminTheme from '../../../theme';

// Mock localStorage
const createMockStorage = () => {
  let store = {};
  return {
    getItem: jest.fn(key => store[key]),
    setItem: jest.fn((key, value) => {
      try {
        JSON.parse(value); // Validate JSON
        store[key] = value;
      } catch (e) {
        throw new Error('Invalid JSON');
      }
    }),
    clear: jest.fn(() => {
      store = {};
    }),
    removeItem: jest.fn(key => {
      delete store[key];
    }),
    getStore: () => ({ ...store }),
    length: 0,
    key: jest.fn((index) => Object.keys(store)[index] || null)
  };
};

let mockStorage;

beforeEach(() => {
  mockStorage = createMockStorage();
  Object.defineProperty(window, 'localStorage', {
    value: mockStorage,
    configurable: true
  });
  jest.clearAllMocks();
});

// Test helpers
const waitForTransition = async () => {
  await act(async () => {
    await Promise.resolve();
    await new Promise(resolve => setTimeout(resolve, 300));
  });
};

const waitForStateUpdates = async () => {
  await act(async () => {
    await Promise.resolve();
    await new Promise(resolve => setTimeout(resolve, 300));
    await Promise.resolve();
  });
};

const renderWithProviders = (component) => {
  return render(
    <MemoryRouter>
      <ThemeProvider theme={adminTheme}>
        {component}
      </ThemeProvider>
    </MemoryRouter>
  );
};

const getToggleButton = () => {
  // Try to find either ChevronLeft or ChevronRight icon's button
  const leftIcon = screen.queryByTestId('ChevronLeftIcon');
  const rightIcon = screen.queryByTestId('ChevronRightIcon');
  const icon = leftIcon || rightIcon;
  return icon?.closest('button');
};

describe('BaseDrawer', () => {
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

  beforeEach(() => {
    mockStorage.clear();
  });

  it('renders with default props', () => {
    renderWithProviders(<BaseDrawer menuItems={testMenuItems} />);
    const drawer = document.querySelector('.MuiDrawer-root');
    expect(drawer).toBeInTheDocument();
    expect(screen.getByText('Dashboard')).toBeInTheDocument();
    expect(screen.getByText('Team')).toBeInTheDocument();
  });

  it('handles drawer toggle correctly', () => {
    renderWithProviders(<BaseDrawer menuItems={testMenuItems} />);
    const drawer = document.querySelector('.MuiDrawer-paper');
    expect(drawer).toHaveStyle({ width: '240px' });
    
    const toggleButton = getToggleButton();
    expect(toggleButton).toBeInTheDocument();
    fireEvent.click(toggleButton);
    expect(drawer).toHaveStyle({ width: '60px' });
  });

  it('expands and collapses menu items with subItems', async () => {
    renderWithProviders(<BaseDrawer menuItems={testMenuItems} />);
    expect(screen.queryByText('Users')).not.toBeInTheDocument();
    
    fireEvent.click(screen.getByText('Team'));
    await waitForTransition();
    expect(screen.getByText('Users')).toBeInTheDocument();
    
    fireEvent.click(screen.getByText('Team'));
    await waitForTransition();
    expect(screen.queryByText('Users')).not.toBeInTheDocument();
  });

  it('maintains parent expanded state when expanding nested items', async () => {
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
    
    fireEvent.click(screen.getByText('Parent'));
    await waitForTransition();
    expect(screen.getByText('Child')).toBeInTheDocument();
    
    fireEvent.click(screen.getByText('Child'));
    await waitForTransition();
    expect(screen.getByText('Grandchild')).toBeInTheDocument();
    expect(screen.getByText('Child')).toBeInTheDocument();
  });

  it('persists drawer state', async () => {
    const { rerender } = renderWithProviders(
      <BaseDrawer id="test" menuItems={testMenuItems} defaultOpen={true} />
    );

    fireEvent.click(screen.getByText('Team'));
    await waitForTransition();
    expect(screen.getByText('Users')).toBeInTheDocument();

    const toggleButton = getToggleButton();
    expect(toggleButton).toBeInTheDocument();
    fireEvent.click(toggleButton);
    await waitForTransition();

    rerender(
      <MemoryRouter>
        <ThemeProvider theme={adminTheme}>
          <BaseDrawer id="test" menuItems={testMenuItems} />
        </ThemeProvider>
      </MemoryRouter>
    );
    await waitForTransition();

    const newToggleButton = getToggleButton();
    expect(newToggleButton).toBeInTheDocument();
    fireEvent.click(newToggleButton);
    await waitForTransition();
    expect(screen.getByText('Users')).toBeInTheDocument();
  });
});
