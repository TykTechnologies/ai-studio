import React from 'react';
import { render, screen } from '@testing-library/react';
import { ThemeProvider } from '@mui/material/styles';
import TeamMembersTable from './TeamMembersTable';
import theme from '../../../theme';

describe('TeamMembersTable', () => {
  const mockColumns = [
    { field: 'name', headerName: 'Name', width: '50%', renderCell: (params) => params.name },
    { field: 'role', headerName: 'Role', width: '50%', renderCell: (params) => params.role },
  ];

  it('should render without crashing', () => {
    render(
      <ThemeProvider theme={theme}>
        <TeamMembersTable rows={[]} columns={mockColumns} />
      </ThemeProvider>
    );
    expect(screen.getByText('Current members')).toBeInTheDocument();
  });

  it('should display "No team members" when there are no rows', () => {
    render(
      <ThemeProvider theme={theme}>
        <TeamMembersTable rows={[]} columns={mockColumns} />
      </ThemeProvider>
    );
    expect(screen.getByText('No team members')).toBeInTheDocument();
  });

  it('should display team members when rows are provided', () => {
    const mockRows = [
      { id: '1', name: 'John Doe', role: 'Admin' },
      { id: '2', name: 'Jane Doe', role: 'Editor' },
    ];
    render(
      <ThemeProvider theme={theme}>
        <TeamMembersTable rows={mockRows} columns={mockColumns} />
      </ThemeProvider>
    );
    expect(screen.getByText('John Doe')).toBeInTheDocument();
    expect(screen.getByText('Admin')).toBeInTheDocument();
    expect(screen.getByText('Jane Doe')).toBeInTheDocument();
    expect(screen.getByText('Editor')).toBeInTheDocument();
  });
}); 