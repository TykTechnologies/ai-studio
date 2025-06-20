
import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import WelcomeStep from './WelcomeStep';
import { useQuickStart } from './QuickStartContext';

// Mock the image import
jest.mock('./welcome_step.png', () => 'mocked-image-path');

// Mock the QuickStartContext hook
jest.mock('./QuickStartContext', () => ({
  useQuickStart: jest.fn(),
}));

// Mock the PrimaryButton component
jest.mock('../../../styles/sharedStyles', () => ({
  PrimaryButton: ({ children, onClick }) => (
    <button data-testid="primary-button" onClick={onClick}>{children}</button>
  ),
}));

// Mock the ActionsContainer component
jest.mock('./styles', () => ({
  ActionsContainer: ({ children }) => (
    <div data-testid="actions-container">{children}</div>
  ),
}));

// Mock useMediaQuery
jest.mock('@mui/material', () => {
  const actual = jest.requireActual('@mui/material');
  return {
    ...actual,
    useMediaQuery: jest.fn().mockReturnValue(false),
  };
});

describe('WelcomeStep Component', () => {
  // Create a mock theme for testing
  const mockTheme = createTheme({
    palette: {
      text: {
        primary: '#000000',
        defaultSubdued: '#666666',
      },
      background: {
        paper: '#ffffff',
        buttonPrimaryDefault: '#343452',
        buttonPrimaryDefaultHover: '#181834',
      },
      border: {
        neutralDefault: '#D8D8DF',
        neutralPressed: '#656582',
      },
      primary: {
        main: '#23E2C2',
      },
      custom: {
        white: '#FFFFFF',
        purpleExtraDark: '#5900CB',
      },
    },
    spacing: (factor) => `${0.25 * factor}rem`,
  });

  // Mock QuickStart context values
  const mockGoToNextStep = jest.fn();
  const mockSkipQuickStart = jest.fn();

  // Default context values
  const defaultContextValues = {
    goToNextStep: mockGoToNextStep,
    skipQuickStart: mockSkipQuickStart,
    currentUser: { id: '123', name: 'User' }
  };

  // Reset mocks before each test
  beforeEach(() => {
    jest.clearAllMocks();
    useQuickStart.mockReturnValue(defaultContextValues);
  });

  // Wrapper component with theme provider
  const renderWithTheme = (ui) => {
    return render(<ThemeProvider theme={mockTheme}>{ui}</ThemeProvider>);
  };

  test('renders with default userName', () => {
    renderWithTheme(<WelcomeStep />);
    
    // Check that the component renders with the default userName
    expect(screen.getByText('Welcome to Tyk, User')).toBeInTheDocument();
    
    // Check that the descriptive text is rendered
    expect(screen.getByText(/Empower your team to build AI Apps with direct access to LLM providers and data sources/)).toBeInTheDocument();
    expect(screen.getByText(/which can be used for code editors, knowledge search, and task automation keeping centralized control, usage tracking, and security/)).toBeInTheDocument();
    
    // Check that the image is rendered
    const image = screen.getByAltText('Welcome to Tyk AI Studio');
    expect(image).toBeInTheDocument();
    expect(image.tagName).toBe('IMG');
    
    // Check that the new explanatory text about AI studio App is rendered
    expect(screen.getByText(/Start by creating an/)).toBeInTheDocument();
    expect(screen.getByText('AI studio App')).toBeInTheDocument();
    expect(screen.getByText(/in three easy steps: configure AI infrastructure, manage access, and add details/)).toBeInTheDocument();
    
    // Check that the buttons are rendered
    expect(screen.getByText('Explore by myself')).toBeInTheDocument();
    expect(screen.getByText('Quick start')).toBeInTheDocument();
  });

  test('renders with custom userName', () => {
    // Mock the context with a custom user name
    useQuickStart.mockReturnValue({
      ...defaultContextValues,
      currentUser: { name: 'John Doe' }
    });
    
    renderWithTheme(<WelcomeStep />);
    
    // Check that the component renders with the custom userName
    expect(screen.getByText('Welcome to Tyk, John Doe')).toBeInTheDocument();
  });

  test('calls skipQuickStart when "Explore by myself" button is clicked', () => {
    renderWithTheme(<WelcomeStep />);
    
    // Click the "Explore by myself" button
    const exploreButton = screen.getByText('Explore by myself');
    fireEvent.click(exploreButton);
    
    // Check that skipQuickStart was called
    expect(mockSkipQuickStart).toHaveBeenCalledTimes(1);
  });

  test('calls goToNextStep when "Quick start" button is clicked', () => {
    renderWithTheme(<WelcomeStep />);
    
    // Click the "Quick start" button
    const quickStartButton = screen.getByText('Quick start');
    fireEvent.click(quickStartButton);
    
    // Check that goToNextStep was called
    expect(mockGoToNextStep).toHaveBeenCalledTimes(1);
  });

  test('applies mobile styles when screen is small', () => {
    // Set useMediaQuery to return true (mobile view)
    require('@mui/material').useMediaQuery.mockReturnValue(true);
    
    renderWithTheme(<WelcomeStep />);
    
    // Check that the component has mobile-specific styling
    // We can't directly test the CSS, but we can check that the component renders
    expect(screen.getByText('Welcome to Tyk, User')).toBeInTheDocument();
    
    // Reset useMediaQuery for other tests
    require('@mui/material').useMediaQuery.mockReturnValue(false);
  });
});