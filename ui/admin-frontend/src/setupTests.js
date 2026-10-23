// jest-dom adds custom jest matchers for asserting on DOM nodes.
// allows you to do things like:
// expect(element).toHaveTextContent(/react/i)
// learn more: https://github.com/testing-library/jest-dom
import '@testing-library/jest-dom';
import { configure } from '@testing-library/react';
import { TextEncoder, TextDecoder } from 'util';

// A busy CI runner can take well over the defaults to render the big MUI
// forms (LLMForm, ToolForm) and settle chained mocked fetches: one ToolForm
// test takes 4.7 s on an idle machine, against Jest's 5 s per-test limit.
// Longer limits weaken no assertion; a passing wait still returns as soon as
// its condition holds. findBy/waitFor stay well under the test timeout so a
// missing element still fails with its own message.
configure({ asyncUtilTimeout: 3000 });
jest.setTimeout(20000);

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
