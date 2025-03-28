import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import WarningDialog from './WarningDialog';
import { ThemeProvider, createTheme } from '@mui/material/styles';

// Mock theme for testing
const theme = createTheme({
  palette: {
    background: {
      surfaceCriticalDefault: '#ffebee',
      buttonCritical: '#ffebee',
      surfaceDefault: '#ffffff',
      iconSuccessDefault: '#4caf50',
    },
    border: {
      criticalDefaultSubdue: '#ffcdd2',
      neutralDefault: '#e0e0e0',
      neutralHovered: '#c0c0c0',
      neutralDefaultSubdued: '#f0f0f0',
      criticalDefault: '#ff0000',
    },
    text: {
      criticalDefault: '#d32f2f',
      defaultSubdued: '#757575',
      primary: '#000000',
    },
    custom: {
      white: '#ffffff',
      emptyStateBackground: '#f5f5f5',
    },
  },
});

// Mock the Icon component
jest.mock('../../../components/common/Icon', () => {
  return function MockIcon(props) {
    return <div data-testid={`icon-${props.name}`} {...props} />;
  };
});

// Mock CloseIcon
jest.mock('@mui/icons-material/Close', () => {
  return function MockCloseIcon(props) {
    return <div data-testid="CloseIcon" {...props} />;
  };
});

const TestWrapper = ({ children }) => (
  <ThemeProvider theme={theme}>{children}</ThemeProvider>
);

describe('WarningDialog', () => {
  const defaultProps = {
    title: 'Warning',
    message: 'This action cannot be undone.',
    buttonLabel: 'Delete',
    open: true,
    onConfirm: jest.fn(),
    onCancel: jest.fn(),
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders title and message', () => {
    render(
      <TestWrapper>
        <WarningDialog {...defaultProps} />
      </TestWrapper>
    );
    
    expect(screen.getByText('Warning')).toBeInTheDocument();
    expect(screen.getByText('This action cannot be undone.')).toBeInTheDocument();
    expect(screen.getByText('Are you sure?')).toBeInTheDocument();
  });

  test('renders warning icon', () => {
    render(
      <TestWrapper>
        <WarningDialog {...defaultProps} />
      </TestWrapper>
    );
    
    expect(screen.getByTestId('icon-hexagon-exclamation')).toBeInTheDocument();
  });

  test('calls onConfirm when confirm button is clicked', () => {
    render(
      <TestWrapper>
        <WarningDialog {...defaultProps} />
      </TestWrapper>
    );
    
    const confirmButton = screen.getByRole('button', { name: 'Delete' });
    fireEvent.click(confirmButton);
    
    expect(defaultProps.onConfirm).toHaveBeenCalledTimes(1);
  });

  test('calls onCancel when cancel button is clicked', () => {
    render(
      <TestWrapper>
        <WarningDialog {...defaultProps} />
      </TestWrapper>
    );
    
    const cancelButton = screen.getByRole('button', { name: 'Cancel' });
    fireEvent.click(cancelButton);
    
    expect(defaultProps.onCancel).toHaveBeenCalledTimes(1);
  });

  test('calls onClose (or onCancel if onClose not provided) when close icon is clicked', () => {
    render(
      <TestWrapper>
        <WarningDialog {...defaultProps} />
      </TestWrapper>
    );
    
    // Find the button with the close icon
    // We need to use a more specific selector since we can't use parentElement
    const closeButton = screen.getByRole('button', { name: '' });
    fireEvent.click(closeButton);
    
    // Since onClose is not provided, onCancel should be called
    expect(defaultProps.onCancel).toHaveBeenCalledTimes(1);
  });

  test('calls custom onClose when provided and close icon is clicked', () => {
    const onCloseMock = jest.fn();
    
    render(
      <TestWrapper>
        <WarningDialog {...defaultProps} onClose={onCloseMock} />
      </TestWrapper>
    );
    
    // Find the button with the close icon
    const closeButton = screen.getByRole('button', { name: '' });
    fireEvent.click(closeButton);
    
    // onClose should be called instead of onCancel
    expect(onCloseMock).toHaveBeenCalledTimes(1);
    expect(defaultProps.onCancel).not.toHaveBeenCalled();
  });

  test('applies correct styling to components', () => {
    render(
      <TestWrapper>
        <WarningDialog {...defaultProps} data-testid="warning-dialog" />
      </TestWrapper>
    );
    
    // Dialog should be rendered
    const dialog = screen.getByRole('dialog');
    expect(dialog).toBeInTheDocument();
    
    // Title should have critical text color
    const title = screen.getByText('Warning');
    expect(title).toHaveClass('MuiTypography-headingMedium');
    
    // Message should have subdued text color
    const message = screen.getByText('This action cannot be undone.');
    expect(message).toHaveClass('MuiTypography-bodyMediumDefault');
    
    // Confirm button should be styled as danger button
    const confirmButton = screen.getByRole('button', { name: 'Delete' });
    expect(confirmButton).toHaveClass('MuiButton-root');
  });

  test('does not render when open is false', () => {
    render(
      <TestWrapper>
        <WarningDialog {...defaultProps} open={false} />
      </TestWrapper>
    );
    
    // Dialog should not be in the document
    expect(screen.queryByText('Warning')).not.toBeInTheDocument();
  });
});