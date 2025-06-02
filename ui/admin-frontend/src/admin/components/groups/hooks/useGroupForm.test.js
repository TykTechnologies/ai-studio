import React from 'react';
import { render, screen, waitFor, act } from '@testing-library/react';
import { fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { useGroupForm } from './useGroupForm';
import { teamsService } from '../../../services/teamsService';
import { CACHE_KEYS } from '../../../utils/constants';

// Mock the teams service
jest.mock('../../../services/teamsService', () => ({
  teamsService: {
    getTeam: jest.fn(),
    createTeam: jest.fn(),
    updateTeam: jest.fn(),
    deleteTeam: jest.fn(),
  }
}));

// Mock useNavigate
const mockNavigate = jest.fn();
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
}));

// Create a test component that uses the hook
const TestComponent = ({ 
  id = null,
  initialCatalogs = [], 
  initialDataCatalogs = [], 
  initialToolCatalogs = [] 
}) => {
  const hookResult = useGroupForm(
    id,
    initialCatalogs,
    initialDataCatalogs,
    initialToolCatalogs
  );
  
  return (
    <div>
      <div data-testid="name">{hookResult.name}</div>
      <div data-testid="loading">{hookResult.loading.toString()}</div>
      <div data-testid="error">{hookResult.error || 'no-error'}</div>
      
      <div data-testid="selected-users">{JSON.stringify(hookResult.selectedUsers)}</div>
      <div data-testid="selected-catalogs">{JSON.stringify(hookResult.selectedCatalogs)}</div>
      <div data-testid="selected-data-catalogs">{JSON.stringify(hookResult.selectedDataCatalogs)}</div>
      <div data-testid="selected-tool-catalogs">{JSON.stringify(hookResult.selectedToolCatalogs)}</div>
      
      <div data-testid="snackbar-open">{hookResult.snackbar.open.toString()}</div>
      <div data-testid="snackbar-message">{hookResult.snackbar.message}</div>
      <div data-testid="snackbar-severity">{hookResult.snackbar.severity}</div>
      
      <div data-testid="warning-dialog-open">{hookResult.warningDialogOpen.toString()}</div>
      
      <input 
        data-testid="name-input" 
        value={hookResult.name} 
        onChange={(e) => hookResult.setName(e.target.value)} 
      />
      
      <button 
        data-testid="submit-form" 
        onClick={(e) => hookResult.handleSubmit(e)}
      >
        Submit Form
      </button>
      
      <button 
        data-testid="close-snackbar" 
        onClick={() => hookResult.handleCloseSnackbar({}, 'escapeKeyDown')}
      >
        Close Snackbar
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
    </div>
  );
};

describe('useGroupForm Hook', () => {
  // Mock data for testing
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
    localStorage.clear(); // Clear local storage before each test
    jest.useFakeTimers();
    
    // Default successful responses
    teamsService.getTeam.mockResolvedValue(mockGroupResponse);
    teamsService.createTeam.mockResolvedValue({});
    teamsService.updateTeam.mockResolvedValue({});
    teamsService.deleteTeam.mockResolvedValue({});
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  test('initializes with default values when no parameters are provided', () => {
    render(<TestComponent />);
    
    expect(screen.getByTestId('name').textContent).toBe('');
    expect(screen.getByTestId('loading').textContent).toBe('false');
    expect(screen.getByTestId('error').textContent).toBe('no-error');
    expect(screen.getByTestId('selected-users').textContent).toBe('[]');
    expect(screen.getByTestId('selected-catalogs').textContent).toBe('[]');
    expect(screen.getByTestId('selected-data-catalogs').textContent).toBe('[]');
    expect(screen.getByTestId('selected-tool-catalogs').textContent).toBe('[]');
    expect(screen.getByTestId('snackbar-open').textContent).toBe('false');
    expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
  });

  test('initializes with provided values', () => {
    render(
      <TestComponent 
        initialCatalogs={initialCatalogs}
        initialDataCatalogs={initialDataCatalogs}
        initialToolCatalogs={initialToolCatalogs}
      />
    );
    
    expect(JSON.parse(screen.getByTestId('selected-catalogs').textContent)).toEqual(initialCatalogs);
    expect(JSON.parse(screen.getByTestId('selected-data-catalogs').textContent)).toEqual(initialDataCatalogs);
    expect(JSON.parse(screen.getByTestId('selected-tool-catalogs').textContent)).toEqual(initialToolCatalogs);
  });

  test('fetches group data when ID is provided', async () => {
    render(<TestComponent id="123" />);
    
    // Initially loading should be true
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    // Wait for data to load
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Check service function was called with correct params
    expect(teamsService.getTeam).toHaveBeenCalledWith('123');
    
    // Check that the form fields are populated with the response data
    expect(screen.getByTestId('name').textContent).toBe('Test Group');
    
    // Check catalogs
    const expectedCatalogs = [
      { value: '1', label: 'Catalog 1' },
      { value: '2', label: 'Catalog 2' }
    ];
    expect(JSON.parse(screen.getByTestId('selected-catalogs').textContent)).toEqual(expectedCatalogs);
    
    // Check data catalogs
    const expectedDataCatalogs = [
      { value: '3', label: 'Data Catalog 1' },
      { value: '4', label: 'Data Catalog 2' }
    ];
    expect(JSON.parse(screen.getByTestId('selected-data-catalogs').textContent)).toEqual(expectedDataCatalogs);
    
    // Check tool catalogs
    const expectedToolCatalogs = [
      { value: '5', label: 'Tool Catalog 1' },
      { value: '6', label: 'Tool Catalog 2' }
    ];
    expect(JSON.parse(screen.getByTestId('selected-tool-catalogs').textContent)).toEqual(expectedToolCatalogs);
  });

  test('handles API fetch errors', async () => {
    // Mock API error
    const errorMessage = 'Failed to fetch group';
    teamsService.getTeam.mockRejectedValue(new Error(errorMessage));
    
    // Spy on console.error
    const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    
    render(<TestComponent id="123" />);
    
    // Initially loading should be true
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    // Wait for loading to finish (after error)
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Check error is set
    expect(screen.getByTestId('error').textContent).toBe('Failed to fetch group');
    
    // Check snackbar shows error
    expect(screen.getByTestId('snackbar-open').textContent).toBe('true');
    expect(screen.getByTestId('snackbar-message').textContent).toBe('Failed to fetch team details');
    expect(screen.getByTestId('snackbar-severity').textContent).toBe('error');
    
    expect(consoleSpy).toHaveBeenCalled();
    
    consoleSpy.mockRestore();
  });

  test('submits form to create a new group', async () => {
    render(<TestComponent />);
    
    // Set name
    fireEvent.change(screen.getByTestId('name-input'), { target: { value: 'New Group' } });
    
    // Set selections
    fireEvent.click(screen.getByTestId('set-users'));
    fireEvent.click(screen.getByTestId('set-catalogs'));
    fireEvent.click(screen.getByTestId('set-data-catalogs'));
    fireEvent.click(screen.getByTestId('set-tool-catalogs'));
    
    // Mock a successful response
    teamsService.createTeam.mockImplementation(() => {
      return Promise.resolve({});
    });
    
    // Submit form and wait for state updates
    fireEvent.click(screen.getByTestId('submit-form'));
    
    // Check loading state during API call
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    // Check that createTeam was called with the correct data
    const expectedData = {
      data: {
        type: 'Group',
        attributes: {
          name: 'New Group',
          members: [123], // Convert to number from string id
          catalogues: [456],
          data_catalogues: [789],
          tool_catalogues: [101]
        }
      }
    };
    expect(teamsService.createTeam).toHaveBeenCalledWith(expectedData);
    
    // Wait for loading to finish
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Check navigation
    expect(mockNavigate).toHaveBeenCalledWith('/admin/groups');
    
    // Check localStorage for the notification
    const notification = JSON.parse(localStorage.getItem(CACHE_KEYS.GROUP_NOTIFICATION));
    expect(notification.operation).toBe('create');
    expect(notification.message).toBe('Team created successfully');
    expect(notification.timestamp).toBeDefined();
    
    // Snackbar should not be open for success messages
    expect(screen.getByTestId('snackbar-open').textContent).toBe('false');
  });

  test('submits form to update an existing group', async () => {
    render(<TestComponent id="123" />);
    
    // Wait for data to load
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Set name and selections
    fireEvent.change(screen.getByTestId('name-input'), { target: { value: 'Updated Group' } });
    
    // Set selections
    fireEvent.click(screen.getByTestId('set-users'));
    fireEvent.click(screen.getByTestId('set-catalogs'));
    fireEvent.click(screen.getByTestId('set-data-catalogs'));
    fireEvent.click(screen.getByTestId('set-tool-catalogs'));
    
    // Mock a successful response
    teamsService.updateTeam.mockImplementation(() => {
      return Promise.resolve({});
    });
    
    // Submit form and wait for state updates
    fireEvent.click(screen.getByTestId('submit-form'));
    
    // Check loading state
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    // Check that updateTeam was called with the correct data
    const expectedData = {
      data: {
        type: 'Group',
        attributes: {
          name: 'Updated Group',
          members: [123], // Convert to number from string id
          catalogues: [456],
          data_catalogues: [789],
          tool_catalogues: [101]
        }
      }
    };
    expect(teamsService.updateTeam).toHaveBeenCalledWith('123', expectedData);
    
    // Wait for loading to finish
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Check navigation
    expect(mockNavigate).toHaveBeenCalledWith('/admin/groups');
    
    // Check localStorage for the notification
    const notification = JSON.parse(localStorage.getItem(CACHE_KEYS.GROUP_NOTIFICATION));
    expect(notification.operation).toBe('update');
    expect(notification.message).toBe('Team updated successfully');
    expect(notification.timestamp).toBeDefined();

    // Snackbar should not be open for success messages
    expect(screen.getByTestId('snackbar-open').textContent).toBe('false');
  });

  test('handles form submission errors', async () => {
    // Mock API error
    const errorMessage = 'Failed to save group';
    teamsService.createTeam.mockRejectedValue(new Error(errorMessage));
    
    // Spy on console.error
    const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    
    render(<TestComponent />);
    
    // Set name
    fireEvent.change(screen.getByTestId('name-input'), { target: { value: 'New Group' } });
    
    // Submit form
    fireEvent.click(screen.getByTestId('submit-form'));
    
    // Wait for API call to reject
    expect(screen.getByTestId('loading').textContent).toBe('true'); // Loading is true during submission
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false'); // Loading finishes after error
    });
    
    // Check error is set
    expect(screen.getByTestId('error').textContent).toBe('Failed to save group');
    
    // Check snackbar shows error
    expect(screen.getByTestId('snackbar-open').textContent).toBe('true');
    expect(screen.getByTestId('snackbar-message').textContent).toBe('Failed to save team. Please try again.');
    expect(screen.getByTestId('snackbar-severity').textContent).toBe('error');
    
    consoleSpy.mockRestore();
  });

  test('handles snackbar close', async () => {
    // Mock API error to trigger snackbar
    const errorMessage = 'Failed to save group';
    teamsService.createTeam.mockRejectedValue(new Error(errorMessage));

    render(<TestComponent />);
    
    // Set name
    fireEvent.change(screen.getByTestId('name-input'), { target: { value: 'New Group' } });
    
    // Submit form to trigger error snackbar
    fireEvent.click(screen.getByTestId('submit-form'));
    
    // Wait for loading to finish (after error)
    expect(screen.getByTestId('loading').textContent).toBe('true');
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Wait for snackbar to be open (due to error)
    await waitFor(() => {
      expect(screen.getByTestId('snackbar-open').textContent).toBe('true');
    });
    expect(screen.getByTestId('snackbar-message').textContent).toBe('Failed to save team. Please try again.');
    
    // Close snackbar
    fireEvent.click(screen.getByTestId('close-snackbar'));
    
    // Check snackbar is closed
    await waitFor(() => {
      expect(screen.getByTestId('snackbar-open').textContent).toBe('false');
    });
  });

  test('handles delete click and cancel', async () => {
    render(<TestComponent id="123" />);
    
    // Click delete
    fireEvent.click(screen.getByTestId('delete-click'));
    
    // Check dialog is open
    await waitFor(() => {
      expect(screen.getByTestId('warning-dialog-open').textContent).toBe('true');
    });
    
    // Cancel delete
    fireEvent.click(screen.getByTestId('cancel-delete'));
    
    // Check dialog is closed
    await waitFor(() => {
      expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
    });
    
    // Verify no delete call was made
    expect(teamsService.deleteTeam).not.toHaveBeenCalled();
  });

  test('handles confirm delete success', async () => {
    render(<TestComponent id="123" />);
    
    // Click delete
    fireEvent.click(screen.getByTestId('delete-click'));
    
    // Check dialog is open
    expect(screen.getByTestId('warning-dialog-open').textContent).toBe('true');
    
    // Confirm delete
    fireEvent.click(screen.getByTestId('confirm-delete'));
    
    // Check loading state
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    // Verify delete call was made
    expect(teamsService.deleteTeam).toHaveBeenCalledWith('123');
    
    // Wait for API call to resolve and loading to finish
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    // Check dialog is closed
    expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
    
    // Check localStorage for the notification
    const notification = JSON.parse(localStorage.getItem(CACHE_KEYS.GROUP_NOTIFICATION));
    expect(notification.operation).toBe('delete');
    expect(notification.message).toBe('Team deleted successfully');
    expect(notification.timestamp).toBeDefined();

    // Snackbar should not be open for success messages
    expect(screen.getByTestId('snackbar-open').textContent).toBe('false');
  });

  test('handles confirm delete error', async () => {
    // Mock API error
    const errorMessage = 'Failed to delete team';
    teamsService.deleteTeam.mockRejectedValue(new Error(errorMessage));
    
    // Spy on console.error
    const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    
    render(<TestComponent id="123" />);
    
    // Click delete
    fireEvent.click(screen.getByTestId('delete-click'));
    
    // Confirm delete
    fireEvent.click(screen.getByTestId('confirm-delete'));
    
    // Wait for API call to reject
    expect(screen.getByTestId('loading').textContent).toBe('true'); // Loading is true during delete attempt
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false'); // Loading finishes after error
    });
    
    // Check dialog is closed
    await waitFor(() => {
      expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
    });
    
    // Check snackbar shows error
    expect(screen.getByTestId('snackbar-open').textContent).toBe('true');
    expect(screen.getByTestId('snackbar-message').textContent).toBe('Failed to delete team. Please try again.');
    expect(screen.getByTestId('snackbar-severity').textContent).toBe('error');
    
    expect(consoleSpy).toHaveBeenCalled();
    
    consoleSpy.mockRestore();
  });

  test('handles state changes for users and catalogs', () => {
    render(<TestComponent />);
    
    // Set users
    fireEvent.click(screen.getByTestId('set-users'));
    expect(JSON.parse(screen.getByTestId('selected-users').textContent)).toEqual([{ id: '123', name: 'New User' }]);
    
    // Set catalogs
    fireEvent.click(screen.getByTestId('set-catalogs'));
    expect(JSON.parse(screen.getByTestId('selected-catalogs').textContent)).toEqual([{ value: '456', label: 'New Catalog' }]);
    
    // Set data catalogs
    fireEvent.click(screen.getByTestId('set-data-catalogs'));
    expect(JSON.parse(screen.getByTestId('selected-data-catalogs').textContent)).toEqual([{ value: '789', label: 'New Data Catalog' }]);
    
    // Set tool catalogs
    fireEvent.click(screen.getByTestId('set-tool-catalogs'));
    expect(JSON.parse(screen.getByTestId('selected-tool-catalogs').textContent)).toEqual([{ value: '101', label: 'New Tool Catalog' }]);
  });
});