import React from 'react';
import { render, fireEvent, screen } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import GroupDeleteDialog from './GroupDeleteDialog';

const mockTheme = createTheme({
  palette: {
    mode: 'light',
    primary: { // Added primary for default button colors if needed
      main: '#23E2C2', 
    },
    border: {
      neutralDefault: '#D8D8DF',
      criticalDefault: '#AE2410', // Added for critical button styles
    },
    text: {
      defaultSubdued: '#414160',
      primary: '#03031C',
    },
    background: {
      paper: '#FFFFFF',
      buttonPrimaryDefaultHover: '#181834',
      default: '#FFFFFF', // Added for completeness, though paper is often key
      buttonCritical: '#D82C0D', // Added for critical button styles
    },
    custom: { // Added custom palette
      white: '#FFFFFF',
    },
    // Minimal stub for other potentially accessed palette properties
    action: { // Often used by buttons for hover/disabled states
      active: 'rgba(0, 0, 0, 0.54)',
      hover: 'rgba(0, 0, 0, 0.04)',
      selected: 'rgba(0, 0, 0, 0.08)',
      disabled: 'rgba(0, 0, 0, 0.26)',
      disabledBackground: 'rgba(0, 0, 0, 0.12)',
    }
  },
  typography: {
    fontFamily: 'Inter-Regular, sans-serif',
    // Provide stubs for variants if needed, or rely on defaults
    // For instance, if a specific variant like 'body1' is used directly by a component:
    body1: { fontSize: '1rem' },
    button: { textTransform: 'none' } // Common override for buttons
  },
  components: {
    MuiSvgIcon: { // Stub for SvgIcon if used
      styleOverrides: {
        root: {
          width: '20px',
          height: '20px',
          fontSize: '20px',
        },
      },
    },
  },
  // Spacing function stub if used directly by styled components outside of theme.spacing
  spacing: (factor) => `${factor * 8}px`,
});

const renderWithTheme = (component) => {
  return render(<ThemeProvider theme={mockTheme}>{component}</ThemeProvider>);
};

describe('GroupDeleteDialog', () => {
  const mockOnConfirm = jest.fn();
  const mockOnCancel = jest.fn();

  beforeEach(() => {
    mockOnConfirm.mockClear();
    mockOnCancel.mockClear();
  });

  const selectedGroup = {
    attributes: {
      name: 'Test Group',
    },
  };

  it('renders correctly when open with a selected group', () => {
    renderWithTheme(
      <GroupDeleteDialog
        open={true}
        selectedGroup={selectedGroup}
        onConfirm={mockOnConfirm}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.getByText('Delete Team')).toBeInTheDocument();
    expect(screen.getByText('Deleting team "Test Group" will remove all users from it.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete team' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
  });

  it('renders correctly when open without a selected group', () => {
    renderWithTheme(
      <GroupDeleteDialog
        open={true}
        selectedGroup={null}
        onConfirm={mockOnConfirm}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.getByText('Delete Team')).toBeInTheDocument();
    expect(screen.getByText('Deleting this team will remove all users from it.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete team' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
  });

  it('calls onConfirm when the confirm button is clicked', () => {
    renderWithTheme(
      <GroupDeleteDialog
        open={true}
        selectedGroup={selectedGroup}
        onConfirm={mockOnConfirm}
        onCancel={mockOnCancel}
      />
    );

    fireEvent.click(screen.getByRole('button', { name: 'Delete team' }));
    expect(mockOnConfirm).toHaveBeenCalledTimes(1);
  });

  it('calls onCancel when the cancel button is clicked', () => {
    renderWithTheme(
      <GroupDeleteDialog
        open={true}
        selectedGroup={selectedGroup}
        onConfirm={mockOnConfirm}
        onCancel={mockOnCancel}
      />
    );

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(mockOnCancel).toHaveBeenCalledTimes(1);
  });

  it('does not render when open is false', () => {
    renderWithTheme(
      <GroupDeleteDialog
        open={false}
        selectedGroup={selectedGroup}
        onConfirm={mockOnConfirm}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.queryByText('Delete Team')).toBeNull();
    expect(screen.queryByText('Deleting team "Test Group" will remove all users from it.')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Delete team' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Cancel' })).toBeNull();
  });
}); 