import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider } from '@mui/material/styles';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import testTheme from '../../utils/testTheme';
import apiClient from '../../utils/apiClient';
import { SyncStatusProvider } from '../../context/SyncStatusContext';
import PushReminderToast from '../common/PushReminderToast';
import FilterForm from './FilterForm';

// #506 end to end in the browser bundle: a real form saves through the real
// API client; its response interceptor, the sync-status refresh and the
// toast then tell the user the change is not live on edges until pushed.
// Only the network (axios adapter) and the sync-status service are faked.

jest.mock('prismjs/themes/prism-tomorrow.css', () => ({}));
jest.mock('../common/EdgeAvailabilitySection', () => () => null);
jest.mock('./ScriptTestPanel', () => () => null);
jest.mock('../edge-gateways/PushConfigurationModal', () => () => null);
jest.mock('../../utils/urlUtils', () => ({
  ...jest.requireActual('../../utils/urlUtils'),
  fetchCSRFToken: jest.fn(),
}));
jest.mock('../../utils/identityStore', () => ({
  ...jest.requireActual('../../utils/identityStore'),
  hasPermissionNow: () => true,
  subscribe: () => () => {},
}));
jest.mock('../../services/syncStatusService', () => ({
  __esModule: true,
  default: { getSyncStatus: jest.fn() },
}));
jest.mock('../../services/edgeGatewayService', () => ({
  __esModule: true,
  default: { getPendingChanges: jest.fn() },
}));

const syncStatusService = require('../../services/syncStatusService').default;
const edgeGatewayService = require('../../services/edgeGatewayService').default;
const { fetchCSRFToken } = require('../../utils/urlUtils');

const savedFilter = {
  type: 'filter',
  id: '7',
  attributes: { name: 'PII', description: '', script: btoa('filter := true'), kind: 'script', response_filter: false },
};

const requests = [];
const fakeNetwork = async (config) => {
  requests.push(`${config.method} ${config.url}`);
  return { data: { data: savedFilter, ...savedFilter }, status: 200, statusText: 'OK', headers: {}, config };
};

const renderApp = () =>
  render(
    <ThemeProvider theme={testTheme}>
      <SyncStatusProvider>
        <MemoryRouter initialEntries={['/admin/filters/edit/7']}>
          <Routes>
            <Route path="/admin/filters/edit/:id" element={<FilterForm />} />
            <Route path="/admin/filters" element={<div>filter list</div>} />
          </Routes>
          <PushReminderToast />
        </MemoryRouter>
      </SyncStatusProvider>
    </ThemeProvider>
  );

describe('FilterForm save: push reminder (#506)', () => {
  const originalAdapter = apiClient.defaults.adapter;

  beforeEach(() => {
    requests.length = 0;
    apiClient.defaults.adapter = fakeNetwork;
    fetchCSRFToken.mockResolvedValue('token');
    edgeGatewayService.getPendingChanges.mockResolvedValue({ total: 1 });
    jest.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    apiClient.defaults.adapter = originalAdapter;
    console.error.mockRestore();
  });

  it('after saving, says the change is not live on edges until pushed', async () => {
    syncStatusService.getSyncStatus
      .mockResolvedValueOnce({ has_pending: false, data: [{ namespace: '', pending_count: 0, stale_count: 0 }] })
      .mockResolvedValue({ has_pending: true, data: [{ namespace: '', pending_count: 1, stale_count: 0 }] });
    renderApp();
    expect(await screen.findByDisplayValue('PII')).toBeInTheDocument();
    expect(screen.queryByTestId('push-reminder')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Update filter' }));

    expect(await screen.findByText('filter list')).toBeInTheDocument();
    expect(await screen.findByTestId('push-reminder', {}, { timeout: 3000 })).toHaveTextContent(
      'Saved, not live on edge gateways yet'
    );
    expect(screen.getByRole('button', { name: 'Push Configuration' })).toBeInTheDocument();
    expect(requests).toContain('patch /filters/7');
  });

  it('stays quiet after saving when no edge needs a push', async () => {
    syncStatusService.getSyncStatus.mockResolvedValue({ has_pending: false, data: [] });
    renderApp();
    expect(await screen.findByDisplayValue('PII')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Update filter' }));
    expect(await screen.findByText('filter list')).toBeInTheDocument();
    // The post-save refresh happens (the reminder decision is made on it)...
    await waitFor(() => expect(syncStatusService.getSyncStatus).toHaveBeenCalledTimes(2), { timeout: 3000 });
    // ...and finds nothing to push.
    expect(screen.queryByTestId('push-reminder')).not.toBeInTheDocument();
  });
});
