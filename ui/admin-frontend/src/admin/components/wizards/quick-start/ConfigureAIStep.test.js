import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import ConfigureAIStep from './ConfigureAIStep';
import { useQuickStart } from './QuickStartContext';
import { createLLM, updateLLM } from '../../../services';
import * as vendorLogos from '../../../utils/vendorLogos';

// Mock the QuickStartContext hook
jest.mock('./QuickStartContext', () => ({
  useQuickStart: jest.fn(),
}));

// Mock the services
jest.mock('../../../services', () => {
  const originalModule = jest.requireActual('../../../services');
  return {
    ...originalModule,
    createLLM: jest.fn(),
    updateLLM: jest.fn(),
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

// Mock the CustomSelect component
jest.mock('../../common/CustomSelect', () => {
  return function MockCustomSelect(props) {
    return (
      <select
        data-testid="mock-custom-select"
        name={props.name}
        value={props.value}
        onChange={props.onChange}
        required={props.required}
      >
        {props.options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    );
  };
});

// Mock the CustomSelectBadge component
jest.mock('../../common/CustomSelectBadge', () => {
  return function MockCustomSelectBadge(props) {
    return <div data-testid="mock-custom-select-badge">{props.config.text}</div>;
  };
});

// Mock the StyledTextField component to make testing easier
jest.mock('../../../styles/sharedStyles', () => ({
  StyledTextField: (props) => {
    // For password fields, render with type="password"
    return (
      <input
        data-testid={props.name || "styled-text-field"}
        type={props.type || "text"}
        name={props.name}
        value={props.value}
        onChange={props.onChange}
        required={props.required}
        autoComplete={props.autoComplete}
      />
    );
  },
  PrimaryButton: (props) => <button {...props}>{props.children}</button>,
  SecondaryLinkButton: (props) => <button {...props}>{props.children}</button>,
  ActionsContainer: ({ children, ...props }) => <div {...props}>{children}</div>
}));

// Mock the vendor logo utilities
jest.mock('../../../utils/vendorLogos', () => ({
  getVendorCodes: jest.fn(),
  getVendorName: jest.fn(),
  getVendorLogo: jest.fn(),
  vendorRequiresAccessDetails: jest.fn(),
}));

describe('ConfigureAIStep Component', () => {
  // Create a mock theme for testing
  const mockTheme = createTheme({
    palette: {
      background: {
        buttonPrimaryDefault: '#1976d2',
        paper: '#ffffff',
        surfaceCriticalDefault: '#d32f2f',
      },
      text: {
        primary: '#000000',
        defaultSubdued: '#666666',
        successDefault: '#2e7d32',
        warningDefault: '#ed6c02',
      },
      border: {
        neutralDefault: '#e0e0e0',
        neutralPressed: '#cccccc',
        successDefaultSubdued: '#e8f5e9',
        warningDefaultSubdued: '#fff3e0',
        criticalDefaultSubdue: '#ffebee',
        criticalHover: '#d32f2f',
      },
      custom: {
        white: '#ffffff',
        leaf: '#21ecba',
        purpleExtraDark: '#5900CB',
        purpleDark: '#8438FA',
        purpleLight: '#B421FA',
        purpleExtraLight: '#F0E4FF',
      },
    },
    spacing: (factor) => `${0.25 * factor}rem`,
  });

  // Mock QuickStart context values
  const mockSetStepValid = jest.fn();
  const mockGoToNextStep = jest.fn();
  const mockSkipQuickStart = jest.fn();
  const mockSetLlmData = jest.fn();
  const mockSetCreatedLlmId = jest.fn();

  // Default context values
  const defaultContextValues = {
    setStepValid: mockSetStepValid,
    goToNextStep: mockGoToNextStep,
    skipQuickStart: mockSkipQuickStart,
    llmData: {},
    setLlmData: mockSetLlmData,
    createdLlmId: null,
    setCreatedLlmId: mockSetCreatedLlmId,
    availableLLMs: []
  };

  // Reset mocks before each test
  beforeEach(() => {
    jest.clearAllMocks();
    useQuickStart.mockReturnValue(defaultContextValues);
    vendorLogos.getVendorCodes.mockReturnValue(['openai', 'anthropic', 'google_ai', 'ollama']);
    vendorLogos.getVendorName.mockImplementation((code) => {
      const vendorMap = {
        'openai': 'OpenAI',
        'anthropic': 'Anthropic',
        'google_ai': 'Google AI',
        'ollama': 'Ollama',
      };
      return vendorMap[code] || code;
    });
    vendorLogos.getVendorLogo.mockImplementation((code) => `/logos/${code}.png`);
    vendorLogos.vendorRequiresAccessDetails.mockImplementation((code) => code !== 'ollama');
  });

  // Wrapper component with theme provider
  const renderWithTheme = (ui) => {
    return render(<ThemeProvider theme={mockTheme}>{ui}</ThemeProvider>);
  };

  test('renders the component with empty form', () => {
    renderWithTheme(<ConfigureAIStep />);
    
    // Check that the component renders with the correct title and fields
    expect(screen.getByText('Name*')).toBeInTheDocument();
    expect(screen.getByText('LLM Provider*')).toBeInTheDocument();
    expect(screen.getByText('API Endpoint')).toBeInTheDocument();
    expect(screen.getByText('API Key')).toBeInTheDocument();
    expect(screen.getByText('Privacy Level')).toBeInTheDocument();
    
    // Check that the buttons are rendered
    expect(screen.getByText('Skip quick start')).toBeInTheDocument();
    expect(screen.getByText('Continue')).toBeInTheDocument();
    
    // Continue button should be disabled initially (no required fields entered)
    const continueButton = screen.getByRole('button', { name: /continue/i });
    expect(continueButton).toBeDisabled();
  });

  test('loads existing LLM data when available', () => {
    const existingLlmData = {
      name: 'Test LLM',
      llmProvider: 'openai',
      apiEndpoint: 'https://api.openai.com/v1',
      apiKey: 'sk-test123',
      privacyLevel: 'internal'
    };
    
    useQuickStart.mockReturnValue({
      ...defaultContextValues,
      llmData: existingLlmData
    });
    
    renderWithTheme(<ConfigureAIStep />);
    
    // Check that the form is populated with existing data
    const nameInput = screen.getAllByRole('textbox')[0]; // First textbox is the name input
    expect(nameInput).toHaveValue('Test LLM');
    
    // Continue button should be enabled (required fields are filled)
    const continueButton = screen.getByRole('button', { name: /continue/i });
    expect(continueButton).not.toBeDisabled();
  });

  test('validates form fields correctly for providers requiring access details', () => {
    renderWithTheme(<ConfigureAIStep />);
    
    // Initially the form should be invalid (no name or provider)
    const continueButton = screen.getByRole('button', { name: /continue/i });
    expect(continueButton).toBeDisabled();
    
    // Enter LLM name
    const nameInput = screen.getAllByRole('textbox')[0]; // First textbox is the name input
    fireEvent.change(nameInput, { target: { value: 'Test LLM' } });
    
    // Form should still be invalid (no provider selected)
    expect(continueButton).toBeDisabled();
    
    // Select LLM provider that requires access details
    const providerSelect = screen.getAllByTestId('mock-custom-select')[0];
    fireEvent.change(providerSelect, { target: { value: 'openai' } });
    
    // Form should still be invalid (no API endpoint and key)
    expect(continueButton).toBeDisabled();
    
    // Enter API endpoint
    const apiEndpointInput = screen.getByTestId('apiEndpoint');
    fireEvent.change(apiEndpointInput, { target: { value: 'https://api.openai.com/v1' } });
    
    // Form should still be invalid (no API key)
    expect(continueButton).toBeDisabled();
    
    // Enter API key
    const apiKeyInput = screen.getByTestId('apiKey');
    fireEvent.change(apiKeyInput, { target: { value: 'sk-test123' } });
    
    // Form should now be valid
    expect(continueButton).not.toBeDisabled();
    expect(mockSetStepValid).toHaveBeenCalledWith('configure-ai', true);
  });
  
  test('validates form fields correctly for Ollama (no access details required)', () => {
    renderWithTheme(<ConfigureAIStep />);
    
    // Initially the form should be invalid (no name or provider)
    const continueButton = screen.getByRole('button', { name: /continue/i });
    expect(continueButton).toBeDisabled();
    
    // Enter LLM name
    const nameInput = screen.getAllByRole('textbox')[0]; // First textbox is the name input
    fireEvent.change(nameInput, { target: { value: 'Test LLM' } });
    
    // Form should still be invalid (no provider selected)
    expect(continueButton).toBeDisabled();
    
    // Select Ollama as provider (doesn't require access details)
    const providerSelect = screen.getAllByTestId('mock-custom-select')[0];
    fireEvent.change(providerSelect, { target: { value: 'ollama' } });
    
    // Form should now be valid without API endpoint and key
    expect(continueButton).not.toBeDisabled();
    expect(mockSetStepValid).toHaveBeenCalledWith('configure-ai', true);
  });

  test('displays asterisks for API fields when required', () => {
    renderWithTheme(<ConfigureAIStep />);
    
    // Initially, no provider is selected, so no asterisks should be shown
    expect(screen.queryByText('API Endpoint*')).not.toBeInTheDocument();
    expect(screen.queryByText('API Key*')).not.toBeInTheDocument();
    
    // Select a provider that requires access details (OpenAI)
    const providerSelect = screen.getAllByTestId('mock-custom-select')[0];
    fireEvent.change(providerSelect, { target: { value: 'openai' } });
    
    // Asterisks should now be shown
    expect(screen.getByText('API Endpoint*')).toBeInTheDocument();
    expect(screen.getByText('API Key*')).toBeInTheDocument();
    
    // Change to Ollama which doesn't require access details
    fireEvent.change(providerSelect, { target: { value: 'ollama' } });
    
    // Asterisks should be removed
    expect(screen.queryByText('API Endpoint*')).not.toBeInTheDocument();
    expect(screen.queryByText('API Key*')).not.toBeInTheDocument();
  });

  test('creates a new LLM when form is submitted', async () => {
    // Mock API response
    createLLM.mockResolvedValue({
      id: '123'
    });
    
    renderWithTheme(<ConfigureAIStep />);
    
    // Fill out the form
    const nameInput = screen.getByTestId('name');
    fireEvent.change(nameInput, { target: { value: 'New Test LLM' } });
    
    const providerSelect = screen.getAllByTestId('mock-custom-select')[0];
    fireEvent.change(providerSelect, { target: { value: 'openai' } });
    
    // Get API endpoint and key inputs
    const apiEndpointInput = screen.getByTestId('apiEndpoint');
    fireEvent.change(apiEndpointInput, { target: { value: 'https://api.openai.com/v1' } });
    
    const apiKeyInput = screen.getByTestId('apiKey');
    fireEvent.change(apiKeyInput, { target: { value: 'sk-test123' } });
    
    // Submit the form
    const continueButton = screen.getByRole('button', { name: /continue/i });
    fireEvent.click(continueButton);
    
    // Wait for API calls to complete
    await waitFor(() => {
      expect(createLLM).toHaveBeenCalledWith(expect.objectContaining({
        name: 'New Test LLM',
        llmProvider: 'openai',
        apiEndpoint: 'https://api.openai.com/v1',
        apiKey: 'sk-test123',
        privacyScore: 25, // public is default
      }));
    });
    
    // Check that context was updated
    expect(mockSetCreatedLlmId).toHaveBeenCalledWith('123');
    expect(mockSetLlmData).toHaveBeenCalled();
    expect(mockGoToNextStep).toHaveBeenCalled();
  });

  test('updates an existing LLM when form is submitted', async () => {
    // Setup context with existing LLM
    const existingLlmData = {
      name: 'Existing LLM',
      llmProvider: 'openai',
      apiEndpoint: 'https://api.openai.com/v1',
      apiKey: 'sk-old123',
      privacyLevel: 'public'
    };
    
    useQuickStart.mockReturnValue({
      ...defaultContextValues,
      createdLlmId: '123',
      llmData: existingLlmData
    });
    
    renderWithTheme(<ConfigureAIStep />);
    
    // Modify the form
    const nameInput = screen.getAllByRole('textbox')[0]; // First textbox is the name input
    fireEvent.change(nameInput, { target: { value: 'Updated LLM' } });
    
    // Submit the form
    const continueButton = screen.getByRole('button', { name: /continue/i });
    fireEvent.click(continueButton);
    
    // Wait for API calls to complete
    await waitFor(() => {
      expect(updateLLM).toHaveBeenCalledWith('123', expect.objectContaining({
        name: 'Updated LLM'
      }));
    });
    
    // Check that context was updated
    expect(mockSetLlmData).toHaveBeenCalled();
    expect(mockGoToNextStep).toHaveBeenCalled();
  });

  test('does not update LLM if data has not changed', async () => {
    // Setup context with existing LLM
    const existingLlmData = {
      name: 'Existing LLM',
      llmProvider: 'openai',
      apiEndpoint: 'https://api.openai.com/v1',
      apiKey: 'sk-old123',
      privacyLevel: 'public'
    };
    
    useQuickStart.mockReturnValue({
      ...defaultContextValues,
      createdLlmId: '123',
      llmData: existingLlmData
    });
    
    renderWithTheme(<ConfigureAIStep />);
    
    // Submit the form without changing anything
    const continueButton = screen.getByRole('button', { name: /continue/i });
    fireEvent.click(continueButton);
    
    // Wait for component to process
    await waitFor(() => {
      // updateLLM should not be called since data hasn't changed
      expect(updateLLM).not.toHaveBeenCalled();
    });
    
    // Context should still be updated
    expect(mockSetLlmData).toHaveBeenCalled();
    expect(mockGoToNextStep).toHaveBeenCalled();
  });

  test('shows error message when API call fails', async () => {
    // Mock API failure
    createLLM.mockRejectedValue(new Error('API Error'));
    
    renderWithTheme(<ConfigureAIStep />);
    
    // Fill out the form
    const nameInput = screen.getAllByRole('textbox')[0]; // First textbox is the name input
    fireEvent.change(nameInput, { target: { value: 'Test LLM' } });
    
    const providerSelect = screen.getAllByTestId('mock-custom-select')[0];
    fireEvent.change(providerSelect, { target: { value: 'openai' } });
    
    // Add API endpoint and key for OpenAI
    const apiEndpointInput = screen.getByTestId('apiEndpoint');
    fireEvent.change(apiEndpointInput, { target: { value: 'https://api.openai.com/v1' } });
    
    const apiKeyInput = screen.getByTestId('apiKey');
    fireEvent.change(apiKeyInput, { target: { value: 'sk-test123' } });
    
    // Submit the form
    const continueButton = screen.getByRole('button', { name: /continue/i });
    fireEvent.click(continueButton);
    
    // Wait for error message to appear
    await waitFor(() => {
      expect(screen.getByText(/Failed to create LLM: API Error/i)).toBeInTheDocument();
    });
    
    // Context should not be updated
    expect(mockGoToNextStep).not.toHaveBeenCalled();
  });

  test('handles privacy level selection correctly', () => {
    renderWithTheme(<ConfigureAIStep />);
    
    // Get privacy level select
    const privacyLevelSelect = screen.getAllByTestId('mock-custom-select')[1];
    
    // Change privacy level to confidential
    fireEvent.change(privacyLevelSelect, { target: { value: 'confidential' } });
    
    // Fill required fields to enable the continue button
    const nameInput = screen.getAllByRole('textbox')[0]; // First textbox is the name input
    fireEvent.change(nameInput, { target: { value: 'Test LLM' } });
    
    const providerSelect = screen.getAllByTestId('mock-custom-select')[0];
    fireEvent.change(providerSelect, { target: { value: 'openai' } });
    
    // Add API endpoint and key for OpenAI
    const apiEndpointInput = screen.getByTestId('apiEndpoint');
    fireEvent.change(apiEndpointInput, { target: { value: 'https://api.openai.com/v1' } });
    
    const apiKeyInput = screen.getByTestId('apiKey');
    fireEvent.change(apiKeyInput, { target: { value: 'sk-test123' } });
    
    // Submit the form
    const continueButton = screen.getByRole('button', { name: /continue/i });
    fireEvent.click(continueButton);
    
    // Wait for API call
    return waitFor(() => {
      expect(createLLM).toHaveBeenCalledWith(expect.objectContaining({
        privacyScore: 75, // confidential level
      }));
    });
  });

  test('handles skip quick start button correctly', () => {
    renderWithTheme(<ConfigureAIStep />);
    
    // Click Skip quick start button
    const skipButton = screen.getByRole('button', { name: /skip quick start/i });
    fireEvent.click(skipButton);
    expect(mockSkipQuickStart).toHaveBeenCalled();
  });

  test('displays loading state during form submission', async () => {
    // Mock delayed API response
    createLLM.mockImplementation(() => new Promise(resolve => {
      setTimeout(() => {
        resolve({
          id: '123'
        });
      }, 100);
    }));
    
    renderWithTheme(<ConfigureAIStep />);
    
    // Fill out the form
    const nameInput = screen.getAllByRole('textbox')[0]; // First textbox is the name input
    fireEvent.change(nameInput, { target: { value: 'Test LLM' } });
    
    const providerSelect = screen.getAllByTestId('mock-custom-select')[0];
    fireEvent.change(providerSelect, { target: { value: 'openai' } });
    
    // Add API endpoint and key for OpenAI
    const apiEndpointInput = screen.getByTestId('apiEndpoint');
    fireEvent.change(apiEndpointInput, { target: { value: 'https://api.openai.com/v1' } });
    
    const apiKeyInput = screen.getByTestId('apiKey');
    fireEvent.change(apiKeyInput, { target: { value: 'sk-test123' } });
    
    // Submit the form
    const continueButton = screen.getByRole('button', { name: /continue/i });
    fireEvent.click(continueButton);
    
    // Check for loading indicator
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
    
    // Wait for API call to complete
    await waitFor(() => {
      expect(mockGoToNextStep).toHaveBeenCalled();
    });
  });
});
