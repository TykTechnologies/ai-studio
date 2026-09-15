import React from 'react';
import { render, screen, waitFor, fireEvent, act } from '@testing-library/react';
import { ThemeProvider } from '@mui/material/styles';
import { AssistantRuntimeProvider, useLocalRuntime } from '@assistant-ui/react';
import { generateTheme } from '../../../admin/theme';
import { ChatUiProvider } from './ChatUiContext';
import StudioThread from './StudioThread';

// jsdom lacks the layout APIs the viewport uses.
beforeAll(() => {
  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  Element.prototype.scrollTo = () => {};
  Element.prototype.scrollIntoView = () => {};
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query) => ({ matches: false, media: query, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} }),
  });
});

const session = {
  session_id: 's1',
  chat: { name: 'Support', description: 'Ask anything', prompt_templates: [{ id: 1, name: 'Greet', prompt: 'Say hello' }] },
};

/** A runtime whose adapter streams a fixed reply with a tool call and a status part. */
const Harness = ({ adapter }) => {
  const runtime = useLocalRuntime(adapter);
  return (
    <ThemeProvider theme={generateTheme()}>
      <ChatUiProvider session={session} userName="Martin">
        <AssistantRuntimeProvider runtime={runtime}>
          <StudioThread />
        </AssistantRuntimeProvider>
      </ChatUiProvider>
    </ThemeProvider>
  );
};

const scriptedAdapter = (seen) => ({
  async *run({ messages }) {
    seen.push(messages[messages.length - 1]);
    yield { content: [{ type: 'data', name: 'status', data: { text: 'Running governance filters' } }] };
    yield {
      content: [
        { type: 'data', name: 'status', data: { text: 'Running governance filters' } },
        { type: 'tool-call', toolCallId: 'c1', toolName: 'getWeather', args: { city: 'Auckland' }, argsText: '{"city":"Auckland"}', result: { temp: 18 } },
        { type: 'text', text: 'It is **18** degrees.' },
      ],
      status: { type: 'complete', reason: 'stop' },
    };
  },
});

describe('StudioThread', () => {
  it('shows the welcome screen with prompt templates and sends a message', async () => {
    const seen = [];
    render(<Harness adapter={scriptedAdapter(seen)} />);

    expect(screen.getByText(/Welcome to Support chat/)).toBeInTheDocument();
    expect(screen.getByText('Ask anything')).toBeInTheDocument();

    // Suggestion chip fills the composer.
    fireEvent.click(screen.getByText('Greet'));
    const input = screen.getByLabelText('Message');
    await waitFor(() => expect(input.value).toBe('Say hello'));

    await act(async () => {
      fireEvent.click(screen.getByLabelText('Send message'));
    });

    await waitFor(() => expect(screen.getByText(/degrees/)).toBeInTheDocument());
    expect(seen).toHaveLength(1);
    expect(seen[0].role).toBe('user');

    // The user bubble, the status line and the tool card rendered.
    expect(screen.getByText('Say hello')).toBeInTheDocument();
    expect(screen.getByTestId('status-chip')).toHaveTextContent('Running governance filters');
    expect(screen.getByTestId('tool-call-card')).toHaveTextContent('Tool result: getWeather');
    // Markdown rendered bold.
    expect(screen.getByText('18').tagName).toBe('STRONG');

    // Welcome screen is gone and the system toggle is present.
    expect(screen.queryByText(/Welcome to Support chat/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByText(/Hide System and Context Messages/));
    expect(screen.queryByTestId('status-chip')).not.toBeInTheDocument();
  });
});
