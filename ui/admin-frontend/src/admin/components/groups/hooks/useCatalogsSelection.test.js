import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import { fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { useCatalogsSelection } from './useCatalogsSelection';
import { getCatalogues, getDataCatalogues, getToolCatalogues } from '../../../services/catalogsService';
import { getFeatureFlags } from '../../../utils/featureUtils';

jest.mock('../../../services/catalogsService', () => ({
  getCatalogues: jest.fn(),
  getDataCatalogues: jest.fn(),
  getToolCatalogues: jest.fn(),
}));

jest.mock('../../../utils/featureUtils', () => ({
  getFeatureFlags: jest.fn(),
}));

const TestComponent = ({ 
  initialCatalogs = [], 
  initialDataCatalogs = [], 
  initialToolCatalogs = [],
  features = { feature_gateway: false, feature_portal: false, feature_chat: false }
}) => {
  const hookResult = useCatalogsSelection(
    initialCatalogs,
    initialDataCatalogs,
    initialToolCatalogs,
    features
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
    
    getCatalogues.mockResolvedValue(mockCatalogs);
    getDataCatalogues.mockResolvedValue(mockDataCatalogs);
    getToolCatalogues.mockResolvedValue(mockToolCatalogs);
    
    getFeatureFlags.mockReturnValue({
      isGatewayOnly: false,
      isPortalEnabled: false,
      isChatEnabled: false
    });
  });

  test('initializes with default values when no parameters are provided', async () => {
    render(<TestComponent />);
    
    expect(screen.getByTestId('selected-catalogs').textContent).toBe('[]');
    expect(screen.getByTestId('selected-data-catalogs').textContent).toBe('[]');
    expect(screen.getByTestId('selected-tool-catalogs').textContent).toBe('[]');
    
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
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
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
  });

  test('fetches catalogs on load and updates state', async () => {
    getFeatureFlags.mockReturnValue({
      isGatewayOnly: false,
      isPortalEnabled: true,
      isChatEnabled: true
    });

    render(<TestComponent />);
    
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(getCatalogues).toHaveBeenCalledWith(1, true);
    expect(getDataCatalogues).toHaveBeenCalledWith(1, true);
    expect(getToolCatalogues).toHaveBeenCalledWith(1, true);
    
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
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    fireEvent.click(screen.getByTestId('change-catalogs'));
    
    const expectedCatalogs = [{ value: '1', label: 'Catalog 1' }];
    expect(JSON.parse(screen.getByTestId('selected-catalogs').textContent)).toEqual(expectedCatalogs);
  });

  test('handles data catalog selection changes', async () => {
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    fireEvent.click(screen.getByTestId('change-data-catalogs'));
    
    const expectedCatalogs = [{ value: '3', label: 'Data Catalog 1' }];
    expect(JSON.parse(screen.getByTestId('selected-data-catalogs').textContent)).toEqual(expectedCatalogs);
  });

  test('handles tool catalog selection changes', async () => {
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    fireEvent.click(screen.getByTestId('change-tool-catalogs'));
    
    const expectedCatalogs = [{ value: '5', label: 'Tool Catalog 1' }];
    expect(JSON.parse(screen.getByTestId('selected-tool-catalogs').textContent)).toEqual(expectedCatalogs);
  });

  test('handles API fetch errors', async () => {
    const errorMessage = 'Failed to load catalogs';
    getCatalogues.mockRejectedValue(new Error(errorMessage));
    getDataCatalogues.mockRejectedValue(new Error(errorMessage));
    getToolCatalogues.mockRejectedValue(new Error(errorMessage));
    
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(screen.getByTestId('error').textContent).toBe('Failed to load catalogs');
  });

  test('formats catalogs correctly for select component', async () => {
    getFeatureFlags.mockReturnValue({
      isGatewayOnly: false,
      isPortalEnabled: true,
      isChatEnabled: true
    });
    
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    const catalogsMissingName = [...mockCatalogs, null, { id: '7', attributes: {} }];
    
    getCatalogues.mockResolvedValue(catalogsMissingName);
    
    fireEvent.click(screen.getByTestId('fetch-catalogs'));
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    const expectedCatalogs = [
      { value: '1', label: 'Catalog 1' },
      { value: '2', label: 'Catalog 2' },
      { value: '7', label: 'Catalog 7' }
    ];
    
    expect(JSON.parse(screen.getByTestId('catalogs').textContent)).toEqual(expectedCatalogs);
  });

  test('handles non-array catalog data', async () => {
    getFeatureFlags.mockReturnValue({
      isGatewayOnly: false,
      isPortalEnabled: true,
      isChatEnabled: false
    });
    
    getCatalogues.mockResolvedValue(null);
    
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(JSON.parse(screen.getByTestId('catalogs').textContent)).toEqual([]);
  });

  test('provides fetchCatalogs method to manually refresh data', async () => {
    getFeatureFlags.mockReturnValue({
      isGatewayOnly: false,
      isPortalEnabled: true,
      isChatEnabled: true
    });
    
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    getCatalogues.mockClear();
    getDataCatalogues.mockClear();
    getToolCatalogues.mockClear();
    
    fireEvent.click(screen.getByTestId('fetch-catalogs'));
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(getCatalogues).toHaveBeenCalledWith(1, true);
    expect(getDataCatalogues).toHaveBeenCalledWith(1, true);
    expect(getToolCatalogues).toHaveBeenCalledWith(1, true);
  });

  test('does not fetch catalogs when in Gateway-only mode', async () => {
    getFeatureFlags.mockReturnValue({
      isGatewayOnly: true,
      isPortalEnabled: false,
      isChatEnabled: false
    });

    render(<TestComponent features={{ feature_gateway: true }} />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(getCatalogues).not.toHaveBeenCalled();
    expect(getDataCatalogues).not.toHaveBeenCalled();
    expect(getToolCatalogues).not.toHaveBeenCalled();
  });

  test('fetches only portal catalogs in Portal-only mode', async () => {
    getFeatureFlags.mockReturnValue({
      isGatewayOnly: false,
      isPortalEnabled: true,
      isChatEnabled: false
    });

    render(<TestComponent features={{ feature_portal: true }} />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(getCatalogues).toHaveBeenCalledWith(1, true);
    expect(getDataCatalogues).toHaveBeenCalledWith(1, true);
    expect(getToolCatalogues).not.toHaveBeenCalled();
  });

  test('fetches only chat catalogs in Chat-only mode', async () => {
    getFeatureFlags.mockReturnValue({
      isGatewayOnly: false,
      isPortalEnabled: false,
      isChatEnabled: true
    });

    render(<TestComponent features={{ feature_chat: true }} />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(getCatalogues).not.toHaveBeenCalled();
    expect(getDataCatalogues).toHaveBeenCalledWith(1, true);
    expect(getToolCatalogues).toHaveBeenCalledWith(1, true);
  });

  test('fetches all catalogs in mixed mode', async () => {
    getFeatureFlags.mockReturnValue({
      isGatewayOnly: false,
      isPortalEnabled: true,
      isChatEnabled: true
    });

    render(<TestComponent features={{ feature_portal: true, feature_chat: true }} />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(getCatalogues).toHaveBeenCalledWith(1, true);
    expect(getDataCatalogues).toHaveBeenCalledWith(1, true);
    expect(getToolCatalogues).toHaveBeenCalledWith(1, true);
  });
});