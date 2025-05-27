import React from 'react';
import { render, screen, waitFor, act } from '@testing-library/react';
import { fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { useTransferListSelectedUsers } from './useTransferListSelectedUsers';
import { teamsService } from '../services/teamsService';

jest.mock('../services/teamsService', () => ({
  teamsService: {
    getTeamUsers: jest.fn()
  }
}));

const TestComponent = ({ groupId = '123', idField = 'id' }) => {
  const hookResult = useTransferListSelectedUsers({
    groupId,
    idField
  });
  
  return (
    <div>
      <div data-testid="members">{JSON.stringify(hookResult.members)}</div>
      <div data-testid="loading">{hookResult.loading.toString()}</div>
      
      <button 
        data-testid="add-member-button" 
        onClick={() => hookResult.addMember({ id: '999', name: 'New Member' })}
      >
        Add Member
      </button>
      
      <button 
        data-testid="add-duplicate-button" 
        onClick={() => hookResult.addMember(hookResult.members[0] || { id: '999', name: 'New Member' })}
      >
        Add Duplicate
      </button>
      
      <button 
        data-testid="remove-member-button" 
        onClick={() => {
          if (hookResult.members.length > 0) {
            hookResult.removeMember(hookResult.members[0]);
          }
        }}
      >
        Remove Member
      </button>
      
      <button 
        data-testid="reset-button" 
        onClick={() => hookResult.reset([{ id: '888', name: 'Reset Member' }])}
      >
        Reset
      </button>
      
      <button
        data-testid="set-members-button"
        onClick={() => hookResult.setMembers([{ id: '777', name: 'Set Member' }])}
      >
        Set Members
      </button>
    </div>
  );
};

describe('useTransferListSelectedUsers Hook', () => {
  const mockMembers = [
    { id: '1', name: 'User 1' },
    { id: '2', name: 'User 2' },
    { id: '3', name: 'User 3' }
  ];
  
  beforeEach(() => {
    jest.clearAllMocks();
    teamsService.getTeamUsers.mockResolvedValue({
      data: mockMembers
    });
  });
  
  test('initializes with loading state and fetches members', async () => {
    render(<TestComponent />);
    
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    expect(teamsService.getTeamUsers).toHaveBeenCalledWith('123', { all: true });
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    const membersElement = screen.getByTestId('members');
    const members = JSON.parse(membersElement.textContent);
    expect(members).toEqual(mockMembers);
  });
  
  test('adds a member successfully', async () => {
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    fireEvent.click(screen.getByTestId('add-member-button'));
    
    const membersElement = screen.getByTestId('members');
    const members = JSON.parse(membersElement.textContent);
    
    expect(members[0].id).toBe('999');
    expect(members[0].name).toBe('New Member');
    expect(members.length).toBe(mockMembers.length + 1);
  });
  
  test('prevents adding duplicate members', async () => {
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    fireEvent.click(screen.getByTestId('add-duplicate-button'));
    
    const membersElement = screen.getByTestId('members');
    const membersAfterClick = JSON.parse(membersElement.textContent);
    
    expect(membersAfterClick).toEqual(mockMembers);
  });
  
  test('removes a member successfully', async () => {
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    fireEvent.click(screen.getByTestId('remove-member-button'));
    
    const membersElement = screen.getByTestId('members');
    const members = JSON.parse(membersElement.textContent);
    
    expect(members.find(member => member.id === '1')).toBeUndefined();
    expect(members.length).toBe(mockMembers.length - 1);
  });
  
  test('resets members list successfully', async () => {
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    fireEvent.click(screen.getByTestId('reset-button'));
    
    const membersElement = screen.getByTestId('members');
    const members = JSON.parse(membersElement.textContent);
    
    expect(members).toEqual([{ id: '888', name: 'Reset Member' }]);
  });
  
  test('directly sets members list with setMembers', async () => {
    render(<TestComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    fireEvent.click(screen.getByTestId('set-members-button'));
    
    const membersElement = screen.getByTestId('members');
    const members = JSON.parse(membersElement.textContent);
    
    expect(members).toEqual([{ id: '777', name: 'Set Member' }]);
  });
  
  test('handles API fetch errors', async () => {
    teamsService.getTeamUsers.mockRejectedValue(new Error('Failed to fetch team users'));
    
    render(<TestComponent />);
    
    expect(screen.getByTestId('loading').textContent).toBe('true');
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    const membersElement = screen.getByTestId('members');
    const members = JSON.parse(membersElement.textContent);
    expect(members).toEqual([]);
  });
  
  test('does not fetch if groupId is not provided', async () => {
    render(<TestComponent groupId={null} />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    expect(teamsService.getTeamUsers).not.toHaveBeenCalled();
    
    const membersElement = screen.getByTestId('members');
    const members = JSON.parse(membersElement.textContent);
    expect(members).toEqual([]);
  });
  
  test('uses custom idField for comparison', async () => {
    const CustomIdFieldComponent = () => {
      const { members, addMember, removeMember, loading } = useTransferListSelectedUsers({
        groupId: null,
        idField: 'customId'
      });
      
      return (
        <div>
          <div data-testid="members">{JSON.stringify(members)}</div>
          <div data-testid="loading">{loading.toString()}</div>
          <button data-testid="add-unique" onClick={() => addMember({ customId: '123', name: 'Unique' })}>
            Add Unique
          </button>
          <button data-testid="add-duplicate" onClick={() => addMember({ customId: '123', name: 'Duplicate' })}>
            Add Duplicate
          </button>
          <button data-testid="remove-by-id" onClick={() => removeMember({ customId: '123' })}>
            Remove By ID
          </button>
        </div>
      );
    };
    
    render(<CustomIdFieldComponent />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    fireEvent.click(screen.getByTestId('add-unique'));
    
    let membersElement = screen.getByTestId('members');
    let members = JSON.parse(membersElement.textContent);
    expect(members.length).toBe(1);
    expect(members[0]).toEqual({ customId: '123', name: 'Unique' });
    
    fireEvent.click(screen.getByTestId('add-duplicate'));
    
    membersElement = screen.getByTestId('members');
    members = JSON.parse(membersElement.textContent);
    expect(members.length).toBe(1);
    expect(members[0]).toEqual({ customId: '123', name: 'Unique' });
    
    fireEvent.click(screen.getByTestId('remove-by-id'));
    
    membersElement = screen.getByTestId('members');
    members = JSON.parse(membersElement.textContent);
    expect(members.length).toBe(0);
  });
  
  test('refetches members when groupId changes', async () => {
    const { rerender } = render(<TestComponent groupId="123" />);
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    teamsService.getTeamUsers.mockReset();
    
    const newGroupMembers = [
      { id: '4', name: 'User 4' },
      { id: '5', name: 'User 5' }
    ];
    
    teamsService.getTeamUsers.mockResolvedValue({
      data: newGroupMembers
    });
    
    rerender(<TestComponent groupId="456" />);
    
    expect(teamsService.getTeamUsers).toHaveBeenCalledWith('456', { all: true });
    
    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false');
    });
    
    await waitFor(() => {
      const membersElement = screen.getByTestId('members');
      const members = JSON.parse(membersElement.textContent);
      expect(members).toEqual(newGroupMembers);
    });
  });
});