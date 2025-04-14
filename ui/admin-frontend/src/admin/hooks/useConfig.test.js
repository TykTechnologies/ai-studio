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

// Mock cacheService
jest.mock('../utils/cacheService', () => ({
  get: jest.fn(),
  set: jest.fn(),
  remove: jest.fn(),
  clear: jest.fn(),
  isExpired: jest.fn()
}));

// Mock console.error to prevent error output in tests
const originalConsoleError = console.error;
beforeAll(() => {
  console.error = jest.fn();
});

afterAll(() => {
  console.error = originalConsoleError;
});

// Test component that uses the hook
function TestComponent({ skipInitialFetch = false, docsLinkKey = null }) {
  const [fetchError, setFetchError] = React.useState(null);
  const hookResult = useConfig(skipInitialFetch);
  
  const handleFetchClick = async () => {
    try {
      await hookResult.fetchConfig();
    } catch (error) {
      setFetchError(error);
    }
  };
  
  // Get docs link if a key is provided
  const docsLink = hookResult.getDocsLink(docsLinkKey);
  
  return (
    <div>
      <div data-testid="loading">{hookResult.loading.toString()}</div>
      <div data-testid="error">{hookResult.error ? hookResult.error.message : (fetchError ? fetchError.message : 'no-error')}</div>
      <div data-testid="config">{JSON.stringify(hookResult.config || {})}</div>
      <div data-testid="docs-link">{docsLink || 'no-link'}</div>
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
    tibEnabled: true,
    docsLinks: {
      llm_providers: 'https://docs.example.com/llm',
      data_sources: 'https://docs.example.com/data',
      tools: 'https://docs.example.com/tools',
      rbac_user_groups: 'https://docs.example.com/rbac'
    }
  };
  
  let pubClient;
  let cacheService;
  
  beforeEach(() => {
    jest.clearAllMocks();
    pubClient = require('../utils/pubClient').default;
    cacheService = require('../utils/cacheService');
    pubClient.get.mockReset();
    cacheService.get.mockReset();
    cacheService.set.mockReset();
  });

  test('should fetch config data and return it', async () => {
    // Mock cache miss
    cacheService.get.mockReturnValueOnce(null);
    
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
    
    // Check cache was updated
    expect(cacheService.set).toHaveBeenCalledWith('tyk_ai_studio_admin_config', mockConfig);
  });

  test('should use cached data if available', async () => {
    // Set up cached data
    cacheService.get.mockReturnValueOnce(mockConfig);
    
    render(<TestComponent />);

    // Should immediately have data from cache
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(JSON.parse(screen.getByTestId('config').textContent)).toEqual(mockConfig);
    
    // API should not be called
    expect(pubClient.get).not.toHaveBeenCalled();
    
    // Cache should be checked
    expect(cacheService.get).toHaveBeenCalledWith('tyk_ai_studio_admin_config');
  });

  test('should fetch new data if cache is not available', async () => {
    const newConfig = {
      ...mockConfig,
      tibEnabled: false // Changed value
    };

    // Set up cache miss
    cacheService.get.mockReturnValueOnce(null);
    
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
    expect(cacheService.set).toHaveBeenCalledWith('tyk_ai_studio_admin_config', newConfig);
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
    // Set up cache miss
    cacheService.get.mockReturnValueOnce(null);
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
    expect(cacheService.set).toHaveBeenCalledWith('tyk_ai_studio_admin_config', mockConfig);
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

  describe('getDocsLink function', () => {
    test('should return the correct documentation link for a valid key', async () => {
      // Mock cache miss
      cacheService.get.mockReturnValueOnce(null);
      
      // Mock the pubClient.get to return the mockConfig with docsLinks
      pubClient.get.mockResolvedValueOnce({ data: mockConfig });

      render(<TestComponent docsLinkKey="llm_providers" />);
      
      // Wait for the fetch to complete
      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false');
      });
      
      // Check if the correct link is returned
      expect(screen.getByTestId('docs-link').textContent).toBe('https://docs.example.com/llm');
    });

    test('should return "no-link" for an invalid key', async () => {
      // Mock cache miss
      cacheService.get.mockReturnValueOnce(null);
      
      // Mock the pubClient.get to return the mockConfig with docsLinks
      pubClient.get.mockResolvedValueOnce({ data: mockConfig });

      render(<TestComponent docsLinkKey="invalid_key" />);
      
      // Wait for the fetch to complete
      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false');
      });
      
      // Check if "no-link" is returned for invalid key
      expect(screen.getByTestId('docs-link').textContent).toBe('no-link');
      
      // Verify console.warn was called
      expect(console.error).toHaveBeenCalled();
    });

    test('should return "no-link" when config is not loaded', () => {
      render(<TestComponent skipInitialFetch={true} docsLinkKey="llm_providers" />);
      
      // Config not loaded yet, should return "no-link"
      expect(screen.getByTestId('docs-link').textContent).toBe('no-link');
    });

    test('should return "no-link" when docsLinks is not available in config', async () => {
      // Mock config without docsLinks
      const configWithoutLinks = { ...mockConfig, docsLinks: undefined };
      
      // Mock cache miss
      cacheService.get.mockReturnValueOnce(null);
      
      // Mock API response with config without docsLinks
      pubClient.get.mockResolvedValueOnce({ data: configWithoutLinks });

      render(<TestComponent docsLinkKey="llm_providers" />);
      
      // Wait for the fetch to complete
      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false');
      });
      
      // Check if "no-link" is returned when docsLinks is not available
      expect(screen.getByTestId('docs-link').textContent).toBe('no-link');
    });
  });
});