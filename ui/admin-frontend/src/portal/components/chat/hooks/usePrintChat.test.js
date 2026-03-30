import { renderHook, act } from '@testing-library/react';
import usePrintChat from './usePrintChat';

describe('usePrintChat', () => {
  let printSpy;

  beforeEach(() => {
    jest.clearAllMocks();
    printSpy = jest.spyOn(window, 'print').mockImplementation(() => {});
  });

  afterEach(() => {
    printSpy.mockRestore();
    // Clean up any injected print headers
    document.querySelectorAll('[data-print-role="print-header"]').forEach(el => el.remove());
  });

  describe('canPrint', () => {
    test('returns false when messages array is empty', () => {
      const { result } = renderHook(() =>
        usePrintChat({ chatName: 'Test', messages: [], messagesContainerRef: { current: null } })
      );
      expect(result.current.canPrint).toBe(false);
    });

    test('returns false when there are only AI messages', () => {
      const messages = [
        { id: '1', type: 'ai', content: 'Hello' },
        { id: '2', type: 'system', content: 'System msg' },
      ];
      const { result } = renderHook(() =>
        usePrintChat({ chatName: 'Test', messages, messagesContainerRef: { current: null } })
      );
      expect(result.current.canPrint).toBe(false);
    });

    test('returns true when there is a user message (type field)', () => {
      const messages = [
        { id: '1', type: 'user', content: 'Hi' },
        { id: '2', type: 'ai', content: 'Hello' },
      ];
      const { result } = renderHook(() =>
        usePrintChat({ chatName: 'Test', messages, messagesContainerRef: { current: null } })
      );
      expect(result.current.canPrint).toBe(true);
    });

    test('returns true when there is a user message (role field, AgentChat)', () => {
      const messages = [
        { id: '1', role: 'user', content: 'Hi' },
        { id: '2', role: 'agent', content: 'Hello' },
      ];
      const { result } = renderHook(() =>
        usePrintChat({ chatName: 'Test', messages, messagesContainerRef: { current: null } })
      );
      expect(result.current.canPrint).toBe(true);
    });

    test('updates when messages change', () => {
      let messages = [];
      const { result, rerender } = renderHook(() =>
        usePrintChat({ chatName: 'Test', messages, messagesContainerRef: { current: null } })
      );
      expect(result.current.canPrint).toBe(false);

      messages = [{ id: '1', type: 'user', content: 'Hi' }];
      rerender();
      expect(result.current.canPrint).toBe(true);
    });
  });

  describe('handlePrint', () => {
    test('does not call window.print when canPrint is false', () => {
      const { result } = renderHook(() =>
        usePrintChat({ chatName: 'Test', messages: [], messagesContainerRef: { current: null } })
      );

      act(() => {
        result.current.handlePrint();
      });

      expect(printSpy).not.toHaveBeenCalled();
    });

    test('calls window.print when canPrint is true', () => {
      const messages = [{ id: '1', type: 'user', content: 'Hi' }];
      const { result } = renderHook(() =>
        usePrintChat({ chatName: 'Test', messages, messagesContainerRef: { current: null } })
      );

      act(() => {
        result.current.handlePrint();
      });

      expect(printSpy).toHaveBeenCalledTimes(1);
    });

    test('injects print header into the messages container', () => {
      const container = document.createElement('div');
      const existingChild = document.createElement('p');
      container.appendChild(existingChild);

      const messages = [{ id: '1', type: 'user', content: 'Hi' }];
      const { result } = renderHook(() =>
        usePrintChat({ chatName: 'My Chat', messages, messagesContainerRef: { current: container } })
      );

      act(() => {
        result.current.handlePrint();
      });

      const header = container.querySelector('[data-print-role="print-header"]');
      expect(header).not.toBeNull();
      expect(header.querySelector('h1').textContent).toBe('My Chat');
      expect(header.querySelector('p').textContent).toContain('Exported on');
      // Header should be the first child
      expect(container.firstChild).toBe(header);
    });

    test('uses fallback chat name when chatName is not provided', () => {
      const container = document.createElement('div');
      const messages = [{ id: '1', type: 'user', content: 'Hi' }];
      const { result } = renderHook(() =>
        usePrintChat({ chatName: '', messages, messagesContainerRef: { current: container } })
      );

      act(() => {
        result.current.handlePrint();
      });

      const header = container.querySelector('[data-print-role="print-header"]');
      expect(header.querySelector('h1').textContent).toBe('Chat Conversation');
    });

    test('injects header into document.body when no container ref', () => {
      const messages = [{ id: '1', type: 'user', content: 'Hi' }];
      const { result } = renderHook(() =>
        usePrintChat({ chatName: 'Test', messages, messagesContainerRef: { current: null } })
      );

      act(() => {
        result.current.handlePrint();
      });

      const header = document.body.querySelector('[data-print-role="print-header"]');
      expect(header).not.toBeNull();
    });

    test('cleans up header on afterprint event', () => {
      const container = document.createElement('div');
      const messages = [{ id: '1', type: 'user', content: 'Hi' }];
      const { result } = renderHook(() =>
        usePrintChat({ chatName: 'Test', messages, messagesContainerRef: { current: container } })
      );

      act(() => {
        result.current.handlePrint();
      });

      expect(container.querySelector('[data-print-role="print-header"]')).not.toBeNull();

      // Simulate afterprint event
      act(() => {
        window.dispatchEvent(new Event('afterprint'));
      });

      expect(container.querySelector('[data-print-role="print-header"]')).toBeNull();
    });

    test('header is hidden on screen via inline style', () => {
      const container = document.createElement('div');
      const messages = [{ id: '1', type: 'user', content: 'Hi' }];
      const { result } = renderHook(() =>
        usePrintChat({ chatName: 'Test', messages, messagesContainerRef: { current: container } })
      );

      act(() => {
        result.current.handlePrint();
      });

      const header = container.querySelector('[data-print-role="print-header"]');
      expect(header.style.display).toBe('none');
    });
  });
});
