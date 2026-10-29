import React from 'react';
import { render, act, waitFor } from '@testing-library/react';
import { AssistantRuntimeProvider } from '@assistant-ui/react';
import { useStudioRuntime } from './useStudioRuntime';
import { fetchHistory } from '../api/chatV2Client';

jest.mock('../api/chatV2Client', () => ({
  streamRun: jest.fn(),
  fetchHistory: jest.fn(),
  uploadFile: jest.fn(),
  cancelRun: jest.fn(),
}));

const endpoints = { upload: '/upload' };

beforeEach(() => {
  // CRA resets mock implementations between tests.
  fetchHistory.mockResolvedValue([]);
});

const Harness = ({ onRuntime }) => {
  const runtime = useStudioRuntime({ sessionId: 's1', endpoints });
  onRuntime(runtime);
  return <AssistantRuntimeProvider runtime={runtime}>{null}</AssistantRuntimeProvider>;
};

// The attachment adapter must accept any file: assistant-ui only treats the
// exact string '*' as "anything", so '*/*' rejected every file silently.
test('chat room composer accepts any attachment type', async () => {
  let runtime;
  render(<Harness onRuntime={(r) => { runtime = r; }} />);
  const composer = runtime.thread.composer;

  for (const file of [
    new File(['hello'], 'upload.txt', { type: 'text/plain' }),
    new File(['%PDF'], 'doc.pdf', { type: 'application/pdf' }),
    new File(['x'], 'noext', { type: '' }),
  ]) {
    await act(async () => {
      await composer.addAttachment(file);
    });
  }

  await waitFor(() => expect(composer.getState().attachments).toHaveLength(3));
  expect(composer.getState().attachments.map((a) => a.name)).toEqual(['upload.txt', 'doc.pdf', 'noext']);
});
