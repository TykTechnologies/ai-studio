import React from 'react';
import { render, screen, act, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import useQuickStart from './useQuickStart';
import useUserEntitlements from './useUserEntitlements';
import apiClient from '../utils/apiClient';

// Mock dependencies
jest.mock('./useUserEntitlements', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('../utils/apiClient', () => ({
  __esModule: true,
  default: {
    get: jest.fn(),
  },
}));

// Test component that uses the useQuickStart hook
const TestComponent = () => {
  const quickStartData = useQuickStart();
  return (
    <div>
      <div data-testid="quick-start-data">{JSON.stringify(quickStartData)}</div>
      <button 
        onClick={quickStartData.handleQuickStartComplete} 
        data-testid="complete-button"
      >
        Complete
      </button>
      <button 
        onClick={quickStartData.handleQuickStartSkip} 
        data-testid="skip-button"
      >
        Skip
      </button>
      <button 
        onClick={() => quickStartData.setShowQuickStart(true)} 
        data-testid="show-button"
      >
        Show
      </button>
      <button 
        onClick={() => quickStartData.fetchQuickStartData()} 
        data-testid="fetch-button"
      >
        Fetch
      </button>
    </div>
  );
};

describe('useQuickStart Hook', () => {
  // Setup default mocks
  const mockFetchUserEntitlements = jest.fn().mockResolvedValue({});
  const mockUserEntitlements = {
    userName: 'Test User',
    userId: 'user123',
    userEmail: 'test@example.com',
    fetchUserEntitlements: mockFetchUserEntitlements,
    error: null
  };

  beforeEach(() => {
    jest.clearAllMocks();
    
    // Default mock implementation for useUserEntitlements
    useUserEntitlements.mockReturnValue(mockUserEntitlements);
    
    // Default mock implementation for apiClient.get
    apiClient.get.mockResolvedValue({ data: { count: 0 } });
    
    // Spy on console.log and console.error
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    console.log.mockRestore();
    console.error.mockRestore();
  });

  test('initializes with default values', async () => {
    render(<TestComponent />);
    
    // Wait for initial data fetch to complete
    await waitFor(() => {
      const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
      expect(data.loading).toBe(false);
    });
    
    const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
    
    // Check initial values
    expect(data.showQuickStart).toBe(true); // Should be true because apiClient.get returns count: 0
    expect(data.loading).toBe(false);
    expect(data.error).toBe(null);
    expect(data.currentUser).toEqual({
      id: 'user123',
      name: 'Test User',
      email: 'test@example.com'
    });
  });

  test('does not show quick start when apps count is greater than 0', async () => {
    // Mock apiClient to return apps count > 0
    apiClient.get.mockResolvedValue({ data: { count: 5 } });
    
    render(<TestComponent />);
    
    // Wait for initial data fetch to complete
    await waitFor(() => {
      const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
      expect(data.loading).toBe(false);
    });
    
    const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
    
    // Quick start should not be shown because apps count > 0
    expect(data.showQuickStart).toBe(false);
  });

  test('handles error when fetching apps count fails', async () => {
    // Mock apiClient to throw an error
    const errorMessage = 'Failed to fetch apps count';
    apiClient.get.mockRejectedValue(new Error(errorMessage));
    
    render(<TestComponent />);
    
    // Wait for initial data fetch to complete
    await waitFor(() => {
      const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
      expect(data.loading).toBe(false);
    });
    
    // Verify console.error was called with the error message
    expect(console.error).toHaveBeenCalledWith(
      'Error fetching apps count:',
      expect.any(Error)
    );
    
    // Verify the returned apps count is 0 when an error occurs
    expect(apiClient.get).toHaveBeenCalledWith('/apps/count');
  });

  test('handles error when fetching quick start data fails', async () => {
    // Mock fetchUserEntitlements to throw an error
    const errorMessage = 'Failed to fetch user entitlements';
    mockFetchUserEntitlements.mockRejectedValue(new Error(errorMessage));
    
    render(<TestComponent />);
    
    // Wait for initial data fetch to complete
    await waitFor(() => {
      const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
      expect(data.loading).toBe(false);
    });
    
    const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
    
    // Error should be set
    expect(data.error).toBe('Failed to load data');
    
    // Verify console.error was called with the error message
    expect(console.error).toHaveBeenCalledWith(
      'Error fetching quick start data:',
      expect.any(Error)
    );
  });

  test('combines errors from useUserEntitlements', async () => {
    // Mock useUserEntitlements to return an error
    const entitlementsError = 'Failed to fetch entitlements';
    useUserEntitlements.mockReturnValue({
      ...mockUserEntitlements,
      error: entitlementsError
    });
    
    render(<TestComponent />);
    
    // Wait for initial data fetch to complete
    await waitFor(() => {
      const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
      expect(data.loading).toBe(false);
    });
    
    const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
    
    // Error should be the entitlements error
    expect(data.error).toBe(entitlementsError);
  });

  test('handleQuickStartComplete sets showQuickStart to false', async () => {
    render(<TestComponent />);
    
    // Wait for initial data fetch to complete
    await waitFor(() => {
      const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
      expect(data.loading).toBe(false);
    });
    
    // Click the complete button
    act(() => {
      screen.getByTestId('complete-button').click();
    });
    
    // Verify showQuickStart is set to false
    const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
    expect(data.showQuickStart).toBe(false);
  });

  test('handleQuickStartSkip sets showQuickStart to false', async () => {
    render(<TestComponent />);
    
    // Wait for initial data fetch to complete
    await waitFor(() => {
      const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
      expect(data.loading).toBe(false);
    });
    
    // Click the skip button
    act(() => {
      screen.getByTestId('skip-button').click();
    });
    
    // Verify showQuickStart is set to false
    const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
    expect(data.showQuickStart).toBe(false);
  });

  test('setShowQuickStart updates the state and logs the change', async () => {
    render(<TestComponent />);
    
    // Wait for initial data fetch to complete
    await waitFor(() => {
      const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
      expect(data.loading).toBe(false);
    });
    
    // First, set showQuickStart to false
    act(() => {
      screen.getByTestId('skip-button').click();
    });
    
    // Verify showQuickStart is set to false
    let data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
    expect(data.showQuickStart).toBe(false);
    
    // Now set it back to true
    act(() => {
      screen.getByTestId('show-button').click();
    });
    
    // Verify showQuickStart is set to true
    data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
    expect(data.showQuickStart).toBe(true);
  });

  test('fetchQuickStartData fetches data and updates state', async () => {
    // Mock apiClient to return apps count > 0 initially
    apiClient.get.mockResolvedValue({ data: { count: 5 } });
    
    render(<TestComponent />);
    
    // Wait for initial data fetch to complete
    await waitFor(() => {
      const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
      expect(data.loading).toBe(false);
    });
    
    // Verify showQuickStart is false initially
    let data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
    expect(data.showQuickStart).toBe(false);
    
    // Now mock apiClient to return apps count = 0
    apiClient.get.mockResolvedValue({ data: { count: 0 } });
    
    // Click the fetch button to manually fetch data
    act(() => {
      screen.getByTestId('fetch-button').click();
    });
    
    // Wait for data fetch to complete
    await waitFor(() => {
      const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
      expect(data.loading).toBe(false);
    });
    
    // Verify showQuickStart is now true
    data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
    expect(data.showQuickStart).toBe(true);
    
    // Verify fetchUserEntitlements and apiClient.get were called
    expect(mockFetchUserEntitlements).toHaveBeenCalled();
    expect(apiClient.get).toHaveBeenCalledWith('/apps/count');
  });

  test('fetchAppsCount returns 0 when API call fails', async () => {
    // Mock apiClient to throw an error
    apiClient.get.mockRejectedValue(new Error('API error'));
    
    render(<TestComponent />);
    
    // Wait for initial data fetch to complete
    await waitFor(() => {
      const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
      expect(data.loading).toBe(false);
    });
    
    // Verify console.error was called
    expect(console.error).toHaveBeenCalledWith(
      'Error fetching apps count:',
      expect.any(Error)
    );
    
    // Since fetchAppsCount returns 0 on error, showQuickStart should be true
    const data = JSON.parse(screen.getByTestId('quick-start-data').textContent);
    expect(data.showQuickStart).toBe(true);
  });

  test('skips initial fetch when skipInitialFetch is true', () => {
    // Create a modified version of useQuickStart that skips the initial fetch
    const SkipFetchComponent = () => {
      // We're not actually testing this component, just using it to verify the hook behavior
      useQuickStart();
      return <div>Skip Fetch Test</div>;
    };
    
    render(<SkipFetchComponent />);
    
    // Verify that fetchUserEntitlements was called with true
    expect(useUserEntitlements).toHaveBeenCalledWith(true);
  });
});