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

// jsdom also omits the Web Streams API, which assistant-stream (the chat
// streaming decoder) needs at import time. Node ships it under stream/web.
{
  // eslint-disable-next-line global-require
  const webStreams = require('stream/web');
  ['ReadableStream', 'WritableStream', 'TransformStream', 'TextDecoderStream', 'TextEncoderStream'].forEach((name) => {
    if (typeof global[name] === 'undefined' && webStreams[name]) {
      global[name] = webStreams[name];
    }
  });
}

// Mock use-debounce
jest.mock('use-debounce', () => ({
  useDebouncedCallback: (fn) => fn,
  useDebounce: (value, delay) => [value, jest.fn()]
}));
