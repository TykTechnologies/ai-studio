import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import { getConfig } from './config';
import pubClient from './admin/utils/pubClient';
import App from './App';

// Sign-in and local-account routes, standalone and with host sign-in
// (window.__TYK_AI_STUDIO__.authMode "host"). Pages and providers are stubs:
// only where each route leads matters here.

jest.mock('./config', () => ({
  loadConfig: jest.fn(),
  getConfig: jest.fn(),
}));
jest.mock('./admin/utils/pubClient', () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn() },
  reinitializePubClient: jest.fn(),
}));
jest.mock('./admin/utils/apiClient', () => ({
  __esModule: true,
  default: { get: jest.fn() },
  reinitializeApiClient: jest.fn(),
}));
jest.mock('./admin/theme', () => ({ generateTheme: () => ({}) }));
jest.mock('@mui/material/styles', () => ({
  ...jest.requireActual('@mui/material/styles'),
  ThemeProvider: ({ children }) => children,
}));
jest.mock('./admin/context/EditionContext', () => ({ EditionProvider: ({ children }) => children }));
jest.mock('./admin/context/PermissionsContext', () => ({ PermissionsProvider: ({ children }) => children }));
jest.mock('./admin/context/SyncStatusContext', () => ({ SyncStatusProvider: ({ children }) => children }));
jest.mock('./admin/context/NotificationContext', () => ({ NotificationProvider: ({ children }) => children }));
jest.mock('./layouts/MainLayout', () => {
  const { Outlet } = jest.requireActual('react-router-dom');
  return () => <Outlet />;
});
jest.mock('./routes/AdminRoutes', () => () => <div>admin console</div>);
jest.mock('./routes/PortalRoutes', () => () => <div>portal</div>);
jest.mock('./routes/ChatRoutes', () => () => <div>chat</div>);
jest.mock('./portal/pages/Login', () => () => <div>studio login</div>);
jest.mock('./portal/pages/Register', () => () => <div>register form</div>);
jest.mock('./portal/pages/ForgotPassword', () => () => <div>forgot password form</div>);
jest.mock('./portal/pages/ResetPassword', () => () => <div>reset password form</div>);
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  BrowserRouter: ({ children }) => children,
}));

const realLocation = window.location;

const signedOut = () => {
  const err = new Error('unauthorized');
  err.response = { status: 401 };
  pubClient.get.mockRejectedValue(err);
};

const signedInAdmin = () => {
  pubClient.get.mockResolvedValue({
    data: { attributes: { is_admin: true, ui_options: {}, entitlements: {}, permissions: ['llms:read'] } },
  });
};

// The browser is at the route under the base path, and the router at the
// route.
const open = (route, { base = '', search = '', hash = '' } = {}) => {
  delete window.location;
  window.location = {
    pathname: `${base}${route}`,
    search,
    hash,
    href: `http://localhost${base}${route}${search}${hash}`,
    assign: jest.fn(),
  };
  return render(
    <MemoryRouter initialEntries={[route]}>
      <App />
    </MemoryRouter>,
  );
};

beforeEach(() => {
  getConfig.mockReturnValue({});
  jest.spyOn(console, 'error').mockImplementation(() => {});
  jest.spyOn(console, 'log').mockImplementation(() => {});
});

afterEach(() => {
  window.location = realLocation;
  delete window.__TYK_AI_STUDIO__;
  jest.restoreAllMocks();
});

describe('standalone (Studio signs users in)', () => {
  test('a signed-out visitor to a deep link goes to Studio\'s login page', async () => {
    signedOut();
    open('/admin/llms');
    expect(await screen.findByText('studio login')).toBeInTheDocument();
    expect(window.location.assign).not.toHaveBeenCalled();
  });

  test.each([
    ['/register', 'register form'],
    ['/forgot-password', 'forgot password form'],
    ['/reset-password', 'reset password form'],
  ])('%s renders for a signed-out visitor', async (route, text) => {
    signedOut();
    open(route);
    expect(await screen.findByText(text)).toBeInTheDocument();
  });
});

describe('host sign-in', () => {
  const host = (loginURL) => {
    window.__TYK_AI_STUDIO__ = { basePath: '/ai-studio', authMode: 'host', loginURL };
  };

  test('a signed-out visitor goes to the host login with the page they asked for', async () => {
    host('/login?next={return_to}');
    signedOut();
    open('/admin/llms/3', { base: '/ai-studio', search: '?tab=keys', hash: '#top' });
    await waitFor(() =>
      expect(window.location.assign).toHaveBeenCalledWith(
        '/login?next=%2Fai-studio%2Fadmin%2Fllms%2F3%3Ftab%3Dkeys%23top',
      ),
    );
  });

  test('a login URL without the placeholder is used as it is', async () => {
    host('/login');
    signedOut();
    open('/admin/llms', { base: '/ai-studio' });
    await waitFor(() => expect(window.location.assign).toHaveBeenCalledWith('/login'));
  });

  test.each(['/register', '/forgot-password', '/reset-password', '/auth/reset-password'])(
    '%s sends a signed-out visitor to the host login, not a local-account form',
    async (route) => {
      host('/login?next={return_to}');
      signedOut();
      open(route, { base: '/ai-studio' });
      await waitFor(() => expect(window.location.assign).toHaveBeenCalledWith('/login?next=%2Fai-studio%2F'));
      expect(screen.queryByText(/form$/)).not.toBeInTheDocument();
    },
  );

  test.each(['/register', '/forgot-password', '/reset-password'])(
    '%s takes a signed-in user to the console',
    async (route) => {
      host('/login');
      signedInAdmin();
      open(route, { base: '/ai-studio' });
      expect(await screen.findByText('admin console')).toBeInTheDocument();
      expect(pubClient.get).toHaveBeenCalledWith('/common/me');
    },
  );
});
