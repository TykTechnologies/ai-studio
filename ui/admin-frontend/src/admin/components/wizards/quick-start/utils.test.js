import {
  generateSlug,
  generateEndpointUrl,
  getBudgetLimitText,
  getOwnerName,
  getCurlExample,
  validateEmail,
  validatePassword
} from './utils';
import { getConfig } from '../../../../config';

// Mock the getConfig function
jest.mock('../../../../config', () => ({
  getConfig: jest.fn()
}));

describe('Quick Start Wizard Utils', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // Set default mock return value for getConfig
    getConfig.mockReturnValue({
      proxyURL: 'https://example.com'
    });
    // Mock window.location.hostname
    Object.defineProperty(window, 'location', {
      value: {
        hostname: 'localhost'
      },
      writable: true
    });
  });

  describe('generateSlug', () => {
    test('converts a simple name to a slug', () => {
      expect(generateSlug('Test App')).toBe('test-app');
    });

    test('handles special characters', () => {
      expect(generateSlug('Test App #1!')).toBe('test-app-1');
    });

    test('removes leading and trailing hyphens', () => {
      expect(generateSlug('--Test App--')).toBe('test-app');
    });

    test('handles empty string', () => {
      expect(generateSlug('')).toBe('');
    });

    test('handles multiple spaces and special characters', () => {
      expect(generateSlug('  Test   App  @#$%^&*()  ')).toBe('test-app');
    });
  });

  describe('generateEndpointUrl', () => {
    test('generates correct endpoint URL with path and name', () => {
      const path = '/api/';
      const name = 'Test App';
      const expected = 'https://example.com/api/test-app/';
      
      expect(generateEndpointUrl(path, name)).toBe(expected);
    });

    test('works with different proxy URLs', () => {
      getConfig.mockReturnValue({
        proxyURL: 'http://api.example.org'
      });
      
      const path = '/api/';
      const name = 'Test App';
      const expected = 'http://api.example.org/api/test-app/';
      
      expect(generateEndpointUrl(path, name)).toBe(expected);
    });

    test('falls back to hostname:9090 when proxyURL is not set', () => {
      getConfig.mockReturnValue({
        proxyURL: null
      });
      
      const path = '/api/';
      const name = 'Test App';
      const expected = '//localhost:9090/api/test-app/';
      
      expect(generateEndpointUrl(path, name)).toBe(expected);
    });

    test('works with different paths', () => {
      const path = '/v1/apps/';
      const name = 'My App';
      const expected = 'https://example.com/v1/apps/my-app/';
      
      expect(generateEndpointUrl(path, name)).toBe(expected);
    });
  });

  describe('getBudgetLimitText', () => {
    test('returns "not set" when setBudget is false', () => {
      const appData = { setBudget: false, monthlyBudget: 100 };
      expect(getBudgetLimitText(appData)).toBe('not set');
    });

    test('returns formatted budget when setBudget is true', () => {
      const appData = { setBudget: true, monthlyBudget: 100 };
      expect(getBudgetLimitText(appData)).toBe('$100 per month');
    });

    test('handles zero budget', () => {
      const appData = { setBudget: true, monthlyBudget: 0 };
      expect(getBudgetLimitText(appData)).toBe('$0 per month');
    });

    test('handles decimal budget values', () => {
      const appData = { setBudget: true, monthlyBudget: 99.99 };
      expect(getBudgetLimitText(appData)).toBe('$99.99 per month');
    });
  });

  describe('getOwnerName', () => {
    test('returns "Current user" when ownerType is current and no name is provided', () => {
      const ownerData = { ownerType: 'current' };
      expect(getOwnerName(ownerData)).toBe('Current user');
    });

    test('returns the name when ownerType is current and name is provided', () => {
      const ownerData = { ownerType: 'current', name: 'John Doe' };
      expect(getOwnerName(ownerData)).toBe('John Doe');
    });

    test('returns "New user" when ownerType is not current and no formData name is provided', () => {
      const ownerData = { ownerType: 'new', formData: {} };
      expect(getOwnerName(ownerData)).toBe('New user');
    });

    test('returns the formData name when ownerType is not current and formData name is provided', () => {
      const ownerData = { ownerType: 'new', formData: { name: 'Jane Smith' } };
      expect(getOwnerName(ownerData)).toBe('Jane Smith');
    });

    test('returns "New user" when formData is undefined', () => {
      const ownerData = { ownerType: 'new' };
      expect(getOwnerName(ownerData)).toBe('New user');
    });
  });

  describe('getCurlExample', () => {
    test('returns OpenAI curl example by default', () => {
      const result = getCurlExample();
      
      expect(result).toContain('curl -X POST "https://example.com/llm/rest/openai/v1/chat/completions"');
      expect(result).toContain('"model": "gpt-4o"');
      expect(result).toContain('"temperature": 0.7');
      expect(result).toContain('"max_tokens": 1000');
    });

    test('returns Anthropic curl example', () => {
      const result = getCurlExample('anthropic', 'Anthropic Claude');
      
      expect(result).toContain('curl -X POST "https://example.com/llm/rest/anthropic-claude/v1/messages"');
      expect(result).toContain('"model": "claude-3-5-sonnet-20240620"');
    });

    test('returns Google AI curl example', () => {
      const result = getCurlExample('google_ai', 'Google Gemini');
      
      expect(result).toContain('curl -X POST "https://example.com/llm/rest/google-gemini/v1/models/gemini-1.5-flash:generateContent?key=YOUR_SECRET"');
      expect(result).toContain('"model": "gemini-1.5-flash"');
      expect(result).toContain('"temperature": 0.7');
      expect(result).toContain('"maxOutputTokens": 1000');
    });

    test('returns Google curl example', () => {
      const result = getCurlExample('google', 'Google AI');
      
      expect(result).toContain('curl -X POST "https://example.com/llm/rest/google-ai/v1/models/gemini-1.5-flash:generateContent?key=YOUR_SECRET"');
      expect(result).toContain('"model": "gemini-1.5-flash"');
      expect(result).toContain('"temperature": 0.7');
      expect(result).toContain('"maxOutputTokens": 1000');
    });

    test('returns Vertex curl example', () => {
      const result = getCurlExample('vertex', 'Vertex AI');
      
      expect(result).toContain('curl -X POST "https://example.com/llm/rest/vertex-ai/v1/models/gemini-pro:generateContent"');
      expect(result).toContain('"model": "gemini-pro"');
    });

    test('returns Ollama curl example', () => {
      const result = getCurlExample('ollama', 'Ollama');
      
      expect(result).toContain('curl -X POST "https://example.com/llm/rest/ollama/api/chat"');
      expect(result).toContain('"model": "llama3"');
    });

    test('returns HuggingFace curl example', () => {
      const result = getCurlExample('huggingface', 'HuggingFace');
      
      expect(result).toContain('curl -X POST "https://example.com/llm/rest/huggingface/models/mistralai/Mistral-7B-Instruct-v0.2"');
      expect(result).toContain('"model": "mistralai/Mistral-7B-Instruct-v0.2"');
    });

    test('uses different name for URL construction', () => {
      const result = getCurlExample('openai', 'Custom OpenAI Name');
      
      expect(result).toContain('curl -X POST "https://example.com/llm/rest/custom-openai-name/v1/chat/completions"');
      expect(result).toContain('"model": "gpt-4o"');
    });
  });

  describe('validateEmail', () => {
    test('returns null for valid email', () => {
      expect(validateEmail('test@example.com')).toBeNull();
    });

    test('returns error message for invalid email format', () => {
      expect(validateEmail('invalid-email')).toBe('Email is invalid');
    });

    test('returns error message for email without domain', () => {
      expect(validateEmail('test@')).toBe('Email is invalid');
    });

    test('returns error message for email without username', () => {
      expect(validateEmail('@example.com')).toBe('Email is invalid');
    });

    test('returns error message for empty email', () => {
      expect(validateEmail('')).toBe('Email is invalid');
    });

    test('returns error message for null email', () => {
      expect(validateEmail(null)).toBe('Email is invalid');
    });
  });

  describe('validatePassword', () => {
    test('returns null when all criteria are met', () => {
      const passwordCriteria = {
        length: true,
        number: true,
        special: true,
        uppercase: true
      };
      
      expect(validatePassword('Password123!', passwordCriteria)).toBeNull();
    });

    test('returns error message when length criterion is not met', () => {
      const passwordCriteria = {
        length: false,
        number: true,
        special: true,
        uppercase: true
      };
      
      expect(validatePassword('Pass1!', passwordCriteria)).toBe('Password must be at least 8 characters');
    });

    test('returns error message when number criterion is not met', () => {
      const passwordCriteria = {
        length: true,
        number: false,
        special: true,
        uppercase: true
      };
      
      expect(validatePassword('Password!', passwordCriteria)).toBe('Password must contain a number');
    });

    test('returns error message when special character criterion is not met', () => {
      const passwordCriteria = {
        length: true,
        number: true,
        special: false,
        uppercase: true
      };
      
      expect(validatePassword('Password123', passwordCriteria)).toBe('Password must contain a special character');
    });

    test('returns error message when uppercase criterion is not met', () => {
      const passwordCriteria = {
        length: true,
        number: true,
        special: true,
        uppercase: false
      };
      
      expect(validatePassword('password123!', passwordCriteria)).toBe('Password must contain an uppercase letter');
    });

    test('checks criteria in the correct order', () => {
      const passwordCriteria = {
        length: false,
        number: false,
        special: false,
        uppercase: false
      };
      
      // Should return the first error (length) even though all criteria fail
      expect(validatePassword('pass', passwordCriteria)).toBe('Password must be at least 8 characters');
    });
  });
});