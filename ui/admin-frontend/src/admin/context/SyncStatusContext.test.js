import React from 'react';
import { render, screen, act, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { SyncStatusProvider, useSyncStatus } from './SyncStatusContext';
import { notifyConfigChanged } from '../utils/configSyncNotifier';

jest.mock('../services/syncStatusService', () => ({
  __esModule: true,
  default: { getSyncStatus: jest.fn() },
}));
jest.mock('../services/edgeGatewayService', () => ({
  __esModule: true,
  default: { getPendingChanges: jest.fn() },
}));
jest.mock('../utils/identityStore', () => ({
  hasPermissionNow: () => true,
  subscribe: () => () => {},
}));

const syncStatusService = require('../services/syncStatusService').default;
const edgeGatewayService = require('../services/edgeGatewayService').default;

const pending = {
  has_pending: true,
  data: [{ namespace: 'default', pending_count: 1, stale_count: 0, last_push_at: '2026-09-14T13:12:00Z' }],
};
const inSync = {
  has_pending: false,
  data: [{ namespace: 'default', pending_count: 0, stale_count: 0, last_push_at: '2026-09-14T13:40:00Z' }],
};

let api;
const Probe = () => {
  api = useSyncStatus();
  return (
    <div>
      <span data-testid="pending">{String(api.hasPendingSync)}</span>
      <span data-testid="count">{api.pendingChanges ? JSON.stringify(api.pendingChanges) : 'unknown'}</span>
      <span data-testid="last-push">{String(api.getLastPushAt('default'))}</span>
      <span data-testid="reminder">{api.pushReminder ? 'shown' : 'none'}</span>
    </div>
  );
};

const flush = async () => {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
};

describe('SyncStatusProvider', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.clearAllMocks();
    syncStatusService.getSyncStatus.mockResolvedValue(pending);
    edgeGatewayService.getPendingChanges.mockResolvedValue({ total: 2, lastPushAt: '2026-09-14T13:12:00Z' });
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('loads pending-change counts for pending namespaces and exposes last push time', async () => {
    render(<SyncStatusProvider><Probe /></SyncStatusProvider>);
    await flush();
    expect(screen.getByTestId('pending')).toHaveTextContent('true');
    expect(edgeGatewayService.getPendingChanges).toHaveBeenCalledWith('default');
    expect(screen.getByTestId('count')).toHaveTextContent('"default":{"total":2');
    expect(screen.getByTestId('last-push')).toHaveTextContent('2026-09-14T13:12:00Z');
  });

  it('leaves counts unknown when the pending-changes endpoint fails', async () => {
    edgeGatewayService.getPendingChanges.mockRejectedValue(new Error('404'));
    render(<SyncStatusProvider><Probe /></SyncStatusProvider>);
    await flush();
    expect(screen.getByTestId('pending')).toHaveTextContent('true');
    expect(screen.getByTestId('count')).toHaveTextContent('unknown');
  });

  it('polls every 3 s after a push until nothing is pending', async () => {
    render(<SyncStatusProvider><Probe /></SyncStatusProvider>);
    await flush();
    expect(syncStatusService.getSyncStatus).toHaveBeenCalledTimes(1);

    // Still pending on the first two polls, in sync on the third.
    syncStatusService.getSyncStatus
      .mockResolvedValueOnce(pending)
      .mockResolvedValueOnce(pending)
      .mockResolvedValueOnce(pending)
      .mockResolvedValue(inSync);

    act(() => { api.notifyConfigPushed(); });
    await flush();
    // Immediate refresh.
    expect(syncStatusService.getSyncStatus).toHaveBeenCalledTimes(2);

    await act(async () => { jest.advanceTimersByTime(3000); });
    await flush();
    expect(syncStatusService.getSyncStatus).toHaveBeenCalledTimes(3);

    await act(async () => { jest.advanceTimersByTime(3000); });
    await flush();
    expect(syncStatusService.getSyncStatus).toHaveBeenCalledTimes(4);

    await act(async () => { jest.advanceTimersByTime(3000); });
    await flush();
    expect(syncStatusService.getSyncStatus).toHaveBeenCalledTimes(5);
    await waitFor(() => expect(screen.getByTestId('pending')).toHaveTextContent('false'));
    expect(screen.getByTestId('last-push')).toHaveTextContent('2026-09-14T13:40:00Z');

    // In sync now: polling has stopped (the 30 s baseline poll is the only
    // call left, and it has not fired yet).
    await act(async () => { jest.advanceTimersByTime(9000); });
    await flush();
    expect(syncStatusService.getSyncStatus).toHaveBeenCalledTimes(5);
  });

  it('gives up polling after 30 s when the edges never acknowledge', async () => {
    render(<SyncStatusProvider><Probe /></SyncStatusProvider>);
    await flush();
    act(() => { api.notifyConfigPushed(); });
    await flush();
    const before = syncStatusService.getSyncStatus.mock.calls.length;

    await act(async () => { jest.advanceTimersByTime(29000); });
    await flush();
    // 9 interval ticks in 29 s.
    expect(syncStatusService.getSyncStatus.mock.calls.length).toBe(before + 9);

    await act(async () => { jest.advanceTimersByTime(1000); });
    await flush();
    const atTimeout = syncStatusService.getSyncStatus.mock.calls.length;

    // Nothing more from the post-push poller; the next call is the 30 s
    // baseline refresh, well after the timeout.
    await act(async () => { jest.advanceTimersByTime(6000); });
    await flush();
    expect(syncStatusService.getSyncStatus.mock.calls.length).toBeLessThanOrEqual(atTimeout + 1);
  });

  // #506: a save that leaves edges out of date raises a push reminder.
  describe('push reminder after a save', () => {
    const saveThenRefresh = async (saved) => {
      notifyConfigChanged({ saved });
      await act(async () => {
        jest.advanceTimersByTime(600);
      });
      await flush();
    };

    it('is raised when a save leaves edges pending, and can be dismissed', async () => {
      render(<SyncStatusProvider><Probe /></SyncStatusProvider>);
      await flush();
      expect(screen.getByTestId('reminder')).toHaveTextContent('none');

      await saveThenRefresh(true);
      expect(screen.getByTestId('reminder')).toHaveTextContent('shown');

      act(() => api.dismissPushReminder());
      expect(screen.getByTestId('reminder')).toHaveTextContent('none');
    });

    it('is not raised when no edge needs a push (none registered, or all in sync)', async () => {
      syncStatusService.getSyncStatus.mockResolvedValue(inSync);
      render(<SyncStatusProvider><Probe /></SyncStatusProvider>);
      await flush();
      await saveThenRefresh(true);
      expect(screen.getByTestId('reminder')).toHaveTextContent('none');
    });

    it('is not raised by a change that was not a save', async () => {
      render(<SyncStatusProvider><Probe /></SyncStatusProvider>);
      await flush();
      await saveThenRefresh(false);
      expect(syncStatusService.getSyncStatus).toHaveBeenCalledTimes(2);
      expect(screen.getByTestId('reminder')).toHaveTextContent('none');
    });
  });
});
