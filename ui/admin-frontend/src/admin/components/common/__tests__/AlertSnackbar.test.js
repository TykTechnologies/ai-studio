import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import AlertSnackbar from '../AlertSnackbar';

jest.mock('@mui/material', () => require('../../../../test-utils/mui-mocks').muiMaterialMock);

const renderComponent = (props = {}) => {
  return render(
    <AlertSnackbar
      open={props.open !== undefined ? props.open : true}
      message={props.message || "Test message"}
      severity={props.severity}
      onClose={props.onClose || jest.fn()}
    />
  );
};

describe('AlertSnackbar', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders correctly with basic props', () => {
    renderComponent();
    
    expect(screen.getByTestId('snackbar')).toBeInTheDocument();
    expect(screen.getByTestId('alert')).toBeInTheDocument();
    expect(screen.getByText('Test message')).toBeInTheDocument();
  });

  it('displays the correct message', () => {
    renderComponent({ message: 'Custom alert message' });
    
    expect(screen.getByText('Custom alert message')).toBeInTheDocument();
  });

  it('passes true open prop to Snackbar', () => {
    renderComponent({ open: true });
    
    const snackbar = screen.getByTestId('snackbar');
    expect(snackbar.dataset.open).toBe('true');
  });
  
  it('passes false open prop to Snackbar', () => {
    renderComponent({ open: false });
    
    const snackbar = screen.getByTestId('snackbar');
    expect(snackbar.dataset.open).toBe('false');
  });

  it('uses correct default severity', () => {
    renderComponent();
    
    const alert = screen.getByTestId('alert');
    expect(alert.dataset.severity).toBe('success');
  });

  it('passes custom severity to Alert', () => {
    renderComponent({ severity: 'error' });
    
    const alert = screen.getByTestId('alert');
    expect(alert.dataset.severity).toBe('error');
  });

  it('sets correct anchor position', () => {
    renderComponent();
    
    const snackbar = screen.getByTestId('snackbar');
    expect(snackbar.dataset.vertical).toBe('bottom');
    expect(snackbar.dataset.horizontal).toBe('center');
  });

  it('sets correct autoHideDuration', () => {
    renderComponent();
    
    const snackbar = screen.getByTestId('snackbar');
    expect(snackbar.dataset.duration).toBe('6000');
  });

  it('calls onClose when Snackbar close button is clicked', () => {
    const onClose = jest.fn();
    renderComponent({ onClose });
    
    const closeButton = screen.getByTestId('snackbar-close-button');
    fireEvent.click(closeButton);
    
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('calls onClose when Alert close button is clicked', () => {
    const onClose = jest.fn();
    renderComponent({ onClose });
    
    const closeButton = screen.getByTestId('alert-close-button');
    fireEvent.click(closeButton);
    
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('renders with success severity', () => {
    renderComponent({ severity: 'success', message: 'success message' });
    
    const alert = screen.getByTestId('alert');
    expect(alert.dataset.severity).toBe('success');
    expect(screen.getByText('success message')).toBeInTheDocument();
  });
  
  it('renders with info severity', () => {
    renderComponent({ severity: 'info', message: 'info message' });
    
    const alert = screen.getByTestId('alert');
    expect(alert.dataset.severity).toBe('info');
    expect(screen.getByText('info message')).toBeInTheDocument();
  });
  
  it('renders with warning severity', () => {
    renderComponent({ severity: 'warning', message: 'warning message' });
    
    const alert = screen.getByTestId('alert');
    expect(alert.dataset.severity).toBe('warning');
    expect(screen.getByText('warning message')).toBeInTheDocument();
  });
  
  it('renders with error severity', () => {
    renderComponent({ severity: 'error', message: 'error message' });
    
    const alert = screen.getByTestId('alert');
    expect(alert.dataset.severity).toBe('error');
    expect(screen.getByText('error message')).toBeInTheDocument();
  });
});