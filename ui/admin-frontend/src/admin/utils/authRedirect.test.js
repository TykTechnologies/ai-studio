import { hostSignInURL, isPublicPath, loginLocation, redirectToLogin } from './authRedirect';

describe('isPublicPath', () => {
  test.each([
    '/login',
    '/register',
    '/forgot-password',
    '/reset-password',
    '/auth/forgot-password',
    '/auth/reset-password',
    '/register/',
  ])('%s is public', (path) => {
    expect(isPublicPath(path)).toBe(true);
  });

  test.each(['/', '/admin', '/admin/login-settings', '/portal/dashboard', '/registered', '/oauth/consent'])(
    '%s is not public',
    (path) => {
      expect(isPublicPath(path)).toBe(false);
    },
  );
});

describe('redirectToLogin', () => {
  const original = window.location;

  afterEach(() => {
    window.location = original;
  });

  const at = (pathname) => {
    delete window.location;
    window.location = { pathname, href: `http://localhost${pathname}` };
  };

  test('stays on the sign-up page', () => {
    at('/register');
    expect(redirectToLogin()).toBe(false);
    expect(window.location.href).toBe('http://localhost/register');
  });

  test('does not reload the login page', () => {
    at('/login');
    expect(redirectToLogin()).toBe(false);
    expect(window.location.href).toBe('http://localhost/login');
  });

  test('sends an expired session on an app page to the login page', () => {
    at('/admin/apps');
    expect(redirectToLogin()).toBe(true);
    expect(window.location.href).toBe('/login');
  });
});

describe('host sign-in return address', () => {
  afterEach(() => {
    delete window.__TYK_AI_STUDIO__;
  });

  const host = (loginURL) => {
    window.__TYK_AI_STUDIO__ = { basePath: '/ai-studio', authMode: 'host', loginURL };
  };
  const loc = (pathname, search = '', hash = '') => ({ pathname, search, hash });

  test('passes the page, query and hash under the base path', () => {
    host('/login?next={return_to}');
    expect(hostSignInURL(loc('/ai-studio/admin/llms/3', '?tab=keys', '#top'))).toBe(
      '/login?next=%2Fai-studio%2Fadmin%2Fllms%2F3%3Ftab%3Dkeys%23top',
    );
  });

  test('sends the visitor home rather than back to a sign-in page', () => {
    host('/login?next={return_to}');
    expect(hostSignInURL(loc('/ai-studio/login'))).toBe('/login?next=%2Fai-studio%2F');
  });

  test('uses a login URL without the placeholder as it is', () => {
    host('https://host.example.com/login');
    expect(hostSignInURL(loc('/ai-studio/admin/llms'))).toBe('https://host.example.com/login');
  });

  test('an expired session goes to the host with its page', () => {
    host('/login?next={return_to}');
    expect(loginLocation()).toBe(`/login?next=${encodeURIComponent(window.location.pathname)}`);
  });

  test("Studio's own sign-in is unchanged", () => {
    window.__TYK_AI_STUDIO__ = { basePath: '/ai-studio', loginURL: '/login?next={return_to}' };
    expect(loginLocation()).toBe('/ai-studio/login');
  });
});
