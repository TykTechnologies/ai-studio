import { renderHook, act } from '@testing-library/react';
import usePagination from './usePagination';

describe('usePagination', () => {
  describe('initialization', () => {
    test('should initialize with default values', () => {
      const { result } = renderHook(() => usePagination());

      expect(result.current.page).toBe(1);
      expect(result.current.pageSize).toBe(10);
      expect(result.current.totalCount).toBe(0);
      expect(result.current.totalPages).toBe(0);
    });

    test('should initialize with custom initial page', () => {
      const { result } = renderHook(() => usePagination(5));

      expect(result.current.page).toBe(5);
      expect(result.current.pageSize).toBe(10);
    });

    test('should initialize with custom initial page and page size', () => {
      const { result } = renderHook(() => usePagination(3, 25));

      expect(result.current.page).toBe(3);
      expect(result.current.pageSize).toBe(25);
    });
  });

  describe('handlePageChange', () => {
    test('should change page when called with number directly', () => {
      const { result } = renderHook(() => usePagination());

      act(() => {
        result.current.handlePageChange(5);
      });

      expect(result.current.page).toBe(5);
    });

    test('should change page when called with event and page (MUI style)', () => {
      const { result } = renderHook(() => usePagination());

      act(() => {
        result.current.handlePageChange({}, 3);
      });

      expect(result.current.page).toBe(3);
    });

    test('should handle page 1', () => {
      const { result } = renderHook(() => usePagination(5));

      act(() => {
        result.current.handlePageChange(1);
      });

      expect(result.current.page).toBe(1);
    });
  });

  describe('handlePageSizeChange', () => {
    test('should change page size and reset to page 1', () => {
      const { result } = renderHook(() => usePagination(5, 10));

      act(() => {
        result.current.handlePageSizeChange({ target: { value: 25 } });
      });

      expect(result.current.pageSize).toBe(25);
      expect(result.current.page).toBe(1);
    });

    test('should handle different page sizes', () => {
      const { result } = renderHook(() => usePagination());

      act(() => {
        result.current.handlePageSizeChange({ target: { value: 50 } });
      });

      expect(result.current.pageSize).toBe(50);
    });
  });

  describe('updatePaginationData', () => {
    test('should update total count and total pages', () => {
      const { result } = renderHook(() => usePagination());

      act(() => {
        result.current.updatePaginationData(100, 10);
      });

      expect(result.current.totalCount).toBe(100);
      expect(result.current.totalPages).toBe(10);
    });

    test('should handle zero values', () => {
      const { result } = renderHook(() => usePagination());

      act(() => {
        result.current.updatePaginationData(0, 0);
      });

      expect(result.current.totalCount).toBe(0);
      expect(result.current.totalPages).toBe(0);
    });
  });

  describe('combined operations', () => {
    test('should maintain state across multiple operations', () => {
      const { result } = renderHook(() => usePagination());

      // Update pagination data
      act(() => {
        result.current.updatePaginationData(100, 10);
      });

      // Change page
      act(() => {
        result.current.handlePageChange(5);
      });

      expect(result.current.page).toBe(5);
      expect(result.current.totalCount).toBe(100);
      expect(result.current.totalPages).toBe(10);

      // Change page size (should reset page to 1)
      act(() => {
        result.current.handlePageSizeChange({ target: { value: 50 } });
      });

      expect(result.current.page).toBe(1);
      expect(result.current.pageSize).toBe(50);
      expect(result.current.totalCount).toBe(100);
      expect(result.current.totalPages).toBe(10);
    });
  });

  describe('returned functions stability', () => {
    test('should return stable handlePageChange function', () => {
      const { result, rerender } = renderHook(() => usePagination());

      const initialHandlePageChange = result.current.handlePageChange;
      rerender();

      expect(result.current.handlePageChange).toBe(initialHandlePageChange);
    });

    test('should return stable updatePaginationData function', () => {
      const { result, rerender } = renderHook(() => usePagination());

      const initialUpdatePaginationData = result.current.updatePaginationData;
      rerender();

      expect(result.current.updatePaginationData).toBe(initialUpdatePaginationData);
    });
  });
});
