import React from 'react';
import { render, screen, waitFor, act } from '@testing-library/react';
import '@testing-library/jest-dom';
import useLicenseDaysLeft from './useLicenseDaysLeft';
import pubClient from '../utils/pubClient';
import cacheService from '../utils/cacheService';
import { CACHE_KEYS } from '../utils/constants';

// Mock pubClient
jest.mock('../utils/pubClient', () => ({
  __esModule: true,
  default: {
    get: jest.fn(),
    interceptors: {
      request: { use: jest.fn() },
      response: { use: jest.fn() }
    }
  }
}));

// Mock cacheService
jest.mock('../utils/cacheService', () => ({
  __esModule: true,
  default: {
    get: jest.fn(),
    set: jest.fn(),
    remove: jest.fn(),
    clear: jest.fn(),
    isExpired: jest.fn()
  }
}));

// Test component that uses the hook
function TestComponent({ skipInitialFetch = false }) {
  const { licenseDaysLeft, loading, error, fetchLicenseDaysLeft } = useLicenseDaysLeft(skipInitialFetch);
  
  return (
    <div>
      <div data-testid="loading">{loading.toString()}</div>
      <div data-testid="error">{error ? 'error' : 'no-error'}</div>
      <div data-testid="license-days-left">{licenseDaysLeft === null ? 'null' : licenseDaysLeft.toString()}</div>
      <button data-testid="fetch-button" onClick={fetchLicenseDaysLeft}>
        Fetch License Days Left
      </button>
    </div>
  );
}

describe('useLicenseDaysLeft hook', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(console, 'error').mockImplementation(() => {});
    // Default cache behavior - no cached data
    cacheService.get.mockReturnValue(null);
  });
  
  afterEach(() => {
    console.error.mockRestore();
  });

  test('should initialize with correct initial values', () => {
    pubClient.get.mockResolvedValueOnce({ data: { license_days_left: 30 } });
    
    render(<TestComponent />);
    
    // Initial loading state should be true
    expect(screen.getByTestId('loading').textContent).toBe('true');
    expect(screen.getByTestId('error').textContent).toBe('no-error');
    expect(screen.getByTestId('license-days-left').textContent).toBe('null');
  });
  
  test('should fetch license days left and update state', async () => {
    pubClient.get.mockResolvedValueOnce({ data: { license_days_left: 30 } });
    
    render(<TestComponent />);
    
    // Wait for the fetch to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(screen.getByTestId('error').textContent).toBe('no-error');
    expect(screen.getByTestId('license-days-left').textContent).toBe('30');
    expect(pubClient.get).toHaveBeenCalledWith('/common/system');
    expect(cacheService.set).toHaveBeenCalledWith(CACHE_KEYS.LICENSE_DAYS_LEFT, 30, 300000);
  });
  
  test('should set error state when API call fails', async () => {
    // Create a special test component for error handling
    const ErrorTestComponent = () => {
      const [fetchError, setFetchError] = React.useState(null);
      const { licenseDaysLeft, loading, error, fetchLicenseDaysLeft } = useLicenseDaysLeft(true);
      
      React.useEffect(() => {
        const fetchData = async () => {
          try {
            await fetchLicenseDaysLeft();
          } catch (err) {
            setFetchError(err);
          }
        };
        fetchData();
      }, [fetchLicenseDaysLeft]);
      
      return (
        <div>
          <div data-testid="loading">{loading.toString()}</div>
          <div data-testid="error">{error ? 'error' : 'no-error'}</div>
          <div data-testid="fetch-error">{fetchError ? 'fetch-error' : 'no-fetch-error'}</div>
        </div>
      );
    };
    
    // Set up the mock to reject
    const error = new Error('API error');
    pubClient.get.mockRejectedValueOnce(error);
    
    render(<ErrorTestComponent />);
    
    // Wait for the error to be set
    await waitFor(() => {
      expect(screen.getByTestId('error').textContent).toBe('error');
    });
    
    expect(screen.getByTestId('fetch-error').textContent).toBe('fetch-error');
    expect(console.error).toHaveBeenCalledWith('Error fetching license days left:', expect.any(Error));
  });
  
  test('should not fetch data initially when skipInitialFetch is true', () => {
    render(<TestComponent skipInitialFetch={true} />);
    
    // Should not be loading
    expect(screen.getByTestId('loading').textContent).toBe('false');
    expect(screen.getByTestId('error').textContent).toBe('no-error');
    expect(screen.getByTestId('license-days-left').textContent).toBe('null');
    
    // API should not be called
    expect(pubClient.get).not.toHaveBeenCalled();
  });
  
  test('should fetch data when fetchLicenseDaysLeft is called manually', async () => {
    pubClient.get.mockResolvedValueOnce({ data: { license_days_left: 30 } });
    
    render(<TestComponent skipInitialFetch={true} />);
    
    // Initially should not fetch
    expect(pubClient.get).not.toHaveBeenCalled();
    
    // Manually trigger fetch
    act(() => {
      screen.getByTestId('fetch-button').click();
    });
    
    // Should be loading
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(screen.getByTestId('error').textContent).toBe('no-error');
    expect(screen.getByTestId('license-days-left').textContent).toBe('30');
    expect(pubClient.get).toHaveBeenCalledWith('/common/system');
    expect(cacheService.set).toHaveBeenCalledWith(CACHE_KEYS.LICENSE_DAYS_LEFT, 30, 300000);
  });
  
  test('should handle error during manual fetch', async () => {
    // Create a component that captures the thrown error
    const ErrorButtonComponent = () => {
      const [fetchError, setFetchError] = React.useState(null);
      const { loading, error, fetchLicenseDaysLeft } = useLicenseDaysLeft(true);
      
      const handleClick = async () => {
        try {
          await fetchLicenseDaysLeft();
        } catch (err) {
          setFetchError(err);
        }
      };
      
      return (
        <div>
          <div data-testid="loading">{loading.toString()}</div>
          <div data-testid="error">{error ? 'error' : 'no-error'}</div>
          <div data-testid="fetch-error">{fetchError ? 'fetch-error' : 'no-fetch-error'}</div>
          <button data-testid="fetch-button" onClick={handleClick}>
            Fetch Data
          </button>
        </div>
      );
    };
    
    // Set up mock to reject with error
    const error = new Error('API error during manual fetch');
    pubClient.get.mockRejectedValueOnce(error);
    
    render(<ErrorButtonComponent />);
    
    // Click button to trigger fetch
    act(() => {
      screen.getByTestId('fetch-button').click();
    });
    
    // Wait for the error to be set
    await waitFor(() => {
      expect(screen.getByTestId('error').textContent).toBe('error');
    });
    
    // Check fetch error separately
    expect(screen.getByTestId('fetch-error').textContent).toBe('fetch-error');
    
    expect(console.error).toHaveBeenCalledWith('Error fetching license days left:', expect.any(Error));
  });
  
  test('should return the fetched days left value when calling fetchLicenseDaysLeft', async () => {
    pubClient.get.mockResolvedValueOnce({ data: { license_days_left: 45 } });
    
    let returnedValue = null;
    
    const FetchComponent = () => {
      const { fetchLicenseDaysLeft } = useLicenseDaysLeft(true);
      
      React.useEffect(() => {
        const fetchData = async () => {
          returnedValue = await fetchLicenseDaysLeft();
        };
        fetchData();
      }, [fetchLicenseDaysLeft]);
      
      return <div>Fetch component</div>;
    };
    
    render(<FetchComponent />);
    
    await waitFor(() => {
      expect(returnedValue).toBe(45);
    });
    
    expect(pubClient.get).toHaveBeenCalledWith('/common/system');
  });

  test('should throw and propagate error when API call fails', async () => {
    const errorMessage = 'Network error';
    const error = new Error(errorMessage);
    pubClient.get.mockRejectedValueOnce(error);
    
    let caughtError = null;
    
    const ErrorComponent = () => {
      const { fetchLicenseDaysLeft } = useLicenseDaysLeft(true);
      
      React.useEffect(() => {
        const fetchData = async () => {
          try {
            await fetchLicenseDaysLeft();
          } catch (err) {
            caughtError = err;
          }
        };
        fetchData();
      }, [fetchLicenseDaysLeft]);
      
      return <div>Error component</div>;
    };
    
    render(<ErrorComponent />);
    
    await waitFor(() => {
      expect(caughtError).not.toBeNull();
    });
    
    expect(caughtError).toBe(error);
    expect(console.error).toHaveBeenCalledWith('Error fetching license days left:', error);
  });

  test('should use cached data when available', async () => {
    // Set up cache to return data
    cacheService.get.mockReturnValueOnce(45);
    
    render(<TestComponent />);
    
    // Wait for the component to render with cached data
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Should use cached data without API call
    expect(screen.getByTestId('license-days-left').textContent).toBe('45');
    expect(pubClient.get).not.toHaveBeenCalled();
  });

  test('should fetch from API when cache is empty', async () => {
    // Ensure cache returns null
    cacheService.get.mockReturnValueOnce(null);
    pubClient.get.mockResolvedValueOnce({ data: { license_days_left: 30 } });
    
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Should fetch from API and cache the result
    expect(screen.getByTestId('license-days-left').textContent).toBe('30');
    expect(pubClient.get).toHaveBeenCalledWith('/common/system');
    expect(cacheService.set).toHaveBeenCalledWith(CACHE_KEYS.LICENSE_DAYS_LEFT, 30, 300000);
  });
});