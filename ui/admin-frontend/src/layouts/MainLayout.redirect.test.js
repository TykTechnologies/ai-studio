import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
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
  default: () => <div data-testid="admin-layout" />,
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
      is_admin: true,
      ui_options: { show_chat: true, show_portal: true },
      chats: [],
      catalogues: [],
      data_catalogues: [],
      tool_catalogues: [],
    },
  },
};

const LocationProbe = () => {
  const location = useLocation();
  return <div data-testid="location">{location.pathname}</div>;
};

const renderAt = (path) =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route element={<MainLayout />}>
          <Route path="/" element={<div data-testid="page">root</div>} />
          <Route path="/portal/dashboard" element={<div data-testid="page">portal page</div>} />
          <Route path="/admin/*" element={<div data-testid="page">admin page</div>} />
        </Route>
      </Routes>
      <LocationProbe />
    </MemoryRouter>
  );

// A full admin used to be bounced off /portal/dashboard to the last admin
// page on mount, so a direct load (bookmark, refresh) of the portal never
// stayed there.
describe('MainLayout mount redirect for full admins', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    usePermissions.mockReturnValue({
      identity: { raw: me.data },
      isFullAdmin: true,
      hasAdminAccess: true,
    });
    useSystemFeatures.mockReturnValue({
      features: { feature_portal: true, feature_chat: true, feature_gateway: true },
    });
    const stored = { drawer_state_admin: JSON.stringify({ selectedPath: '/admin/llms' }) };
    Object.defineProperty(window, 'localStorage', {
      value: {
        getItem: jest.fn((key) => stored[key] ?? null),
        setItem: jest.fn(),
        removeItem: jest.fn(),
        clear: jest.fn(),
      },
      configurable: true,
    });
  });

  it('stays on /portal/dashboard when loaded directly and shows the portal tab', async () => {
    renderAt('/portal/dashboard');
    expect(await screen.findByText('portal page')).toBeInTheDocument();
    expect(screen.getByTestId('location')).toHaveTextContent('/portal/dashboard');
    expect(screen.getByTestId('portal-drawer')).toBeInTheDocument();
    expect(screen.queryByTestId('admin-layout')).not.toBeInTheDocument();
  });

  it('bounces "/" to the stored admin path', async () => {
    renderAt('/');
    expect(await screen.findByTestId('admin-layout')).toBeInTheDocument();
    expect(screen.getByTestId('location')).toHaveTextContent('/admin/llms');
  });
});
