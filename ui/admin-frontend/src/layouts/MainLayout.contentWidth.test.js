import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import MainLayout from './MainLayout';
import pubClient from '../admin/utils/pubClient';
import useSystemFeatures from '../admin/hooks/useSystemFeatures';

jest.mock('../admin/utils/pubClient', () => ({
  __esModule: true,
  default: { get: jest.fn() },
  logout: jest.fn(),
}));
jest.mock('../admin/hooks/useSystemFeatures');
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
        </Route>
      </Routes>
    </MemoryRouter>
  );

describe('MainLayout content width', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pubClient.get.mockResolvedValue(me);
    useSystemFeatures.mockReturnValue({
      features: { feature_portal: true, feature_chat: true, feature_gateway: true },
    });
    Object.defineProperty(window, 'localStorage', {
      value: { getItem: jest.fn(() => null), setItem: jest.fn(), removeItem: jest.fn(), clear: jest.fn() },
      configurable: true,
    });
  });

  it('constrains portal pages to the xl container width', async () => {
    renderAt('/portal/dashboard');
    const page = await screen.findByTestId('page');
    const container = page.closest('.MuiContainer-maxWidthXl');
    expect(container).not.toBeNull();
    // Centred in the space beside the drawer (MUI Container default)
    expect(container).toHaveStyle({ marginLeft: 'auto', marginRight: 'auto' });
    expect(screen.getByTestId('portal-drawer')).toBeInTheDocument();
  });

  it('leaves chat pages full-width', async () => {
    renderAt('/chat/dashboard');
    const page = await screen.findByTestId('page');
    expect(page.closest('.MuiContainer-root')).toBeNull();
    expect(screen.getByTestId('chat-drawer')).toBeInTheDocument();
  });

  it('constrains common pages such as notifications', async () => {
    renderAt('/notifications');
    const page = await screen.findByTestId('page');
    expect(page.closest('.MuiContainer-maxWidthXl')).not.toBeNull();
  });
});
