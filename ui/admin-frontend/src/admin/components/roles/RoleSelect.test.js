import React from 'react';
import { render, screen, fireEvent, within } from '@testing-library/react';
import '@testing-library/jest-dom';
import RoleSelect from './RoleSelect';

jest.mock('../../services/rbacService', () => ({
  listRoles: jest.fn(),
  sortRoles: (r) => r,
}));

const roles = [
  { id: '3', attributes: { name: 'Viewer', slug: 'viewer', is_system: true, description: 'Read' } },
  { id: '4', attributes: { name: 'Auditor', slug: 'auditor', is_system: true, description: 'Audit' } },
];

// A locked role (assigned by the host application) cannot be unselected.
it('keeps locked roles selected and disabled', () => {
  const onChange = jest.fn();
  render(<RoleSelect value={[3, 4]} lockedIds={[4]} onChange={onChange} roles={roles} />);
  expect(screen.getByTestId('role-chip-locked-4')).toBeInTheDocument();

  fireEvent.mouseDown(screen.getByRole('combobox'));
  const option = screen.getByTestId('role-option-4');
  expect(option).toHaveAttribute('aria-disabled', 'true');
  expect(within(option).getByText('Assigned by the host application')).toBeInTheDocument();

  fireEvent.click(screen.getByTestId('role-option-3'));
  expect(onChange).toHaveBeenLastCalledWith([4]);
});
