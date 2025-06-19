import React from 'react';
import { screen, waitFor } from '@testing-library/react';
import { fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { renderWithTheme } from '../../../../test-utils/render-with-theme';
import { mockTeamsService, mockNavigate } from '../../../../test-utils/service-mocks';
import { useGroupForm } from './useGroupForm';
import { CACHE_KEYS } from '../../../utils/constants';

jest.mock('../../../services/teamsService', () => ({
  teamsService: mockTeamsService
}));

jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
}));

const TestComponent = ({ 
  id = null,
  initialCatalogs = [], 
  initialDataCatalogs = [], 
  initialToolCatalogs = [],
  mockShowSnackbar
}) => {
  const hookResult = useGroupForm(
    id,
    mockShowSnackbar,
    initialCatalogs,
    initialDataCatalogs,
    initialToolCatalogs
  );
  
  return (
    <div>
      <div data-testid="name">{hookResult.name}</div>
      <div data-testid="loading">{hookResult.loading.toString()}</div>
      
      <div data-testid="selected-users">{JSON.stringify(hookResult.selectedUsers)}</div>
      <div data-testid="selected-catalogs">{JSON.stringify(hookResult.selectedCatalogs)}</div>
      <div data-testid="selected-data-catalogs">{JSON.stringify(hookResult.selectedDataCatalogs)}</div>
      <div data-testid="selected-tool-catalogs">{JSON.stringify(hookResult.selectedToolCatalogs)}</div>
      
      <div data-testid="warning-dialog-open">{hookResult.warningDialogOpen.toString()}</div>
      
      <input 
        data-testid="name-input" 
        value={hookResult.name} 
        onChange={(e) => hookResult.setName(e.target.value)} 
      />
      
      <button 
        data-testid="submit-form" 
        type="submit"
        onClick={(e) => {
          e.preventDefault = jest.fn();
          hookResult.handleSubmit(e);
        }}
      >
        Submit Form
      </button>
      
      <button 
        data-testid="delete-click" 
        onClick={() => hookResult.handleDeleteClick()}
      >
        Delete
      </button>
      
      <button 
        data-testid="cancel-delete" 
        onClick={() => hookResult.handleCancelDelete()}
      >
        Cancel Delete
      </button>
      
      <button 
        data-testid="confirm-delete" 
        onClick={() => hookResult.handleConfirmDelete()}
      >
        Confirm Delete
      </button>
      
      <button 
        data-testid="set-users"
        onClick={() => hookResult.setSelectedUsers([{ id: '123', name: 'New User' }])}
      >
        Set Users
      </button>
      
      <button 
        data-testid="set-catalogs"
        onClick={() => hookResult.setSelectedCatalogs([{ value: '456', label: 'New Catalog' }])}
      >
        Set Catalogs
      </button>
      
      <button 
        data-testid="set-data-catalogs"
        onClick={() => hookResult.setSelectedDataCatalogs([{ value: '789', label: 'New Data Catalog' }])}
      >
        Set Data Catalogs
      </button>
      
      <button 
        data-testid="set-tool-catalogs"
        onClick={() => hookResult.setSelectedToolCatalogs([{ value: '101', label: 'New Tool Catalog' }])}
      >
        Set Tool Catalogs
      </button>
      
      <div data-testid="show-snackbar-calls">{mockShowSnackbar.mock.calls.length}</div>
      <div data-testid="last-snackbar-call">
        {mockShowSnackbar.mock.calls.length > 0 ? 
          JSON.stringify(mockShowSnackbar.mock.calls[mockShowSnackbar.mock.calls.length - 1]) : 
          'null'
        }
      </div>
    </div>
  );
};

describe('useGroupForm Hook', () => {
  const mockGroupResponse = {
    data: {
      attributes: {
        name: 'Test Group',
        users: [
          { id: '1', name: 'User 1' },
          { id: '2', name: 'User 2' }
        ],
        catalogues: [
          { id: '1', attributes: { name: 'Catalog 1' } },
          { id: '2', attributes: { name: 'Catalog 2' } }
        ],
        data_catalogues: [
          { id: '3', attributes: { name: 'Data Catalog 1' } },
          { id: '4', attributes: { name: 'Data Catalog 2' } }
        ],
        tool_catalogues: [
          { id: '5', attributes: { name: 'Tool Catalog 1' } },
          { id: '6', attributes: { name: 'Tool Catalog 2' } }
        ]
      }
    }
  };

  const initialCatalogs = [{ value: '1', label: 'Catalog 1' }];
  const initialDataCatalogs = [{ value: '3', label: 'Data Catalog 1' }];
  const initialToolCatalogs = [{ value: '5', label: 'Tool Catalog 1' }];

  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    
    jest.spyOn(require('../../../services/utils/errorHandler'), 'handleApiError').mockImplementation(e => ({
      message: e.message || 'API Error'
    }));
    
    mockTeamsService.getTeam.mockResolvedValue(mockGroupResponse);
    mockTeamsService.createTeam.mockResolvedValue({});
    mockTeamsService.updateTeam.mockResolvedValue({});
    mockTeamsService.deleteTeam.mockResolvedValue({});
  });

  test('initializes with default values when no parameters are provided', () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent mockShowSnackbar={mockShowSnackbar} />);
    
    expect(screen.getByTestId('name').textContent).toBe('');
    expect(screen.getByTestId('loading').textContent).toBe('false');
    expect(screen.getByTestId('selected-users').textContent).toBe('[]');
    expect(screen.getByTestId('selected-catalogs').textContent).toBe('[]');
    expect(screen.getByTestId('selected-data-catalogs').textContent).toBe('[]');
    expect(screen.getByTestId('selected-tool-catalogs').textContent).toBe('[]');
    expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
  });

  test('initializes with provided values', () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(
      <TestComponent 
        initialCatalogs={initialCatalogs}
        initialDataCatalogs={initialDataCatalogs}
        initialToolCatalogs={initialToolCatalogs}
        mockShowSnackbar={mockShowSnackbar}
      />
    );
    
    expect(JSON.parse(screen.getByTestId('selected-catalogs').textContent)).toEqual(initialCatalogs);
    expect(JSON.parse(screen.getByTestId('selected-data-catalogs').textContent)).toEqual(initialDataCatalogs);
    expect(JSON.parse(screen.getByTestId('selected-tool-catalogs').textContent)).toEqual(initialToolCatalogs);
  });

  test('fetches group data when ID is provided', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent id="123" mockShowSnackbar={mockShowSnackbar} />);
    
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(mockTeamsService.getTeam).toHaveBeenCalledWith('123');
    
    expect(screen.getByTestId('name').textContent).toBe('Test Group');
    
    const expectedCatalogs = [
      { value: '1', label: 'Catalog 1' },
      { value: '2', label: 'Catalog 2' }
    ];
    expect(JSON.parse(screen.getByTestId('selected-catalogs').textContent)).toEqual(expectedCatalogs);
    
    const expectedDataCatalogs = [
      { value: '3', label: 'Data Catalog 1' },
      { value: '4', label: 'Data Catalog 2' }
    ];
    expect(JSON.parse(screen.getByTestId('selected-data-catalogs').textContent)).toEqual(expectedDataCatalogs);
    
    const expectedToolCatalogs = [
      { value: '5', label: 'Tool Catalog 1' },
      { value: '6', label: 'Tool Catalog 2' }
    ];
    expect(JSON.parse(screen.getByTestId('selected-tool-catalogs').textContent)).toEqual(expectedToolCatalogs);
  });

  test('handles API fetch errors', async () => {
    const errorMessage = 'Failed to fetch team details';
    mockTeamsService.getTeam.mockRejectedValue(new Error(errorMessage));
    
    const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    const mockShowSnackbar = jest.fn();
    
    renderWithTheme(<TestComponent id="123" mockShowSnackbar={mockShowSnackbar} />);
    
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(screen.getByTestId('show-snackbar-calls').textContent).toBe('1');
    const lastCall = JSON.parse(screen.getByTestId('last-snackbar-call').textContent);
    expect(lastCall[0]).toBe(errorMessage);
    expect(lastCall[1]).toBe('error');
    
    expect(consoleSpy).toHaveBeenCalled();
    
    consoleSpy.mockRestore();
  });

  test('submits form to create a new group', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent mockShowSnackbar={mockShowSnackbar} />);
    
    fireEvent.change(screen.getByTestId('name-input'), { target: { value: 'New Group' } });
    
    fireEvent.click(screen.getByTestId('set-users'));
    fireEvent.click(screen.getByTestId('set-catalogs'));
    fireEvent.click(screen.getByTestId('set-data-catalogs'));
    fireEvent.click(screen.getByTestId('set-tool-catalogs'));
    
    const mockEvent = { preventDefault: jest.fn() };
    fireEvent.click(screen.getByTestId('submit-form'));
    
    const expectedData = {
      data: {
        type: 'Group',
        attributes: {
          name: 'New Group',
          members: [123],
          catalogues: [456],
          data_catalogues: [789],
          tool_catalogues: [101]
        }
      }
    };

    await waitFor(() => {
      expect(mockTeamsService.createTeam).toHaveBeenCalledWith(expectedData);
    });
    
    expect(mockNavigate).toHaveBeenCalledWith('/admin/groups');
    
    const notification = JSON.parse(localStorage.getItem(CACHE_KEYS.GROUP_NOTIFICATION));
    expect(notification.operation).toBe('create');
    expect(notification.message).toBe('Team created successfully');
    expect(notification.timestamp).toBeDefined();
  });

  test('submits form to update an existing group', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent id="123" mockShowSnackbar={mockShowSnackbar} />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    fireEvent.change(screen.getByTestId('name-input'), { target: { value: 'Updated Group' } });
    
    fireEvent.click(screen.getByTestId('set-users'));
    fireEvent.click(screen.getByTestId('set-catalogs'));
    fireEvent.click(screen.getByTestId('set-data-catalogs'));
    fireEvent.click(screen.getByTestId('set-tool-catalogs'));
    
    fireEvent.click(screen.getByTestId('submit-form'));
    
    const expectedData = {
      data: {
        type: 'Group',
        attributes: {
          name: 'Updated Group',
          members: [123],
          catalogues: [456],
          data_catalogues: [789],
          tool_catalogues: [101]
        }
      }
    };

    await waitFor(() => {
      expect(mockTeamsService.updateTeam).toHaveBeenCalledWith('123', expectedData);
    });
    
    expect(mockNavigate).toHaveBeenCalledWith('/admin/groups');
    
    const notification = JSON.parse(localStorage.getItem(CACHE_KEYS.GROUP_NOTIFICATION));
    expect(notification.operation).toBe('update');
    expect(notification.message).toBe('Team updated successfully');
    expect(notification.timestamp).toBeDefined();
  });

  test('handles form submission errors', async () => {
    const errorMessage = 'Failed to save team. Please try again.';
    mockTeamsService.createTeam.mockRejectedValue(new Error(errorMessage));
    
    const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    const mockShowSnackbar = jest.fn();
    
    renderWithTheme(<TestComponent mockShowSnackbar={mockShowSnackbar} />);
    
    fireEvent.change(screen.getByTestId('name-input'), { target: { value: 'New Group' } });
    
    fireEvent.click(screen.getByTestId('set-users'));
    fireEvent.click(screen.getByTestId('set-catalogs'));
    fireEvent.click(screen.getByTestId('set-data-catalogs'));
    fireEvent.click(screen.getByTestId('set-tool-catalogs'));
    
    const submitButton = screen.getByTestId('submit-form');
    fireEvent.click(submitButton);
    
    await waitFor(() => {
      expect(mockTeamsService.createTeam).toHaveBeenCalled();
    }, { timeout: 3000 });
    
    await waitFor(() => {
      expect(mockShowSnackbar).toHaveBeenCalled();
    }, { timeout: 3000 });
    
    expect(mockShowSnackbar).toHaveBeenCalledWith(errorMessage, 'error');
    
    consoleSpy.mockRestore();
  });

  test('handles delete click and cancel', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent id="123" mockShowSnackbar={mockShowSnackbar} />);
    
    fireEvent.click(screen.getByTestId('delete-click'));
    
    await waitFor(() => {
      expect(screen.getByTestId('warning-dialog-open').textContent).toBe('true');
    });
    
    fireEvent.click(screen.getByTestId('cancel-delete'));
    
    await waitFor(() => {
      expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
    });
    
    expect(mockTeamsService.deleteTeam).not.toHaveBeenCalled();
  });

  test('handles confirm delete success', async () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent id="123" mockShowSnackbar={mockShowSnackbar} />);
    
    fireEvent.click(screen.getByTestId('delete-click'));
    
    expect(screen.getByTestId('warning-dialog-open').textContent).toBe('true');
    
    fireEvent.click(screen.getByTestId('confirm-delete'));
    
    await waitFor(() => {
      expect(mockTeamsService.deleteTeam).toHaveBeenCalledWith('123');
    });
    
    await waitFor(() => {
      expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
    });
    
    const notification = JSON.parse(localStorage.getItem(CACHE_KEYS.GROUP_NOTIFICATION));
    expect(notification.operation).toBe('delete');
    expect(notification.message).toBe('Team deleted successfully');
    expect(notification.timestamp).toBeDefined();
  });

  test('handles confirm delete error', async () => {
    const errorMessage = 'Failed to delete team. Please try again.';
    mockTeamsService.deleteTeam.mockRejectedValue(new Error(errorMessage));
    
    const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    const mockShowSnackbar = jest.fn();
    
    renderWithTheme(<TestComponent id="123" mockShowSnackbar={mockShowSnackbar} />);
    
    fireEvent.click(screen.getByTestId('delete-click'));
    
    fireEvent.click(screen.getByTestId('confirm-delete'));
    
    await waitFor(() => {
      expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
    });
    
    await waitFor(() => {
      expect(screen.getByTestId('show-snackbar-calls').textContent).toBe('1');
    });
    
    const lastCall = JSON.parse(screen.getByTestId('last-snackbar-call').textContent);
    expect(lastCall[0]).toBe(errorMessage);
    expect(lastCall[1]).toBe('error');
    
    expect(consoleSpy).toHaveBeenCalled();
    
    consoleSpy.mockRestore();
  });

  test('handles state changes for users and catalogs', () => {
    const mockShowSnackbar = jest.fn();
    renderWithTheme(<TestComponent mockShowSnackbar={mockShowSnackbar} />);
    
    fireEvent.click(screen.getByTestId('set-users'));
    expect(JSON.parse(screen.getByTestId('selected-users').textContent)).toEqual([{ id: '123', name: 'New User' }]);
    
    fireEvent.click(screen.getByTestId('set-catalogs'));
    expect(JSON.parse(screen.getByTestId('selected-catalogs').textContent)).toEqual([{ value: '456', label: 'New Catalog' }]);
    
    fireEvent.click(screen.getByTestId('set-data-catalogs'));
    expect(JSON.parse(screen.getByTestId('selected-data-catalogs').textContent)).toEqual([{ value: '789', label: 'New Data Catalog' }]);
    
    fireEvent.click(screen.getByTestId('set-tool-catalogs'));
    expect(JSON.parse(screen.getByTestId('selected-tool-catalogs').textContent)).toEqual([{ value: '101', label: 'New Tool Catalog' }]);
  });
});