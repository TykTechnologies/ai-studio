import { findParentItemsForPath, generateRandomId, saveSelectedPath } from './utils';

// Mock localStorage for saveSelectedPath tests
const createMockStorage = () => {
  let store = {};
  return {
    getItem: jest.fn(key => store[key]),
    setItem: jest.fn((key, value) => {
      store[key] = value;
    }),
    clear: jest.fn(() => {
      store = {};
    }),
    removeItem: jest.fn(key => {
      delete store[key];
    }),
    getStore: () => ({ ...store }),
  };
};

let mockStorage;

beforeEach(() => {
  mockStorage = createMockStorage();
  Object.defineProperty(window, 'localStorage', {
    value: mockStorage,
    configurable: true
  });
  jest.clearAllMocks();
});

describe('findParentItemsForPath', () => {
  // Test data
  const testItems = [
    {
      id: 'dashboard',
      text: 'Dashboard',
      path: '/admin',
    },
    {
      id: 'team',
      text: 'Team',
      subItems: [
        {
          id: 'users',
          text: 'Users',
          path: '/admin/users',
        },
        {
          id: 'groups',
          text: 'Groups',
          path: '/admin/groups',
        }
      ],
    },
    {
      id: 'settings',
      text: 'Settings',
      subItems: [
        {
          id: 'general',
          text: 'General',
          path: '/admin/settings/general',
        },
        {
          id: 'security',
          text: 'Security',
          path: '/admin/settings/security',
        }
      ],
    },
    {
      id: 'noSubItems',
      text: 'No SubItems',
    },
    {
      // Item without ID
      text: 'No ID',
      subItems: [
        {
          id: 'child',
          text: 'Child',
          path: '/admin/no-id/child',
        }
      ],
    }
  ];

  it('should find parent items with exact path match', () => {
    const result = findParentItemsForPath(testItems, '/admin/users');
    expect(result).toEqual(['team']);
  });

  it('should find parent items with path prefix match', () => {
    const result = findParentItemsForPath(testItems, '/admin/users/detail/123');
    expect(result).toEqual(['team']);
  });

  it('should find multiple parent items if multiple matches exist', () => {
    // Create test data with multiple parent items that have matching children
    const multipleMatchItems = [
      {
        id: 'parent1',
        text: 'Parent 1',
        subItems: [
          {
            id: 'child1',
            text: 'Child 1',
            path: '/shared/path',
          }
        ],
      },
      {
        id: 'parent2',
        text: 'Parent 2',
        subItems: [
          {
            id: 'child2',
            text: 'Child 2',
            path: '/shared',
          }
        ],
      }
    ];
    
    const result = findParentItemsForPath(multipleMatchItems, '/shared/path');
    expect(result).toContain('parent1');
    expect(result).toContain('parent2');
    expect(result.length).toBe(2);
  });

  it('should return empty array for empty items', () => {
    const result = findParentItemsForPath([], '/admin/users');
    expect(result).toEqual([]);
  });

  it('should return empty array when no matches found', () => {
    const result = findParentItemsForPath(testItems, '/non-existent/path');
    expect(result).toEqual([]);
  });

  it('should skip items without IDs', () => {
    const result = findParentItemsForPath(testItems, '/admin/no-id/child');
    expect(result).toEqual([]);
  });

  it('should handle null/undefined currentPath', () => {
    const resultNull = findParentItemsForPath(testItems, null);
    expect(resultNull).toEqual([]);
    
    const resultUndefined = findParentItemsForPath(testItems, undefined);
    expect(resultUndefined).toEqual([]);
  });

  it('should handle items without subItems', () => {
    const result = findParentItemsForPath(testItems, '/admin');
    expect(result).toEqual([]);
  });

  it('should handle items with empty subItems array', () => {
    const itemsWithEmptySubItems = [
      {
        id: 'emptyParent',
        text: 'Empty Parent',
        subItems: []
      }
    ];
    
    const result = findParentItemsForPath(itemsWithEmptySubItems, '/any/path');
    expect(result).toEqual([]);
  });

  it('should handle deeply nested paths correctly', () => {
    const nestedItems = [
      {
        id: 'level1',
        text: 'Level 1',
        subItems: [
          {
            id: 'level2',
            text: 'Level 2',
            path: '/level1/level2',
            subItems: [
              {
                id: 'level3',
                text: 'Level 3',
                path: '/level1/level2/level3',
              }
            ]
          }
        ]
      }
    ];
    
    // The function should find level1 as parent for level2's path
    const result = findParentItemsForPath(nestedItems, '/level1/level2');
    expect(result).toEqual(['level1']);
    
    // The function should find level1 as parent for level3's path
    // Note: This depends on how the function is implemented - it might not find grandparents
    const result2 = findParentItemsForPath(nestedItems, '/level1/level2/level3');
    expect(result2).toEqual(['level1']);
  });
});

describe('generateRandomId', () => {
  it('should generate a random string', () => {
    const id1 = generateRandomId();
    const id2 = generateRandomId();
    
    // Check that it returns a string
    expect(typeof id1).toBe('string');
    expect(typeof id2).toBe('string');
    
    // Check that it returns different values on different calls
    expect(id1).not.toBe(id2);
    
    // Check that the string is not empty
    expect(id1.length).toBeGreaterThan(0);
  });
});

describe('saveSelectedPath', () => {
  it('should save the path to localStorage', () => {
    const storageKey = 'test-drawer';
    const path = '/admin/users';
    
    saveSelectedPath(storageKey, path);
    
    // Check that localStorage.setItem was called with the correct arguments
    expect(mockStorage.setItem).toHaveBeenCalledWith(
      storageKey,
      JSON.stringify({ selectedPath: path })
    );
  });
  
  it('should preserve existing state in localStorage', () => {
    const storageKey = 'test-drawer';
    const existingState = { open: true, expandedItems: ['team'] };
    const path = '/admin/users';
    
    // Set up existing state in localStorage
    mockStorage.getItem.mockReturnValueOnce(JSON.stringify(existingState));
    
    saveSelectedPath(storageKey, path);
    
    // Check that localStorage.setItem was called with the merged state
    expect(mockStorage.setItem).toHaveBeenCalledWith(
      storageKey,
      JSON.stringify({
        ...existingState,
        selectedPath: path,
      })
    );
  });
  
  it('should handle localStorage errors gracefully', () => {
    const storageKey = 'test-drawer';
    const path = '/admin/users';
    
    // Mock console.error to prevent actual error output during test
    const originalConsoleError = console.error;
    console.error = jest.fn();
    
    // Simulate an error when getting from localStorage
    mockStorage.getItem.mockImplementationOnce(() => {
      throw new Error('Test error');
    });
    
    // Function should not throw
    expect(() => saveSelectedPath(storageKey, path)).not.toThrow();
    
    // Error should be logged
    expect(console.error).toHaveBeenCalledWith(
      'Error saving selected path:',
      expect.any(Error)
    );
    
    // Restore console.error
    console.error = originalConsoleError;
  });
  
  it('should handle invalid JSON in localStorage gracefully', () => {
    const storageKey = 'test-drawer';
    const path = '/admin/users';
    
    // Mock console.error to prevent actual error output during test
    const originalConsoleError = console.error;
    console.error = jest.fn();
    
    // Return invalid JSON from localStorage
    mockStorage.getItem.mockReturnValueOnce('invalid-json');
    
    // Function should not throw
    expect(() => saveSelectedPath(storageKey, path)).not.toThrow();
    
    // Error should be logged
    expect(console.error).toHaveBeenCalledWith(
      'Error saving selected path:',
      expect.any(Error)
    );
    
    // Restore console.error
    console.error = originalConsoleError;
  });
});