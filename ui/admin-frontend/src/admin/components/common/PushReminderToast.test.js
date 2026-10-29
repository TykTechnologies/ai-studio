import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import PushReminderToast from './PushReminderToast';
import { useOptionalSyncStatus } from '../../context/SyncStatusContext';

jest.mock('../../context/SyncStatusContext', () => ({
  useOptionalSyncStatus: jest.fn(),
}));
jest.mock('../edge-gateways/PushConfigurationModal', () => ({ open }) =>
  open ? <div data-testid="push-modal">push modal</div> : <div data-testid="push-modal-closed" />
);

describe('PushReminderToast (#506)', () => {
  const dismissPushReminder = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders nothing without a reminder', () => {
    useOptionalSyncStatus.mockReturnValue({ pushReminder: null, dismissPushReminder });
    render(<PushReminderToast />);
    expect(screen.queryByTestId('push-reminder')).not.toBeInTheDocument();
  });

  // The modal loads namespaces (edges:read) when it mounts, and this toast is
  // on every page: mounting it closed made every user without edges:read
  // fetch /namespaces and get a 403 on each page load (lsj.29).
  it('does not mount the push modal until it is opened', () => {
    useOptionalSyncStatus.mockReturnValue({ pushReminder: { id: 1 }, dismissPushReminder });
    render(<PushReminderToast />);
    expect(screen.queryByTestId('push-modal-closed')).not.toBeInTheDocument();
    expect(screen.queryByTestId('push-modal')).not.toBeInTheDocument();
  });

  it('renders nothing outside the sync-status provider', () => {
    useOptionalSyncStatus.mockReturnValue(null);
    const { container } = render(<PushReminderToast />);
    expect(container).toBeEmptyDOMElement();
  });

  it('says the save is not live on edges and offers the push', () => {
    useOptionalSyncStatus.mockReturnValue({ pushReminder: { id: 1 }, dismissPushReminder });
    render(<PushReminderToast />);
    expect(screen.getByTestId('push-reminder')).toHaveTextContent('Saved, not live on edge gateways yet');

    fireEvent.click(screen.getByRole('button', { name: 'Push Configuration' }));
    expect(dismissPushReminder).toHaveBeenCalled();
    expect(screen.getByTestId('push-modal')).toBeInTheDocument();
  });

  it('can be dismissed without pushing', () => {
    useOptionalSyncStatus.mockReturnValue({ pushReminder: { id: 1 }, dismissPushReminder });
    render(<PushReminderToast />);
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
    expect(dismissPushReminder).toHaveBeenCalled();
    expect(screen.queryByTestId('push-modal')).not.toBeInTheDocument();
  });
});
