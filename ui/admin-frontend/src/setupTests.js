// jest-dom adds custom jest matchers for asserting on DOM nodes.
// allows you to do things like:
// expect(element).toHaveTextContent(/react/i)
// learn more: https://github.com/testing-library/jest-dom
import '@testing-library/jest-dom';
import { TextEncoder, TextDecoder } from 'util';

// jsdom omits TextEncoder/TextDecoder, which browsers have had for years.
if (typeof global.TextEncoder === 'undefined') {
  global.TextEncoder = TextEncoder;
}
if (typeof global.TextDecoder === 'undefined') {
  global.TextDecoder = TextDecoder;
}

// Mock use-debounce
jest.mock('use-debounce', () => ({
  useDebouncedCallback: (fn) => fn,
  useDebounce: (value, delay) => [value, jest.fn()]
}));
