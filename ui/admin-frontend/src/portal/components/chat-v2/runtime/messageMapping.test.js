import {
  isResume,
  toThreadMessageLike,
  extractText,
  extractFileRefs,
  buildRunBody,
  toRunResult,
  ROOT_MESSAGE_ID,
} from './messageMapping';

describe('toThreadMessageLike', () => {
  it('maps text, tool-call and data parts', () => {
    const like = toThreadMessageLike({
      id: 42,
      role: 'assistant',
      created_at: '2026-09-15T10:00:00Z',
      parts: [
        { type: 'text', text: 'Let me check.' },
        {
          type: 'tool-call',
          toolCallId: 'c1',
          toolName: 'getWeather',
          args: { city: 'Auckland' },
          argsText: '{"city":"Auckland"}',
          result: { temp: 18 },
        },
        { type: 'data', name: 'context', data: { text: 'doc', source: 'rag' } },
        { type: 'mystery' },
      ],
    });
    expect(like.id).toBe('42');
    expect(like.role).toBe('assistant');
    expect(like.status).toEqual({ type: 'complete', reason: 'stop' });
    expect(like.createdAt).toBeInstanceOf(Date);
    expect(like.content).toHaveLength(3);
    expect(like.content[1]).toMatchObject({ type: 'tool-call', toolCallId: 'c1', result: { temp: 18 } });
    expect(like.content[2]).toEqual({ type: 'data', name: 'context', data: { text: 'doc', source: 'rag' } });
  });

  it('defaults tool args and marks errors', () => {
    const like = toThreadMessageLike({
      id: '7',
      role: 'user',
      parts: [{ type: 'tool-call', toolCallId: 'x', toolName: 'broken', isError: true, result: 'ERROR: nope' }],
    });
    expect(like.status).toBeUndefined();
    expect(like.content[0]).toMatchObject({ args: {}, argsText: '{}', isError: true, result: 'ERROR: nope' });
  });
});

describe('extractText / extractFileRefs', () => {
  it('joins text parts and collects file refs from attachments and content', () => {
    const msg = {
      content: [
        { type: 'text', text: 'hello' },
        { type: 'data', name: 'file_ref', data: { filename: 'a.txt' } },
      ],
      attachments: [
        { content: [{ type: 'data', name: 'file_ref', data: { filename: 'b.pdf' } }] },
        { content: [{ type: 'data', name: 'file_ref', data: { filename: 'a.txt' } }] },
      ],
    };
    expect(extractText(msg)).toBe('hello');
    expect(extractFileRefs(msg)).toEqual(['b.pdf', 'a.txt']);
  });
});

describe('buildRunBody', () => {
  const user = (id, text) => ({ id, role: 'user', content: [{ type: 'text', text }] });
  const ai = (id) => ({ id, role: 'assistant', content: [{ type: 'text', text: 'ok' }] });

  it('sends a plain turn when appending at the backend head', () => {
    const idMap = new Map([['u1', '10'], ['a1', '11']]);
    const body = buildRunBody([user('u1', 'hi'), ai('a1'), user('u2', 'more')], idMap, '11');
    expect(body).toEqual({ message: 'more', file_refs: [] });
  });

  it('first message of a fresh thread has no rewind', () => {
    expect(buildRunBody([user('u1', 'hi')], new Map(), null)).toEqual({ message: 'hi', file_refs: [] });
  });

  it('editing the opening message rewinds to root', () => {
    const body = buildRunBody([user('u9', 'edited')], new Map([['u1', '10']]), '11');
    expect(body.after_message_id).toBe(ROOT_MESSAGE_ID);
  });

  it('editing a later message rewinds to after its parent', () => {
    const idMap = new Map([['u1', '10'], ['a1', '11'], ['u2', '12'], ['a2', '13']]);
    const body = buildRunBody([user('u1', 'hi'), ai('a1'), user('u3', 'edited second')], idMap, '13');
    expect(body).toEqual({ message: 'edited second', file_refs: [], after_message_id: '11' });
  });

  it('reloading an assistant reply regenerates after the user turn', () => {
    const idMap = new Map([['u1', '10'], ['a1', '11']]);
    const body = buildRunBody([user('u1', 'hi')], idMap, '11');
    expect(body).toEqual({ regenerate: true, after_message_id: '10' });
  });

  it('does not rewind when the parent id is unknown', () => {
    const body = buildRunBody([user('u1', 'hi'), ai('a-unknown'), user('u2', 'next')], new Map([['u1', '10']]), '11');
    expect(body.after_message_id).toBeUndefined();
  });
});

describe('buildRunBody (client tools)', () => {
  it('sends tool results when resuming a parked assistant turn', () => {
    const last = {
      id: 'a1', role: 'assistant',
      content: [
        { type: 'text', text: 'Confirm?' },
        { type: 'tool-call', toolCallId: 'call_1', toolName: 'ask-approval', args: {}, result: { approved: true } },
        { type: 'tool-call', toolCallId: 'call_2', toolName: 'getWeather', args: {}, result: { temp: 1 } },
        { type: 'tool-call', toolCallId: 'call_3', toolName: 'ask-form', args: {} },
      ],
    };
    const messages = [{ id: 'u1', role: 'user', content: [{ type: 'text', text: 'go' }] }];
    const body = buildRunBody(messages, new Map(), null, new Set(['ask-approval', 'ask-form']), last);
    expect(body).toEqual({ tool_results: [{ tool_call_id: 'call_1', result: '{"approved":true}', is_error: false }] });
  });

  it('treats an unanswered parked message as a fresh turn', () => {
    const current = { id: 'a1', role: 'assistant', content: [{ type: 'tool-call', toolCallId: 'c', toolName: 'ask-form', args: {} }] };
    const messages = [{ id: 'u1', role: 'user', content: [{ type: 'text', text: 'go' }] }];
    expect(isResume(current, new Set(['ask-form']))).toBe(false);
    expect(buildRunBody(messages, new Map(), null, new Set(['ask-form']), current)).toEqual({ message: 'go', file_refs: [] });
  });
});

describe('toRunResult', () => {
  it('keeps content, status and metadata but drops the cumulative steps', () => {
    const value = {
      content: [{ type: 'text', text: 'hi' }],
      status: { type: 'requires-action', reason: 'tool-calls' },
      metadata: { steps: [{ state: 'started' }, { state: 'finished' }], custom: { a: 1 } },
    };
    expect(toRunResult(value)).toEqual({
      content: value.content,
      status: value.status,
      metadata: { custom: { a: 1 } },
    });
  });

  it('omits metadata when only steps were present', () => {
    expect(toRunResult({ content: [], status: { type: 'running' }, metadata: { steps: [] } })).toEqual({
      content: [],
      status: { type: 'running' },
    });
  });
});
