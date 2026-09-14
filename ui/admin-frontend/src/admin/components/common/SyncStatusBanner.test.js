import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import testTheme from '../../utils/testTheme';
import SyncStatusBanner, { buildSyncMessage } from './SyncStatusBanner';

let mockSyncContext;
jest.mock('../../context/SyncStatusContext', () => ({
  useSyncStatus: () => mockSyncContext,
}));
jest.mock('../edge-gateways/PushConfigurationModal', () => ({
  __esModule: true,
  default: ({ open }) => (open ? <div data-testid="push-modal" /> : null),
}));

const status = (namespaces) => ({
  has_pending: namespaces.some((ns) => ns.pending_count > 0 || ns.stale_count > 0),
  data: namespaces,
});

const renderBanner = () =>
  render(
    <MemoryRouter>
      <ThemeProvider theme={testTheme}>
        <SyncStatusBanner />
      </ThemeProvider>
    </MemoryRouter>
  );

describe('buildSyncMessage', () => {
  const ns = [{ namespace: 'default', pending_count: 2, stale_count: 0 }];

  it('counts changes when the pending-changes lookup is available', () => {
    expect(buildSyncMessage(ns, { default: { total: 3 } })).toMatch(
      /^3 changes not yet pushed to 2 edge gateways in namespace "default"\./
    );
    expect(buildSyncMessage([{ namespace: 'default', pending_count: 1 }], { default: { total: 1 } })).toMatch(
      /^1 change not yet pushed to 1 edge gateway in namespace "default"\./
    );
  });

  it('falls back to the edge count wording when the lookup failed or is empty', () => {
    expect(buildSyncMessage(ns, null)).toMatch(
      /^2 edge gateways have configuration updates pending in namespace "default"\./
    );
    expect(buildSyncMessage(ns, { default: { total: 0 } })).toMatch(/^2 edge gateways have configuration updates pending/);
    expect(buildSyncMessage(ns, { other: { total: 4 } })).toMatch(/^2 edge gateways have/);
  });

  it('sums across namespaces', () => {
    const many = [
      { namespace: 'default', pending_count: 1, stale_count: 0 },
      { namespace: 'team-a', pending_count: 0, stale_count: 1 },
    ];
    expect(buildSyncMessage(many, { default: { total: 2 }, 'team-a': { total: 5 } })).toMatch(
      /^7 changes not yet pushed to 2 edge gateways across 2 namespaces\./
    );
  });
});

describe('SyncStatusBanner', () => {
  it('renders the change count wording', () => {
    mockSyncContext = {
      syncStatus: status([{ namespace: 'default', pending_count: 1, stale_count: 0 }]),
      hasPendingSync: true,
      pendingCount: 1,
      pendingChanges: { default: { total: 4 } },
    };
    renderBanner();
    expect(screen.getByText(/4 changes not yet pushed to 1 edge gateway in namespace "default"/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Push Configuration' })).toBeInTheDocument();
  });

  it('renders the fallback wording when counts are unknown', () => {
    mockSyncContext = {
      syncStatus: status([{ namespace: 'default', pending_count: 1, stale_count: 0 }]),
      hasPendingSync: true,
      pendingCount: 1,
      pendingChanges: null,
    };
    renderBanner();
    expect(screen.getByText(/1 edge gateway has configuration updates pending in namespace "default"/)).toBeInTheDocument();
  });

  it('renders nothing when nothing is pending', () => {
    mockSyncContext = {
      syncStatus: status([{ namespace: 'default', pending_count: 0, stale_count: 0 }]),
      hasPendingSync: false,
      pendingCount: 0,
      pendingChanges: {},
    };
    const { container } = renderBanner();
    expect(container).toBeEmptyDOMElement();
  });
});
