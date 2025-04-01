import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { MemoryRouter } from 'react-router-dom';
import Overview from './Overview';
import useOverviewData from '../hooks/useOverviewData';

// Mock the coordinator hook
jest.mock('../hooks/useOverviewData', () => ({
  __esModule: true,
  default: jest.fn(),
}));

// Mock the useNavigate hook
const mockNavigate = jest.fn();
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
}));

// Mock the components used in Overview
jest.mock('../components/common/BasicCard', () => ({
  __esModule: true,
  default: ({ children, primaryAction, secondaryAction }) => (
    <div data-testid="basic-card">
      {children}
      {primaryAction && (
        <button 
          data-testid="primary-action" 
          onClick={primaryAction.onClick}
          disabled={primaryAction.disabled}
        >
          {primaryAction.label}
        </button>
      )}
      {secondaryAction && (
        <button 
          data-testid="secondary-action" 
          onClick={secondaryAction.onClick}
          disabled={secondaryAction.disabled}
        >
          {secondaryAction.label}
        </button>
      )}
    </div>
  ),
}));

jest.mock('../components/common/IconBadge', () => ({
  __esModule: true,
  default: ({ iconName }) => <div data-testid="icon-badge" data-icon-name={iconName}>{iconName}</div>,
}));

// Create a mock theme for testing
const mockTheme = createTheme({
  palette: {
    background: {
      paper: '#ffffff',
      buttonPrimaryDefault: '#000000',
    },
    border: {
      neutralDefault: '#cccccc',
    },
    text: {
      primary: '#000000',
      defaultSubdued: '#666666',
    },
    custom: {
      white: '#ffffff',
      purpleExtraDark: '#5900CB',
    },
  },
  spacing: (factor) => `${0.25 * factor}rem`,
});

// Wrap component with ThemeProvider and MemoryRouter for testing
const renderWithProviders = (ui, { entitlements, features, hasLLMs, loading, error } = {}) => {
  // Set up default mock values for the coordinator hook
  useOverviewData.mockReturnValue({
    userEntitlements: entitlements?.userEntitlements || {},
    userName: entitlements?.userName || 'Test User',
    features: features?.features || {
      feature_chat: false,
      feature_gateway: false,
      feature_portal: false,
    },
    hasLLMs: hasLLMs !== undefined ? hasLLMs : false,
    loading: loading !== undefined ? loading : false,
    error: error || null,
  });

  return render(
    <ThemeProvider theme={mockTheme}>
      <MemoryRouter>
        {ui}
      </MemoryRouter>
    </ThemeProvider>
  );
};

describe('Overview Component', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders loading state when data is loading', () => {
    renderWithProviders(<Overview />, {
      loading: true
    });
    
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });

  test('renders error state when there is an error', () => {
    const errorMessage = 'Failed to load data';
    renderWithProviders(<Overview />, {
      error: errorMessage
    });
    
    expect(screen.getByText(errorMessage)).toBeInTheDocument();
  });

  test('renders welcome message with user name', () => {
    renderWithProviders(<Overview />, {
      entitlements: { userName: 'John Doe' },
    });
    
    expect(screen.getByText(/Hi John Doe, welcome to Tyk AI Studio!/i)).toBeInTheDocument();
  });

  test('renders welcome message with placeholder when user name is not available', () => {
    // Override the default mock for this specific test
    useOverviewData.mockReturnValueOnce({
      userEntitlements: {},
      userName: null,
      features: {
        feature_chat: false,
        feature_gateway: false,
        feature_portal: false,
      },
      hasLLMs: false,
      loading: false,
      error: null,
    });

    renderWithProviders(<Overview />);
    
    expect(screen.getByText(/Hi \[user name\], welcome to Tyk AI Studio!/i)).toBeInTheDocument();
  });

  test('renders all infrastructure cards', () => {
    renderWithProviders(<Overview />);
    
    // Check for the section titles
    expect(screen.getByText('Start building your AI infrastructure')).toBeInTheDocument();
    expect(screen.getByText('Govern AI')).toBeInTheDocument();
    
    // Check for the cards by their primary action buttons
    expect(screen.getByText('Add LLM provider')).toBeInTheDocument();
    expect(screen.getByText('Add Data source')).toBeInTheDocument();
    expect(screen.getByText('Add Tool')).toBeInTheDocument();
    expect(screen.getByText('Add user')).toBeInTheDocument();
    expect(screen.getByText('Learn Filters')).toBeInTheDocument();
  });

  test('renders Apps card when feature_gateway is enabled', () => {
    renderWithProviders(<Overview />, {
      features: { features: { feature_gateway: true, feature_portal: false, feature_chat: false } },
    });
    
    // Check for the Add Apps button which indicates the Apps card is rendered
    expect(screen.getByText('Add Apps')).toBeInTheDocument();
  });

  test('renders Apps card when feature_portal is enabled', () => {
    renderWithProviders(<Overview />, {
      features: { features: { feature_portal: true, feature_gateway: false, feature_chat: false } },
    });
    
    // Check for the Add Apps button which indicates the Apps card is rendered
    expect(screen.getByText('Add Apps')).toBeInTheDocument();
  });

  test('renders Chats card when feature_chat is enabled', () => {
    renderWithProviders(<Overview />, {
      features: { features: { feature_chat: true, feature_gateway: false, feature_portal: false } },
    });
    
    // Check for the Add Chats button which indicates the Chats card is rendered
    expect(screen.getByText('Add Chats')).toBeInTheDocument();
  });

  test('does not render Apps card when feature_gateway and feature_portal are disabled', () => {
    renderWithProviders(<Overview />, {
      features: { features: { feature_gateway: false, feature_portal: false } },
    });
    
    expect(screen.queryByText(/Apps enable your devs to use any tooling/i)).not.toBeInTheDocument();
  });

  test('does not render Chats card when feature_chat is disabled', () => {
    renderWithProviders(<Overview />, {
      features: { features: { feature_chat: false } },
    });
    
    expect(screen.queryByText(/Chats provide an easy-to-use interface/i)).not.toBeInTheDocument();
  });

  test('enables Add Chats button when LLMs are available', () => {
    renderWithProviders(<Overview />, {
      features: { features: { feature_chat: true } },
      hasLLMs: true
    });
    
    const addChatsButton = screen.getByText('Add Chats');
    expect(addChatsButton).not.toBeDisabled();
  });

  test('disables Add Chats button when no LLMs are available', () => {
    renderWithProviders(<Overview />, {
      features: { features: { feature_chat: true } },
      hasLLMs: false
    });
    
    const addChatsButton = screen.getByText('Add Chats');
    expect(addChatsButton).toBeDisabled();
  });

  test('enables Add Apps button when LLMs are available', () => {
    renderWithProviders(<Overview />, {
      features: { features: { feature_gateway: true } },
      hasLLMs: true
    });
    
    const addAppsButton = screen.getByText('Add Apps');
    expect(addAppsButton).not.toBeDisabled();
  });

  test('disables Add Apps button when no LLMs are available', () => {
    renderWithProviders(<Overview />, {
      features: { features: { feature_gateway: true } },
      hasLLMs: false
    });
    
    const addAppsButton = screen.getByText('Add Apps');
    expect(addAppsButton).toBeDisabled();
  });

  test('navigates to correct route when Add LLM provider button is clicked', () => {
    renderWithProviders(<Overview />);
    
    fireEvent.click(screen.getByText('Add LLM provider'));
    expect(mockNavigate).toHaveBeenCalledWith('/admin/llms/new');
  });

  test('navigates to correct route when Add Data source button is clicked', () => {
    renderWithProviders(<Overview />);
    
    fireEvent.click(screen.getByText('Add Data source'));
    expect(mockNavigate).toHaveBeenCalledWith('/admin/datasources/new');
  });

  test('navigates to correct route when Add Tool button is clicked', () => {
    renderWithProviders(<Overview />);
    
    fireEvent.click(screen.getByText('Add Tool'));
    expect(mockNavigate).toHaveBeenCalledWith('/admin/tools/new');
  });

  test('navigates to correct route when Add user button is clicked', () => {
    renderWithProviders(<Overview />);
    
    fireEvent.click(screen.getByText('Add user'));
    expect(mockNavigate).toHaveBeenCalledWith('/admin/users/new');
  });

  test('navigates to correct route when Add Apps button is clicked', () => {
    renderWithProviders(<Overview />, {
      features: { features: { feature_gateway: true } },
      hasLLMs: true
    });
    
    fireEvent.click(screen.getByText('Add Apps'));
    expect(mockNavigate).toHaveBeenCalledWith('/admin/apps/new');
  });

  test('navigates to correct route when Add Chats button is clicked', () => {
    renderWithProviders(<Overview />, {
      features: { features: { feature_chat: true } },
      hasLLMs: true
    });
    
    fireEvent.click(screen.getByText('Add Chats'));
    expect(mockNavigate).toHaveBeenCalledWith('/admin/chats/new');
  });

  test('renders correct icon badges for each card', () => {
    renderWithProviders(<Overview />);
    
    const iconBadges = screen.getAllByTestId('icon-badge');
    
    // Check that we have the expected number of icon badges
    expect(iconBadges).toHaveLength(5);
    
    // Check that each icon badge has the correct icon name
    expect(iconBadges[0]).toHaveAttribute('data-icon-name', 'microchip-ai');
    expect(iconBadges[1]).toHaveAttribute('data-icon-name', 'book-sparkles');
    expect(iconBadges[2]).toHaveAttribute('data-icon-name', 'screwdriver-wrench');
    expect(iconBadges[3]).toHaveAttribute('data-icon-name', 'users');
    expect(iconBadges[4]).toHaveAttribute('data-icon-name', 'shield');
  });
});