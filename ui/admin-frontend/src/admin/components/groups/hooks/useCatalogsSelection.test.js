import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import { fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { useCatalogsSelection } from './useCatalogsSelection';
import { getCatalogues, getDataCatalogues, getToolCatalogues } from '../../../services/catalogsService';

// Mock the catalog service functions
jest.mock('../../../services/catalogsService', () => ({
  getCatalogues: jest.fn(),
  getDataCatalogues: jest.fn(),
  getToolCatalogues: jest.fn(),
}));

// Create a test component that uses the hook
const TestComponent = ({ 
  initialCatalogs = [], 
  initialDataCatalogs = [], 
  initialToolCatalogs = [] 
}) => {
  const hookResult = useCatalogsSelection(
    initialCatalogs,
    initialDataCatalogs,
    initialToolCatalogs
  );
  
  return (
    <div>
      <div data-testid="loading">{hookResult.loading.toString()}</div>
      <div data-testid="error">{hookResult.error || 'no-error'}</div>
      
      <div data-testid="catalogs">{JSON.stringify(hookResult.catalogs)}</div>
      <div data-testid="selected-catalogs">{JSON.stringify(hookResult.selectedCatalogs)}</div>
      
      <div data-testid="data-catalogs">{JSON.stringify(hookResult.dataCatalogs)}</div>
      <div data-testid="selected-data-catalogs">{JSON.stringify(hookResult.selectedDataCatalogs)}</div>
      
      <div data-testid="tool-catalogs">{JSON.stringify(hookResult.toolCatalogs)}</div>
      <div data-testid="selected-tool-catalogs">{JSON.stringify(hookResult.selectedToolCatalogs)}</div>
      
      <button
        data-testid="change-catalogs"
        onClick={() => hookResult.setSelectedCatalogs([{ value: '1', label: 'Catalog 1' }])}
      >
        Change Catalogs
      </button>
      
      <button
        data-testid="change-data-catalogs"
        onClick={() => hookResult.setSelectedDataCatalogs([{ value: '3', label: 'Data Catalog 1' }])}
      >
        Change Data Catalogs
      </button>
      
      <button
        data-testid="change-tool-catalogs"
        onClick={() => hookResult.setSelectedToolCatalogs([{ value: '5', label: 'Tool Catalog 1' }])}
      >
        Change Tool Catalogs
      </button>
      
      <button 
        data-testid="fetch-catalogs" 
        onClick={() => hookResult.fetchCatalogs()}
      >
        Fetch Catalogs
      </button>
    </div>
  );
};

describe('useCatalogsSelection Hook', () => {
  // Mock data for testing
  const mockCatalogs = [
    { id: '1', attributes: { name: 'Catalog 1' } },
    { id: '2', attributes: { name: 'Catalog 2' } }
  ];
  
  const mockDataCatalogs = [
    { id: '3', attributes: { name: 'Data Catalog 1' } },
    { id: '4', attributes: { name: 'Data Catalog 2' } }
  ];
  
  const mockToolCatalogs = [
    { id: '5', attributes: { name: 'Tool Catalog 1' } },
    { id: '6', attributes: { name: 'Tool Catalog 2' } }
  ];

  const initialSelectedCatalogs = [{ value: '1', label: 'Catalog 1' }];
  const initialSelectedDataCatalogs = [{ value: '3', label: 'Data Catalog 1' }];
  const initialSelectedToolCatalogs = [{ value: '5', label: 'Tool Catalog 1' }];

  beforeEach(() => {
    jest.clearAllMocks();
    
    // Default successful responses
    getCatalogues.mockResolvedValue(mockCatalogs);
    getDataCatalogues.mockResolvedValue(mockDataCatalogs);
    getToolCatalogues.mockResolvedValue(mockToolCatalogs);
  });

  test('initializes with default values when no parameters are provided', async () => {
    render(<TestComponent />);
    
    expect(screen.getByTestId('selected-catalogs').textContent).toBe('[]');
    expect(screen.getByTestId('selected-data-catalogs').textContent).toBe('[]');
    expect(screen.getByTestId('selected-tool-catalogs').textContent).toBe('[]');
    
    // Initial loading state
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    // Wait for data to load
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
  });

  test('initializes with provided values', async () => {
    render(
      <TestComponent 
        initialCatalogs={initialSelectedCatalogs}
        initialDataCatalogs={initialSelectedDataCatalogs}
        initialToolCatalogs={initialSelectedToolCatalogs}
      />
    );
    
    expect(JSON.parse(screen.getByTestId('selected-catalogs').textContent)).toEqual(initialSelectedCatalogs);
    expect(JSON.parse(screen.getByTestId('selected-data-catalogs').textContent)).toEqual(initialSelectedDataCatalogs);
    expect(JSON.parse(screen.getByTestId('selected-tool-catalogs').textContent)).toEqual(initialSelectedToolCatalogs);
    
    // Wait for data to load
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
  });

  test('fetches catalogs on load and updates state', async () => {
    render(<TestComponent />);
    
    // Initially loading should be true
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    // Wait for data to load
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Check service functions were called with correct params
    expect(getCatalogues).toHaveBeenCalledWith(1, true);
    expect(getDataCatalogues).toHaveBeenCalledWith(1, true);
    expect(getToolCatalogues).toHaveBeenCalledWith(1, true);
    
    // Check that the catalogs are formatted correctly
    const expectedCatalogs = [
      { value: '1', label: 'Catalog 1' },
      { value: '2', label: 'Catalog 2' }
    ];
    
    const expectedDataCatalogs = [
      { value: '3', label: 'Data Catalog 1' },
      { value: '4', label: 'Data Catalog 2' }
    ];
    
    const expectedToolCatalogs = [
      { value: '5', label: 'Tool Catalog 1' },
      { value: '6', label: 'Tool Catalog 2' }
    ];
    
    expect(JSON.parse(screen.getByTestId('catalogs').textContent)).toEqual(expectedCatalogs);
    expect(JSON.parse(screen.getByTestId('data-catalogs').textContent)).toEqual(expectedDataCatalogs);
    expect(JSON.parse(screen.getByTestId('tool-catalogs').textContent)).toEqual(expectedToolCatalogs);
  });

  test('handles catalog selection changes', async () => {
    render(<TestComponent />);
    
    // Wait for initial data load
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Change catalogs
    fireEvent.click(screen.getByTestId('change-catalogs'));
    
    // Check updated selection
    const expectedCatalogs = [{ value: '1', label: 'Catalog 1' }];
    expect(JSON.parse(screen.getByTestId('selected-catalogs').textContent)).toEqual(expectedCatalogs);
  });

  test('handles data catalog selection changes', async () => {
    render(<TestComponent />);
    
    // Wait for initial data load
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Change data catalogs
    fireEvent.click(screen.getByTestId('change-data-catalogs'));
    
    // Check updated selection
    const expectedCatalogs = [{ value: '3', label: 'Data Catalog 1' }];
    expect(JSON.parse(screen.getByTestId('selected-data-catalogs').textContent)).toEqual(expectedCatalogs);
  });

  test('handles tool catalog selection changes', async () => {
    render(<TestComponent />);
    
    // Wait for initial data load
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Change tool catalogs
    fireEvent.click(screen.getByTestId('change-tool-catalogs'));
    
    // Check updated selection
    const expectedCatalogs = [{ value: '5', label: 'Tool Catalog 1' }];
    expect(JSON.parse(screen.getByTestId('selected-tool-catalogs').textContent)).toEqual(expectedCatalogs);
  });

  test('handles API fetch errors', async () => {
    // Mock API errors
    const errorMessage = 'Failed to load catalogs';
    getCatalogues.mockRejectedValue(new Error(errorMessage));
    getDataCatalogues.mockRejectedValue(new Error(errorMessage));
    getToolCatalogues.mockRejectedValue(new Error(errorMessage));
    
    // Spy on console.error
    const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    
    render(<TestComponent />);
    
    // Wait for loading to finish
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Check error is set
    expect(screen.getByTestId('error').textContent).toBe('Failed to load catalogs');
    
    expect(consoleSpy).toHaveBeenCalled();
    
    consoleSpy.mockRestore();
  });

  test('formats catalogs correctly for select component', async () => {
    render(<TestComponent />);
    
    // Wait for initial data load
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Test with undefined catalog and missing name
    const catalogsMissingName = [...mockCatalogs, null, { id: '7', attributes: {} }];
    
    // Setup new mock response
    getCatalogues.mockResolvedValue(catalogsMissingName);
    
    // Manually fetch catalogs again
    fireEvent.click(screen.getByTestId('fetch-catalogs'));
    
    // Wait for data to refresh
    await waitFor(() => {
      // First it will be true during loading
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Should handle null catalog and missing name
    const expectedCatalogs = [
      { value: '1', label: 'Catalog 1' },
      { value: '2', label: 'Catalog 2' },
      { value: '7', label: 'Catalog 7' }
    ];
    
    expect(JSON.parse(screen.getByTestId('catalogs').textContent)).toEqual(expectedCatalogs);
  });

  test('handles non-array catalog data', async () => {
    getCatalogues.mockResolvedValue(null);
    
    render(<TestComponent />);
    
    // Wait for data load
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(JSON.parse(screen.getByTestId('catalogs').textContent)).toEqual([]);
  });

  test('provides fetchCatalogs method to manually refresh data', async () => {
    render(<TestComponent />);
    
    // Wait for initial data load
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Clear the mocks to verify next calls
    getCatalogues.mockClear();
    getDataCatalogues.mockClear();
    getToolCatalogues.mockClear();
    
    // Call fetchCatalogs manually
    fireEvent.click(screen.getByTestId('fetch-catalogs'));
    
    // Wait for loading to finish
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Verify the service functions were called again
    expect(getCatalogues).toHaveBeenCalledWith(1, true);
    expect(getDataCatalogues).toHaveBeenCalledWith(1, true);
    expect(getToolCatalogues).toHaveBeenCalledWith(1, true);
  });
});