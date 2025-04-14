import React from 'react';
import { render, screen, waitFor, act } from '@testing-library/react';
import '@testing-library/jest-dom';
import useLLMs from './useLLMs';
import apiClient from '../utils/apiClient';

// Mock the apiClient
jest.mock('../utils/apiClient', () => ({
  __esModule: true,
  default: {
    get: jest.fn(),
  },
}));

// Mock console.error to prevent error output in tests
beforeAll(() => {
  jest.spyOn(console, 'error').mockImplementation(() => {});
});

afterAll(() => {
  console.error.mockRestore();
});

// Test component that uses the hook
function TestComponent({ hookProps = {} }) {
  const hookResult = useLLMs(hookProps);
  
  return (
    <div>
      <div data-testid="loading">{hookResult.loading.toString()}</div>
      <div data-testid="error">{hookResult.error || 'no-error'}</div>
      <div data-testid="llms">{JSON.stringify(hookResult.llms)}</div>
      <div data-testid="totalCount">{hookResult.totalCount}</div>
      <div data-testid="totalPages">{hookResult.totalPages}</div>
      <div data-testid="hasLLMs">{hookResult.hasLLMs.toString()}</div>
      <button
        data-testid="fetch-button"
        onClick={() => hookResult.fetchLLMs()}
      >
        Fetch LLMs
      </button>
      <button
        data-testid="fetch-with-options-button"
        onClick={() => hookResult.fetchLLMs({
          page: 3,
          pageSize: 5,
          sortBy: 'name',
          sortDirection: 'desc',
        })}
      >
        Fetch with options
      </button>
    </div>
  );
}

describe('useLLMs Hook', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('should initialize with default values', async () => {
    apiClient.get.mockResolvedValueOnce({
      data: { data: [] },
      headers: {},
    });
    
    render(<TestComponent />);
    
    // Initially loading should be true
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    // Wait for the initial fetch to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(screen.getByTestId('error').textContent).toBe('no-error');
    expect(screen.getByTestId('llms').textContent).toBe('[]');
    expect(screen.getByTestId('totalCount').textContent).toBe('0');
    expect(screen.getByTestId('totalPages').textContent).toBe('0');
    expect(screen.getByTestId('hasLLMs').textContent).toBe('false');
  });

  test('should initialize with skipInitialFetch=true', () => {
    render(<TestComponent hookProps={{ skipInitialFetch: true }} />);
    
    expect(screen.getByTestId('loading').textContent).toBe('false');
    expect(apiClient.get).not.toHaveBeenCalled();
  });

  test('should fetch LLMs successfully', async () => {
    const mockLLMs = [
      { id: '1', name: 'LLM 1' },
      { id: '2', name: 'LLM 2' },
    ];
    
    apiClient.get.mockResolvedValueOnce({
      data: { data: mockLLMs },
      headers: {
        'x-total-count': '10',
        'x-total-pages': '2',
      },
    });
    
    render(<TestComponent />);
    
    // Wait for the fetch to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(screen.getByTestId('llms').textContent).toBe(JSON.stringify(mockLLMs));
    expect(screen.getByTestId('totalCount').textContent).toBe('10');
    expect(screen.getByTestId('totalPages').textContent).toBe('2');
    expect(screen.getByTestId('hasLLMs').textContent).toBe('true');
    expect(screen.getByTestId('error').textContent).toBe('no-error');
    
    expect(apiClient.get).toHaveBeenCalledWith('/llms', {
      params: {
        page: 1,
        page_size: 10,
        sort_by: null,
        sort_direction: 'asc',
      },
    });
  });

  test('should handle empty response', async () => {
    apiClient.get.mockResolvedValueOnce({
      data: {},
      headers: {
        'x-total-count': '0',
        'x-total-pages': '0',
      },
    });
    
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(screen.getByTestId('llms').textContent).toBe('[]');
    expect(screen.getByTestId('hasLLMs').textContent).toBe('false');
  });

  // Create a wrapper component that catches errors
  function ErrorBoundary({ children }) {
    return (
      <React.Fragment>
        {children}
      </React.Fragment>
    );
  }

  test('should fetch LLMs with custom pagination', async () => {
    apiClient.get.mockResolvedValueOnce({
      data: { data: [] },
      headers: {},
    });
    
    render(<TestComponent hookProps={{ page: 2, pageSize: 20 }} />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(apiClient.get).toHaveBeenCalledWith('/llms', {
      params: {
        page: 2,
        page_size: 20,
        sort_by: null,
        sort_direction: 'asc',
      },
    });
  });

  test('should fetch LLMs with sorting', async () => {
    apiClient.get.mockResolvedValueOnce({
      data: { data: [] },
      headers: {},
    });
    
    render(<TestComponent hookProps={{ sortBy: 'name', sortDirection: 'desc' }} />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(apiClient.get).toHaveBeenCalledWith('/llms', {
      params: {
        page: 1,
        page_size: 10,
        sort_by: 'name',
        sort_direction: 'desc',
      },
    });
  });

  test('should fetch only one LLM when checkExistenceOnly is true', async () => {
    apiClient.get.mockResolvedValueOnce({
      data: { data: [{ id: '1', name: 'LLM 1' }] },
      headers: {},
    });
    
    render(<TestComponent hookProps={{ checkExistenceOnly: true }} />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(apiClient.get).toHaveBeenCalledWith('/llms', {
      params: {
        page: 1,
        page_size: 1,
        sort_by: null,
        sort_direction: 'asc',
      },
    });
    
    expect(screen.getByTestId('hasLLMs').textContent).toBe('true');
  });

  test('should allow manual fetching of LLMs', async () => {
    const mockLLMs = [{ id: '1', name: 'LLM 1' }];
    
    apiClient.get.mockResolvedValueOnce({
      data: { data: mockLLMs },
      headers: {},
    });
    
    render(<TestComponent hookProps={{ skipInitialFetch: true }} />);
    
    expect(apiClient.get).not.toHaveBeenCalled();
    
    // Click the fetch button
    await act(async () => {
      screen.getByTestId('fetch-button').click();
    });
    
    await waitFor(() => {
      expect(screen.getByTestId('llms').textContent).toBe(JSON.stringify(mockLLMs));
    });
    
    expect(apiClient.get).toHaveBeenCalledTimes(1);
  });

  test('should fetch with custom options when provided to fetchLLMs', async () => {
    apiClient.get.mockResolvedValueOnce({
      data: { data: [] },
      headers: {},
    }).mockResolvedValueOnce({
      data: { data: [] },
      headers: {},
    });
    
    render(<TestComponent hookProps={{
      page: 1,
      pageSize: 10,
      sortBy: 'id',
      sortDirection: 'asc'
    }} />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Click the fetch with options button
    await act(async () => {
      screen.getByTestId('fetch-with-options-button').click();
    });
    
    expect(apiClient.get).toHaveBeenCalledTimes(2);
    expect(apiClient.get).toHaveBeenLastCalledWith('/llms', {
      params: {
        page: 3,
        page_size: 5,
        sort_by: 'name',
        sort_direction: 'desc',
      },
    });
  });
});