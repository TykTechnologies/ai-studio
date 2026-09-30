import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import MainLayout from './MainLayout';
import useSystemFeatures from '../admin/hooks/useSystemFeatures';
import { usePermissions } from '../admin/context/PermissionsContext';

jest.mock('../admin/utils/pubClient', () => ({
  __esModule: true,
  default: { get: jest.fn() },
  logout: jest.fn(),
}));
jest.mock('../admin/hooks/useSystemFeatures');
jest.mock('../admin/context/PermissionsContext', () => ({
  usePermissions: jest.fn(),
}));
jest.mock('../components/common/TopNavigation', () => ({
  __esModule: true,
  default: () => <div data-testid="top-nav" />,
}));
jest.mock('../admin/components/layout/MainLayout', () => ({
  __esModule: true,
  default: ({ hideDrawer }) => <div data-testid="admin-layout" data-hide-drawer={String(!!hideDrawer)} />,
}));
jest.mock('../admin/components/layout/ChatDrawer', () => ({
  __esModule: true,
  default: () => <div data-testid="chat-drawer" />,
}));
jest.mock('../admin/components/layout/PortalDrawer', () => ({
  __esModule: true,
  default: () => <div data-testid="portal-drawer" />,
}));

const me = {
  data: {
    attributes: {
      is_admin: false,
      ui_options: { show_chat: true, show_portal: true },
      chats: [],
      catalogues: [],
      data_catalogues: [],
      tool_catalogues: [],
    },
  },
};

const renderAt = (path) =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route element={<MainLayout />}>
          <Route path="/portal/dashboard" element={<div data-testid="page">portal page</div>} />
          <Route path="/chat/dashboard" element={<div data-testid="page">chat page</div>} />
          <Route path="/notifications" element={<div data-testid="page">notifications</div>} />
          <Route path="/admin" element={<div data-testid="page">admin</div>} />
        </Route>
      </Routes>
    </MemoryRouter>
  );

describe('MainLayout chrome', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    usePermissions.mockReturnValue({
      identity: { raw: me.data },
      isFullAdmin: false,
      hasAdminAccess: true,
    });
    useSystemFeatures.mockReturnValue({
      features: { feature_portal: true, feature_chat: true, feature_gateway: true },
    });
    Object.defineProperty(window, 'localStorage', {
      value: { getItem: jest.fn(() => null), setItem: jest.fn(), removeItem: jest.fn(), clear: jest.fn() },
      configurable: true,
    });
  });

  afterEach(() => {
    delete window.__TYK_AI_STUDIO__;
  });

  it('draws the top bar and drawers by default', async () => {
    renderAt('/portal/dashboard');
    await screen.findByTestId('page');
    expect(screen.getByTestId('top-nav')).toBeInTheDocument();
    expect(screen.getByTestId('portal-drawer')).toBeInTheDocument();
  });

  it.each(['/portal/dashboard', '/chat/dashboard'])(
    'renders %s without top bar or drawer when the host draws navigation',
    async (path) => {
      window.__TYK_AI_STUDIO__ = { chrome: 'none' };
      renderAt(path);
      const page = await screen.findByTestId('page');
      expect(page).toBeInTheDocument();
      expect(screen.queryByTestId('top-nav')).toBeNull();
      expect(screen.queryByTestId('portal-drawer')).toBeNull();
      expect(screen.queryByTestId('chat-drawer')).toBeNull();
    }
  );

  it('asks the admin layout to leave out its drawer', async () => {
    window.__TYK_AI_STUDIO__ = { chrome: 'none' };
    renderAt('/admin');
    const admin = await screen.findByTestId('admin-layout');
    expect(admin).toHaveAttribute('data-hide-drawer', 'true');
    expect(screen.queryByTestId('top-nav')).toBeNull();
  });

  it('keeps the admin drawer by default', async () => {
    renderAt('/admin');
    const admin = await screen.findByTestId('admin-layout');
    expect(admin).toHaveAttribute('data-hide-drawer', 'false');
  });
});
