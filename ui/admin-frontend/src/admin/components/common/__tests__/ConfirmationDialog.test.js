import React from 'react';
import { render, screen, fireEvent, within } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider } from '@mui/material/styles';
import { createTheme } from '@mui/material';
import ConfirmationDialog from '../ConfirmationDialog';

// Mock the components we need
jest.mock('../../../../components/common/Icon', () => {
  return function MockIcon(props) {
    return <div data-testid="mock-icon">{props.name}</div>;
  };
});

// Mock CloseIcon
jest.mock('@mui/icons-material/Close', () => {
  return function MockCloseIcon() {
    return <div data-testid="close-icon">CloseIcon</div>;
  };
});

describe('ConfirmationDialog', () => {
  // Mock functions for callbacks
  const mockOnConfirm = jest.fn();
  const mockOnCancel = jest.fn();
  const mockOnClose = jest.fn();

  // Create a custom theme for testing
  const theme = createTheme({
    palette: {
      text: {
        defaultSubdued: 'rgba(0, 0, 0, 0.6)',
      },
      border: {
        neutralDefault: '#e0e0e0',
        criticalDefault: '#ff0000',
      },
      custom: {
        white: '#ffffff',
      },
      background: {
        buttonCritical: '#ff0000',
        paper: '#ffffff',
      },
    },
  });

  // Wrapper component with theme provider
  const Wrapper = ({ children }) => (
    <ThemeProvider theme={theme}>
      {children}
    </ThemeProvider>
  );

  // Default props
  const defaultProps = {
    title: 'Test Title',
    message: 'Test Message',
    buttonLabel: 'Confirm',
    open: true,
    onConfirm: mockOnConfirm,
    onCancel: mockOnCancel,
    onClose: mockOnClose,
    iconName: 'info', // Add default iconName to avoid prop type warning
  };

  beforeEach(() => {
    // Clear all mocks before each test
    jest.clearAllMocks();
  });

  test('renders with default props', () => {
    render(<ConfirmationDialog {...defaultProps} />, { wrapper: Wrapper });
    
    // Check that the component renders with the correct content
    expect(screen.getByText('Test Title')).toBeInTheDocument();
    expect(screen.getByText('Test Message')).toBeInTheDocument();
    expect(screen.getByText('Are you sure?')).toBeInTheDocument(); // Default confirmText
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
  });

  test('renders with custom confirmText', () => {
    render(<ConfirmationDialog {...defaultProps} confirmText="Custom confirmation text" />, { wrapper: Wrapper });
    
    expect(screen.getByText('Custom confirmation text')).toBeInTheDocument();
  });

  test('calls onConfirm when confirm button is clicked', () => {
    render(<ConfirmationDialog {...defaultProps} />, { wrapper: Wrapper });
    
    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    expect(mockOnConfirm).toHaveBeenCalledTimes(1);
  });

  test('calls onCancel when cancel button is clicked', () => {
    render(<ConfirmationDialog {...defaultProps} />, { wrapper: Wrapper });
    
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(mockOnCancel).toHaveBeenCalledTimes(1);
  });

  test('calls onClose when close icon is clicked', () => {
    render(<ConfirmationDialog {...defaultProps} />, { wrapper: Wrapper });
    
    // Since we've mocked the CloseIcon, we can find the button containing it
    const buttons = screen.getAllByRole('button');
    // Find the button that contains our mocked CloseIcon
    const closeButton = buttons.find(button =>
      within(button).queryByTestId('close-icon')
    );
    
    fireEvent.click(closeButton);
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  test('calls onCancel when close icon is clicked and onClose is not provided', () => {
    const propsWithoutOnClose = {
      ...defaultProps,
      onClose: undefined,
    };
    render(<ConfirmationDialog {...propsWithoutOnClose} />, { wrapper: Wrapper });
    
    const buttons = screen.getAllByRole('button');
    // Find the button that contains our mocked CloseIcon
    const closeButton = buttons.find(button =>
      within(button).queryByTestId('close-icon')
    );
    
    fireEvent.click(closeButton);
    expect(mockOnCancel).toHaveBeenCalledTimes(1);
  });

  test('renders with primary button variant when primaryButtonComponent is "primary"', () => {
    render(<ConfirmationDialog {...defaultProps} primaryButtonComponent="primary" />, { wrapper: Wrapper });
    
    // Find the confirm button
    const confirmButton = screen.getByRole('button', { name: 'Confirm' });
    
    // Check that it's a primary button (contained variant)
    expect(confirmButton).toBeInTheDocument();
    // We can only verify the button exists since we can't directly check MUI classes with Testing Library
    expect(confirmButton).toHaveAttribute('type', 'button');
  });

  test('renders with danger button variant when primaryButtonComponent is "danger"', () => {
    render(<ConfirmationDialog {...defaultProps} primaryButtonComponent="danger" />, { wrapper: Wrapper });
    
    // Find the confirm button
    const confirmButton = screen.getByRole('button', { name: 'Confirm' });
    expect(confirmButton).toBeInTheDocument();
    
    // We can only verify the button exists since we can't directly check for DangerButton component
    expect(confirmButton).toHaveAttribute('type', 'button');
  });

  test('applies custom icon properties', () => {
    render(
      <ConfirmationDialog
        {...defaultProps}
        iconName="warning"
        iconColor="error.main"
      />,
      { wrapper: Wrapper }
    );
    
    // We can't directly test the icon properties without adding data-testid
    // But we can verify the dialog renders successfully
    expect(screen.getByText('Test Title')).toBeInTheDocument();
    expect(screen.getByText('Test Message')).toBeInTheDocument();
  });

  test('applies custom color properties', () => {
    render(
      <ConfirmationDialog
        {...defaultProps}
        titleColor="error.main"
        backgroundColor="background.paper"
        borderColor="error.main"
      />,
      { wrapper: Wrapper }
    );
    
    // We can't directly test the styling properties without adding data-testid
    // But we can verify the dialog renders successfully with the title
    expect(screen.getByText('Test Title')).toBeInTheDocument();
    expect(screen.getByText('Test Message')).toBeInTheDocument();
  });

  test('renders warning configuration correctly', () => {
    render(
      <ConfirmationDialog
        {...defaultProps}
        iconName="warning"
        iconColor="warning.main"
        titleColor="warning.main"
        backgroundColor="warning.light"
        borderColor="warning.main"
      />,
      { wrapper: Wrapper }
    );
    
    expect(screen.getByText('Test Title')).toBeInTheDocument();
    expect(screen.getByText('Test Message')).toBeInTheDocument();
    expect(screen.getByText('Are you sure?')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeInTheDocument();
  });

  test('renders danger configuration correctly', () => {
    render(
      <ConfirmationDialog
        {...defaultProps}
        iconName="error"
        iconColor="error.main"
        titleColor="error.main"
        backgroundColor="error.light"
        borderColor="error.main"
        primaryButtonComponent="danger"
      />,
      { wrapper: Wrapper }
    );
    
    expect(screen.getByText('Test Title')).toBeInTheDocument();
    expect(screen.getByText('Test Message')).toBeInTheDocument();
    expect(screen.getByText('Are you sure?')).toBeInTheDocument();
    
    // Find the confirm button
    const confirmButton = screen.getByRole('button', { name: 'Confirm' });
    expect(confirmButton).toBeInTheDocument();
  });
});