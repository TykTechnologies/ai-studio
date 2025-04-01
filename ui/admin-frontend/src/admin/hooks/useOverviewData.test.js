import React from 'react';
import { render, screen, waitFor, act } from '@testing-library/react';
import '@testing-library/jest-dom';
import useOverviewData from './useOverviewData';
import useUserEntitlements from './useUserEntitlements';
import useSystemFeatures from './useSystemFeatures';
import useLLMs from './useLLMs';

// Mock the dependency hooks
jest.mock('./useUserEntitlements', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('./useSystemFeatures', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('./useLLMs', () => ({
  __esModule: true,
  default: jest.fn(),
}));

// Mock console.error to prevent error output in tests
beforeAll(() => {
  jest.spyOn(console, 'error').mockImplementation(() => {});
});

afterAll(() => {
  console.error.mockRestore();
});

// Test component that uses the hook
function TestComponent() {
  const hookResult = useOverviewData();
  
  return (
    <div>
      <div data-testid="loading">{hookResult.loading.toString()}</div>
      <div data-testid="error">{hookResult.error || 'no-error'}</div>
      <div data-testid="userEntitlements">{JSON.stringify(hookResult.userEntitlements)}</div>
      <div data-testid="userName">{hookResult.userName || 'no-username'}</div>
      <div data-testid="features">{JSON.stringify(hookResult.features)}</div>
      <div data-testid="hasLLMs">{hookResult.hasLLMs.toString()}</div>
      <button
        data-testid="fetch-button"
        onClick={() => hookResult.fetchAllData()}
      >
        Fetch All Data
      </button>
    </div>
  );
}

describe('useOverviewData Hook', () => {
  // Mock implementation for the dependency hooks
  const mockFetchUserEntitlements = jest.fn();
  const mockFetchFeatures = jest.fn();
  const mockFetchLLMs = jest.fn();
  
  const mockUserEntitlements = { role: 'admin' };
  const mockUserName = 'Test User';
  const mockFeatures = { feature_chat: true, feature_gateway: true };
  const mockHasLLMs = true;
  
  beforeEach(() => {
    jest.clearAllMocks();
    
    // Setup default mock implementations
    useUserEntitlements.mockReturnValue({
      userEntitlements: mockUserEntitlements,
      userName: mockUserName,
      fetchUserEntitlements: mockFetchUserEntitlements,
      error: null
    });
    
    useSystemFeatures.mockReturnValue({
      features: mockFeatures,
      fetchFeatures: mockFetchFeatures,
      error: null
    });
    
    useLLMs.mockReturnValue({
      hasLLMs: mockHasLLMs,
      fetchLLMs: mockFetchLLMs,
      error: null
    });
    
    // Default successful promise resolutions
    mockFetchUserEntitlements.mockResolvedValue(mockUserEntitlements);
    mockFetchFeatures.mockResolvedValue(mockFeatures);
    mockFetchLLMs.mockResolvedValue({ hasLLMs: mockHasLLMs });
  });
  
  test('should initialize with loading state and fetch all data', async () => {
    render(<TestComponent />);
    
    // Initially loading should be true
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    // Verify that all fetch functions were called
    expect(mockFetchUserEntitlements).toHaveBeenCalledTimes(1);
    expect(mockFetchFeatures).toHaveBeenCalledTimes(1);
    expect(mockFetchLLMs).toHaveBeenCalledTimes(1);
    
    // Wait for the initial fetch to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Verify the data is correctly set
    expect(screen.getByTestId('error').textContent).toBe('no-error');
    expect(screen.getByTestId('userEntitlements').textContent).toBe(JSON.stringify(mockUserEntitlements));
    expect(screen.getByTestId('userName').textContent).toBe(mockUserName);
    expect(screen.getByTestId('features').textContent).toBe(JSON.stringify(mockFeatures));
    expect(screen.getByTestId('hasLLMs').textContent).toBe(mockHasLLMs.toString());
  });
  
  test('should initialize with skipInitialFetch=true when passed to dependency hooks', async () => {
    // Render the component with the hook
    render(<TestComponent />);
    
    // Verify that all hooks were initialized with skipInitialFetch=true
    expect(useUserEntitlements).toHaveBeenCalledWith(true);
    expect(useSystemFeatures).toHaveBeenCalledWith(true);
    expect(useLLMs).toHaveBeenCalledWith({ skipInitialFetch: true, checkExistenceOnly: true });
  });
  
  test('should handle manual data fetching', async () => {
    render(<TestComponent />);
    
    // Wait for the initial fetch to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Clear the mock calls from the initial fetch
    mockFetchUserEntitlements.mockClear();
    mockFetchFeatures.mockClear();
    mockFetchLLMs.mockClear();
    
    // Setup promises that won't resolve immediately to ensure loading state can be checked
    const delayedPromise = new Promise(resolve => setTimeout(() => resolve({}), 100));
    mockFetchUserEntitlements.mockReturnValue(delayedPromise);
    mockFetchFeatures.mockReturnValue(delayedPromise);
    mockFetchLLMs.mockReturnValue(delayedPromise);
    
    // Trigger manual fetch
    act(() => {
      screen.getByTestId('fetch-button').click();
    });
    
    // Wait for the loading state to be set to true
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('true');
    });
    
    // Verify that all fetch functions were called again
    expect(mockFetchUserEntitlements).toHaveBeenCalledTimes(1);
    expect(mockFetchFeatures).toHaveBeenCalledTimes(1);
    expect(mockFetchLLMs).toHaveBeenCalledTimes(1);
    
    // Wait for the fetch to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Verify the data is correctly set
    expect(screen.getByTestId('error').textContent).toBe('no-error');
  });
  
  test('should handle error from useUserEntitlements', async () => {
    const entitlementsError = 'Failed to fetch user entitlements';
    
    // Mock error from useUserEntitlements
    useUserEntitlements.mockReturnValue({
      userEntitlements: null,
      userName: null,
      fetchUserEntitlements: mockFetchUserEntitlements,
      error: entitlementsError
    });
    
    render(<TestComponent />);
    
    // Wait for the fetch to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Verify the error is correctly set
    expect(screen.getByTestId('error').textContent).toBe(entitlementsError);
  });
  
  test('should handle error from useSystemFeatures', async () => {
    const featuresError = 'Failed to fetch system features';
    
    // Mock error from useSystemFeatures
    useSystemFeatures.mockReturnValue({
      features: null,
      fetchFeatures: mockFetchFeatures,
      error: featuresError
    });
    
    render(<TestComponent />);
    
    // Wait for the fetch to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Verify the error is correctly set
    expect(screen.getByTestId('error').textContent).toBe(featuresError);
  });
  
  test('should handle error from useLLMs', async () => {
    const llmsError = 'Failed to fetch LLMs';
    
    // Mock error from useLLMs
    useLLMs.mockReturnValue({
      hasLLMs: false,
      fetchLLMs: mockFetchLLMs,
      error: llmsError
    });
    
    render(<TestComponent />);
    
    // Wait for the fetch to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Verify the error is correctly set
    expect(screen.getByTestId('error').textContent).toBe(llmsError);
  });
  
  test('should handle error during fetchAllData', async () => {
    // Mock a rejection from one of the fetch functions
    mockFetchUserEntitlements.mockRejectedValueOnce(new Error('Network error'));
    
    render(<TestComponent />);
    
    // Wait for the fetch to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Verify the error is correctly set
    expect(screen.getByTestId('error').textContent).toBe('Failed to load data');
    
    // Verify console.error was called
    expect(console.error).toHaveBeenCalledWith(
      'Error fetching overview data:',
      expect.any(Error)
    );
  });
  
  test('should prioritize errors from dependency hooks over fetchAllData error', async () => {
    const featuresError = 'Failed to fetch system features';
    
    // Mock error from useSystemFeatures
    useSystemFeatures.mockReturnValue({
      features: null,
      fetchFeatures: mockFetchFeatures,
      error: featuresError
    });
    
    // Also mock a rejection during fetchAllData
    mockFetchUserEntitlements.mockRejectedValueOnce(new Error('Network error'));
    
    render(<TestComponent />);
    
    // Wait for the fetch to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Verify the error from useSystemFeatures is prioritized
    expect(screen.getByTestId('error').textContent).toBe(featuresError);
  });
  
  test('should combine multiple errors from dependency hooks', async () => {
    const entitlementsError = 'Failed to fetch user entitlements';
    const featuresError = 'Failed to fetch system features';
    
    // Mock errors from multiple hooks
    useUserEntitlements.mockReturnValue({
      userEntitlements: null,
      userName: null,
      fetchUserEntitlements: mockFetchUserEntitlements,
      error: entitlementsError
    });
    
    useSystemFeatures.mockReturnValue({
      features: null,
      fetchFeatures: mockFetchFeatures,
      error: featuresError
    });
    
    render(<TestComponent />);
    
    // Wait for the fetch to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Verify the first error is used (entitlementsError)
    expect(screen.getByTestId('error').textContent).toBe(entitlementsError);
  });
});