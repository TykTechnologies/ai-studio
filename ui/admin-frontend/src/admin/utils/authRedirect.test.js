import {
  hostSignInURL,
  isPublicPath,
  loginLocation,
  redirectToLogin,
  requestedReturnPath,
  safeReturnPath,
  studioLoginPath,
} from './authRedirect';

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

  test('sends an expired session on an app page to the login page, then back', () => {
    at('/admin/apps');
    expect(redirectToLogin()).toBe(true);
    expect(window.location.href).toBe('/login?next=%2Fadmin%2Fapps');
  });
});

describe('Studio sign-in return address (?next=)', () => {
  afterEach(() => {
    delete window.__TYK_AI_STUDIO__;
  });

  test.each([
    '/admin/apps/222',
    '/admin/filters/new',
    '/portal/catalog/tools/3?tab=docs',
    '/admin/llms/3#models',
    '/chat/7?continue_id=abc',
  ])('accepts the console route %s', (path) => {
    expect(safeReturnPath(path)).toBe(path);
  });

  test.each([
    ['nothing', null],
    ['an empty string', ''],
    ['the root', '/'],
    ['a relative path', 'admin/apps'],
    ['a protocol-relative URL', '//evil.example/admin'],
    ['a backslash host', '/\\evil.example'],
    ['a backslash later on', '/admin\\..\\x'],
    ['an absolute URL', 'https://evil.example/admin'],
    // eslint-disable-next-line no-script-url
    ['a script URL', 'javascript:alert(1)'],
    ['a control character', '/admin\n/apps'],
    ['the login page', '/login?next=/admin'],
    ['the login page behind dot segments', '/admin/../login'],
    ['a protocol-relative URL behind a dot segment', '/.//evil.example'],
    ['a protocol-relative URL behind dot-dot', '/a/..//evil.example/x'],
    ['the root behind dot segments', '/admin/..'],
    ['a password page', '/reset-password?token=x'],
    ['an oversized value', `/admin/${'a'.repeat(2100)}`],
  ])('rejects %s', (_, raw) => {
    expect(safeReturnPath(raw)).toBeNull();
  });

  test('studioLoginPath carries the route, query and hash without the base path', () => {
    window.__TYK_AI_STUDIO__ = { basePath: '/ai-studio' };
    expect(studioLoginPath({ pathname: '/ai-studio/admin/llms/3', search: '?tab=keys', hash: '#top' })).toBe(
      '/login?next=%2Fadmin%2Fllms%2F3%3Ftab%3Dkeys%23top',
    );
  });

  test('studioLoginPath has no return address for the root or a sign-in page', () => {
    expect(studioLoginPath({ pathname: '/', search: '', hash: '' })).toBe('/login');
    expect(studioLoginPath({ pathname: '/register', search: '', hash: '' })).toBe('/login');
  });

  test('requestedReturnPath reads a safe ?next= and drops an unsafe one', () => {
    expect(requestedReturnPath('?next=%2Fadmin%2Fapps%2F1')).toBe('/admin/apps/1');
    expect(requestedReturnPath('?next=%2F%2Fevil.example')).toBeNull();
    expect(requestedReturnPath('')).toBeNull();
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

  test("Studio's own sign-in under a base path carries the deep page without the base", () => {
    const original = window.location;
    delete window.location;
    window.location = { pathname: '/ai-studio/admin/apps/1', search: '', hash: '' };
    try {
      window.__TYK_AI_STUDIO__ = { basePath: '/ai-studio' };
      expect(loginLocation()).toBe('/ai-studio/login?next=%2Fadmin%2Fapps%2F1');
    } finally {
      window.location = original;
    }
  });
});
