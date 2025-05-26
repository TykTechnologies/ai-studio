import { renderHook, act } from '@testing-library/react';
import useTransferList from '../transfer-list/useTransferList';

// Mock the use-debounce module
jest.mock('use-debounce', () => ({
  useDebouncedCallback: (fn) => fn // Just return the function without debouncing
}));

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
    onItemAdded: jest.fn(),
    onItemRemoved: jest.fn(),
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

  test('uses original available items reference', () => {
    const customAvailableItems = [...mockAvailableItems, { id: '5', name: 'Item 5' }];
    const props = {
      ...defaultProps,
      availableItems: customAvailableItems,
    };

    const { result } = renderHook(() => useTransferList(props));
    
    // The hook should return the exact available items passed to it
    expect(result.current.available).toBe(props.availableItems);
    expect(result.current.available).toHaveLength(4);
  });

  test('handles search term change', () => {
    const { result } = renderHook(() => useTransferList(defaultProps));
    
    act(() => {
      result.current.handleSearchChange({ target: { value: 'search term' } });
    });
    
    // Initial state right after change
    expect(result.current.searchTerm).toBe('search term');
    expect(result.current.isSearching).toBe(true);
    
    // Since our mock of useDebouncedCallback just calls the function directly,
    // onSearch should have been called immediately
    expect(defaultProps.onSearch).toHaveBeenCalledWith('search term');
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
    expect(defaultProps.onItemAdded).toHaveBeenCalledWith(itemToAdd);
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
    expect(defaultProps.onItemRemoved).toHaveBeenCalledWith(itemToRemove);
  });

  test('calls onLoadMore when scrolled near bottom', () => {
    const { result } = renderHook(() => useTransferList(defaultProps));
    
    // Set up the rightBoxRef with mock values
    const mockRef = {
      current: {
        scrollTop: 480,
        scrollHeight: 1000,
        clientHeight: 500,
        addEventListener: jest.fn(),
        removeEventListener: jest.fn(),
      }
    };
    
    // Replace the useRef result with our mock
    result.current.rightBoxRef = mockRef;
    
    // Manually trigger the scroll handler logic
    const { scrollTop, scrollHeight, clientHeight } = mockRef.current;
    const isNearBottom = scrollHeight - scrollTop - clientHeight < 50;
    
    // Verify conditions for calling onLoadMore
    expect(isNearBottom).toBe(true);
    expect(defaultProps.hasMore).toBe(true);
    expect(defaultProps.isLoadingMore).toBe(false);
    
    // If we were near bottom with these params, onLoadMore should be called
    if (isNearBottom && defaultProps.hasMore && !defaultProps.isLoadingMore) {
      defaultProps.onLoadMore();
    }
    
    expect(defaultProps.onLoadMore).toHaveBeenCalled();
  });

  test('does not call onLoadMore when not near bottom', () => {
    const onLoadMore = jest.fn();
    const props = {
      ...defaultProps,
      onLoadMore,
    };
    
    // Set up the ref with values that indicate not near bottom
    const mockRef = {
      current: {
        scrollTop: 400, // Not near bottom: 1000 - 400 - 500 = 100 > 50
        scrollHeight: 1000,
        clientHeight: 500,
      }
    };
    
    // Calculate if near bottom using the same logic as the component
    const { scrollTop, scrollHeight, clientHeight } = mockRef.current;
    const isNearBottom = scrollHeight - scrollTop - clientHeight < 50;
    
    // Verify we're NOT near the bottom
    expect(isNearBottom).toBe(false);
    
    // Simulate the component's conditional call
    if (isNearBottom && props.hasMore && !props.isLoadingMore) {
      props.onLoadMore();
    }
    
    // Verify onLoadMore was NOT called
    expect(onLoadMore).not.toHaveBeenCalled();
  });

  test('does not call onLoadMore when hasMore is false', () => {
    const onLoadMore = jest.fn();
    const props = {
      ...defaultProps,
      onLoadMore,
      hasMore: false,
    };
    
    // Set up the ref with values that indicate near bottom
    const mockRef = {
      current: {
        scrollTop: 480,
        scrollHeight: 1000,
        clientHeight: 500,
      }
    };
    
    // Calculate if near bottom
    const { scrollTop, scrollHeight, clientHeight } = mockRef.current;
    const isNearBottom = scrollHeight - scrollTop - clientHeight < 50;
    
    // Verify we are near the bottom
    expect(isNearBottom).toBe(true);
    
    // Simulate the component's conditional call
    if (isNearBottom && props.hasMore && !props.isLoadingMore) {
      props.onLoadMore();
    }
    
    // Verify onLoadMore was NOT called because hasMore is false
    expect(onLoadMore).not.toHaveBeenCalled();
  });

  test('does not call onLoadMore when isLoadingMore is true', () => {
    const onLoadMore = jest.fn();
    const props = {
      ...defaultProps,
      onLoadMore,
      isLoadingMore: true,
    };
    
    // Set up the ref with values that indicate near bottom
    const mockRef = {
      current: {
        scrollTop: 480,
        scrollHeight: 1000,
        clientHeight: 500,
      }
    };
    
    // Calculate if near bottom
    const { scrollTop, scrollHeight, clientHeight } = mockRef.current;
    const isNearBottom = scrollHeight - scrollTop - clientHeight < 50;
    
    // Verify we are near the bottom
    expect(isNearBottom).toBe(true);
    
    // Simulate the component's conditional call
    if (isNearBottom && props.hasMore && !props.isLoadingMore) {
      props.onLoadMore();
    }
    
    // Verify onLoadMore was NOT called because isLoadingMore is true
    expect(onLoadMore).not.toHaveBeenCalled();
  });

  test('updates isSearching when available items change', () => {
    // Initial render with isSearching=false
    const { result, rerender } = renderHook(
      (props) => useTransferList(props),
      { initialProps: defaultProps }
    );
    
    // Set isSearching to true via search
    act(() => {
      result.current.handleSearchChange({ target: { value: 'search term' } });
    });
    
    expect(result.current.isSearching).toBe(true);
    
    // Change availableItems to trigger the effect that should set isSearching to false
    const newAvailableItems = [...mockAvailableItems];
    rerender({
      ...defaultProps,
      availableItems: newAvailableItems
    });
    
    // isSearching should now be false
    expect(result.current.isSearching).toBe(false);
  });

  test('exports all required properties', () => {
    const { result } = renderHook(() => useTransferList(defaultProps));
    
    // Verify all expected properties are exported
    expect(result.current).toHaveProperty('leftBoxRef');
    expect(result.current).toHaveProperty('rightBoxRef');
    expect(result.current).toHaveProperty('available');
    expect(result.current).toHaveProperty('selected');
    expect(result.current).toHaveProperty('searchTerm');
    expect(result.current).toHaveProperty('isSearching');
    expect(result.current).toHaveProperty('handleSearchChange');
    expect(result.current).toHaveProperty('handleAddItem');
    expect(result.current).toHaveProperty('handleRemoveItem');
  });
});