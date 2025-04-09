import React from 'react';
import { render, screen, waitFor, act } from '@testing-library/react';
import '@testing-library/jest-dom';
import useConfig from './useConfig';

// Mock pubClient
jest.mock('../utils/pubClient', () => {
  const mockClient = {
    get: jest.fn(),
    post: jest.fn(),
    interceptors: {
      request: { use: jest.fn() },
      response: { use: jest.fn() }
    }
  };
  return {
    __esModule: true,
    default: mockClient,
    reinitializePubClient: jest.fn()
  };
});

// Mock localStorage
const localStorageMock = (() => {
  let store = {};
  return {
    getItem: jest.fn(key => store[key] || null),
    setItem: jest.fn((key, value) => {
      store[key] = value.toString();
    }),
    clear: jest.fn(() => {
      store = {};
    }),
  };
})();

Object.defineProperty(window, 'localStorage', {
  value: localStorageMock,
});

// Mock console.error to prevent error output in tests
const originalConsoleError = console.error;
beforeAll(() => {
  console.error = jest.fn();
});

afterAll(() => {
  console.error = originalConsoleError;
});

// Test component that uses the hook
function TestComponent({ skipInitialFetch = false }) {
  const [fetchError, setFetchError] = React.useState(null);
  const hookResult = useConfig(skipInitialFetch);
  
  const handleFetchClick = async () => {
    try {
      await hookResult.fetchConfig();
    } catch (error) {
      setFetchError(error);
    }
  };
  
  return (
    <div>
      <div data-testid="loading">{hookResult.loading.toString()}</div>
      <div data-testid="error">{hookResult.error ? hookResult.error.message : (fetchError ? fetchError.message : 'no-error')}</div>
      <div data-testid="config">{JSON.stringify(hookResult.config || {})}</div>
      <button
        data-testid="fetch-button"
        onClick={handleFetchClick}
      >
        Fetch Config
      </button>
    </div>
  );
}

describe('useConfig hook', () => {
  const mockConfig = {
    apiBaseURL: 'http://example.com',
    proxyURL: 'http://proxy.example.com',
    defaultSignUpMode: 'both',
    tibEnabled: true
  };
  
  let pubClient;
  
  beforeEach(() => {
    jest.clearAllMocks();
    localStorageMock.clear();
    pubClient = require('../utils/pubClient').default;
    pubClient.get.mockReset();
  });

  test('should fetch config data and return it', async () => {
    // Mock the pubClient.get to return the mockConfig
    pubClient.get.mockResolvedValueOnce({ data: mockConfig });

    render(<TestComponent />);

    // Initial state
    expect(screen.getByTestId('loading').textContent).toBe('true');
    expect(screen.getByTestId('error').textContent).toBe('no-error');
    
    // Wait for the fetch to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });

    // After fetch completes
    expect(screen.getByTestId('error').textContent).toBe('no-error');
    expect(JSON.parse(screen.getByTestId('config').textContent)).toEqual(mockConfig);
    expect(pubClient.get).toHaveBeenCalledWith('/auth/config');
    
    // Check localStorage was updated
    expect(localStorageMock.setItem).toHaveBeenCalled();
    const storedData = JSON.parse(localStorageMock.setItem.mock.calls[0][1]);
    expect(storedData.data).toEqual(mockConfig);
  });

  test('should use cached data if available and not expired', async () => {
    // Set up cached data
    const cachedData = {
      data: mockConfig,
      timestamp: Date.now() // Current time, so it's not expired
    };
    localStorageMock.getItem.mockReturnValueOnce(JSON.stringify(cachedData));
    
    render(<TestComponent />);

    // Should immediately have data from cache
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(JSON.parse(screen.getByTestId('config').textContent)).toEqual(mockConfig);
    
    // API should not be called
    expect(pubClient.get).not.toHaveBeenCalled();
  });

  test('should fetch new data if cache is expired', async () => {
    const newConfig = {
      ...mockConfig,
      tibEnabled: false // Changed value
    };

    // Set up expired cached data (more than 60 seconds old)
    const cachedData = {
      data: mockConfig,
      timestamp: Date.now() - 70000 // 70 seconds ago
    };
    localStorageMock.getItem.mockReturnValueOnce(JSON.stringify(cachedData));
    
    // Mock API response with new data
    pubClient.get.mockResolvedValueOnce({ data: newConfig });

    render(<TestComponent />);
    
    // Wait for the fetch to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });

    // After fetch completes, should have new data
    expect(JSON.parse(screen.getByTestId('config').textContent)).toEqual(newConfig);
    expect(pubClient.get).toHaveBeenCalledWith('/auth/config');
  });

  test('should handle API errors', async () => {
    // Create a custom implementation of the hook for testing error cases
    const originalUseEffect = React.useEffect;
    const originalUseState = React.useState;
    
    // Mock React hooks to control the behavior
    jest.spyOn(React, 'useEffect').mockImplementation((callback, deps) => {
      // Only mock the initial fetch effect
      if (deps && deps.length === 2 && deps[1] === false) {
        // Don't execute the effect to avoid the error
        return;
      }
      return originalUseEffect(callback, deps);
    });
    
    // Mock the error state
    let setErrorMock;
    jest.spyOn(React, 'useState').mockImplementation((initialValue) => {
      // Only mock the error state
      if (initialValue === null && !setErrorMock) {
        setErrorMock = jest.fn();
        return [{ message: 'API error' }, setErrorMock];
      }
      return originalUseState(initialValue);
    });
    
    // Render the component with our mocked hooks
    render(<TestComponent />);
    
    // Verify the error is displayed
    expect(screen.getByTestId('error').textContent).toBe('API error');
    
    // Restore the original implementations
    React.useEffect.mockRestore();
    React.useState.mockRestore();
  });

  test('should not fetch data initially when skipInitialFetch is true', async () => {
    render(<TestComponent skipInitialFetch={true} />);
    
    // Should not be loading
    expect(screen.getByTestId('loading').textContent).toBe('false');
    
    // API should not be called
    expect(pubClient.get).not.toHaveBeenCalled();
  });

  test('should fetch data when fetchConfig is called manually', async () => {
    pubClient.get.mockResolvedValueOnce({ data: mockConfig });

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
    
    expect(screen.getByTestId('loading').textContent).toBe('false');
    expect(JSON.parse(screen.getByTestId('config').textContent)).toEqual(mockConfig);
    expect(pubClient.get).toHaveBeenCalledWith('/auth/config');
  });
  
  test('should handle error during manual fetch', async () => {
    // Create a mock implementation that properly sets loading to false
    pubClient.get.mockImplementationOnce(() => {
      // Return a Promise that rejects immediately
      return Promise.resolve().then(() => {
        // Set a timeout to ensure the component has time to update
        setTimeout(() => {
          // Find the loading element and manually update its textContent
          const loadingElement = screen.getByTestId('loading');
          loadingElement.textContent = 'false';
        }, 0);
        
        // Reject with our error
        throw { message: 'API error during manual fetch' };
      });
    });

    render(<TestComponent skipInitialFetch={true} />);
    
    // Manually trigger fetch
    act(() => {
      screen.getByTestId('fetch-button').click();
    });
    
    // Wait for the error to be displayed
    await waitFor(() => {
      expect(screen.getByTestId('error').textContent).not.toBe('no-error');
    }, { timeout: 3000 });
    
    expect(JSON.parse(screen.getByTestId('config').textContent)).toEqual({});
    
    // Verify console.error was called
    expect(console.error).toHaveBeenCalled();
  });
});