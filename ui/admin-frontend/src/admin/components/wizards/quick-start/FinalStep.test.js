import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import FinalStep from './FinalStep';
import { useQuickStart } from './QuickStartContext';
import { useNavigate } from 'react-router-dom';
import useConfig from '../../../hooks/useConfig';
import { createDocsLinkHandler } from '../../../utils/docsLinkUtils';

// Mock the QuickStartContext hook
jest.mock('./QuickStartContext', () => ({
  useQuickStart: jest.fn(),
}));

// Mock the react-router-dom's useNavigate hook
jest.mock('react-router-dom', () => ({
  useNavigate: jest.fn(),
  NavLink: function MockNavLink(props) {
    return <a href={props.to} {...props}>{props.children}</a>;
  }
}));

// Mock the useConfig hook
jest.mock('../../../hooks/useConfig', () => ({
  __esModule: true,
  default: jest.fn(),
}));

// Mock the createDocsLinkHandler function
jest.mock('../../../utils/docsLinkUtils', () => ({
  createDocsLinkHandler: jest.fn(),
}));

// Mock the sharedStyles
jest.mock('../../../styles/sharedStyles', () => ({
  PrimaryButton: function MockPrimaryButton(props) {
    return (
      <button
        onClick={props.onClick}
        disabled={props.disabled}
        data-testid="primary-button"
      >
        {props.children}
      </button>
    );
  }
}));

// Mock the styles
jest.mock('./styles', () => ({
  ActionsContainer: function MockActionsContainer(props) {
    return <div data-testid="actions-container">{props.children}</div>;
  }
}));

// Mock the Icon component
jest.mock('../../../../components/common/Icon', () => {
  return function MockIcon(props) {
    return <div data-testid="mock-icon" data-icon-name={props.name} style={props.sx}>{props.name}</div>;
  };
});

// Mock the image import
jest.mock('./final_step.png', () => 'mock-image-path');

// Mock the BasicCard component
jest.mock('../../../components/common/BasicCard', () => {
  return function MockBasicCard(props) {
    return (
      <div data-testid="mock-basic-card">
        <div>{props.children}</div>
        {props.secondaryAction && (
          <button 
            onClick={props.secondaryAction.onClick}
            data-testid="secondary-action-button"
          >
            {props.secondaryAction.label}
          </button>
        )}
      </div>
    );
  };
});

// Mock the IconBadge component
jest.mock('../../../components/common/IconBadge', () => {
  return function MockIconBadge(props) {
    return <div data-testid="mock-icon-badge" data-icon-name={props.iconName}>{props.iconName}</div>;
  };
});

describe('FinalStep Component', () => {
  // Create a mock theme for testing
  const mockTheme = createTheme({
    palette: {
      background: {
        paper: '#ffffff',
      },
      text: {
        primary: '#000000',
      },
      border: {
        neutralDefault: '#e0e0e0',
      },
    },
    spacing: (factor) => `${0.25 * factor}rem`,
  });

  // Mock QuickStart context values
  const mockSkipQuickStart = jest.fn();
  const mockNavigate = jest.fn();
  const mockGetDocsLink = jest.fn();
  const mockCreateDocsLinkHandler = jest.fn();

  // Default context values
  const defaultContextValues = {
    skipQuickStart: mockSkipQuickStart,
    createdAppId: 'app123',
  };

  // Reset mocks before each test
  beforeEach(() => {
    jest.clearAllMocks();
    useQuickStart.mockReturnValue(defaultContextValues);
    useNavigate.mockReturnValue(mockNavigate);
    useConfig.mockReturnValue({ getDocsLink: mockGetDocsLink });
    createDocsLinkHandler.mockReturnValue(mockCreateDocsLinkHandler);
  });

  // Wrapper component with theme provider
  const renderWithTheme = (ui) => {
    return render(<ThemeProvider theme={mockTheme}>{ui}</ThemeProvider>);
  };

  test('renders the component with congratulations message and image', () => {
    renderWithTheme(<FinalStep />);
    
    // Check that the component renders with the correct title
    expect(screen.getByText('Congratulations on creating your first App!')).toBeInTheDocument();
    
    // Check that the image is rendered
    const image = screen.getByAltText('Congratulations on creating your first App!');
    expect(image).toBeInTheDocument();
    
    // Check that the buttons are rendered
    expect(screen.getByText('Proceed to overview')).toBeInTheDocument();
    expect(screen.getByText('Go to my app')).toBeInTheDocument();
    
    // Check that the "what to do next" section is rendered
    expect(screen.getByText('Wondering what to do next? Explore more of Tyk AI Studio features')).toBeInTheDocument();
    
    // Check that the cards are rendered
    expect(screen.getAllByTestId('mock-basic-card')).toHaveLength(3);
    
    // Check that the card content is rendered
    expect(screen.getByText(/Enhance AI responses, add relevant context with/)).toBeInTheDocument();
    expect(screen.getByText(/Keep data safe with/)).toBeInTheDocument();
    expect(screen.getByText(/Manage which teams can have access to AI and data through/)).toBeInTheDocument();
    
    // Check that the icon badges are rendered
    expect(screen.getAllByTestId('mock-icon-badge')).toHaveLength(3);
    
    // Check that the "Learn more" buttons are rendered
    expect(screen.getAllByText('Learn more')).toHaveLength(3);
  });

  test('calls skipQuickStart when "Proceed to overview" button is clicked', () => {
    renderWithTheme(<FinalStep />);
    
    // Click the "Proceed to overview" button
    const proceedButton = screen.getByText('Proceed to overview');
    fireEvent.click(proceedButton);
    
    // Check that skipQuickStart was called
    expect(mockSkipQuickStart).toHaveBeenCalled();
  });

  test('navigates to app page when "Go to my app" button is clicked', () => {
    renderWithTheme(<FinalStep />);
    
    // Click the "Go to my app" button
    const goToAppButton = screen.getByText('Go to my app');
    fireEvent.click(goToAppButton);
    
    // Check that navigate was called with the correct path
    expect(mockNavigate).toHaveBeenCalledWith('/admin/apps/app123');
  });

  test('does not navigate when createdAppId is null', () => {
    // Override the default context values
    useQuickStart.mockReturnValue({
      ...defaultContextValues,
      createdAppId: null,
    });
    
    renderWithTheme(<FinalStep />);
    
    // Click the "Go to my app" button
    const goToAppButton = screen.getByText('Go to my app');
    fireEvent.click(goToAppButton);
    
    // Check that navigate was not called
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  test('creates docs link handlers for each card', () => {
    renderWithTheme(<FinalStep />);
    
    // Check that createDocsLinkHandler was called for each card
    expect(createDocsLinkHandler).toHaveBeenCalledTimes(3);
    expect(createDocsLinkHandler).toHaveBeenCalledWith(mockGetDocsLink, 'data_sources');
    expect(createDocsLinkHandler).toHaveBeenCalledWith(mockGetDocsLink, 'filters');
    expect(createDocsLinkHandler).toHaveBeenCalledWith(mockGetDocsLink, 'catalogs');
  });

  test('calls the docs link handler when "Learn more" button is clicked', () => {
    renderWithTheme(<FinalStep />);
    
    // Click the first "Learn more" button
    const learnMoreButtons = screen.getAllByText('Learn more');
    fireEvent.click(learnMoreButtons[0]);
    
    // Check that the docs link handler was called
    expect(mockCreateDocsLinkHandler).toHaveBeenCalled();
  });
});