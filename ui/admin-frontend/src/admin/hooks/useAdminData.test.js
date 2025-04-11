import { renderHook, act, waitFor } from '@testing-library/react';
import useAdminData from './useAdminData';
import useUserEntitlements from './useUserEntitlements';
import useSystemFeatures from './useSystemFeatures';
import useConfig from './useConfig';

// Mock the hooks that useAdminData depends on
jest.mock('./useUserEntitlements', () => jest.fn());
jest.mock('./useSystemFeatures', () => jest.fn());
jest.mock('./useConfig', () => jest.fn());

describe('useAdminData hook', () => {
  // Mock data for each hook
  const mockUiOptions = { show_sso_config: true };
  const mockFeatures = { feature_chat: true };
  const mockConfig = { tibEnabled: true };
  
  // Mock fetch functions
  const mockFetchUserEntitlements = jest.fn().mockResolvedValue(mockUiOptions);
  const mockFetchFeatures = jest.fn().mockResolvedValue(mockFeatures);
  const mockFetchConfig = jest.fn().mockResolvedValue(mockConfig);

  beforeEach(() => {
    jest.clearAllMocks();
    
    // Set up mock return values for each hook
    useUserEntitlements.mockReturnValue({
      uiOptions: null,
      fetchUserEntitlements: mockFetchUserEntitlements,
      error: null
    });
    
    useSystemFeatures.mockReturnValue({
      features: null,
      fetchFeatures: mockFetchFeatures,
      error: null
    });
    
    useConfig.mockReturnValue({
      config: null,
      fetchConfig: mockFetchConfig,
      error: null
    });
  });

  it('should fetch all data on initial render', async () => {
    const { result } = renderHook(() => useAdminData());
    
    // Initial state
    expect(result.current.loading).toBe(true);
    expect(result.current.error).toBe(null);
    expect(result.current.uiOptions).toBe(null);
    expect(result.current.features).toBe(null);
    expect(result.current.config).toBe(null);
    
    // All fetch functions should be called
    expect(mockFetchUserEntitlements).toHaveBeenCalled();
    expect(mockFetchFeatures).toHaveBeenCalled();
    expect(mockFetchConfig).toHaveBeenCalled();
    
    // Update the mock hooks with resolved data
    useUserEntitlements.mockReturnValue({
      uiOptions: mockUiOptions,
      fetchUserEntitlements: mockFetchUserEntitlements,
      error: null
    });
    
    useSystemFeatures.mockReturnValue({
      features: mockFeatures,
      fetchFeatures: mockFetchFeatures,
      error: null
    });
    
    useConfig.mockReturnValue({
      config: mockConfig,
      fetchConfig: mockFetchConfig,
      error: null
    });
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    // After fetch completes
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBe(null);
    expect(result.current.uiOptions).toBe(mockUiOptions);
    expect(result.current.features).toBe(mockFeatures);
    expect(result.current.config).toBe(mockConfig);
  });

  it('should handle errors from any of the hooks', async () => {
    const error = new Error('API error');
    
    // Set up one of the hooks to return an error
    useUserEntitlements.mockReturnValue({
      uiOptions: null,
      fetchUserEntitlements: mockFetchUserEntitlements,
      error: error
    });
    
    const { result } = renderHook(() => useAdminData());
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    // Should have the error from the hook
    expect(result.current.error).toBe(error);
  });

  it('should manually fetch all data when fetchAllData is called', async () => {
    // Start with data already loaded
    useUserEntitlements.mockReturnValue({
      uiOptions: mockUiOptions,
      fetchUserEntitlements: mockFetchUserEntitlements,
      error: null
    });
    
    useSystemFeatures.mockReturnValue({
      features: mockFeatures,
      fetchFeatures: mockFetchFeatures,
      error: null
    });
    
    useConfig.mockReturnValue({
      config: mockConfig,
      fetchConfig: mockFetchConfig,
      error: null
    });
    
    const { result } = renderHook(() => useAdminData());
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    // Clear the mocks to check if they're called again
    mockFetchUserEntitlements.mockClear();
    mockFetchFeatures.mockClear();
    mockFetchConfig.mockClear();
    
    // Call fetchAllData manually
    act(() => {
      result.current.fetchAllData();
    });
    
    // All fetch functions should be called again
    expect(mockFetchUserEntitlements).toHaveBeenCalled();
    expect(mockFetchFeatures).toHaveBeenCalled();
    expect(mockFetchConfig).toHaveBeenCalled();
  });

  it('should combine errors from multiple hooks', async () => {
    const error1 = new Error('API error 1');
    const error2 = new Error('API error 2');
    
    // Set up two hooks to return errors
    useUserEntitlements.mockReturnValue({
      uiOptions: null,
      fetchUserEntitlements: mockFetchUserEntitlements,
      error: error1
    });
    
    useSystemFeatures.mockReturnValue({
      features: null,
      fetchFeatures: mockFetchFeatures,
      error: error2
    });
    
    const { result } = renderHook(() => useAdminData());
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    // Should have the first error (the implementation uses || for combining errors)
    expect(result.current.error).toBe(error1);
  });
});