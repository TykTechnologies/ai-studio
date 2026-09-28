import { hostLoginURL, hostLogoutURL, isHostAuth, stripBase, withBase } from '../../runtimeConfig';

// Pages a signed-out visitor must be able to stay on. A 401 there is the
// expected answer to any background request, not an expired session, so it
// must not send the browser to /login (which would bounce a visitor off the
// sign-up form, or reload the login page itself).
const PUBLIC_PATHS = [
  '/login',
  '/register',
  '/forgot-password',
  '/reset-password',
  '/auth/forgot-password',
  '/auth/reset-password',
];

export const isPublicPath = (pathname) => {
  const route = stripBase(pathname);
  return PUBLIC_PATHS.some((p) => route === p || route.startsWith(`${p}/`));
};

// loginLocation is where a signed-out user goes: the host application's
// sign-in page when it authenticates users, else Studio's own.
export const loginLocation = () =>
  (isHostAuth() && hostLoginURL()) || withBase('/login');

// redirectToLogin sends the browser to the login page after a 401, unless the
// current page is already a public one.
export const redirectToLogin = () => {
  if (isPublicPath(window.location.pathname)) {
    return false;
  }
  window.location.href = loginLocation();
  return true;
};

// redirectToLogout sends the browser on after signing out of Studio: to the
// host application's sign-out when it authenticates users, else to the login
// page.
export const redirectToLogout = () => {
  window.location.href = (isHostAuth() && hostLogoutURL()) || withBase('/login');
};
