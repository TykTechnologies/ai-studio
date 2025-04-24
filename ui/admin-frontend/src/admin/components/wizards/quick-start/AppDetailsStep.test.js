import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import AppDetailsStep from './AppDetailsStep';
import { useQuickStart } from './QuickStartContext';
import { createApp, updateApp, activateCredential, getCredential } from '../../../services';

// Mock the QuickStartContext hook
jest.mock('./QuickStartContext', () => ({
  useQuickStart: jest.fn(),
}));

// Mock the services
jest.mock('../../../services', () => {
  const originalModule = jest.requireActual('../../../services');
  return {
    ...originalModule,
    createApp: jest.fn(),
    updateApp: jest.fn(),
    activateCredential: jest.fn(),
    getCredential: jest.fn(),
  };
});

// Mock the Icon component
jest.mock('../../../../components/common/Icon', () => {
  return function MockIcon(props) {
    return <div data-testid="mock-icon" data-icon-name={props.name} style={props.sx}>{props.name}</div>;
  };
});

// Mock the CustomNote component
jest.mock('../../common/CustomNote', () => {
  return function MockCustomNote(props) {
    return <div data-testid="mock-custom-note">{props.message}</div>;
  };
});

describe('AppDetailsStep Component', () => {
  // Create a mock theme for testing
  const mockTheme = createTheme({
    palette: {
      background: {
        buttonPrimaryDefault: '#1976d2',
        buttonPrimaryDefaultHover: '#1565c0',
        buttonPrimaryOutlineHover: '#e3f2fd',
        buttonCritical: '#d32f2f',
        buttonCriticalHover: '#c62828',
        paper: '#ffffff',
        defaultSubdued: '#e0e0e0',
        surfaceNeutralDisabled: '#f5f5f5',
        surfaceNeutralHover: '#f5f5f5',
        secondaryExtraLight: '#f5f5f5',
      },
      text: {
        primary: '#000000',
        defaultSubdued: '#666666',
        neutralDisabled: '#9e9e9e',
      },
      border: {
        neutralDefault: '#e0e0e0',
        neutralPressed: '#cccccc',
        neutralHovered: '#bdbdbd',
        criticalDefault: '#c62828',
        criticalHover: '#b71c1c',
        criticalDefaultSubdue: '#ffcdd2',
      },
      custom: {
        white: '#ffffff',
        teal: '#21ecba',
        lightTeal: 'rgba(33, 236, 186, 0.07)',
        hoverTeal: 'rgba(33, 236, 186, 0.47)',
        purpleExtraDark: '#5900CB',
      },
    },
    spacing: (factor) => `${0.25 * factor}rem`,
  });

  // Mock QuickStart context values
  const mockSetStepValid = jest.fn();
  const mockGoToNextStep = jest.fn();
  const mockGoToPreviousStep = jest.fn();
  const mockSkipQuickStart = jest.fn();
  const mockSetAppData = jest.fn();
  const mockSetCreatedAppId = jest.fn();
  const mockSetCredentialData = jest.fn();

  // Default context values
  const defaultContextValues = {
    setStepValid: mockSetStepValid,
    goToNextStep: mockGoToNextStep,
    goToPreviousStep: mockGoToPreviousStep,
    skipQuickStart: mockSkipQuickStart,
    appData: {},
    setAppData: mockSetAppData,
    createdAppId: null,
    setCreatedAppId: mockSetCreatedAppId,
    ownerData: { userId: '123' },
    createdLlmId: '456',
    setCredentialData: mockSetCredentialData,
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

  test('renders the component with empty form', () => {
    renderWithTheme(<AppDetailsStep />);
    
    // Check that the component renders with the correct title and fields
    expect(screen.getByText('App Name*')).toBeInTheDocument();
    expect(screen.getByText('Description')).toBeInTheDocument();
    expect(screen.getByText('Set budget limit (optional)')).toBeInTheDocument();
    
    // Check that the buttons are rendered
    expect(screen.getByText('Skip quick start')).toBeInTheDocument();
    expect(screen.getByText('Back')).toBeInTheDocument();
    expect(screen.getByText('Create app')).toBeInTheDocument();
    
    // Create app button should be disabled initially (no name entered)
    const createButton = screen.getByRole('button', { name: /create app/i });
    expect(createButton).toBeDisabled();
  });

  test('loads existing app data when available', () => {
    const existingAppData = {
      name: 'Test App',
      description: 'Test Description',
      setBudget: false,
      monthlyBudget: '',
      budgetStartDate: '2025-01-01'
    };
    
    useQuickStart.mockReturnValue({
      ...defaultContextValues,
      appData: existingAppData
    });
    
    renderWithTheme(<AppDetailsStep />);
    
    // Check that the form is populated with existing data
    const inputs = screen.getAllByRole('textbox');
    const nameInput = inputs.find(input => input.getAttribute('name') === 'name');
    const descriptionInput = inputs.find(input => input.getAttribute('name') === 'description');
    
    expect(nameInput).toHaveValue('Test App');
    expect(descriptionInput).toHaveValue('Test Description');
    
    // Create app button should be enabled (name is filled)
    const createButton = screen.getByRole('button', { name: /create app/i });
    expect(createButton).not.toBeDisabled();
  });

  test('validates form fields correctly', () => {
    renderWithTheme(<AppDetailsStep />);
    
    // Initially the form should be invalid (no name)
    const createButton = screen.getByRole('button', { name: /create app/i });
    expect(createButton).toBeDisabled();
    
    // Enter app name
    const inputs = screen.getAllByRole('textbox');
    const nameInput = inputs.find(input => input.getAttribute('name') === 'name');
    fireEvent.change(nameInput, { target: { value: 'Test App' } });
    
    // Form should now be valid
    expect(createButton).not.toBeDisabled();
    expect(mockSetStepValid).toHaveBeenCalledWith('app-details', true);
    
    // Enable budget but don't enter amount
    const budgetSwitch = screen.getByRole('checkbox');
    fireEvent.click(budgetSwitch);
    
    // Form should be invalid again
    expect(createButton).toBeDisabled();
    expect(mockSetStepValid).toHaveBeenCalledWith('app-details', false);
    
    // Enter budget amount
    const budgetInput = screen.getByRole('spinbutton');
    fireEvent.change(budgetInput, { target: { value: '1000' } });
    
    // Form should be valid again
    expect(createButton).not.toBeDisabled();
    expect(mockSetStepValid).toHaveBeenCalledWith('app-details', true);
  });

  test('shows budget fields when budget toggle is enabled', () => {
    renderWithTheme(<AppDetailsStep />);
    
    // Budget fields should not be visible initially
    expect(screen.queryByText('Monthly budget*')).not.toBeInTheDocument();
    expect(screen.queryByText('Budget cycle start date')).not.toBeInTheDocument();
    
    // Enable budget
    const budgetSwitch = screen.getByRole('checkbox');
    fireEvent.click(budgetSwitch);
    
    // Budget fields should now be visible
    expect(screen.getByText('Monthly budget*')).toBeInTheDocument();
    expect(screen.getByText('Budget cycle start date')).toBeInTheDocument();
    expect(screen.getByTestId('mock-custom-note')).toBeInTheDocument();
  });

  test('creates a new app when form is submitted', async () => {
    // Mock API responses
    createApp.mockResolvedValue({
      id: '789',
      attributes: {
        credential_id: '101112'
      }
    });
    
    activateCredential.mockResolvedValue({});
    
    getCredential.mockResolvedValue({
      attributes: {
        key_id: 'key123',
        secret: 'secret456',
        active: true
      }
    });
    
    renderWithTheme(<AppDetailsStep />);
    
    // Fill out the form
    const inputs = screen.getAllByRole('textbox');
    const nameInput = inputs.find(input => input.getAttribute('name') === 'name');
    fireEvent.change(nameInput, { target: { value: 'New Test App' } });
    
    const descInput = inputs.find(input => input.getAttribute('name') === 'description');
    fireEvent.change(descInput, { target: { value: 'New Test Description' } });
    
    // Submit the form
    const createButton = screen.getByRole('button', { name: /create app/i });
    fireEvent.click(createButton);
    
    // Wait for API calls to complete
    await waitFor(() => {
      expect(createApp).toHaveBeenCalledWith(expect.objectContaining({
        name: 'New Test App',
        description: 'New Test Description',
        userId: expect.any(String),
        llmIds: expect.any(Array),
        datasourceIds: expect.any(Array),
        setBudget: expect.any(Boolean),
        monthlyBudget: expect.any(String),
        budgetStartDate: expect.any(String)
      }));
    });
    
    expect(activateCredential).toHaveBeenCalledWith('789');
    expect(getCredential).toHaveBeenCalledWith('101112');
    
    // Check that context was updated
    expect(mockSetCreatedAppId).toHaveBeenCalledWith('789');
    expect(mockSetCredentialData).toHaveBeenCalledWith({
      keyID: 'key123',
      secret: 'secret456',
      active: true
    });
    expect(mockSetAppData).toHaveBeenCalled();
    expect(mockGoToNextStep).toHaveBeenCalled();
  });

  test('updates an existing app when form is submitted', async () => {
    // Setup context with existing app
    useQuickStart.mockReturnValue({
      ...defaultContextValues,
      createdAppId: '789',
      appData: {
        name: 'Existing App',
        description: 'Existing Description',
        setBudget: false,
        monthlyBudget: '',
        budgetStartDate: '2025-01-01'
      }
    });
    
    // Mock appService.updateApp for update scenario
    updateApp.mockResolvedValue({
      id: '789',
      attributes: {}
    });
    
    renderWithTheme(<AppDetailsStep />);
    
    // Modify the form so it's different from initial appData
    const inputs = screen.getAllByRole('textbox');
    const nameInput = inputs.find(input => input.getAttribute('name') === 'name');
    fireEvent.change(nameInput, { target: { value: 'Updated App' } });
    const descInput = inputs.find(input => input.getAttribute('name') === 'description');
    fireEvent.change(descInput, { target: { value: 'Updated Description' } });
    
    // Submit the form
    const updateButton = screen.getByRole('button', { name: /update app|create app/i });
    fireEvent.click(updateButton);
    
    // Wait for service function to be called
    await waitFor(() => {
      expect(updateApp).toHaveBeenCalledWith('789', expect.objectContaining({
        name: 'Updated App',
        description: 'Updated Description',
        userId: expect.any(String),
        llmIds: expect.any(Array),
        datasourceIds: expect.any(Array),
        setBudget: expect.any(Boolean),
        monthlyBudget: expect.any(String),
        budgetStartDate: expect.any(String)
      }));
    });
    
    // Check that context was updated
    expect(mockSetAppData).toHaveBeenCalled();
    expect(mockGoToNextStep).toHaveBeenCalled();
  });

  test('shows error message when API call fails', async () => {
    // Mock API failure
    createApp.mockRejectedValue(new Error('API Error'));
    
    renderWithTheme(<AppDetailsStep />);
    
    // Fill out the form
    const inputs = screen.getAllByRole('textbox');
    const nameInput = inputs.find(input => input.getAttribute('name') === 'name');
    fireEvent.change(nameInput, { target: { value: 'Test App' } });
    
    // Submit the form
    const createButton = screen.getByRole('button', { name: /create app/i });
    fireEvent.click(createButton);
    
    // Wait for error message to appear
    await waitFor(() => {
      expect(screen.getByText(/Failed to create app: API Error/i)).toBeInTheDocument();
    });
    
    // Context should not be updated
    expect(mockGoToNextStep).not.toHaveBeenCalled();
  });

  test('handles navigation buttons correctly', () => {
    renderWithTheme(<AppDetailsStep />);
    
    // Click Back button
    const backButton = screen.getByRole('button', { name: /back/i });
    fireEvent.click(backButton);
    expect(mockGoToPreviousStep).toHaveBeenCalled();
    
    // Click Skip quick start button
    const skipButton = screen.getByRole('button', { name: /skip quick start/i });
    fireEvent.click(skipButton);
    expect(mockSkipQuickStart).toHaveBeenCalled();
  });

  test('does not update app if data has not changed', async () => {
    // Setup context with existing app
    const existingAppData = {
      name: 'Existing App',
      description: 'Existing Description',
      setBudget: false,
      monthlyBudget: '',
      budgetStartDate: '2025-01-01'
    };
    
    useQuickStart.mockReturnValue({
      ...defaultContextValues,
      createdAppId: '789',
      appData: existingAppData
    });
    
    renderWithTheme(<AppDetailsStep />);
    
    // Submit the form without changing anything
    const createButton = screen.getByRole('button', { name: /create app/i });
    fireEvent.click(createButton);
    
    // Wait for component to process
    await waitFor(() => {
      // Patch should not be called since data hasn't changed
      expect(updateApp).not.toHaveBeenCalled();
    });
    
    // Context should still be updated
    expect(mockSetAppData).toHaveBeenCalled();
    expect(mockGoToNextStep).toHaveBeenCalled();
  });
});