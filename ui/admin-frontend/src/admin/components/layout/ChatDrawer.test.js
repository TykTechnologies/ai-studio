import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import ChatDrawer from './ChatDrawer';
import adminTheme from '../../theme';

jest.mock('../../utils/pubClient', () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock('../../context/PermissionsContext', () => ({
  usePermissions: () => ({ permissions: new Set(), canAny: () => false }),
}));
jest.mock('../../../components/common/Icon', () => ({
  __esModule: true,
  default: ({ name }) => <span data-testid={`icon-${name}`} />,
}));

const pubClient = require('../../utils/pubClient').default;

// The chat menu as GET /common/nav returns it (api/nav.go chatNav).
const chat = [
  { id: 'overview', text: 'Overview', icon: 'house', path: '/chat/dashboard' },
  { id: 'chat-rooms', text: 'Chats', icon: 'message-lines', items: [{ id: 'chat-3', text: 'Support', path: '/chat/3' }] },
  {
    id: 'past-conversations',
    text: 'Past Conversations',
    icon: 'rectangle-history',
    items: [
      { id: 'history-9', text: 'Yesterday', path: '/chat/3?continue_id=s-1', exact: true },
      { id: 'view-all-conversations', text: 'View all conversations', path: '/chat/dashboard', exact: true },
    ],
  },
  { id: 'agents', text: 'Agents', icon: 'microchip-ai', items: [{ id: 'agent-4', text: 'Helper', path: '/chat/agent/4' }] },
];

const renderDrawer = () =>
  render(
    <ThemeProvider theme={adminTheme}>
      <MemoryRouter initialEntries={['/chat/dashboard']}>
        <ChatDrawer />
      </MemoryRouter>
    </ThemeProvider>
  );

describe('ChatDrawer', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pubClient.get.mockResolvedValue({ data: { surfaces: [], admin: [], portal: [], chat } });
    Object.defineProperty(window, 'localStorage', {
      value: { getItem: jest.fn(() => null), setItem: jest.fn(), removeItem: jest.fn(), clear: jest.fn() },
      configurable: true,
    });
  });

  it('renders rooms and agents expanded from the navigation manifest', async () => {
    renderDrawer();
    expect(await screen.findByRole('link', { name: 'Support' })).toHaveAttribute('href', '/chat/3');
    expect(screen.getByRole('link', { name: 'Helper' })).toHaveAttribute('href', '/chat/agent/4');
    expect(screen.getByText('Past Conversations')).toBeInTheDocument();
    expect(pubClient.get).toHaveBeenCalledWith('/common/nav');
    expect(pubClient.get).toHaveBeenCalledTimes(1);
  });

  it('renders an empty drawer when the manifest cannot be loaded', async () => {
    pubClient.get.mockRejectedValue(new Error('offline'));
    renderDrawer();
    await waitFor(() => expect(pubClient.get).toHaveBeenCalled());
    expect(screen.queryByText('Agents')).not.toBeInTheDocument();
  });
});
