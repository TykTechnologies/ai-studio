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
          text: 'Teams',
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
describe('findSelectedItem', () => {
  const { findSelectedItem } = require('./utils');
  const items = [
    { id: 'overview', text: 'Overview', path: '/admin', exact: true },
    { id: 'catalogs', text: 'Catalogs', subItems: [{ id: 'catalog-llms', text: 'LLM providers', path: '/admin/catalogs/llms' }] },
    { id: 'llm-management', text: 'LLM management', subItems: [{ id: 'llms', text: 'LLM providers', path: '/admin/llms' }] },
    { id: 'ai-portal', text: 'AI Portal', subItems: [{ id: 'portal-apps', text: 'Apps', path: '/admin/apps' }] },
    { id: 'chat', text: 'Chat', subItems: [{ id: 'room', text: 'Room', path: '/chat?continue_id=7' }] },
  ];

  it('matches /admin only exactly', () => {
    expect(findSelectedItem(items, '/admin').key).toBe('overview');
    expect(findSelectedItem(items, '/admin/apps/1').key).not.toBe('overview');
  });

  it('picks the longest matching path and reports its ancestors', () => {
    const sel = findSelectedItem(items, '/admin/apps/1');
    expect(sel.key).toBe('ai-portal>portal-apps');
    expect(sel.ancestorIds).toEqual(['ai-portal']);
  });

  it('compares full paths so the catalog entry beats the bare llms entry', () => {
    expect(findSelectedItem(items, '/admin/catalogs/llms/2').key).toBe('catalogs>catalog-llms');
    expect(findSelectedItem(items, '/admin/llms/2').key).toBe('llm-management>llms');
  });

  it('requires an exact match, search included, for query-string paths', () => {
    expect(findSelectedItem(items, '/chat', '?continue_id=7').key).toBe('chat>room');
    expect(findSelectedItem(items, '/chat', '?continue_id=8')).toBeNull();
  });

  it('returns null when nothing matches', () => {
    expect(findSelectedItem(items, '/portal/apps')).toBeNull();
    expect(findSelectedItem([], '/admin')).toBeNull();
  });
});
