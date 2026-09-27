import { isPublicPath, redirectToLogin } from './authRedirect';

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
