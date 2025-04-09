import cacheService from './cacheService';

const localStorageMock = (() => {
  let store = {};
  return {
    getItem: jest.fn(key => store[key] || null),
    setItem: jest.fn((key, value) => {
      store[key] = value.toString();
    }),
    removeItem: jest.fn(key => {
      delete store[key];
    }),
    clear: jest.fn(() => {
      store = {};
    }),
    key: jest.fn(index => {
      return Object.keys(store)[index] || null;
    }),
    get length() {
      return Object.keys(store).length;
    }
  };
})();

Object.defineProperty(window, 'localStorage', {
  value: localStorageMock
});

describe('cacheService', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorageMock.clear();
    jest.spyOn(Date, 'now').mockImplementation(() => 1000);
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  test('should set and get data from cache', () => {
    const testData = { name: 'Test Data' };
    cacheService.set('test-key', testData);
    
    expect(localStorageMock.setItem).toHaveBeenCalledWith(
      'test-key',
      JSON.stringify({
        data: testData,
        timestamp: 1000,
        expiry: cacheService.DEFAULT_EXPIRY
      })
    );
    
    localStorageMock.getItem.mockReturnValueOnce(JSON.stringify({
      data: testData,
      timestamp: 1000,
      expiry: cacheService.DEFAULT_EXPIRY
    }));
    
    const result = cacheService.get('test-key');
    expect(result).toEqual(testData);
    expect(localStorageMock.getItem).toHaveBeenCalledWith('test-key');
  });

  test('should return null for expired cache items and remove them', () => {
    const expiredData = { name: 'Expired Data' };
    const expiry = 500;
    
    localStorageMock.getItem.mockReturnValueOnce(JSON.stringify({
      data: expiredData,
      timestamp: 400,
      expiry
    }));
    
    const result = cacheService.get('expired-key');
    
    expect(result).toBeNull();
    expect(localStorageMock.removeItem).toHaveBeenCalledWith('expired-key');
  });

  test('should handle invalid JSON in cache', () => {
    localStorageMock.getItem.mockReturnValueOnce('invalid-json');
    
    const result = cacheService.get('invalid-key');
    
    expect(result).toBeNull();
    expect(localStorageMock.removeItem).toHaveBeenCalledWith('invalid-key');
  });

  test('should remove specific cache item', () => {
    cacheService.remove('test-key');
    expect(localStorageMock.removeItem).toHaveBeenCalledWith('test-key');
  });

  test('should clear all cache items', () => {
    cacheService.clear();
    expect(localStorageMock.clear).toHaveBeenCalled();
  });

  test('should clear cache items with specific prefix', () => {
    Object.defineProperty(localStorageMock, 'length', { value: 3 });
    localStorageMock.key
      .mockReturnValueOnce('prefix_key1')
      .mockReturnValueOnce('other_key')
      .mockReturnValueOnce('prefix_key2');
    
    cacheService.clear('prefix_');
    
    expect(localStorageMock.removeItem).toHaveBeenCalledTimes(2);
    expect(localStorageMock.removeItem).toHaveBeenCalledWith('prefix_key1');
    expect(localStorageMock.removeItem).toHaveBeenCalledWith('prefix_key2');
    expect(localStorageMock.removeItem).not.toHaveBeenCalledWith('other_key');
  });

  test('should use custom expiry time', () => {
    const testData = { name: 'Custom Expiry' };
    const customExpiry = 120000;
    
    cacheService.set('custom-key', testData, customExpiry);
    
    expect(localStorageMock.setItem).toHaveBeenCalledWith(
      'custom-key',
      JSON.stringify({
        data: testData,
        timestamp: 1000,
        expiry: customExpiry
      })
    );
  });
});