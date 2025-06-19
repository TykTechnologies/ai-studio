import { renderHook, act } from '@testing-library/react';
import { useSnackbarState } from './useSnackbarState';

describe('useSnackbarState hook', () => {
  it('should initialize with default state', () => {
    const { result } = renderHook(() => useSnackbarState());
    
    expect(result.current.snackbarState).toEqual({
      open: false,
      message: '',
      severity: 'success'
    });
    expect(typeof result.current.showSnackbar).toBe('function');
    expect(typeof result.current.hideSnackbar).toBe('function');
  });

  it('should show snackbar with message and default severity', () => {
    const { result } = renderHook(() => useSnackbarState());
    
    act(() => {
      result.current.showSnackbar('Test message');
    });
    
    expect(result.current.snackbarState).toEqual({
      open: true,
      message: 'Test message',
      severity: 'success'
    });
  });

  it('should show snackbar with message and custom severity', () => {
    const { result } = renderHook(() => useSnackbarState());
    
    act(() => {
      result.current.showSnackbar('Error message', 'error');
    });
    
    expect(result.current.snackbarState).toEqual({
      open: true,
      message: 'Error message',
      severity: 'error'
    });
  });

  it('should show snackbar with warning severity', () => {
    const { result } = renderHook(() => useSnackbarState());
    
    act(() => {
      result.current.showSnackbar('Warning message', 'warning');
    });
    
    expect(result.current.snackbarState).toEqual({
      open: true,
      message: 'Warning message',
      severity: 'warning'
    });
  });

  it('should show snackbar with info severity', () => {
    const { result } = renderHook(() => useSnackbarState());
    
    act(() => {
      result.current.showSnackbar('Info message', 'info');
    });
    
    expect(result.current.snackbarState).toEqual({
      open: true,
      message: 'Info message',
      severity: 'info'
    });
  });

  it('should hide snackbar when hideSnackbar is called', () => {
    const { result } = renderHook(() => useSnackbarState());
    
    act(() => {
      result.current.showSnackbar('Test message');
    });
    
    expect(result.current.snackbarState.open).toBe(true);
    
    act(() => {
      result.current.hideSnackbar();
    });
    
    expect(result.current.snackbarState.open).toBe(false);
    expect(result.current.snackbarState.message).toBe('Test message');
    expect(result.current.snackbarState.severity).toBe('success');
  });

  it('should not hide snackbar when reason is clickaway', () => {
    const { result } = renderHook(() => useSnackbarState());
    
    act(() => {
      result.current.showSnackbar('Test message');
    });
    
    expect(result.current.snackbarState.open).toBe(true);
    
    act(() => {
      result.current.hideSnackbar({}, 'clickaway');
    });
    
    expect(result.current.snackbarState.open).toBe(true);
  });

  it('should hide snackbar when reason is not clickaway', () => {
    const { result } = renderHook(() => useSnackbarState());
    
    act(() => {
      result.current.showSnackbar('Test message');
    });
    
    expect(result.current.snackbarState.open).toBe(true);
    
    act(() => {
      result.current.hideSnackbar({}, 'timeout');
    });
    
    expect(result.current.snackbarState.open).toBe(false);
  });

  it('should preserve message and severity when hiding snackbar', () => {
    const { result } = renderHook(() => useSnackbarState());
    
    act(() => {
      result.current.showSnackbar('Important message', 'error');
    });
    
    act(() => {
      result.current.hideSnackbar();
    });
    
    expect(result.current.snackbarState).toEqual({
      open: false,
      message: 'Important message',
      severity: 'error'
    });
  });

  it('should update state when showing new message while already open', () => {
    const { result } = renderHook(() => useSnackbarState());
    
    act(() => {
      result.current.showSnackbar('First message', 'success');
    });
    
    expect(result.current.snackbarState).toEqual({
      open: true,
      message: 'First message',
      severity: 'success'
    });
    
    act(() => {
      result.current.showSnackbar('Second message', 'error');
    });
    
    expect(result.current.snackbarState).toEqual({
      open: true,
      message: 'Second message',
      severity: 'error'
    });
  });
}); 