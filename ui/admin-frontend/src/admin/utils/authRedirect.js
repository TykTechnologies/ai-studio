import { basePath, hostLoginURL, hostLogoutURL, isHostAuth, stripBase, withBase } from '../../runtimeConfig';

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

// RETURN_TO_PLACEHOLDER in the host's login URL (studio.Options.LoginURL,
// e.g. "/login?next={return_to}") is replaced with the page the visitor was
// on, so the host can send them back to it after signing in.
export const RETURN_TO_PLACEHOLDER = '{return_to}';

// hostSignInURL is the host application's sign-in page for a visitor at loc.
// The return address is a path on Studio's origin, under the base path, with
// the query and hash, URL-encoded. From a sign-in or password page it is
// the console's home instead, which must not lead back to sign-in. A login
// URL without the placeholder is used as it is.
export const hostSignInURL = (loc = window.location) => {
  const url = hostLoginURL();
  if (!url.includes(RETURN_TO_PLACEHOLDER)) {
    return url;
  }
  const back = isPublicPath(loc.pathname)
    ? `${basePath()}/`
    : `${loc.pathname}${loc.search || ''}${loc.hash || ''}`;
  return url.split(RETURN_TO_PLACEHOLDER).join(encodeURIComponent(back));
};

// loginLocation is where a signed-out user goes: the host application's
// sign-in page when it authenticates users, else Studio's own.
export const loginLocation = () =>
  (isHostAuth() && hostLoginURL() && hostSignInURL()) || withBase('/login');

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
