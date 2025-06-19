import { renderHook, act } from '@testing-library/react';
import { useFormValidation } from './useFormValidation';

describe('useFormValidation', () => {
  describe('Basic functionality', () => {
    it('should initialize with provided initial value', () => {
      const { result } = renderHook(() => useFormValidation('initial'));
      
      expect(result.current.value).toBe('initial');
      expect(result.current.error).toBeNull();
    });

    it('should initialize with empty string when no initial value provided', () => {
      const { result } = renderHook(() => useFormValidation());
      
      expect(result.current.value).toBe('');
      expect(result.current.error).toBeNull();
    });

    it('should update value when setValue is called', () => {
      const { result } = renderHook(() => useFormValidation());
      
      act(() => {
        result.current.setValue('new value');
      });
      
      expect(result.current.value).toBe('new value');
    });

    it('should handle input change events', () => {
      const { result } = renderHook(() => useFormValidation());
      
      const mockEvent = {
        target: { value: 'typed value' }
      };
      
      act(() => {
        result.current.handleChange(mockEvent);
      });
      
      expect(result.current.value).toBe('typed value');
    });

    it('should clear error when typing after error was set for non-validation mode', () => {
      const { result } = renderHook(() => useFormValidation(''));
      
      act(() => {
        result.current.setError('some error');
      });
      
      expect(result.current.error).toBe('some error');
      
      const mockEvent = {
        target: { value: 'new input' }
      };
      
      act(() => {
        result.current.handleChange(mockEvent);
      });
      
      expect(result.current.error).toBeNull();
    });

    it('should update initial value when prop changes', () => {
      const { result, rerender } = renderHook(
        ({ initialValue }) => useFormValidation(initialValue),
        { initialProps: { initialValue: 'first' } }
      );
      
      expect(result.current.value).toBe('first');
      
      rerender({ initialValue: 'second' });
      
      expect(result.current.value).toBe('second');
    });
  });

  describe('Email validation', () => {
    it('should validate valid email addresses', () => {
      const { result } = renderHook(() => useFormValidation('test@example.com', false, true));
      
      let isValid;
      act(() => {
        isValid = result.current.validate();
      });
      
      expect(isValid).toBe(true);
      expect(result.current.error).toBeNull();
    });

    it('should invalidate email without @ symbol', async () => {
      const { result } = renderHook(() => useFormValidation('invalid-email', false, true));
      
      let isValid;
      act(() => {
        isValid = result.current.validate();
      });
      
      expect(isValid).toBe(false);
      expect(result.current.error).toBe('Email is invalid');
    });

    it('should invalidate email without domain', async () => {
      const { result } = renderHook(() => useFormValidation('test@', false, true));
      
      let isValid;
      act(() => {
        isValid = result.current.validate();
      });
      
      expect(isValid).toBe(false);
      expect(result.current.error).toBe('Email is invalid');
    });

    it('should invalidate empty email', async () => {
      const { result } = renderHook(() => useFormValidation('', false, true));
      
      let isValid;
      act(() => {
        isValid = result.current.validate();
      });
      
      expect(isValid).toBe(false);
      expect(result.current.error).toBe('Email is invalid');
    });

    it('should validate email in real-time during typing', () => {
      const { result } = renderHook(() => useFormValidation('', false, true));
      
      const mockEvent = {
        target: { value: 'invalid' }
      };
      
      act(() => {
        result.current.handleChange(mockEvent);
      });
      
      expect(result.current.error).toBe('Email is invalid');
      
      const validEvent = {
        target: { value: 'valid@email.com' }
      };
      
      act(() => {
        result.current.handleChange(validEvent);
      });
      
      expect(result.current.error).toBeNull();
    });
  });

  describe('Password validation', () => {
    it('should initialize password criteria as false', () => {
      const { result } = renderHook(() => useFormValidation('', true));
      
      expect(result.current.passwordCriteria).toEqual({
        length: false,
        number: false,
        special: false,
        uppercase: false,
        lowercase: false,
      });
    });

    it('should update password criteria based on input', () => {
      const { result } = renderHook(() => useFormValidation('Test123!@#', true));
      
      expect(result.current.passwordCriteria).toEqual({
        length: true,
        number: true,
        special: true,
        uppercase: true,
        lowercase: true,
      });
    });

    it('should validate password with all criteria met', () => {
      const { result } = renderHook(() => useFormValidation('Test123!', true));
      
      let isValid;
      act(() => {
        isValid = result.current.validate();
      });
      
      expect(isValid).toBe(true);
      expect(result.current.error).toBeNull();
    });

    it('should invalidate password with insufficient length', async () => {
      const { result } = renderHook(() => useFormValidation('Test1!', true));
      
      let isValid;
      act(() => {
        isValid = result.current.validate();
      });
      
      expect(isValid).toBe(false);
      expect(result.current.error).toBe('Password must be at least 8 characters');
    });

    it('should invalidate password without number', async () => {
      const { result } = renderHook(() => useFormValidation('TestTest!', true));
      
      let isValid;
      act(() => {
        isValid = result.current.validate();
      });
      
      expect(isValid).toBe(false);
      expect(result.current.error).toBe('Password must contain a number');
    });

    it('should invalidate password without special character', async () => {
      const { result } = renderHook(() => useFormValidation('TestTest1', true));
      
      let isValid;
      act(() => {
        isValid = result.current.validate();
      });
      
      expect(isValid).toBe(false);
      expect(result.current.error).toBe('Password must contain a special character');
    });

    it('should invalidate password without uppercase letter', async () => {
      const { result } = renderHook(() => useFormValidation('testtest1!', true));
      
      let isValid;
      act(() => {
        isValid = result.current.validate();
      });
      
      expect(isValid).toBe(false);
      expect(result.current.error).toBe('Password must contain an uppercase letter');
    });

    it('should invalidate password without lowercase letter', async () => {
      const { result } = renderHook(() => useFormValidation('TESTTEST1!', true));
      
      let isValid;
      act(() => {
        isValid = result.current.validate();
      });
      
      expect(isValid).toBe(false);
      expect(result.current.error).toBe('Password must contain a lowercase letter');
    });

    it('should validate password in real-time during typing', () => {
      const { result } = renderHook(() => useFormValidation('', true));
      
      const mockEvent = {
        target: { value: 'weak' }
      };
      
      act(() => {
        result.current.handleChange(mockEvent);
      });
      
      expect(result.current.error).toBe('Password must be at least 8 characters');
      
      const strongEvent = {
        target: { value: 'Strong123!' }
      };
      
      act(() => {
        result.current.handleChange(strongEvent);
      });
      
      expect(result.current.error).toBeNull();
    });

    it('should update criteria when password value changes', () => {
      const { result } = renderHook(() => useFormValidation('', true));
      
      const mockEvent = {
        target: { value: 'TestPassword123!' }
      };
      
      act(() => {
        result.current.handleChange(mockEvent);
      });
      
      expect(result.current.passwordCriteria).toEqual({
        length: true,
        number: true,
        special: true,
        uppercase: true,
        lowercase: true,
      });
    });

    it('should handle special characters correctly', () => {
      const { result } = renderHook(() => useFormValidation('', true));
      
      const mockEvent = {
        target: { value: 'TestTest1!' }
      };
      
      act(() => {
        result.current.handleChange(mockEvent);
      });
      
      expect(result.current.passwordCriteria.special).toBe(true);
      expect(result.current.error).toBeNull();
    });
  });

  describe('Non-validation mode', () => {
    it('should always return true for validation when not email or password', () => {
      const { result } = renderHook(() => useFormValidation('any value'));
      
      let isValid;
      act(() => {
        isValid = result.current.validate();
      });
      
      expect(isValid).toBe(true);
      expect(result.current.error).toBeNull();
    });

    it('should not update error during typing when not email or password', () => {
      const { result } = renderHook(() => useFormValidation(''));
      
      const mockEvent = {
        target: { value: 'any input' }
      };
      
      act(() => {
        result.current.handleChange(mockEvent);
      });
      
      expect(result.current.error).toBeNull();
    });
  });

  describe('Edge cases', () => {
    it('should handle empty string for password validation', async () => {
      const { result } = renderHook(() => useFormValidation('', true));
      
      let isValid;
      act(() => {
        isValid = result.current.validate();
      });
      
      expect(isValid).toBe(false);
      expect(result.current.error).toBe('Password must be at least 8 characters');
    });

    it('should handle setValue with different value types', () => {
      const { result } = renderHook(() => useFormValidation(''));
      
      act(() => {
        result.current.setValue('string value');
      });
      
      expect(result.current.value).toBe('string value');
    });

    it('should manually set and clear errors', () => {
      const { result } = renderHook(() => useFormValidation(''));
      
      act(() => {
        result.current.setError('manual error');
      });
      
      expect(result.current.error).toBe('manual error');
      
      act(() => {
        result.current.setError(null);
      });
      
      expect(result.current.error).toBeNull();
    });
  });
}); 