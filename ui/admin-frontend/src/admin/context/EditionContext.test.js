import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import axios from 'axios';
import { EditionProvider, useEdition } from './EditionContext';

// Mock axios
jest.mock('axios');

// Test component that uses the context
const TestConsumer = () => {
  const { edition, version, isEnterprise, loading } = useEdition();
  return (
    <div>
      <span data-testid="edition">{edition}</span>
      <span data-testid="version">{version}</span>
      <span data-testid="isEnterprise">{String(isEnterprise)}</span>
      <span data-testid="loading">{String(loading)}</span>
    </div>
  );
};

describe('EditionContext', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    console.error.mockRestore?.();
  });

  describe('EditionProvider', () => {
    test('should provide default community edition while loading', async () => {
      axios.get.mockResolvedValueOnce({ data: { edition: 'community', version: '1.0.0' } });

      render(
        <EditionProvider>
          <TestConsumer />
        </EditionProvider>
      );

      // Initially loading
      expect(screen.getByTestId('edition')).toHaveTextContent('community');
      expect(screen.getByTestId('loading')).toHaveTextContent('true');
    });

    test('should set community edition after successful API call', async () => {
      axios.get.mockResolvedValueOnce({ data: { edition: 'community', version: '1.0.0' } });

      render(
        <EditionProvider>
          <TestConsumer />
        </EditionProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('false');
      });

      expect(screen.getByTestId('edition')).toHaveTextContent('community');
      expect(screen.getByTestId('version')).toHaveTextContent('1.0.0');
      expect(screen.getByTestId('isEnterprise')).toHaveTextContent('false');
    });

    test('should set enterprise edition when API returns enterprise', async () => {
      axios.get.mockResolvedValueOnce({ data: { edition: 'enterprise', version: '2.0.0' } });

      render(
        <EditionProvider>
          <TestConsumer />
        </EditionProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('false');
      });

      expect(screen.getByTestId('edition')).toHaveTextContent('enterprise');
      expect(screen.getByTestId('version')).toHaveTextContent('2.0.0');
      expect(screen.getByTestId('isEnterprise')).toHaveTextContent('true');
    });

    test('should default to community edition when API returns empty edition', async () => {
      axios.get.mockResolvedValueOnce({ data: {} });

      render(
        <EditionProvider>
          <TestConsumer />
        </EditionProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('false');
      });

      expect(screen.getByTestId('edition')).toHaveTextContent('community');
      expect(screen.getByTestId('isEnterprise')).toHaveTextContent('false');
      expect(screen.getByTestId('version')).toHaveTextContent('');
    });

    test('should fallback to community edition on API error', async () => {
      axios.get.mockRejectedValueOnce(new Error('Network error'));

      render(
        <EditionProvider>
          <TestConsumer />
        </EditionProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('false');
      });

      expect(screen.getByTestId('edition')).toHaveTextContent('community');
      expect(screen.getByTestId('isEnterprise')).toHaveTextContent('false');
      expect(console.error).toHaveBeenCalledWith('Error fetching edition info:', expect.any(Error));
    });

    test('should call correct API endpoint', async () => {
      axios.get.mockResolvedValueOnce({ data: { edition: 'community', version: '1.0.0' } });

      render(
        <EditionProvider>
          <TestConsumer />
        </EditionProvider>
      );

      await waitFor(() => {
        expect(axios.get).toHaveBeenCalledWith('/common/system');
      });
    });
  });

  describe('useEdition', () => {
    test('should throw error when used outside provider', () => {
      // Suppress console.error for this test since we expect an error
      const originalError = console.error;
      console.error = jest.fn();

      expect(() => {
        render(<TestConsumer />);
      }).toThrow('useEdition must be used within an EditionProvider');

      console.error = originalError;
    });
  });
});
