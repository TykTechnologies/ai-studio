import { render, screen, act, fireEvent } from '@testing-library/react';
import { useNavigate } from 'react-router-dom';
import { teamsService } from '../../../services/teamsService';
import useGroupActions from './useGroupActions';

jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: jest.fn(),
}));

jest.mock('../../../services/teamsService', () => ({
  teamsService: {
    deleteTeam: jest.fn(),
  },
}));

const mockGroup = { id: '1', attributes: { name: 'Test Group' } };

const TestComponent = ({ customRefreshGroups, customSetSnackbar }) => {
  const {
    selectedGroup,
    warningDialogOpen,
    handleEdit,
    handleDelete,
    handleCancelDelete,
    handleConfirmDelete,
    handleGroupClick,
  } = useGroupActions(customRefreshGroups || jest.fn(), customSetSnackbar || jest.fn());

  return (
    <div>
      <div data-testid="selected-group">{selectedGroup ? JSON.stringify(selectedGroup) : 'null'}</div>
      <div data-testid="warning-dialog-open">{warningDialogOpen.toString()}</div>
      <button onClick={() => handleEdit(mockGroup)} data-testid="edit-button">Edit</button>
      <button onClick={() => handleEdit()} data-testid="edit-button-no-arg">Edit No Arg</button>
      <button onClick={() => handleDelete(mockGroup)} data-testid="delete-button">Delete</button>
      <button onClick={() => handleDelete(null)} data-testid="delete-button-null-arg">Delete Null Arg</button>
      <button onClick={handleCancelDelete} data-testid="cancel-delete-button">Cancel Delete</button>
      <button onClick={handleConfirmDelete} data-testid="confirm-delete-button">Confirm Delete</button>
      <button onClick={() => handleGroupClick(mockGroup)} data-testid="group-click-button">Group Click</button>
    </div>
  );
};

describe('useGroupActions', () => {
  let mockNavigate;
  let mockRefreshGroups;
  let mockSetSnackbar;

  beforeEach(() => {
    mockNavigate = jest.fn();
    useNavigate.mockReturnValue(mockNavigate);
    mockRefreshGroups = jest.fn();
    mockSetSnackbar = jest.fn();
    teamsService.deleteTeam.mockClear();
  });

  const renderTestComponent = (refreshGroups, setSnackbar) => {
    return render(<TestComponent customRefreshGroups={refreshGroups} customSetSnackbar={setSnackbar} />);
  };

  it('should initialize with default values', () => {
    renderTestComponent(mockRefreshGroups, mockSetSnackbar);
    expect(screen.getByTestId('selected-group').textContent).toBe('null');
    expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
  });

  describe('handleEdit', () => {
    it('should navigate to edit page with group id when group is provided', () => {
      renderTestComponent(mockRefreshGroups, mockSetSnackbar);
      fireEvent.click(screen.getByTestId('edit-button'));
      expect(mockNavigate).toHaveBeenCalledWith('/admin/groups/edit/1');
    });

    it('should navigate to edit page with selectedGroup id when no group is provided but selectedGroup exists', () => {
      renderTestComponent(mockRefreshGroups, mockSetSnackbar);
      fireEvent.click(screen.getByTestId('delete-button')); // Select a group first
      fireEvent.click(screen.getByTestId('edit-button-no-arg'));
      expect(mockNavigate).toHaveBeenCalledWith('/admin/groups/edit/1');
    });

    it('should not navigate if no group is provided and no group is selected', () => {
      renderTestComponent(mockRefreshGroups, mockSetSnackbar);
      fireEvent.click(screen.getByTestId('edit-button-no-arg'));
      expect(mockNavigate).not.toHaveBeenCalled();
    });
  });

  describe('handleDelete', () => {
    it('should set selectedGroup and open warning dialog', () => {
      renderTestComponent(mockRefreshGroups, mockSetSnackbar);
      fireEvent.click(screen.getByTestId('delete-button'));
      expect(screen.getByTestId('selected-group').textContent).toBe(JSON.stringify(mockGroup));
      expect(screen.getByTestId('warning-dialog-open').textContent).toBe('true');
    });

     it('should not do anything if no group is provided', () => {
      renderTestComponent(mockRefreshGroups, mockSetSnackbar);
      fireEvent.click(screen.getByTestId('delete-button-null-arg'));
      expect(screen.getByTestId('selected-group').textContent).toBe('null');
      expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
    });
  });

  describe('handleCancelDelete', () => {
    it('should close warning dialog and clear selectedGroup', () => {
      renderTestComponent(mockRefreshGroups, mockSetSnackbar);
      fireEvent.click(screen.getByTestId('delete-button')); // Open dialog first
      expect(screen.getByTestId('warning-dialog-open').textContent).toBe('true');
      expect(screen.getByTestId('selected-group').textContent).toBe(JSON.stringify(mockGroup));
      fireEvent.click(screen.getByTestId('cancel-delete-button'));
      expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
      expect(screen.getByTestId('selected-group').textContent).toBe('null');
    });
  });

  describe('handleConfirmDelete', () => {
    it('should delete group, show success snackbar, refresh groups and close dialog on successful deletion', async () => {
      teamsService.deleteTeam.mockResolvedValueOnce({});
      renderTestComponent(mockRefreshGroups, mockSetSnackbar);

      fireEvent.click(screen.getByTestId('delete-button'));
      
      // eslint-disable-next-line testing-library/no-unnecessary-act
      await act(async () => {
        fireEvent.click(screen.getByTestId('confirm-delete-button'));
      });

      expect(teamsService.deleteTeam).toHaveBeenCalledWith('1');
      expect(mockSetSnackbar).toHaveBeenCalledWith({
        open: true,
        message: 'Team "Test Group" deleted successfully!',
        severity: 'success',
      });
      expect(mockRefreshGroups).toHaveBeenCalledTimes(1);
      expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
      expect(screen.getByTestId('selected-group').textContent).toBe('null');
    });

    it('should show error snackbar and close dialog on failed deletion', async () => {
      teamsService.deleteTeam.mockRejectedValueOnce(new Error('Deletion failed'));
      renderTestComponent(mockRefreshGroups, mockSetSnackbar);
      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation(() => {});

      fireEvent.click(screen.getByTestId('delete-button'));
      
      // eslint-disable-next-line testing-library/no-unnecessary-act
      await act(async () => {
        fireEvent.click(screen.getByTestId('confirm-delete-button'));
      });

      expect(teamsService.deleteTeam).toHaveBeenCalledWith('1');
      expect(mockSetSnackbar).toHaveBeenCalledWith({
        open: true,
        message: 'Failed to delete team "Test Group".',
        severity: 'error',
      });
      expect(mockRefreshGroups).not.toHaveBeenCalled();
      expect(screen.getByTestId('warning-dialog-open').textContent).toBe('false');
      expect(screen.getByTestId('selected-group').textContent).toBe('null');
      expect(consoleErrorSpy).toHaveBeenCalledWith('Error deleting group', new Error('Deletion failed'));
      consoleErrorSpy.mockRestore();
    });

    it('should not do anything if no group is selected', async () => {
      renderTestComponent(mockRefreshGroups, mockSetSnackbar);
      
      // eslint-disable-next-line testing-library/no-unnecessary-act
      await act(async () => {
        fireEvent.click(screen.getByTestId('confirm-delete-button'));
      });

      expect(teamsService.deleteTeam).not.toHaveBeenCalled();
      expect(mockSetSnackbar).not.toHaveBeenCalled();
      expect(mockRefreshGroups).not.toHaveBeenCalled();
    });
  });

  describe('handleGroupClick', () => {
    it('should navigate to group details page', () => {
      renderTestComponent(mockRefreshGroups, mockSetSnackbar);
      fireEvent.click(screen.getByTestId('group-click-button'));
      expect(mockNavigate).toHaveBeenCalledWith('/admin/groups/1');
    });
  });
}); 