import React from 'react';
import { render, screen, act } from '@testing-library/react';
import '@testing-library/jest-dom';
import useLicenseDaysLeft from './useLicenseDaysLeft';

// Test component that uses the hook
function TestComponent() {
  const { licenseDaysLeft, loading, error, fetchLicenseDaysLeft } = useLicenseDaysLeft();
  const [fetchResult, setFetchResult] = React.useState(null);
  
  const handleFetch = async () => {
    const result = await fetchLicenseDaysLeft();
    setFetchResult(result);
  };
  
  return (
    <div>
      <div data-testid="loading">{loading.toString()}</div>
      <div data-testid="error">{error ? 'error' : 'no-error'}</div>
      <div data-testid="license-days-left">{licenseDaysLeft === null ? 'null' : licenseDaysLeft.toString()}</div>
      <div data-testid="fetch-result">{fetchResult === null ? 'null' : fetchResult.toString()}</div>
      <button data-testid="fetch-button" onClick={handleFetch}>
        Fetch License Days Left
      </button>
    </div>
  );
}

describe('useLicenseDaysLeft hook', () => {
  test('should always return null values', () => {
    render(<TestComponent />);
    
    // Should always return null/false values since licensing is removed
    expect(screen.getByTestId('loading').textContent).toBe('false');
    expect(screen.getByTestId('error').textContent).toBe('no-error');
    expect(screen.getByTestId('license-days-left').textContent).toBe('null');
  });
  
  test('fetchLicenseDaysLeft should return null', async () => {
    render(<TestComponent />);
    
    // Click the fetch button
    act(() => {
      screen.getByTestId('fetch-button').click();
    });
    
    // The fetch function should return null
    expect(screen.getByTestId('fetch-result').textContent).toBe('null');
    
    // State should remain unchanged
    expect(screen.getByTestId('loading').textContent).toBe('false');
    expect(screen.getByTestId('error').textContent).toBe('no-error');
    expect(screen.getByTestId('license-days-left').textContent).toBe('null');
  });
  
  test('hook should be backward compatible', () => {
    // Test that the hook still exports the expected shape for backward compatibility
    const HookShapeTest = () => {
      const hookResult = useLicenseDaysLeft();
      
      // Check that all expected properties exist
      expect(hookResult).toHaveProperty('licenseDaysLeft');
      expect(hookResult).toHaveProperty('loading');
      expect(hookResult).toHaveProperty('error');
      expect(hookResult).toHaveProperty('fetchLicenseDaysLeft');
      
      // Check types
      expect(hookResult.licenseDaysLeft).toBeNull();
      expect(typeof hookResult.loading).toBe('boolean');
      expect(hookResult.error).toBeNull();
      expect(typeof hookResult.fetchLicenseDaysLeft).toBe('function');
      
      return null;
    };
    
    render(<HookShapeTest />);
  });
});
