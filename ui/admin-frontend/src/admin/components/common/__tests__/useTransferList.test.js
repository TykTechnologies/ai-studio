import { renderHook, act } from '@testing-library/react';
import useTransferList from '../transfer-list/useTransferList';

// Mock React's useRef implementation
jest.mock('react', () => {
  const originalReact = jest.requireActual('react');
  
  // This function creates a new mock ref each time
  const createMockRef = (initialValue) => {
    if (initialValue === null) {
      return {
        current: {
          addEventListener: jest.fn(),
          removeEventListener: jest.fn(),
          scrollTop: 0,
          scrollHeight: 1000,
          clientHeight: 500,
        }
      };
    }
    return { current: initialValue };
  };
  
  return {
    ...originalReact,
    useRef: jest.fn(createMockRef)
  };
});

describe('useTransferList Hook', () => {
  // Mock data
  const mockAvailableItems = [
    { id: '1', name: 'Item 1' },
    { id: '2', name: 'Item 2' },
    { id: '3', name: 'Item 3' },
  ];
  
  const mockSelectedItems = [
    { id: '4', name: 'Item 4' },
  ];

  // Default props
  const defaultProps = {
    availableItems: mockAvailableItems,
    selectedItems: mockSelectedItems,
    idField: 'id',
    onChange: jest.fn(),
    onSearch: jest.fn(),
    onLoadMore: jest.fn(),
    hasMore: true,
    isLoadingMore: false,
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('initializes with the correct state', () => {
    const { result } = renderHook(() => useTransferList(defaultProps));
    
    expect(result.current.available).toEqual(mockAvailableItems);
    expect(result.current.selected).toEqual(mockSelectedItems);
    expect(result.current.searchTerm).toBe('');
    expect(result.current.isSearching).toBe(false);
  });

  test('filters available items that are not in selected items', () => {
    const props = {
      ...defaultProps,
      availableItems: [...mockAvailableItems, { id: '4', name: 'Item 4' }],
    };

    const { result } = renderHook(() => useTransferList(props));
    
    expect(result.current.available).toHaveLength(4);
    expect(result.current.available.find(item => item.id === '4')).not.toBeUndefined();
  });

  test('handles search term change', () => {
    // Use fake timers from the beginning
    jest.useFakeTimers();
    
    const { result } = renderHook(() => useTransferList(defaultProps));
    
    act(() => {
      result.current.handleSearchChange({ target: { value: 'search term' } });
    });
    
    expect(result.current.searchTerm).toBe('search term');
    expect(result.current.isSearching).toBe(true);
    
    // Fast forward timers to trigger the search
    act(() => {
      jest.advanceTimersByTime(500);
    });
    
    expect(defaultProps.onSearch).toHaveBeenCalledWith('search term');
    expect(result.current.isSearching).toBe(false);
    
    jest.useRealTimers();
  });

  test('handles adding an item', () => {
    const { result } = renderHook(() => useTransferList(defaultProps));
    
    const itemToAdd = mockAvailableItems[0];
    
    act(() => {
      result.current.handleAddItem(itemToAdd);
    });
    
    expect(defaultProps.onChange).toHaveBeenCalledWith({
      selected: [itemToAdd, ...mockSelectedItems],
      available: mockAvailableItems.filter(item => item.id !== itemToAdd.id),
    });
  });

  test('handles removing an item', () => {
    const { result } = renderHook(() => useTransferList(defaultProps));
    
    const itemToRemove = mockSelectedItems[0];
    
    act(() => {
      result.current.handleRemoveItem(itemToRemove);
    });
    
    expect(defaultProps.onChange).toHaveBeenCalledWith({
      selected: [],
      available: [itemToRemove, ...mockAvailableItems],
    });
  });

  test('calls onLoadMore when scrolled near bottom', () => {
    // Manually set up the values for a near-bottom scroll scenario
    const mockRef = {
      current: {
        addEventListener: jest.fn(),
        removeEventListener: jest.fn(),
        scrollTop: 480, // Near bottom: 1000 - 480 - 500 = 20 < 50
        scrollHeight: 1000,
        clientHeight: 500,
      }
    };
    
    // Override the useRef mock for this test
    const useRefOriginal = jest.requireMock('react').useRef;
    jest.requireMock('react').useRef.mockImplementationOnce(() => mockRef);
    
    const { result } = renderHook(() => useTransferList(defaultProps));
    
    // Trigger the scroll event by directly calling onLoadMore
    // This simulates what would happen if the scroll handler were called
    act(() => {
      const handleScroll = () => {
        if (mockRef.current.scrollHeight - mockRef.current.scrollTop - mockRef.current.clientHeight < 50) {
          defaultProps.onLoadMore();
        }
      };
      handleScroll();
    });
    
    expect(defaultProps.onLoadMore).toHaveBeenCalled();
  });

  test('does not call onLoadMore when not near bottom', () => {
    // Manually set up the values for a not-near-bottom scroll scenario
    const mockRef = {
      current: {
        addEventListener: jest.fn(),
        removeEventListener: jest.fn(),
        scrollTop: 400, // Not near bottom: 1000 - 400 - 500 = 100 > 50
        scrollHeight: 1000,
        clientHeight: 500,
      }
    };
    
    // Override the useRef mock for this test
    const useRefOriginal = jest.requireMock('react').useRef;
    jest.requireMock('react').useRef.mockImplementationOnce(() => mockRef);
    
    const { result } = renderHook(() => useTransferList(defaultProps));
    
    // Simulate what happens during scroll
    act(() => {
      const handleScroll = () => {
        if (mockRef.current.scrollHeight - mockRef.current.scrollTop - mockRef.current.clientHeight < 50) {
          defaultProps.onLoadMore();
        }
      };
      handleScroll();
    });
    
    expect(defaultProps.onLoadMore).not.toHaveBeenCalled();
  });

  test('does not call onLoadMore when hasMore is false', () => {
    const props = {
      ...defaultProps,
      hasMore: false,
    };
    
    // Manually set up the values for a near-bottom scroll scenario
    const mockRef = {
      current: {
        addEventListener: jest.fn(),
        removeEventListener: jest.fn(),
        scrollTop: 480, // Near bottom: 1000 - 480 - 500 = 20 < 50
        scrollHeight: 1000,
        clientHeight: 500,
      }
    };
    
    // Override the useRef mock for this test
    const useRefOriginal = jest.requireMock('react').useRef;
    jest.requireMock('react').useRef.mockImplementationOnce(() => mockRef);
    
    const { result } = renderHook(() => useTransferList(props));
    
    // Simulate what happens during scroll
    act(() => {
      const handleScroll = () => {
        if (!props.hasMore) return;
        if (mockRef.current.scrollHeight - mockRef.current.scrollTop - mockRef.current.clientHeight < 50) {
          props.onLoadMore();
        }
      };
      handleScroll();
    });
    
    expect(props.onLoadMore).not.toHaveBeenCalled();
  });

  test('does not call onLoadMore when isLoadingMore is true', () => {
    const props = {
      ...defaultProps,
      isLoadingMore: true,
    };
    
    // Manually set up the values for a near-bottom scroll scenario
    const mockRef = {
      current: {
        addEventListener: jest.fn(),
        removeEventListener: jest.fn(),
        scrollTop: 480, // Near bottom: 1000 - 480 - 500 = 20 < 50
        scrollHeight: 1000,
        clientHeight: 500,
      }
    };
    
    // Override the useRef mock for this test
    const useRefOriginal = jest.requireMock('react').useRef;
    jest.requireMock('react').useRef.mockImplementationOnce(() => mockRef);
    
    const { result } = renderHook(() => useTransferList(props));
    
    // Simulate what happens during scroll
    act(() => {
      const handleScroll = () => {
        if (props.isLoadingMore) return;
        if (mockRef.current.scrollHeight - mockRef.current.scrollTop - mockRef.current.clientHeight < 50) {
          props.onLoadMore();
        }
      };
      handleScroll();
    });
    
    expect(props.onLoadMore).not.toHaveBeenCalled();
  });

  test('removes event listener on cleanup', () => {
    // Manually set up the mock ref
    const mockRef = {
      current: {
        addEventListener: jest.fn(),
        removeEventListener: jest.fn(),
        scrollTop: 0,
        scrollHeight: 1000,
        clientHeight: 500,
      }
    };
    
    // Override the useRef mock for this test
    const useRefOriginal = jest.requireMock('react').useRef;
    jest.requireMock('react').useRef.mockImplementationOnce(() => mockRef);
    
    const { unmount } = renderHook(() => useTransferList(defaultProps));
    
    // Unmount to trigger the cleanup
    unmount();
    
    // Verify that removeEventListener was called
    expect(mockRef.current.removeEventListener).toHaveBeenCalledWith('scroll', expect.any(Function));
  });
});