import pubClient from './pubClient';

// The 401 branch of pubClient's response interceptor.
const onError = pubClient.interceptors.response.handlers[0].rejected;

describe('pubClient 401 handling', () => {
  const original = window.location;

  afterEach(() => {
    window.location = original;
  });

  const at = (pathname) => {
    delete window.location;
    window.location = { pathname, href: `http://localhost${pathname}` };
  };

  const unauthorized = { response: { status: 401 } };

  test('a signed-out visitor on the sign-up page stays there', async () => {
    at('/register');
    await expect(onError(unauthorized)).rejects.toBe(unauthorized);
    expect(window.location.href).toBe('http://localhost/register');
  });

  test('an expired session on an app page goes to the login page', async () => {
    at('/portal/dashboard');
    await expect(onError(unauthorized)).rejects.toBe(unauthorized);
    expect(window.location.href).toBe('/login');
  });

  test('other errors do not redirect', async () => {
    at('/portal/dashboard');
    const failure = { response: { status: 500 } };
    await expect(onError(failure)).rejects.toBe(failure);
    expect(window.location.href).toBe('http://localhost/portal/dashboard');
  });
});
