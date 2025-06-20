import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ThemeProvider } from '@mui/material/styles';
import SummaryStep from './SummaryStep';
import theme from '../../../theme';
import * as utils from './utils';

// Mock useQuickStart and utility functions
jest.mock('./QuickStartContext', () => ({
  useQuickStart: jest.fn(),
}));

// Mock the utils module
jest.mock('./utils', () => ({
  generateEndpointUrl: jest.fn((path, provider) => `/mocked${path}${provider}`),
  getBudgetLimitText: jest.fn(() => 'No budget limit'),
  getOwnerName: jest.fn(() => 'Mock Owner'),
  getCurlExample: jest.fn((provider, name) => 'curl example'),
  generateSlug: jest.fn(name => name.toLowerCase()),
}));

// No need to mock getBaseUrl anymore since we're using getConfig in utils.js
jest.mock('@mui/icons-material/ContentCopy', () => () => <div data-testid="content-copy-icon" />);

const { useQuickStart } = require('./QuickStartContext');

const renderWithTheme = (ui) =>
  render(<ThemeProvider theme={theme}>{ui}</ThemeProvider>);

describe('SummaryStep', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // Mock clipboard
    Object.assign(navigator, {
      clipboard: { writeText: jest.fn() },
    });
  });

  const baseMockData = {
    goToNextStep: jest.fn(),
    goToPreviousStep: jest.fn(),
    skipQuickStart: jest.fn(),
    llmData: { llmProvider: 'openai', name: 'OpenAI' },
    ownerData: { ownerType: 'current', name: 'Mock Owner' },
    appData: { name: 'Test App', description: 'Test Description' },
    credentialData: { keyID: 'key123', secret: 'secret123' },
  };

  it('renders summary info and credentials', () => {
    useQuickStart.mockReturnValue(baseMockData);
    renderWithTheme(<SummaryStep />);
    expect(
      screen.queryAllByText((content) => content.includes('Your app has been created. Please review the details below and copy the access information and credentials to interact with the LLM in your app.')).length
    ).toBeGreaterThan(0);
    expect(screen.getByText('LLM provider')).toBeInTheDocument();
    expect(screen.getByText('openai')).toBeInTheDocument();
    expect(screen.getByText('Owner')).toBeInTheDocument();
    // The getOwnerName function is mocked to return 'Mock Owner'
    // We need to directly check the mock function was called with the right data
    expect(utils.getOwnerName).toHaveBeenCalledWith(baseMockData.ownerData);
    expect(screen.getByText('App name')).toBeInTheDocument();
    expect(screen.getByText('Test App')).toBeInTheDocument();
    expect(screen.getByText('Description')).toBeInTheDocument();
    expect(screen.getByText('Test Description')).toBeInTheDocument();
    expect(screen.getByText('Budget limit')).toBeInTheDocument();
    // Check that getBudgetLimitText was called with the right data
    expect(utils.getBudgetLimitText).toHaveBeenCalledWith(baseMockData.appData);
    expect(screen.getByText('Key ID')).toBeInTheDocument();
    expect(screen.getByText('key123')).toBeInTheDocument();
    expect(screen.getByText('Secret')).toBeInTheDocument();
    // Masked secret
    expect(screen.getByText('••••••••••••••••')).toBeInTheDocument();
  });

  it('shows Not available for missing credentials', () => {
    useQuickStart.mockReturnValue({ ...baseMockData, credentialData: {} });
    renderWithTheme(<SummaryStep />);
    expect(
      screen.queryAllByText((content) => content.includes('Not available')).length
    ).toBeGreaterThan(0);
  });

  it('copies keyID to clipboard and shows tooltip', async () => {
    useQuickStart.mockReturnValue(baseMockData);
    renderWithTheme(<SummaryStep />);
    const copyButtons = screen.getAllByRole('button');
    // Find the button for keyID (first copy button)
    fireEvent.click(copyButtons[0]);
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('key123');
    // Tooltip should appear
    await waitFor(() => {
      expect(screen.getByText('Copied!')).toBeInTheDocument();
    });
  });

  it('copies secret to clipboard and shows tooltip', async () => {
    useQuickStart.mockReturnValue(baseMockData);
    renderWithTheme(<SummaryStep />);
    const copyButtons = screen.getAllByRole('button');
    // Find the button for secret (second copy button)
    fireEvent.click(copyButtons[1]);
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('secret123');
    await waitFor(() => {
      expect(screen.getByText('Copied!')).toBeInTheDocument();
    });
  });

  it('renders mocked endpoints and copies REST API url', async () => {
    useQuickStart.mockReturnValue(baseMockData);
    renderWithTheme(<SummaryStep />);
    // Use a flexible matcher for the endpoint text
    // Check that the generateEndpointUrl function was called with the right parameters
    expect(utils.generateEndpointUrl).toHaveBeenCalledWith('/llm/rest/', 'OpenAI');
    const copyButtons = screen.getAllByRole('button');
    // Third copy button is REST API
    fireEvent.click(copyButtons[2]);
    // Check that the clipboard was called with the result of generateEndpointUrl
    const mockedEndpoint = utils.generateEndpointUrl('/llm/rest/', 'openai');
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(mockedEndpoint);
    await waitFor(() => {
      expect(screen.getByText('Copied!')).toBeInTheDocument();
    });
  });
});
